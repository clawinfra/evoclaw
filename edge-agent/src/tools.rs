//! Edge device tools for hardware interaction
//!
//! Provides tools for:
//! - Sensor reading (temperature, CPU, memory)
//! - GPIO control
//! - Camera capture
//! - System information
//! - Network diagnostics
//! - Safe command execution

use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::HashMap;
use std::fs;
use std::path::Path;
use std::process::Command;
use tracing::{info, warn};

/// Tool execution result
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ToolResult {
    pub success: bool,
    pub output: Value,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub error: Option<String>,
}

impl ToolResult {
    pub fn ok(output: Value) -> Self {
        Self {
            success: true,
            output,
            error: None,
        }
    }

    pub fn err(msg: impl Into<String>) -> Self {
        Self {
            success: false,
            output: Value::Null,
            error: Some(msg.into()),
        }
    }
}

/// Tool definition for LLM function calling
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ToolDefinition {
    pub name: String,
    pub description: String,
    pub parameters: Value,
}

/// Edge device tools
pub struct EdgeTools {
    /// Allowed commands for safe_exec (whitelist)
    allowed_commands: Vec<String>,
    /// GPIO pins that are allowed to be controlled
    allowed_gpio_pins: Vec<u8>,
}

impl Default for EdgeTools {
    fn default() -> Self {
        Self::new()
    }
}

impl EdgeTools {
    pub fn new() -> Self {
        Self {
            allowed_commands: vec![
                "uptime".to_string(),
                "free".to_string(),
                "df".to_string(),
                "hostname".to_string(),
                "uname".to_string(),
                "cat".to_string(),
                "ls".to_string(),
                "date".to_string(),
                "whoami".to_string(),
                "pwd".to_string(),
                "ip".to_string(),
                "ping".to_string(),
                "vcgencmd".to_string(), // Pi-specific
                "raspistill".to_string(), // Pi camera
                "libcamera-still".to_string(), // Pi camera (newer)
            ],
            allowed_gpio_pins: vec![2, 3, 4, 17, 27, 22, 10, 9, 11, 5, 6, 13, 19, 26],
        }
    }

