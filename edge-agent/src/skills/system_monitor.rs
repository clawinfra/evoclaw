use std::collections::VecDeque;

use async_trait::async_trait;
use serde_json::Value;
use tracing::warn;

use super::{Skill, SkillReport};

/// Thresholds for alert generation
#[derive(Debug, Clone)]
pub struct AlertThresholds {
    pub cpu_pct: f64,
    pub memory_pct: f64,
    pub temperature_c: f64,
    pub disk_pct: f64,
}

impl Default for AlertThresholds {
    fn default() -> Self {
        Self {
            cpu_pct: 90.0,
            memory_pct: 80.0,
            temperature_c: 70.0,
            disk_pct: 90.0,
        }
    }
}

/// A single metrics snapshot
#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct MetricsSnapshot {
    pub timestamp: u64,
    pub cpu_pct: f64,
    pub memory_used_mb: f64,
    pub memory_total_mb: f64,
    pub memory_pct: f64,
    pub disk_used_gb: f64,
    pub disk_total_gb: f64,
    pub disk_pct: f64,
    pub temperature_c: Option<f64>,
    pub uptime_secs: u64,
    pub load_1m: f64,
    pub load_5m: f64,
    pub load_15m: f64,
    pub net_rx_bytes: u64,
    pub net_tx_bytes: u64,
}

/// System Monitor Skill — monitors system health
pub struct SystemMonitorSkill {
    tick_interval: u64,
    history: VecDeque<MetricsSnapshot>,
    max_history: usize,
    thresholds: AlertThresholds,
    prev_cpu_idle: u64,
    prev_cpu_total: u64,
}

impl SystemMonitorSkill {
    pub fn new(tick_interval: u64) -> Self {
        Self {
            tick_interval,
            history: VecDeque::new(),
            max_history: 100,
            thresholds: AlertThresholds::default(),
            prev_cpu_idle: 0,
            prev_cpu_total: 0,
        }
    }

    /// Collect all current system metrics
    fn collect_metrics(&mut self) -> MetricsSnapshot {
        let timestamp = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs();

        let cpu_pct = self.read_cpu_usage();
        let (memory_used_mb, memory_total_mb, memory_pct) = self.read_memory();
        let (disk_used_gb, disk_total_gb, disk_pct) = self.read_disk();
        let temperature_c = self.read_temperature();
        let uptime_secs = self.read_uptime();
        let (load_1m, load_5m, load_15m) = self.read_load_average();
        let (net_rx_bytes, net_tx_bytes) = self.read_network();

        MetricsSnapshot {
            timestamp,
            cpu_pct,
            memory_used_mb,
            memory_total_mb,
            memory_pct,
            disk_used_gb,
            disk_total_gb,
            disk_pct,
            temperature_c,
            uptime_secs,
            load_1m,
            load_5m,
            load_15m,
            net_rx_bytes,
            net_tx_bytes,
        }
    }

    /// Check thresholds and generate alerts
    fn check_thresholds(&self, snapshot: &MetricsSnapshot) -> Vec<SkillReport> {
        let mut alerts = Vec::new();

        if snapshot.cpu_pct > self.thresholds.cpu_pct {
            alerts.push(SkillReport {
                skill: "system_monitor".to_string(),
                report_type: "alert".to_string(),
                payload: serde_json::json!({
                    "alert": "cpu_high",
                    "value": snapshot.cpu_pct,
                    "threshold": self.thresholds.cpu_pct,
                    "message": format!("CPU usage at {:.1}% (threshold: {:.0}%)", snapshot.cpu_pct, self.thresholds.cpu_pct)
                }),
            });
        }

        if snapshot.memory_pct > self.thresholds.memory_pct {
            alerts.push(SkillReport {
                skill: "system_monitor".to_string(),
                report_type: "alert".to_string(),
                payload: serde_json::json!({
                    "alert": "memory_high",
                    "value": snapshot.memory_pct,
                    "threshold": self.thresholds.memory_pct,
                    "message": format!("Memory usage at {:.1}% (threshold: {:.0}%)", snapshot.memory_pct, self.thresholds.memory_pct)
                }),
            });
        }

        if let Some(temp) = snapshot.temperature_c {
            if temp > self.thresholds.temperature_c {
                alerts.push(SkillReport {
                    skill: "system_monitor".to_string(),
                    report_type: "alert".to_string(),
                    payload: serde_json::json!({
                        "alert": "temperature_high",
                        "value": temp,
                        "threshold": self.thresholds.temperature_c,
                        "message": format!("CPU temperature at {:.1}°C (threshold: {:.0}°C)", temp, self.thresholds.temperature_c)
                    }),
                });
            }
        }

        if snapshot.disk_pct > self.thresholds.disk_pct {
            alerts.push(SkillReport {
                skill: "system_monitor".to_string(),
                report_type: "alert".to_string(),
                payload: serde_json::json!({
                    "alert": "disk_high",
                    "value": snapshot.disk_pct,
                    "threshold": self.thresholds.disk_pct,
                    "message": format!("Disk usage at {:.1}% (threshold: {:.0}%)", snapshot.disk_pct, self.thresholds.disk_pct)
                }),
            });
        }

        alerts
    }

