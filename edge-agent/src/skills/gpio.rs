use std::collections::HashMap;
use std::path::{Path, PathBuf};

use async_trait::async_trait;
use serde_json::Value;
use tracing::{info, warn};

use super::{Skill, SkillReport};

/// Valid BCM pin numbers for Raspberry Pi
const VALID_BCM_PINS: &[u8] = &[
    2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26,
    27,
];

/// Pin state
#[derive(Debug, Clone, Copy, PartialEq)]
pub enum PinValue {
    Low = 0,
    High = 1,
}

impl From<u8> for PinValue {
    fn from(v: u8) -> Self {
        if v == 0 {
            PinValue::Low
        } else {
            PinValue::High
        }
    }
}

/// Pin direction
#[derive(Debug, Clone, Copy, PartialEq)]
pub enum PinDirection {
    Input,
    Output,
}

impl PinDirection {
    fn as_str(&self) -> &str {
        match self {
            PinDirection::Input => "in",
            PinDirection::Output => "out",
        }
    }

    fn from_str(s: &str) -> Option<Self> {
        match s {
            "in" | "input" => Some(PinDirection::Input),
            "out" | "output" => Some(PinDirection::Output),
            _ => None,
        }
    }
}

/// State of a single GPIO pin
#[derive(Debug, Clone)]
pub struct PinState {
    pub pin: u8,
    pub direction: PinDirection,
    pub value: PinValue,
    pub exported: bool,
}

/// GPIO Skill for Raspberry Pi GPIO control via sysfs
pub struct GpioSkill {
    allowed_pins: Vec<u8>,
    pin_states: HashMap<u8, PinState>,
    gpio_available: bool,
    sysfs_base: PathBuf,
    prev_input_values: HashMap<u8, PinValue>,
    /// Offset for gpiochip (Pi 1/2 = 512, Pi 3/4 = 0, Pi 5 = 571)
    gpio_offset: u32,
}

impl GpioSkill {
    pub fn new(pins: Vec<u8>) -> Self {
        let sysfs_base = PathBuf::from("/sys/class/gpio");
        let gpio_offset = Self::detect_gpio_offset(&sysfs_base);
        Self {
            allowed_pins: pins,
            pin_states: HashMap::new(),
            gpio_available: false,
            sysfs_base,
            prev_input_values: HashMap::new(),
            gpio_offset,
        }
    }

    /// Detect gpiochip offset by reading /sys/class/gpio/gpiochipN
    /// Pi 1/2: gpiochip512, Pi 3/4: gpiochip0, Pi 5: gpiochip571
    fn detect_gpio_offset(sysfs_base: &Path) -> u32 {
        if let Ok(entries) = std::fs::read_dir(sysfs_base) {
            for entry in entries.flatten() {
                let name = entry.file_name();
                let name_str = name.to_string_lossy();
                if name_str.starts_with("gpiochip") {
                    if let Ok(offset) = name_str.trim_start_matches("gpiochip").parse::<u32>() {
                        if offset > 0 {
                            tracing::info!(offset, "detected GPIO chip offset");
                            return offset;
                        }
                    }
                }
            }
        }
        0 // Default: no offset (Pi 3/4 or non-Pi)
    }

    /// Convert BCM pin number to sysfs GPIO number
    fn bcm_to_sysfs(&self, pin: u8) -> u32 {
        self.gpio_offset + pin as u32
    }

    /// Create with a custom sysfs path (for testing)
    #[cfg(test)]
    pub fn with_sysfs_base(pins: Vec<u8>, base: PathBuf) -> Self {
        Self {
            allowed_pins: pins,
            pin_states: HashMap::new(),
            gpio_available: false,
            sysfs_base: base,
            prev_input_values: HashMap::new(),
            gpio_offset: 0, // Tests use no offset
        }
    }

    /// Check if a pin is allowed
    fn is_allowed(&self, pin: u8) -> bool {
        self.allowed_pins.contains(&pin) && VALID_BCM_PINS.contains(&pin)
    }

    /// Check if GPIO sysfs is available
    fn check_gpio_available(&self) -> bool {
        self.sysfs_base.exists()
    }