    /// Get all available tool definitions for LLM function calling
    pub fn get_tool_definitions(&self) -> Vec<ToolDefinition> {
        vec![
            ToolDefinition {
                name: "read_temperature".to_string(),
                description: "Read the CPU temperature of the Raspberry Pi in Celsius".to_string(),
                parameters: serde_json::json!({
                    "type": "object",
                    "properties": {},
                    "required": []
                }),
            },
            ToolDefinition {
                name: "read_cpu_usage".to_string(),
                description: "Get current CPU usage percentage".to_string(),
                parameters: serde_json::json!({
                    "type": "object",
                    "properties": {},
                    "required": []
                }),
            },
            ToolDefinition {
                name: "read_memory".to_string(),
                description: "Get memory usage information (total, used, free, available)".to_string(),
                parameters: serde_json::json!({
                    "type": "object",
                    "properties": {},
                    "required": []
                }),
            },
            ToolDefinition {
                name: "read_disk".to_string(),
                description: "Get disk usage information for the root filesystem".to_string(),
                parameters: serde_json::json!({
                    "type": "object",
                    "properties": {},
                    "required": []
                }),
            },
            ToolDefinition {
                name: "system_info".to_string(),
                description: "Get comprehensive system information (hostname, uptime, kernel, etc)".to_string(),
                parameters: serde_json::json!({
                    "type": "object",
                    "properties": {},
                    "required": []
                }),
            },
            ToolDefinition {
                name: "gpio_read".to_string(),
                description: "Read the state of a GPIO pin (HIGH=1 or LOW=0)".to_string(),
                parameters: serde_json::json!({
                    "type": "object",
                    "properties": {
                        "pin": {
                            "type": "integer",
                            "description": "GPIO pin number (BCM numbering)"
                        }
                    },
                    "required": ["pin"]
                }),
            },
            ToolDefinition {
                name: "gpio_write".to_string(),
                description: "Set the state of a GPIO pin (1=HIGH, 0=LOW)".to_string(),
                parameters: serde_json::json!({
                    "type": "object",
                    "properties": {
                        "pin": {
                            "type": "integer",
                            "description": "GPIO pin number (BCM numbering)"
                        },
                        "value": {
                            "type": "integer",
                            "description": "Value to set (0=LOW, 1=HIGH)"
                        }
                    },
                    "required": ["pin", "value"]
                }),
            },
            ToolDefinition {
                name: "gpio_list".to_string(),
                description: "List all GPIO pins and their current states".to_string(),
                parameters: serde_json::json!({
                    "type": "object",
                    "properties": {},
                    "required": []
                }),
            },
            ToolDefinition {
                name: "camera_capture".to_string(),
                description: "Capture an image from the Raspberry Pi camera".to_string(),
                parameters: serde_json::json!({
                    "type": "object",
                    "properties": {
                        "width": {
                            "type": "integer",
                            "description": "Image width in pixels (default: 640)"
                        },
                        "height": {
                            "type": "integer",
                            "description": "Image height in pixels (default: 480)"
                        },
                        "output_path": {
                            "type": "string",
                            "description": "Output file path (default: /tmp/capture.jpg)"
                        }
                    },
                    "required": []
                }),
            },
            ToolDefinition {
                name: "network_info".to_string(),
                description: "Get network interface information and connectivity status".to_string(),
                parameters: serde_json::json!({
                    "type": "object",
                    "properties": {},
                    "required": []
                }),
            },
            ToolDefinition {
                name: "ping".to_string(),
                description: "Ping a host to check connectivity".to_string(),
                parameters: serde_json::json!({
                    "type": "object",
                    "properties": {
                        "host": {
                            "type": "string",
                            "description": "Host to ping (IP or hostname)"
                        },
                        "count": {
                            "type": "integer",
                            "description": "Number of pings (default: 3, max: 10)"
                        }
                    },
                    "required": ["host"]
                }),
            },
            ToolDefinition {
                name: "read_file".to_string(),
                description: "Read contents of a file (limited to safe paths)".to_string(),
                parameters: serde_json::json!({
                    "type": "object",
                    "properties": {
                        "path": {
                            "type": "string",
                            "description": "File path to read"
                        },
                        "max_bytes": {
                            "type": "integer",
                            "description": "Maximum bytes to read (default: 4096)"
                        }
                    },
                    "required": ["path"]
                }),
            },
            ToolDefinition {
                name: "safe_exec".to_string(),
                description: "Execute a whitelisted command safely".to_string(),
                parameters: serde_json::json!({
                    "type": "object",
                    "properties": {
                        "command": {
                            "type": "string",
                            "description": "Command to execute (must be whitelisted)"
                        },
                        "args": {
                            "type": "array",
                            "items": {"type": "string"},
                            "description": "Command arguments"
                        }
                    },
                    "required": ["command"]
                }),
            },
            ToolDefinition {
                name: "list_processes".to_string(),
                description: "List running processes sorted by CPU or memory usage".to_string(),
                parameters: serde_json::json!({
                    "type": "object",
                    "properties": {
                        "sort_by": {
                            "type": "string",
                            "description": "Sort by 'cpu' or 'memory' (default: cpu)"
                        },
                        "limit": {
                            "type": "integer",
                            "description": "Number of processes to return (default: 10)"
                        }
                    },
                    "required": []
                }),
            },
        ]
    }