    // --- System metric readers ---

    fn read_cpu_usage(&mut self) -> f64 {
        self.read_cpu_usage_from_path("/proc/stat")
    }

    /// Read CPU usage from /proc/stat (or any provided path for testing)
    pub fn read_cpu_usage_from_path(&mut self, path: &str) -> f64 {
        if let Ok(content) = std::fs::read_to_string(path) {
            if let Some(cpu_line) = content.lines().next() {
                let parts: Vec<&str> = cpu_line.split_whitespace().collect();
                if parts.len() >= 5 && parts[0] == "cpu" {
                    let values: Vec<u64> = parts[1..]
                        .iter()
                        .filter_map(|s| s.parse().ok())
                        .collect();
                    if values.len() >= 4 {
                        let idle = values[3];
                        let total: u64 = values.iter().sum();

                        let idle_diff = idle.saturating_sub(self.prev_cpu_idle);
                        let total_diff = total.saturating_sub(self.prev_cpu_total);

                        self.prev_cpu_idle = idle;
                        self.prev_cpu_total = total;

                        if total_diff > 0 {
                            return (1.0 - idle_diff as f64 / total_diff as f64) * 100.0;
                        }
                    }
                }
            }
        }
        0.0
    }

    fn read_memory(&self) -> (f64, f64, f64) {
        self.read_memory_from_path("/proc/meminfo")
    }

    /// Read memory info from /proc/meminfo (or any path for testing)
    pub fn read_memory_from_path(&self, path: &str) -> (f64, f64, f64) {
        if let Ok(content) = std::fs::read_to_string(path) {
            let mut total_kb: u64 = 0;
            let mut available_kb: u64 = 0;

            for line in content.lines() {
                if line.starts_with("MemTotal:") {
                    total_kb = line
                        .split_whitespace()
                        .nth(1)
                        .and_then(|v| v.parse().ok())
                        .unwrap_or(0);
                } else if line.starts_with("MemAvailable:") {
                    available_kb = line
                        .split_whitespace()
                        .nth(1)
                        .and_then(|v| v.parse().ok())
                        .unwrap_or(0);
                }
            }

            if total_kb > 0 {
                let total_mb = total_kb as f64 / 1024.0;
                let used_mb = (total_kb - available_kb) as f64 / 1024.0;
                let pct = used_mb / total_mb * 100.0;
                return (used_mb, total_mb, pct);
            }
        }
        (0.0, 0.0, 0.0)
    }

    fn read_disk(&self) -> (f64, f64, f64) {
        // Use statvfs on Linux
        #[cfg(target_os = "linux")]
        {
            return self.read_disk_statvfs("/");
        }
        #[cfg(not(target_os = "linux"))]
        {
            (0.0, 0.0, 0.0)
        }
    }

    /// Read disk usage via statvfs
    #[cfg(target_os = "linux")]
    fn read_disk_statvfs(&self, path: &str) -> (f64, f64, f64) {
        use std::ffi::CString;
        use std::mem::MaybeUninit;

        let c_path = match CString::new(path) {
            Ok(p) => p,
            Err(_) => return (0.0, 0.0, 0.0),
        };

        unsafe {
            let mut stat = MaybeUninit::<libc::statvfs>::uninit();
            if libc::statvfs(c_path.as_ptr(), stat.as_mut_ptr()) == 0 {
                let stat = stat.assume_init();
                let block_size = stat.f_frsize as f64;
                let total = stat.f_blocks as f64 * block_size;
                let free = stat.f_bfree as f64 * block_size;
                let used = total - free;

                let total_gb = total / (1024.0 * 1024.0 * 1024.0);
                let used_gb = used / (1024.0 * 1024.0 * 1024.0);
                let pct = if total > 0.0 { used / total * 100.0 } else { 0.0 };
                return (used_gb, total_gb, pct);
            }
        }
        (0.0, 0.0, 0.0)
    }

