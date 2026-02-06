use serde::{Deserialize, Serialize};
use std::collections::HashMap;

/// Agent metrics for evolution engine
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct Metrics {
    pub uptime_sec: u64,
    pub actions_total: u64,
    pub actions_success: u64,
    pub actions_failed: u64,
    pub memory_bytes: u64,
    pub custom: HashMap<String, f64>,
}

impl Metrics {
    pub fn new() -> Self {
        Self::default()
    }

    /// Update memory usage from system
    pub fn update_memory(&mut self) {
        #[cfg(target_os = "linux")]
        {
            if let Ok(status) = std::fs::read_to_string("/proc/self/status") {
                for line in status.lines() {
                    if line.starts_with("VmRSS:") {
                        if let Some(kb) = line.split_whitespace().nth(1) {
                            if let Ok(kb) = kb.parse::<u64>() {
                                self.memory_bytes = kb * 1024;
                            }
                        }
                    }
                }
            }
        }
    }

    /// Record a successful action
    pub fn record_success(&mut self) {
        self.actions_total += 1;
        self.actions_success += 1;
    }

    /// Record a failed action
    pub fn record_failure(&mut self) {
        self.actions_total += 1;
        self.actions_failed += 1;
    }

    /// Set a custom metric
    #[allow(dead_code)]
    pub fn set_custom(&mut self, key: impl Into<String>, value: f64) {
        self.custom.insert(key.into(), value);
    }
    
    /// Get success rate as percentage
    #[allow(dead_code)]
    pub fn success_rate(&self) -> f64 {
        if self.actions_total == 0 {
            return 100.0;
        }
        (self.actions_success as f64 / self.actions_total as f64) * 100.0
    }

    /// Increment uptime (typically called every heartbeat interval)
    pub fn increment_uptime(&mut self, seconds: u64) {
        self.uptime_sec += seconds;
    }
}