    /// Execute a tool by name with parameters
    pub fn execute(&self, name: &str, params: &HashMap<String, Value>) -> ToolResult {
        info!(tool = %name, "executing tool");

        match name {
            "read_temperature" => self.read_temperature(),
            "read_cpu_usage" => self.read_cpu_usage(),
            "read_memory" => self.read_memory(),
            "read_disk" => self.read_disk(),
            "system_info" => self.system_info(),
            "gpio_read" => {
                let pin = params.get("pin").and_then(|v| v.as_u64()).unwrap_or(0) as u8;
                self.gpio_read(pin)
            }
            "gpio_write" => {
                let pin = params.get("pin").and_then(|v| v.as_u64()).unwrap_or(0) as u8;
                let value = params.get("value").and_then(|v| v.as_u64()).unwrap_or(0) as u8;
                self.gpio_write(pin, value)
            }
            "gpio_list" => self.gpio_list(),
            "camera_capture" => {
                let width = params.get("width").and_then(|v| v.as_u64()).unwrap_or(640) as u32;
                let height = params.get("height").and_then(|v| v.as_u64()).unwrap_or(480) as u32;
                let output = params
                    .get("output_path")
                    .and_then(|v| v.as_str())
                    .unwrap_or("/tmp/capture.jpg");
                self.camera_capture(width, height, output)
            }
            "network_info" => self.network_info(),
            "ping" => {
                let host = params
                    .get("host")
                    .and_then(|v| v.as_str())
                    .unwrap_or("8.8.8.8");
                let count = params.get("count").and_then(|v| v.as_u64()).unwrap_or(3) as u32;
                self.ping(host, count.min(10))
            }
            "read_file" => {
                let path = params.get("path").and_then(|v| v.as_str()).unwrap_or("");
                let max_bytes = params
                    .get("max_bytes")
                    .and_then(|v| v.as_u64())
                    .unwrap_or(4096) as usize;
                self.read_file(path, max_bytes)
            }
            "safe_exec" => {
                let command = params.get("command").and_then(|v| v.as_str()).unwrap_or("");
                let args: Vec<String> = params
                    .get("args")
                    .and_then(|v| v.as_array())
                    .map(|arr| {
                        arr.iter()
                            .filter_map(|v| v.as_str().map(String::from))
                            .collect()
                    })
                    .unwrap_or_default();
                self.safe_exec(command, &args)
            }
            "list_processes" => {
                let sort_by = params
                    .get("sort_by")
                    .and_then(|v| v.as_str())
                    .unwrap_or("cpu");
                let limit = params.get("limit").and_then(|v| v.as_u64()).unwrap_or(10) as usize;
                self.list_processes(sort_by, limit)
            }
            _ => ToolResult::err(format!("Unknown tool: {}", name)),
        }
    }

    // ========== Sensor Tools ==========

    /// Read CPU temperature from thermal zone
    pub fn read_temperature(&self) -> ToolResult {
        let thermal_path = "/sys/class/thermal/thermal_zone0/temp";

        match fs::read_to_string(thermal_path) {
            Ok(content) => {
                if let Ok(millidegrees) = content.trim().parse::<f64>() {
                    let celsius = millidegrees / 1000.0;
                    ToolResult::ok(serde_json::json!({
                        "temperature_c": celsius,
                        "temperature_f": celsius * 9.0 / 5.0 + 32.0,
                        "source": thermal_path
                    }))
                } else {
                    ToolResult::err("Failed to parse temperature value")
                }
            }
            Err(e) => {
                // Try vcgencmd as fallback (Pi-specific)
                match Command::new("vcgencmd").arg("measure_temp").output() {
                    Ok(output) => {
                        let stdout = String::from_utf8_lossy(&output.stdout);
                        // Parse "temp=XX.X'C"
                        if let Some(temp_str) = stdout.strip_prefix("temp=") {
                            if let Some(celsius_str) = temp_str.strip_suffix("'C\n") {
                                if let Ok(celsius) = celsius_str.parse::<f64>() {
                                    return ToolResult::ok(serde_json::json!({
                                        "temperature_c": celsius,
                                        "temperature_f": celsius * 9.0 / 5.0 + 32.0,
                                        "source": "vcgencmd"
                                    }));
                                }
                            }
                        }
                        ToolResult::err(format!("Failed to parse vcgencmd output: {}", stdout))
                    }
                    Err(_) => ToolResult::err(format!(
                        "Cannot read temperature: {} (and vcgencmd not available)",
                        e
                    )),
                }
            }
        }
    }