    fn read_temperature(&self) -> Option<f64> {
        self.read_temperature_from_path("/sys/class/thermal/thermal_zone0/temp")
    }

    /// Read CPU temperature from thermal zone
    pub fn read_temperature_from_path(&self, path: &str) -> Option<f64> {
        if let Ok(content) = std::fs::read_to_string(path) {
            if let Ok(millidegrees) = content.trim().parse::<u64>() {
                return Some(millidegrees as f64 / 1000.0);
            }
        }
        None
    }

    fn read_uptime(&self) -> u64 {
        self.read_uptime_from_path("/proc/uptime")
    }

    /// Read system uptime
    pub fn read_uptime_from_path(&self, path: &str) -> u64 {
        if let Ok(content) = std::fs::read_to_string(path) {
            if let Some(secs_str) = content.split_whitespace().next() {
                if let Ok(secs) = secs_str.parse::<f64>() {
                    return secs as u64;
                }
            }
        }
        0
    }

    fn read_load_average(&self) -> (f64, f64, f64) {
        self.read_load_average_from_path("/proc/loadavg")
    }

    /// Read load average
    pub fn read_load_average_from_path(&self, path: &str) -> (f64, f64, f64) {
        if let Ok(content) = std::fs::read_to_string(path) {
            let parts: Vec<&str> = content.split_whitespace().collect();
            if parts.len() >= 3 {
                let l1 = parts[0].parse().unwrap_or(0.0);
                let l5 = parts[1].parse().unwrap_or(0.0);
                let l15 = parts[2].parse().unwrap_or(0.0);
                return (l1, l5, l15);
            }
        }
        (0.0, 0.0, 0.0)
    }

    fn read_network(&self) -> (u64, u64) {
        self.read_network_from_path("/proc/net/dev")
    }

    /// Read network bytes
    pub fn read_network_from_path(&self, path: &str) -> (u64, u64) {
        if let Ok(content) = std::fs::read_to_string(path) {
            let mut rx_total: u64 = 0;
            let mut tx_total: u64 = 0;

            for line in content.lines().skip(2) {
                // Skip header lines
                let parts: Vec<&str> = line.split_whitespace().collect();
                if parts.len() >= 10 {
                    let iface = parts[0].trim_end_matches(':');
                    // Skip loopback
                    if iface == "lo" {
                        continue;
                    }
                    if let Ok(rx) = parts[1].parse::<u64>() {
                        rx_total += rx;
                    }
                    if let Ok(tx) = parts[9].parse::<u64>() {
                        tx_total += tx;
                    }
                }
            }
            return (rx_total, tx_total);
        }
        (0, 0)
    }
}

#[async_trait]
impl Skill for SystemMonitorSkill {
    fn name(&self) -> &str {
        "system_monitor"
    }

    fn capabilities(&self) -> Vec<String> {
        vec![
            "system.cpu".to_string(),
            "system.memory".to_string(),
            "system.disk".to_string(),
            "system.temperature".to_string(),
            "system.uptime".to_string(),
            "system.load".to_string(),
            "system.network".to_string(),
        ]
    }