    /// Export a pin via sysfs
    fn export_pin(&mut self, pin: u8) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        if !self.is_allowed(pin) {
            return Err(format!("pin {} is not in the allowed list", pin).into());
        }

        let sysfs_pin = self.bcm_to_sysfs(pin);
        let pin_path = self.sysfs_base.join(format!("gpio{}", sysfs_pin));
        if !pin_path.exists() {
            let export_path = self.sysfs_base.join("export");
            std::fs::write(&export_path, sysfs_pin.to_string())?;
            // Wait briefly for sysfs to create the directory
            std::thread::sleep(std::time::Duration::from_millis(50));
        }

        self.pin_states.insert(
            pin,
            PinState {
                pin,
                direction: PinDirection::Input,
                value: PinValue::Low,
                exported: true,
            },
        );

        Ok(())
    }

    /// Unexport a pin via sysfs
    fn unexport_pin(&mut self, pin: u8) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        let sysfs_pin = self.bcm_to_sysfs(pin);
        let unexport_path = self.sysfs_base.join("unexport");
        let _ = std::fs::write(&unexport_path, sysfs_pin.to_string());
        self.pin_states.remove(&pin);
        Ok(())
    }

    /// Set pin direction
    fn set_direction(
        &mut self,
        pin: u8,
        direction: PinDirection,
    ) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        if !self.is_allowed(pin) {
            return Err(format!("pin {} is not allowed", pin).into());
        }

        let sysfs_pin = self.bcm_to_sysfs(pin);
        let direction_path = self
            .sysfs_base
            .join(format!("gpio{}", sysfs_pin))
            .join("direction");
        std::fs::write(&direction_path, direction.as_str())?;

        if let Some(state) = self.pin_states.get_mut(&pin) {
            state.direction = direction;
        }

        Ok(())
    }

    /// Read pin value
    fn read_pin(&self, pin: u8) -> Result<PinValue, Box<dyn std::error::Error + Send + Sync>> {
        if !self.is_allowed(pin) {
            return Err(format!("pin {} is not allowed", pin).into());
        }

        let sysfs_pin = self.bcm_to_sysfs(pin);
        let value_path = self
            .sysfs_base
            .join(format!("gpio{}", sysfs_pin))
            .join("value");
        let content = std::fs::read_to_string(&value_path)?;
        let val: u8 = content.trim().parse().unwrap_or(0);
        Ok(PinValue::from(val))
    }

    /// Write pin value
    fn write_pin(
        &mut self,
        pin: u8,
        value: PinValue,
    ) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        if !self.is_allowed(pin) {
            return Err(format!("pin {} is not allowed", pin).into());
        }

        let sysfs_pin = self.bcm_to_sysfs(pin);
        let value_path = self
            .sysfs_base
            .join(format!("gpio{}", sysfs_pin))
            .join("value");
        std::fs::write(&value_path, (value as u8).to_string())?;

        if let Some(state) = self.pin_states.get_mut(&pin) {
            state.value = value;
        }

        Ok(())
    }

    /// Get all pin statuses
    fn get_all_status(
        &self,
    ) -> Vec<serde_json::Value> {
        self.pin_states
            .values()
            .map(|state| {
                serde_json::json!({
                    "pin": state.pin,
                    "direction": state.direction.as_str(),
                    "value": state.value as u8,
                    "exported": state.exported,
                })
            })
            .collect()
    }

    /// Blink a pin (blocking, runs in a loop)
    async fn blink_pin(
        &mut self,
        pin: u8,
        count: u32,
        interval_ms: u64,
    ) -> Result<Value, Box<dyn std::error::Error + Send + Sync>> {
        if !self.is_allowed(pin) {
            return Err(format!("pin {} is not allowed", pin).into());
        }

        for _ in 0..count {
            self.write_pin(pin, PinValue::High)?;
            tokio::time::sleep(std::time::Duration::from_millis(interval_ms)).await;
            self.write_pin(pin, PinValue::Low)?;
            tokio::time::sleep(std::time::Duration::from_millis(interval_ms)).await;
        }

        Ok(serde_json::json!({
            "status": "ok",
            "pin": pin,
            "blinked": count
        }))
    }
}