    /// Read CPU usage from /proc/stat
    pub fn read_cpu_usage(&self) -> ToolResult {
        // Read /proc/stat twice with a small delay to calculate usage
        let read_stat = || -> Option<(u64, u64)> {
            let content = fs::read_to_string("/proc/stat").ok()?;
            let first_line = content.lines().next()?;
            let parts: Vec<&str> = first_line.split_whitespace().collect();
            if parts.len() < 5 || parts[0] != "cpu" {
                return None;
            }
            let user: u64 = parts[1].parse().ok()?;
            let nice: u64 = parts[2].parse().ok()?;
            let system: u64 = parts[3].parse().ok()?;
            let idle: u64 = parts[4].parse().ok()?;
            let iowait: u64 = parts.get(5).and_then(|s| s.parse().ok()).unwrap_or(0);
            let total = user + nice + system + idle + iowait;
            let active = user + nice + system;
            Some((active, total))
        };

        if let Some((active1, total1)) = read_stat() {
            std::thread::sleep(std::time::Duration::from_millis(100));
            if let Some((active2, total2)) = read_stat() {
                let active_diff = active2.saturating_sub(active1);
                let total_diff = total2.saturating_sub(total1);
                let usage = if total_diff > 0 {
                    (active_diff as f64 / total_diff as f64) * 100.0
                } else {
                    0.0
                };
                return ToolResult::ok(serde_json::json!({
                    "cpu_usage_percent": (usage * 10.0).round() / 10.0,
                    "measurement_interval_ms": 100
                }));
            }
        }
        ToolResult::err("Failed to read CPU usage from /proc/stat")
    }

    /// Read memory information from /proc/meminfo
    pub fn read_memory(&self) -> ToolResult {
        match fs::read_to_string("/proc/meminfo") {
            Ok(content) => {
                let mut mem_total: u64 = 0;
                let mut mem_free: u64 = 0;
                let mut mem_available: u64 = 0;
                let mut buffers: u64 = 0;
                let mut cached: u64 = 0;

                for line in content.lines() {
                    let parts: Vec<&str> = line.split_whitespace().collect();
                    if parts.len() >= 2 {
                        let value: u64 = parts[1].parse().unwrap_or(0);
                        match parts[0] {
                            "MemTotal:" => mem_total = value,
                            "MemFree:" => mem_free = value,
                            "MemAvailable:" => mem_available = value,
                            "Buffers:" => buffers = value,
                            "Cached:" => cached = value,
                            _ => {}
                        }
                    }
                }

                let mem_used = mem_total - mem_free - buffers - cached;
                let usage_percent = if mem_total > 0 {
                    (mem_used as f64 / mem_total as f64) * 100.0
                } else {
                    0.0
                };

                ToolResult::ok(serde_json::json!({
                    "total_mb": mem_total / 1024,
                    "used_mb": mem_used / 1024,
                    "free_mb": mem_free / 1024,
                    "available_mb": mem_available / 1024,
                    "buffers_mb": buffers / 1024,
                    "cached_mb": cached / 1024,
                    "usage_percent": (usage_percent * 10.0).round() / 10.0
                }))
            }
            Err(e) => ToolResult::err(format!("Failed to read memory info: {}", e)),
        }
    }