    async fn init(&mut self) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        // Initial CPU read to prime the diff calculation
        let _ = self.read_cpu_usage();
        Ok(())
    }

    async fn handle(
        &mut self,
        command: &str,
        payload: Value,
    ) -> Result<Value, Box<dyn std::error::Error + Send + Sync>> {
        match command {
            "status" => {
                let snapshot = self.collect_metrics();
                Ok(serde_json::to_value(&snapshot)?)
            }
            "history" => {
                let count = payload
                    .get("count")
                    .and_then(|v| v.as_u64())
                    .unwrap_or(10) as usize;
                let history: Vec<&MetricsSnapshot> = self
                    .history
                    .iter()
                    .rev()
                    .take(count)
                    .collect();
                Ok(serde_json::json!({
                    "count": history.len(),
                    "snapshots": history
                }))
            }
            "alert_threshold" => {
                if let Some(cpu) = payload.get("cpu_pct").and_then(|v| v.as_f64()) {
                    self.thresholds.cpu_pct = cpu;
                }
                if let Some(mem) = payload.get("memory_pct").and_then(|v| v.as_f64()) {
                    self.thresholds.memory_pct = mem;
                }
                if let Some(temp) = payload.get("temperature_c").and_then(|v| v.as_f64()) {
                    self.thresholds.temperature_c = temp;
                }
                if let Some(disk) = payload.get("disk_pct").and_then(|v| v.as_f64()) {
                    self.thresholds.disk_pct = disk;
                }
                Ok(serde_json::json!({
                    "status": "thresholds_updated",
                    "cpu_pct": self.thresholds.cpu_pct,
                    "memory_pct": self.thresholds.memory_pct,
                    "temperature_c": self.thresholds.temperature_c,
                    "disk_pct": self.thresholds.disk_pct
                }))
            }
            _ => Err(format!("unknown command: {}", command).into()),
        }
    }

    async fn tick(&mut self) -> Option<SkillReport> {
        let snapshot = self.collect_metrics();

        // Check thresholds — alerts are collected but we return the main metric report
        // Alerts could be published separately in a production setup
        let _alerts = self.check_thresholds(&snapshot);

        // Store in history
        self.history.push_back(snapshot.clone());
        if self.history.len() > self.max_history {
            self.history.pop_front();
        }

        Some(SkillReport {
            skill: "system_monitor".to_string(),
            report_type: "metric".to_string(),
            payload: serde_json::to_value(&snapshot).unwrap_or_default(),
        })
    }

    fn tick_interval_secs(&self) -> u64 {
        self.tick_interval
    }
}