#[async_trait]
impl Skill for GpioSkill {
    fn name(&self) -> &str {
        "gpio"
    }

    fn capabilities(&self) -> Vec<String> {
        vec![
            "gpio.read".to_string(),
            "gpio.write".to_string(),
            "gpio.mode".to_string(),
            "gpio.pwm".to_string(),
            "gpio.watch".to_string(),
        ]
    }

    async fn init(&mut self) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        self.gpio_available = self.check_gpio_available();

        if !self.gpio_available {
            warn!("GPIO sysfs not available — GPIO skill will operate in simulation mode");
            // Still mark as "init'd" — just won't do real I/O
            return Ok(());
        }

        // Export configured pins
        for &pin in &self.allowed_pins.clone() {
            match self.export_pin(pin) {
                Ok(_) => info!(pin = pin, "GPIO pin exported"),
                Err(e) => warn!(pin = pin, error = %e, "failed to export GPIO pin"),
            }
        }

        Ok(())
    }

    async fn handle(
        &mut self,
        command: &str,
        payload: Value,
    ) -> Result<Value, Box<dyn std::error::Error + Send + Sync>> {
        match command {
            "read" => {
                let pin = payload
                    .get("pin")
                    .and_then(|v| v.as_u64())
                    .ok_or("missing pin")? as u8;

                if !self.gpio_available {
                    let simulated = self
                        .pin_states
                        .get(&pin)
                        .map(|s| s.value as u8)
                        .unwrap_or(0);
                    return Ok(serde_json::json!({"pin": pin, "value": simulated, "simulated": true}));
                }

                let value = self.read_pin(pin)?;
                Ok(serde_json::json!({"pin": pin, "value": value as u8}))
            }
            "write" => {
                let pin = payload
                    .get("pin")
                    .and_then(|v| v.as_u64())
                    .ok_or("missing pin")? as u8;
                let value = payload
                    .get("value")
                    .and_then(|v| v.as_u64())
                    .ok_or("missing value")? as u8;

                let pin_value = PinValue::from(value);

                if !self.gpio_available {
                    // Simulate
                    if let Some(state) = self.pin_states.get_mut(&pin) {
                        state.value = pin_value;
                    } else if self.is_allowed(pin) {
                        self.pin_states.insert(pin, PinState {
                            pin,
                            direction: PinDirection::Output,
                            value: pin_value,
                            exported: false,
                        });
                    }
                    return Ok(
                        serde_json::json!({"pin": pin, "value": value, "status": "ok", "simulated": true}),
                    );
                }

                self.write_pin(pin, pin_value)?;
                Ok(serde_json::json!({"pin": pin, "value": value, "status": "ok"}))
            }
            "mode" => {
                let pin = payload
                    .get("pin")
                    .and_then(|v| v.as_u64())
                    .ok_or("missing pin")? as u8;
                let direction_str = payload
                    .get("direction")
                    .and_then(|v| v.as_str())
                    .ok_or("missing direction")?;
                let direction = PinDirection::from_str(direction_str)
                    .ok_or("invalid direction (use 'input' or 'output')")?;

                if !self.gpio_available {
                    if let Some(state) = self.pin_states.get_mut(&pin) {
                        state.direction = direction;
                    } else if self.is_allowed(pin) {
                        self.pin_states.insert(pin, PinState {
                            pin,
                            direction,
                            value: PinValue::Low,
                            exported: false,
                        });
                    }
                    return Ok(serde_json::json!({"pin": pin, "direction": direction_str, "simulated": true}));
                }

                self.set_direction(pin, direction)?;
                Ok(serde_json::json!({"pin": pin, "direction": direction_str}))
            }
            "status" => {
                let pins = self.get_all_status();
                Ok(serde_json::json!({
                    "gpio_available": self.gpio_available,
                    "pins": pins,
                    "allowed_pins": self.allowed_pins,
                    "count": pins.len()
                }))
            }
            "blink" => {
                let pin = payload
                    .get("pin")
                    .and_then(|v| v.as_u64())
                    .ok_or("missing pin")? as u8;
                let count = payload
                    .get("count")
                    .and_then(|v| v.as_u64())
                    .unwrap_or(5) as u32;
                let interval_ms = payload
                    .get("interval_ms")
                    .and_then(|v| v.as_u64())
                    .unwrap_or(500);

                if !self.gpio_available {
                    return Ok(serde_json::json!({
                        "status": "ok",
                        "pin": pin,
                        "blinked": count,
                        "simulated": true
                    }));
                }

                self.blink_pin(pin, count, interval_ms).await
            }
            _ => Err(format!("unknown GPIO command: {}", command).into()),
        }
    }

    async fn tick(&mut self) -> Option<SkillReport> {
        if !self.gpio_available {
            return None;
        }

        // Read all input pins and detect state changes
        let mut changes = Vec::new();

        for &pin in &self.allowed_pins.clone() {
            if let Some(state) = self.pin_states.get(&pin) {
                if state.direction != PinDirection::Input {
                    continue;
                }
            } else {
                continue;
            }

            if let Ok(value) = self.read_pin(pin) {
                let prev = self.prev_input_values.get(&pin).copied();
                if prev.is_some() && prev != Some(value) {
                    changes.push(serde_json::json!({
                        "pin": pin,
                        "from": prev.map(|v| v as u8),
                        "to": value as u8
                    }));
                }
                self.prev_input_values.insert(pin, value);
            }
        }

        if changes.is_empty() {
            return None;
        }

        Some(SkillReport {
            skill: "gpio".to_string(),
            report_type: "data".to_string(),
            payload: serde_json::json!({
                "changes": changes,
                "count": changes.len()
            }),
        })
    }

    fn tick_interval_secs(&self) -> u64 {
        1 // Check pins every second
    }

    async fn shutdown(&mut self) {
        // Unexport all pins
        let pins: Vec<u8> = self.pin_states.keys().cloned().collect();
        for pin in pins {
            if let Err(e) = self.unexport_pin(pin) {
                warn!(pin = pin, error = %e, "failed to unexport pin on shutdown");
            }
        }
        info!("GPIO skill shutdown complete");
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    fn setup_mock_sysfs(pins: &[u8]) -> (TempDir, GpioSkill) {
        let tmp = TempDir::new().unwrap();
        let base = tmp.path().to_path_buf();

        // Create export/unexport files
        std::fs::write(base.join("export"), "").unwrap();
        std::fs::write(base.join("unexport"), "").unwrap();

        // Create pin directories
        for &pin in pins {
            let pin_dir = base.join(format!("gpio{}", pin));
            std::fs::create_dir_all(&pin_dir).unwrap();
            std::fs::write(pin_dir.join("direction"), "in").unwrap();
            std::fs::write(pin_dir.join("value"), "0").unwrap();
        }

        let skill = GpioSkill::with_sysfs_base(pins.to_vec(), base);
        (tmp, skill)
    }

    #[test]
    fn test_gpio_skill_new() {
        let skill = GpioSkill::new(vec![17, 27, 22]);
        assert_eq!(skill.allowed_pins, vec![17, 27, 22]);
        assert!(skill.pin_states.is_empty());
        assert!(!skill.gpio_available);
    }

    #[test]
    fn test_skill_name() {
        let skill = GpioSkill::new(vec![]);
        assert_eq!(skill.name(), "gpio");
    }

    #[test]
    fn test_skill_capabilities() {
        let skill = GpioSkill::new(vec![]);
        let caps = skill.capabilities();
        assert_eq!(caps.len(), 5);
        assert!(caps.contains(&"gpio.read".to_string()));
        assert!(caps.contains(&"gpio.write".to_string()));
    }

    #[test]
    fn test_tick_interval() {
        let skill = GpioSkill::new(vec![]);
        assert_eq!(skill.tick_interval_secs(), 1);
    }

    #[test]
    fn test_is_allowed() {
        let skill = GpioSkill::new(vec![17, 27]);
        assert!(skill.is_allowed(17));
        assert!(skill.is_allowed(27));
        assert!(!skill.is_allowed(22));
        assert!(!skill.is_allowed(99)); // Not a valid BCM pin
    }

    #[test]
    fn test_pin_value_from() {
        assert_eq!(PinValue::from(0), PinValue::Low);
        assert_eq!(PinValue::from(1), PinValue::High);
        assert_eq!(PinValue::from(5), PinValue::High); // Nonzero = high
    }

    #[test]
    fn test_pin_direction_from_str() {
        assert_eq!(PinDirection::from_str("in"), Some(PinDirection::Input));
        assert_eq!(PinDirection::from_str("input"), Some(PinDirection::Input));
        assert_eq!(PinDirection::from_str("out"), Some(PinDirection::Output));
        assert_eq!(PinDirection::from_str("output"), Some(PinDirection::Output));
        assert_eq!(PinDirection::from_str("invalid"), None);
    }

    #[test]
    fn test_pin_direction_as_str() {
        assert_eq!(PinDirection::Input.as_str(), "in");
        assert_eq!(PinDirection::Output.as_str(), "out");
    }

    #[test]
    fn test_export_pin_with_mock_sysfs() {
        let (_tmp, mut skill) = setup_mock_sysfs(&[17, 27]);
        skill.gpio_available = true;

        let result = skill.export_pin(17);
        assert!(result.is_ok());
        assert!(skill.pin_states.contains_key(&17));
    }

    #[test]
    fn test_export_pin_not_allowed() {
        let (_tmp, mut skill) = setup_mock_sysfs(&[17]);
        let result = skill.export_pin(22); // Not in allowed list
        assert!(result.is_err());
    }

    #[test]
    fn test_read_pin_mock() {
        let (tmp, mut skill) = setup_mock_sysfs(&[17]);
        skill.gpio_available = true;

        // Write a known value
        let value_path = tmp.path().join("gpio17").join("value");
        std::fs::write(&value_path, "1").unwrap();

        let value = skill.read_pin(17);
        assert!(value.is_ok());
        assert_eq!(value.unwrap(), PinValue::High);
    }

    #[test]
    fn test_write_pin_mock() {
        let (tmp, mut skill) = setup_mock_sysfs(&[17]);
        skill.gpio_available = true;
        skill.export_pin(17).unwrap();

        let result = skill.write_pin(17, PinValue::High);
        assert!(result.is_ok());

        let value_path = tmp.path().join("gpio17").join("value");
        let content = std::fs::read_to_string(&value_path).unwrap();
        assert_eq!(content, "1");
    }

    #[test]
    fn test_set_direction_mock() {
        let (tmp, mut skill) = setup_mock_sysfs(&[17]);
        skill.gpio_available = true;
        skill.export_pin(17).unwrap();

        let result = skill.set_direction(17, PinDirection::Output);
        assert!(result.is_ok());

        let dir_path = tmp.path().join("gpio17").join("direction");
        let content = std::fs::read_to_string(&dir_path).unwrap();
        assert_eq!(content, "out");
    }

    #[test]
    fn test_get_all_status() {
        let (_tmp, mut skill) = setup_mock_sysfs(&[17, 27]);
        skill.gpio_available = true;
        skill.export_pin(17).unwrap();
        skill.export_pin(27).unwrap();

        let status = skill.get_all_status();
        assert_eq!(status.len(), 2);
    }

    fn make_no_gpio_skill(pins: Vec<u8>) -> GpioSkill {
        // Use a non-existent sysfs base so GPIO is detected as unavailable
        GpioSkill::with_sysfs_base(pins, PathBuf::from("/tmp/evoclaw_fake_gpio_nonexistent"))
    }

    #[tokio::test]
    async fn test_init_no_gpio() {
        let mut skill = make_no_gpio_skill(vec![17, 27]);
        let result = skill.init().await;
        assert!(result.is_ok());
        assert!(!skill.gpio_available);
    }

    #[tokio::test]
    async fn test_handle_read_simulated() {
        let mut skill = make_no_gpio_skill(vec![17]);
        skill.init().await.unwrap();

        let result = skill
            .handle("read", serde_json::json!({"pin": 17}))
            .await;
        assert!(result.is_ok());
        let val = result.unwrap();
        assert_eq!(val["pin"], 17);
        assert!(val.get("simulated").is_some());
    }

    #[tokio::test]
    async fn test_handle_write_simulated() {
        let mut skill = make_no_gpio_skill(vec![17]);
        skill.init().await.unwrap();

        let result = skill
            .handle("write", serde_json::json!({"pin": 17, "value": 1}))
            .await;
        assert!(result.is_ok());
        let val = result.unwrap();
        assert_eq!(val["status"], "ok");
        assert_eq!(val["pin"], 17);
        assert_eq!(val["value"], 1);
    }

    #[tokio::test]
    async fn test_handle_mode_simulated() {
        let mut skill = make_no_gpio_skill(vec![17]);
        skill.init().await.unwrap();

        let result = skill
            .handle("mode", serde_json::json!({"pin": 17, "direction": "output"}))
            .await;
        assert!(result.is_ok());
        let val = result.unwrap();
        assert_eq!(val["pin"], 17);
        assert_eq!(val["direction"], "output");
    }

    #[tokio::test]
    async fn test_handle_status() {
        let mut skill = make_no_gpio_skill(vec![17, 27]);
        skill.init().await.unwrap();

        let result = skill.handle("status", serde_json::json!({})).await;
        assert!(result.is_ok());
        let val = result.unwrap();
        assert!(val.get("gpio_available").is_some());
        assert!(val.get("pins").is_some());
        assert!(val.get("allowed_pins").is_some());
    }

    #[tokio::test]
    async fn test_handle_blink_simulated() {
        let mut skill = make_no_gpio_skill(vec![17]);
        skill.init().await.unwrap();

        let result = skill
            .handle(
                "blink",
                serde_json::json!({"pin": 17, "count": 2, "interval_ms": 10}),
            )
            .await;
        assert!(result.is_ok());
        let val = result.unwrap();
        assert_eq!(val["blinked"], 2);
    }

    #[tokio::test]
    async fn test_handle_unknown() {
        let mut skill = make_no_gpio_skill(vec![17]);
        let result = skill.handle("invalid", serde_json::json!({})).await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_handle_read_missing_pin() {
        let mut skill = make_no_gpio_skill(vec![17]);
        let result = skill.handle("read", serde_json::json!({})).await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_handle_write_missing_value() {
        let mut skill = make_no_gpio_skill(vec![17]);
        let result = skill.handle("write", serde_json::json!({"pin": 17})).await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_handle_mode_invalid_direction() {
        let mut skill = make_no_gpio_skill(vec![17]);
        let result = skill
            .handle(
                "mode",
                serde_json::json!({"pin": 17, "direction": "invalid"}),
            )
            .await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_tick_no_gpio() {
        let mut skill = make_no_gpio_skill(vec![17]);
        let report = skill.tick().await;
        assert!(report.is_none());
    }

    #[tokio::test]
    async fn test_shutdown() {
        let mut skill = make_no_gpio_skill(vec![17]);
        // Should not panic
        skill.shutdown().await;
    }

    #[test]
    fn test_unexport_pin() {
        let (_tmp, mut skill) = setup_mock_sysfs(&[17]);
        skill.gpio_available = true;
        skill.export_pin(17).unwrap();
        assert!(skill.pin_states.contains_key(&17));

        let result = skill.unexport_pin(17);
        assert!(result.is_ok());
        assert!(!skill.pin_states.contains_key(&17));
    }

    #[test]
    fn test_read_pin_not_allowed() {
        let (_tmp, skill) = setup_mock_sysfs(&[17]);
        let result = skill.read_pin(22); // Not allowed
        assert!(result.is_err());
    }

    #[test]
    fn test_write_pin_not_allowed() {
        let (_tmp, mut skill) = setup_mock_sysfs(&[17]);
        let result = skill.write_pin(22, PinValue::High);
        assert!(result.is_err());
    }
}