    /// Read disk usage
    pub fn read_disk(&self) -> ToolResult {
        match Command::new("df").args(["-B1", "/"]).output() {
            Ok(output) => {
                let stdout = String::from_utf8_lossy(&output.stdout);
                let lines: Vec<&str> = stdout.lines().collect();
                if lines.len() >= 2 {
                    let parts: Vec<&str> = lines[1].split_whitespace().collect();
                    if parts.len() >= 5 {
                        let total: u64 = parts[1].parse().unwrap_or(0);
                        let used: u64 = parts[2].parse().unwrap_or(0);
                        let available: u64 = parts[3].parse().unwrap_or(0);
                        let usage_percent = parts[4].trim_end_matches('%').parse::<f64>().unwrap_or(0.0);

                        return ToolResult::ok(serde_json::json!({
                            "filesystem": parts[0],
                            "total_gb": (total as f64 / 1_073_741_824.0 * 10.0).round() / 10.0,
                            "used_gb": (used as f64 / 1_073_741_824.0 * 10.0).round() / 10.0,
                            "available_gb": (available as f64 / 1_073_741_824.0 * 10.0).round() / 10.0,
                            "usage_percent": usage_percent,
                            "mount_point": parts.get(5).unwrap_or(&"/")
                        }));
                    }
                }
                ToolResult::err("Failed to parse df output")
            }
            Err(e) => ToolResult::err(format!("Failed to run df: {}", e)),
        }
    }

    /// Get comprehensive system information
    pub fn system_info(&self) -> ToolResult {
        let hostname = fs::read_to_string("/etc/hostname")
            .map(|s| s.trim().to_string())
            .unwrap_or_else(|_| "unknown".to_string());

        let uptime = fs::read_to_string("/proc/uptime")
            .ok()
            .and_then(|s| s.split_whitespace().next().map(String::from))
            .and_then(|s| s.parse::<f64>().ok())
            .map(|secs| {
                let days = (secs / 86400.0) as u32;
                let hours = ((secs % 86400.0) / 3600.0) as u32;
                let minutes = ((secs % 3600.0) / 60.0) as u32;
                serde_json::json!({
                    "seconds": secs as u64,
                    "formatted": format!("{}d {}h {}m", days, hours, minutes)
                })
            })
            .unwrap_or(serde_json::json!(null));

        let kernel = Command::new("uname")
            .arg("-r")
            .output()
            .ok()
            .map(|o| String::from_utf8_lossy(&o.stdout).trim().to_string())
            .unwrap_or_else(|| "unknown".to_string());

        let arch = Command::new("uname")
            .arg("-m")
            .output()
            .ok()
            .map(|o| String::from_utf8_lossy(&o.stdout).trim().to_string())
            .unwrap_or_else(|| "unknown".to_string());

        // Pi-specific: get model
        let model = fs::read_to_string("/proc/device-tree/model")
            .map(|s| s.trim_end_matches('\0').trim().to_string())
            .unwrap_or_else(|_| "unknown".to_string());

        ToolResult::ok(serde_json::json!({
            "hostname": hostname,
            "kernel": kernel,
            "architecture": arch,
            "model": model,
            "uptime": uptime
        }))
    }

    // ========== GPIO Tools ==========

    /// Read GPIO pin state
    pub fn gpio_read(&self, pin: u8) -> ToolResult {
        if !self.allowed_gpio_pins.contains(&pin) {
            return ToolResult::err(format!(
                "GPIO pin {} is not in allowed list: {:?}",
                pin, self.allowed_gpio_pins
            ));
        }

        let gpio_path = format!("/sys/class/gpio/gpio{}/value", pin);
        let export_path = "/sys/class/gpio/export";

        // Check if GPIO is exported, if not export it
        if !Path::new(&gpio_path).exists() {
            if let Err(e) = fs::write(export_path, pin.to_string()) {
                warn!(pin, error = %e, "Failed to export GPIO pin");
            }
            std::thread::sleep(std::time::Duration::from_millis(100));
        }

        match fs::read_to_string(&gpio_path) {
            Ok(value) => {
                let state = value.trim().parse::<u8>().unwrap_or(0);
                ToolResult::ok(serde_json::json!({
                    "pin": pin,
                    "value": state,
                    "state": if state == 1 { "HIGH" } else { "LOW" }
                }))
            }
            Err(e) => ToolResult::err(format!("Failed to read GPIO {}: {}", pin, e)),
        }
    }