#[cfg(target_os = "linux")]
extern crate libc;

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Write;
    use tempfile::NamedTempFile;

    #[test]
    fn test_system_monitor_new() {
        let skill = SystemMonitorSkill::new(30);
        assert_eq!(skill.tick_interval, 30);
        assert_eq!(skill.max_history, 100);
        assert!(skill.history.is_empty());
    }

    #[test]
    fn test_skill_name() {
        let skill = SystemMonitorSkill::new(30);
        assert_eq!(skill.name(), "system_monitor");
    }

    #[test]
    fn test_skill_capabilities() {
        let skill = SystemMonitorSkill::new(30);
        let caps = skill.capabilities();
        assert_eq!(caps.len(), 7);
        assert!(caps.contains(&"system.cpu".to_string()));
        assert!(caps.contains(&"system.memory".to_string()));
        assert!(caps.contains(&"system.disk".to_string()));
        assert!(caps.contains(&"system.temperature".to_string()));
        assert!(caps.contains(&"system.uptime".to_string()));
        assert!(caps.contains(&"system.load".to_string()));
        assert!(caps.contains(&"system.network".to_string()));
    }

    #[test]
    fn test_tick_interval() {
        let skill = SystemMonitorSkill::new(60);
        assert_eq!(skill.tick_interval_secs(), 60);
    }

    #[test]
    fn test_read_cpu_usage_from_file() {
        let mut f = NamedTempFile::new().unwrap();
        writeln!(f, "cpu  1000 200 300 5000 100 50 30 0 0 0").unwrap();
        let path = f.path().to_str().unwrap();

        let mut skill = SystemMonitorSkill::new(30);
        // First read primes the counters
        let _ = skill.read_cpu_usage_from_path(path);

        // Second read with increased values
        let mut f2 = NamedTempFile::new().unwrap();
        writeln!(f2, "cpu  1100 210 310 5200 110 55 35 0 0 0").unwrap();
        let path2 = f2.path().to_str().unwrap();

        let cpu = skill.read_cpu_usage_from_path(path2);
        // Diff: total_diff = (1100+210+310+5200+110+55+35) - (1000+200+300+5000+100+50+30) = 7020 - 6680 = 340
        // idle_diff = 5200 - 5000 = 200
        // cpu = (1 - 200/340) * 100 ≈ 41.17%
        assert!(cpu > 0.0);
        assert!(cpu < 100.0);
    }

    #[test]
    fn test_read_cpu_usage_invalid_file() {
        let mut skill = SystemMonitorSkill::new(30);
        let cpu = skill.read_cpu_usage_from_path("/nonexistent/file");
        assert_eq!(cpu, 0.0);
    }

    #[test]
    fn test_read_memory_from_file() {
        let mut f = NamedTempFile::new().unwrap();
        writeln!(f, "MemTotal:       16384000 kB").unwrap();
        writeln!(f, "MemFree:         1000000 kB").unwrap();
        writeln!(f, "MemAvailable:    8000000 kB").unwrap();
        let path = f.path().to_str().unwrap();

        let skill = SystemMonitorSkill::new(30);
        let (used, total, pct) = skill.read_memory_from_path(path);

        assert!((total - 16000.0).abs() < 1.0); // 16384000/1024 = 16000
        assert!(used > 0.0);
        assert!(pct > 0.0);
        assert!(pct < 100.0);
    }

    #[test]
    fn test_read_memory_invalid_file() {
        let skill = SystemMonitorSkill::new(30);
        let (used, total, pct) = skill.read_memory_from_path("/nonexistent/file");
        assert_eq!(used, 0.0);
        assert_eq!(total, 0.0);
        assert_eq!(pct, 0.0);
    }

    #[test]
    fn test_read_temperature_from_file() {
        let mut f = NamedTempFile::new().unwrap();
        write!(f, "48300").unwrap();
        let path = f.path().to_str().unwrap();

        let skill = SystemMonitorSkill::new(30);
        let temp = skill.read_temperature_from_path(path);
        assert_eq!(temp, Some(48.3));
    }

    #[test]
    fn test_read_temperature_missing() {
        let skill = SystemMonitorSkill::new(30);
        let temp = skill.read_temperature_from_path("/nonexistent/file");
        assert_eq!(temp, None);
    }

    #[test]
    fn test_read_uptime_from_file() {
        let mut f = NamedTempFile::new().unwrap();
        write!(f, "3847.23 7000.45").unwrap();
        let path = f.path().to_str().unwrap();

        let skill = SystemMonitorSkill::new(30);
        let uptime = skill.read_uptime_from_path(path);
        assert_eq!(uptime, 3847);
    }

    #[test]
    fn test_read_uptime_missing() {
        let skill = SystemMonitorSkill::new(30);
        let uptime = skill.read_uptime_from_path("/nonexistent/file");
        assert_eq!(uptime, 0);
    }

    #[test]
    fn test_read_load_average_from_file() {
        let mut f = NamedTempFile::new().unwrap();
        write!(f, "0.15 0.12 0.17 1/234 5678").unwrap();
        let path = f.path().to_str().unwrap();

        let skill = SystemMonitorSkill::new(30);
        let (l1, l5, l15) = skill.read_load_average_from_path(path);
        assert!((l1 - 0.15).abs() < 0.001);
        assert!((l5 - 0.12).abs() < 0.001);
        assert!((l15 - 0.17).abs() < 0.001);
    }

    #[test]
    fn test_read_load_average_missing() {
        let skill = SystemMonitorSkill::new(30);
        let (l1, l5, l15) = skill.read_load_average_from_path("/nonexistent/file");
        assert_eq!(l1, 0.0);
        assert_eq!(l5, 0.0);
        assert_eq!(l15, 0.0);
    }

    #[test]
    fn test_read_network_from_file() {
        let mut f = NamedTempFile::new().unwrap();
        writeln!(f, "Inter-|   Receive                                                |  Transmit").unwrap();
        writeln!(f, " face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed").unwrap();
        writeln!(f, "    lo: 1000000       100    0    0    0     0          0         0  1000000       100    0    0    0     0       0          0").unwrap();
        writeln!(f, "  eth0: 1234567      2000    0    0    0     0          0         0   987654      1500    0    0    0     0       0          0").unwrap();
        writeln!(f, " wlan0:  500000      1000    0    0    0     0          0         0   300000       800    0    0    0     0       0          0").unwrap();
        let path = f.path().to_str().unwrap();

        let skill = SystemMonitorSkill::new(30);
        let (rx, tx) = skill.read_network_from_path(path);
        // Should exclude loopback
        assert_eq!(rx, 1234567 + 500000);
        assert_eq!(tx, 987654 + 300000);
    }

    #[test]
    fn test_read_network_missing() {
        let skill = SystemMonitorSkill::new(30);
        let (rx, tx) = skill.read_network_from_path("/nonexistent/file");
        assert_eq!(rx, 0);
        assert_eq!(tx, 0);
    }

    #[test]
    fn test_alert_thresholds_default() {
        let thresholds = AlertThresholds::default();
        assert_eq!(thresholds.cpu_pct, 90.0);
        assert_eq!(thresholds.memory_pct, 80.0);
        assert_eq!(thresholds.temperature_c, 70.0);
        assert_eq!(thresholds.disk_pct, 90.0);
    }

    #[test]
    fn test_check_thresholds_no_alerts() {
        let skill = SystemMonitorSkill::new(30);
        let snapshot = MetricsSnapshot {
            timestamp: 0,
            cpu_pct: 10.0,
            memory_used_mb: 100.0,
            memory_total_mb: 1000.0,
            memory_pct: 10.0,
            disk_used_gb: 1.0,
            disk_total_gb: 10.0,
            disk_pct: 10.0,
            temperature_c: Some(40.0),
            uptime_secs: 1000,
            load_1m: 0.1,
            load_5m: 0.1,
            load_15m: 0.1,
            net_rx_bytes: 0,
            net_tx_bytes: 0,
        };
        let alerts = skill.check_thresholds(&snapshot);
        assert!(alerts.is_empty());
    }

    #[test]
    fn test_check_thresholds_cpu_alert() {
        let skill = SystemMonitorSkill::new(30);
        let snapshot = MetricsSnapshot {
            timestamp: 0,
            cpu_pct: 95.0,
            memory_used_mb: 100.0,
            memory_total_mb: 1000.0,
            memory_pct: 10.0,
            disk_used_gb: 1.0,
            disk_total_gb: 10.0,
            disk_pct: 10.0,
            temperature_c: None,
            uptime_secs: 1000,
            load_1m: 0.1,
            load_5m: 0.1,
            load_15m: 0.1,
            net_rx_bytes: 0,
            net_tx_bytes: 0,
        };
        let alerts = skill.check_thresholds(&snapshot);
        assert_eq!(alerts.len(), 1);
        assert_eq!(alerts[0].payload["alert"], "cpu_high");
    }

    #[test]
    fn test_check_thresholds_memory_alert() {
        let skill = SystemMonitorSkill::new(30);
        let snapshot = MetricsSnapshot {
            timestamp: 0,
            cpu_pct: 10.0,
            memory_used_mb: 900.0,
            memory_total_mb: 1000.0,
            memory_pct: 90.0,
            disk_used_gb: 1.0,
            disk_total_gb: 10.0,
            disk_pct: 10.0,
            temperature_c: None,
            uptime_secs: 1000,
            load_1m: 0.1,
            load_5m: 0.1,
            load_15m: 0.1,
            net_rx_bytes: 0,
            net_tx_bytes: 0,
        };
        let alerts = skill.check_thresholds(&snapshot);
        assert_eq!(alerts.len(), 1);
        assert_eq!(alerts[0].payload["alert"], "memory_high");
    }

    #[test]
    fn test_check_thresholds_temperature_alert() {
        let skill = SystemMonitorSkill::new(30);
        let snapshot = MetricsSnapshot {
            timestamp: 0,
            cpu_pct: 10.0,
            memory_used_mb: 100.0,
            memory_total_mb: 1000.0,
            memory_pct: 10.0,
            disk_used_gb: 1.0,
            disk_total_gb: 10.0,
            disk_pct: 10.0,
            temperature_c: Some(75.0),
            uptime_secs: 1000,
            load_1m: 0.1,
            load_5m: 0.1,
            load_15m: 0.1,
            net_rx_bytes: 0,
            net_tx_bytes: 0,
        };
        let alerts = skill.check_thresholds(&snapshot);
        assert_eq!(alerts.len(), 1);
        assert_eq!(alerts[0].payload["alert"], "temperature_high");
    }

    #[test]
    fn test_check_thresholds_multiple_alerts() {
        let skill = SystemMonitorSkill::new(30);
        let snapshot = MetricsSnapshot {
            timestamp: 0,
            cpu_pct: 95.0,
            memory_used_mb: 900.0,
            memory_total_mb: 1000.0,
            memory_pct: 90.0,
            disk_used_gb: 9.5,
            disk_total_gb: 10.0,
            disk_pct: 95.0,
            temperature_c: Some(75.0),
            uptime_secs: 1000,
            load_1m: 0.1,
            load_5m: 0.1,
            load_15m: 0.1,
            net_rx_bytes: 0,
            net_tx_bytes: 0,
        };
        let alerts = skill.check_thresholds(&snapshot);
        assert_eq!(alerts.len(), 4);
    }

    #[tokio::test]
    async fn test_init() {
        let mut skill = SystemMonitorSkill::new(30);
        let result = skill.init().await;
        assert!(result.is_ok());
    }

    #[tokio::test]
    async fn test_handle_status() {
        let mut skill = SystemMonitorSkill::new(30);
        let result = skill.handle("status", serde_json::json!({})).await;
        assert!(result.is_ok());
        let val = result.unwrap();
        assert!(val.get("cpu_pct").is_some());
        assert!(val.get("memory_used_mb").is_some());
        assert!(val.get("uptime_secs").is_some());
    }

    #[tokio::test]
    async fn test_handle_history_empty() {
        let mut skill = SystemMonitorSkill::new(30);
        let result = skill.handle("history", serde_json::json!({})).await;
        assert!(result.is_ok());
        let val = result.unwrap();
        assert_eq!(val["count"], 0);
    }

    #[tokio::test]
    async fn test_handle_alert_threshold() {
        let mut skill = SystemMonitorSkill::new(30);
        let result = skill
            .handle(
                "alert_threshold",
                serde_json::json!({
                    "cpu_pct": 95.0,
                    "memory_pct": 85.0,
                    "temperature_c": 65.0
                }),
            )
            .await;
        assert!(result.is_ok());
        let val = result.unwrap();
        assert_eq!(val["cpu_pct"], 95.0);
        assert_eq!(val["memory_pct"], 85.0);
        assert_eq!(val["temperature_c"], 65.0);
        assert_eq!(val["disk_pct"], 90.0); // unchanged
    }

    #[tokio::test]
    async fn test_handle_unknown_command() {
        let mut skill = SystemMonitorSkill::new(30);
        let result = skill.handle("unknown", serde_json::json!({})).await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_tick() {
        let mut skill = SystemMonitorSkill::new(30);
        skill.init().await.unwrap();

        let report = skill.tick().await;
        assert!(report.is_some());
        let report = report.unwrap();
        assert_eq!(report.skill, "system_monitor");
        assert_eq!(report.report_type, "metric");
        assert!(report.payload.get("cpu_pct").is_some());

        // History should have one entry
        assert_eq!(skill.history.len(), 1);
    }

    #[tokio::test]
    async fn test_tick_history_max() {
        let mut skill = SystemMonitorSkill::new(1);
        skill.max_history = 3;
        skill.init().await.unwrap();

        for _ in 0..5 {
            skill.tick().await;
        }

        // History should be capped at max_history
        assert_eq!(skill.history.len(), 3);
    }

    #[test]
    fn test_metrics_snapshot_serialization() {
        let snapshot = MetricsSnapshot {
            timestamp: 1234567890,
            cpu_pct: 12.5,
            memory_used_mb: 112.0,
            memory_total_mb: 427.0,
            memory_pct: 26.2,
            disk_used_gb: 2.1,
            disk_total_gb: 3.1,
            disk_pct: 67.7,
            temperature_c: Some(48.3),
            uptime_secs: 3847,
            load_1m: 0.15,
            load_5m: 0.12,
            load_15m: 0.17,
            net_rx_bytes: 1234567,
            net_tx_bytes: 987654,
        };
        let json = serde_json::to_string(&snapshot).unwrap();
        let deserialized: MetricsSnapshot = serde_json::from_str(&json).unwrap();
        assert_eq!(deserialized.cpu_pct, 12.5);
        assert_eq!(deserialized.temperature_c, Some(48.3));
        assert_eq!(deserialized.net_rx_bytes, 1234567);
    }

    #[tokio::test]
    async fn test_handle_history_with_data() {
        let mut skill = SystemMonitorSkill::new(30);
        skill.init().await.unwrap();

        // Generate some ticks
        skill.tick().await;
        skill.tick().await;
        skill.tick().await;

        let result = skill.handle("history", serde_json::json!({"count": 2})).await;
        assert!(result.is_ok());
        let val = result.unwrap();
        assert_eq!(val["count"], 2);
    }
}