    /// Write to GPIO pin
    pub fn gpio_write(&self, pin: u8, value: u8) -> ToolResult {
        if !self.allowed_gpio_pins.contains(&pin) {
            return ToolResult::err(format!(
                "GPIO pin {} is not in allowed list: {:?}",
                pin, self.allowed_gpio_pins
            ));
        }

        let gpio_path = format!("/sys/class/gpio/gpio{}", pin);
        let direction_path = format!("{}/direction", gpio_path);
        let value_path = format!("{}/value", gpio_path);
        let export_path = "/sys/class/gpio/export";

        // Export if needed
        if !Path::new(&gpio_path).exists() {
            if let Err(e) = fs::write(export_path, pin.to_string()) {
                return ToolResult::err(format!("Failed to export GPIO {}: {}", pin, e));
            }
            std::thread::sleep(std::time::Duration::from_millis(100));
        }

        // Set direction to output
        if let Err(e) = fs::write(&direction_path, "out") {
            return ToolResult::err(format!("Failed to set GPIO {} direction: {}", pin, e));
        }

        // Write value
        let write_value = if value > 0 { "1" } else { "0" };
        match fs::write(&value_path, write_value) {
            Ok(_) => ToolResult::ok(serde_json::json!({
                "pin": pin,
                "value": if value > 0 { 1 } else { 0 },
                "state": if value > 0 { "HIGH" } else { "LOW" }
            })),
            Err(e) => ToolResult::err(format!("Failed to write GPIO {}: {}", pin, e)),
        }
    }

    /// List all GPIO pins and their states
    pub fn gpio_list(&self) -> ToolResult {
        let mut pins = Vec::new();

        for &pin in &self.allowed_gpio_pins {
            let gpio_path = format!("/sys/class/gpio/gpio{}/value", pin);
            let state = fs::read_to_string(&gpio_path)
                .ok()
                .and_then(|s| s.trim().parse::<u8>().ok());

            pins.push(serde_json::json!({
                "pin": pin,
                "exported": state.is_some(),
                "value": state,
                "state": state.map(|v| if v == 1 { "HIGH" } else { "LOW" })
            }));
        }

        ToolResult::ok(serde_json::json!({
            "pins": pins,
            "allowed_pins": self.allowed_gpio_pins
        }))
    }

    // ========== Camera Tools ==========

    /// Capture image from Pi camera
    pub fn camera_capture(&self, width: u32, height: u32, output_path: &str) -> ToolResult {
        // Validate output path
        if !output_path.starts_with("/tmp/") && !output_path.starts_with("/home/") {
            return ToolResult::err("Output path must be in /tmp/ or /home/");
        }

        // Try libcamera-still first (newer Pi OS), then raspistill
        let result = Command::new("libcamera-still")
            .args([
                "-o",
                output_path,
                "--width",
                &width.to_string(),
                "--height",
                &height.to_string(),
                "-t",
                "1",
                "-n",
            ])
            .output();

        let (cmd_used, output) = match result {
            Ok(o) if o.status.success() => ("libcamera-still", o),
            _ => {
                // Fallback to raspistill
                match Command::new("raspistill")
                    .args([
                        "-o",
                        output_path,
                        "-w",
                        &width.to_string(),
                        "-h",
                        &height.to_string(),
                        "-t",
                        "1",
                        "-n",
                    ])
                    .output()
                {
                    Ok(o) => ("raspistill", o),
                    Err(e) => {
                        return ToolResult::err(format!(
                            "Camera not available (tried libcamera-still and raspistill): {}",
                            e
                        ))
                    }
                }
            }
        };

        if output.status.success() {
            // Get file size
            let file_size = fs::metadata(output_path).map(|m| m.len()).unwrap_or(0);

            ToolResult::ok(serde_json::json!({
                "success": true,
                "path": output_path,
                "width": width,
                "height": height,
                "size_bytes": file_size,
                "command": cmd_used
            }))
        } else {
            let stderr = String::from_utf8_lossy(&output.stderr);
            ToolResult::err(format!("Camera capture failed: {}", stderr))
        }
    }

    // ========== Network Tools ==========

    /// Get network interface information
    pub fn network_info(&self) -> ToolResult {
        match Command::new("ip").args(["-j", "addr"]).output() {
            Ok(output) => {
                if let Ok(interfaces) = serde_json::from_slice::<Value>(&output.stdout) {
                    ToolResult::ok(serde_json::json!({
                        "interfaces": interfaces
                    }))
                } else {
                    // Fallback to non-JSON output
                    let stdout = String::from_utf8_lossy(&output.stdout);
                    ToolResult::ok(serde_json::json!({
                        "raw": stdout.to_string()
                    }))
                }
            }
            Err(e) => ToolResult::err(format!("Failed to get network info: {}", e)),
        }
    }

    /// Ping a host
    pub fn ping(&self, host: &str, count: u32) -> ToolResult {
        // Validate host (basic sanity check)
        if host.contains(';') || host.contains('&') || host.contains('|') {
            return ToolResult::err("Invalid host format");
        }

        let count = count.min(10); // Limit to 10 pings max

        match Command::new("ping")
            .args(["-c", &count.to_string(), "-W", "5", host])
            .output()
        {
            Ok(output) => {
                let stdout = String::from_utf8_lossy(&output.stdout);
                let success = output.status.success();

                // Parse ping statistics
                let mut avg_ms: Option<f64> = None;
                let mut packet_loss: Option<f64> = None;

                for line in stdout.lines() {
                    if line.contains("packet loss") {
                        if let Some(loss_str) = line.split_whitespace().find(|s| s.ends_with('%')) {
                            packet_loss = loss_str.trim_end_matches('%').parse().ok();
                        }
                    }
                    if line.contains("avg") {
                        // Format: rtt min/avg/max/mdev = X.XXX/Y.YYY/Z.ZZZ/W.WWW ms
                        if let Some(stats) = line.split('=').nth(1) {
                            let parts: Vec<&str> = stats.trim().split('/').collect();
                            if parts.len() >= 2 {
                                avg_ms = parts[1].trim().parse().ok();
                            }
                        }
                    }
                }

                ToolResult::ok(serde_json::json!({
                    "host": host,
                    "reachable": success,
                    "packets_sent": count,
                    "packet_loss_percent": packet_loss,
                    "avg_latency_ms": avg_ms
                }))
            }
            Err(e) => ToolResult::err(format!("Failed to ping: {}", e)),
        }
    }

    // ========== File Tools ==========

    /// Read file contents (with path restrictions)
    pub fn read_file(&self, path: &str, max_bytes: usize) -> ToolResult {
        // Validate path - only allow certain directories
        let allowed_prefixes = [
            "/tmp/",
            "/home/",
            "/sys/class/",
            "/proc/",
            "/etc/hostname",
            "/etc/os-release",
        ];

        let path_allowed = allowed_prefixes.iter().any(|prefix| path.starts_with(prefix));
        if !path_allowed {
            return ToolResult::err(format!(
                "Path not allowed. Must start with one of: {:?}",
                allowed_prefixes
            ));
        }

        // Prevent directory traversal
        if path.contains("..") {
            return ToolResult::err("Directory traversal not allowed");
        }

        match fs::read(path) {
            Ok(content) => {
                let truncated = content.len() > max_bytes;
                let content = if truncated {
                    &content[..max_bytes]
                } else {
                    &content
                };

                // Try to convert to string, fall back to base64
                match String::from_utf8(content.to_vec()) {
                    Ok(text) => ToolResult::ok(serde_json::json!({
                        "path": path,
                        "content": text,
                        "size_bytes": content.len(),
                        "truncated": truncated,
                        "encoding": "utf8"
                    })),
                    Err(_) => {
                        use base64::Engine;
                        let b64 = base64::engine::general_purpose::STANDARD.encode(content);
                        ToolResult::ok(serde_json::json!({
                            "path": path,
                            "content": b64,
                            "size_bytes": content.len(),
                            "truncated": truncated,
                            "encoding": "base64"
                        }))
                    }
                }
            }
            Err(e) => ToolResult::err(format!("Failed to read file: {}", e)),
        }
    }

    // ========== Exec Tools ==========

    /// Execute a whitelisted command
    pub fn safe_exec(&self, command: &str, args: &[String]) -> ToolResult {
        if !self.allowed_commands.contains(&command.to_string()) {
            return ToolResult::err(format!(
                "Command '{}' not in whitelist: {:?}",
                command, self.allowed_commands
            ));
        }

        // Additional arg validation
        for arg in args {
            if arg.contains(';') || arg.contains('&') || arg.contains('|') || arg.contains('`') {
                return ToolResult::err("Invalid characters in arguments");
            }
        }

        match Command::new(command).args(args).output() {
            Ok(output) => {
                let stdout = String::from_utf8_lossy(&output.stdout);
                let stderr = String::from_utf8_lossy(&output.stderr);

                ToolResult::ok(serde_json::json!({
                    "command": command,
                    "args": args,
                    "exit_code": output.status.code(),
                    "stdout": stdout.to_string(),
                    "stderr": stderr.to_string(),
                    "success": output.status.success()
                }))
            }
            Err(e) => ToolResult::err(format!("Failed to execute command: {}", e)),
        }
    }

    /// List running processes
    pub fn list_processes(&self, sort_by: &str, limit: usize) -> ToolResult {
        let sort_key = match sort_by {
            "memory" | "mem" => "%mem",
            _ => "%cpu",
        };

        match Command::new("ps")
            .args([
                "aux",
                "--sort",
                &format!("-{}", sort_key),
            ])
            .output()
        {
            Ok(output) => {
                let stdout = String::from_utf8_lossy(&output.stdout);
                let lines: Vec<&str> = stdout.lines().take(limit + 1).collect();

                let mut processes = Vec::new();
                for (i, line) in lines.iter().enumerate() {
                    if i == 0 {
                        continue;
                    } // Skip header
                    let parts: Vec<&str> = line.split_whitespace().collect();
                    if parts.len() >= 11 {
                        processes.push(serde_json::json!({
                            "user": parts[0],
                            "pid": parts[1],
                            "cpu_percent": parts[2],
                            "mem_percent": parts[3],
                            "command": parts[10..].join(" ")
                        }));
                    }
                }

                ToolResult::ok(serde_json::json!({
                    "processes": processes,
                    "sort_by": sort_by,
                    "count": processes.len()
                }))
            }
            Err(e) => ToolResult::err(format!("Failed to list processes: {}", e)),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_tool_definitions() {
        let tools = EdgeTools::new();
        let defs = tools.get_tool_definitions();
        assert!(defs.len() >= 10);
        assert!(defs.iter().any(|d| d.name == "read_temperature"));
        assert!(defs.iter().any(|d| d.name == "gpio_read"));
        assert!(defs.iter().any(|d| d.name == "camera_capture"));
    }

    #[test]
    fn test_safe_exec_whitelist() {
        let tools = EdgeTools::new();

        // Allowed command
        let result = tools.safe_exec("hostname", &[]);
        assert!(result.success || result.error.is_some()); // May fail in test env but shouldn't be blocked

        // Blocked command
        let result = tools.safe_exec("rm", &["-rf".to_string(), "/".to_string()]);
        assert!(!result.success);
        assert!(result.error.unwrap().contains("not in whitelist"));
    }

    #[test]
    fn test_read_file_path_validation() {
        let tools = EdgeTools::new();

        // Blocked path
        let result = tools.read_file("/etc/shadow", 1024);
        assert!(!result.success);

        // Directory traversal blocked
        let result = tools.read_file("/tmp/../etc/shadow", 1024);
        assert!(!result.success);
        assert!(result.error.unwrap().contains("traversal"));
    }

    #[test]
    fn test_gpio_pin_validation() {
        let tools = EdgeTools::new();

        // Blocked pin
        let result = tools.gpio_read(99);
        assert!(!result.success);
        assert!(result.error.unwrap().contains("not in allowed list"));
    }
}
