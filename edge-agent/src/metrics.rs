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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_metrics_new() {
        let metrics = Metrics::new();
        assert_eq!(metrics.uptime_sec, 0);
        assert_eq!(metrics.actions_total, 0);
        assert_eq!(metrics.actions_success, 0);
        assert_eq!(metrics.actions_failed, 0);
        assert_eq!(metrics.memory_bytes, 0);
        assert!(metrics.custom.is_empty());
    }

    #[test]
    fn test_metrics_default() {
        let metrics = Metrics::default();
        assert_eq!(metrics.uptime_sec, 0);
        assert_eq!(metrics.actions_total, 0);
    }

    #[test]
    fn test_record_success() {
        let mut metrics = Metrics::new();
        metrics.record_success();
        assert_eq!(metrics.actions_total, 1);
        assert_eq!(metrics.actions_success, 1);
        assert_eq!(metrics.actions_failed, 0);

        metrics.record_success();
        assert_eq!(metrics.actions_total, 2);
        assert_eq!(metrics.actions_success, 2);
    }

    #[test]
    fn test_record_failure() {
        let mut metrics = Metrics::new();
        metrics.record_failure();
        assert_eq!(metrics.actions_total, 1);
        assert_eq!(metrics.actions_success, 0);
        assert_eq!(metrics.actions_failed, 1);

        metrics.record_failure();
        assert_eq!(metrics.actions_total, 2);
        assert_eq!(metrics.actions_failed, 2);
    }

    #[test]
    fn test_record_mixed() {
        let mut metrics = Metrics::new();
        metrics.record_success();
        metrics.record_success();
        metrics.record_failure();
        metrics.record_success();

        assert_eq!(metrics.actions_total, 4);
        assert_eq!(metrics.actions_success, 3);
        assert_eq!(metrics.actions_failed, 1);
    }

    #[test]
    fn test_success_rate_zero_actions() {
        let metrics = Metrics::new();
        assert_eq!(metrics.success_rate(), 100.0);
    }

    #[test]
    fn test_success_rate_all_success() {
        let mut metrics = Metrics::new();
        metrics.record_success();
        metrics.record_success();
        metrics.record_success();
        assert_eq!(metrics.success_rate(), 100.0);
    }

    #[test]
    fn test_success_rate_all_failures() {
        let mut metrics = Metrics::new();
        metrics.record_failure();
        metrics.record_failure();
        assert_eq!(metrics.success_rate(), 0.0);
    }

    #[test]
    fn test_success_rate_mixed() {
        let mut metrics = Metrics::new();
        metrics.record_success();
        metrics.record_success();
        metrics.record_failure();
        metrics.record_success();
        // 3 success out of 4 = 75%
        assert_eq!(metrics.success_rate(), 75.0);
    }

    #[test]
    fn test_set_custom_metric() {
        let mut metrics = Metrics::new();
        metrics.set_custom("latency_ms", 42.5);
        metrics.set_custom("cpu_percent", 75.0);

        assert_eq!(metrics.custom.len(), 2);
        assert_eq!(metrics.custom.get("latency_ms"), Some(&42.5));
        assert_eq!(metrics.custom.get("cpu_percent"), Some(&75.0));
    }

    #[test]
    fn test_set_custom_metric_overwrite() {
        let mut metrics = Metrics::new();
        metrics.set_custom("value", 100.0);
        metrics.set_custom("value", 200.0);

        assert_eq!(metrics.custom.len(), 1);
        assert_eq!(metrics.custom.get("value"), Some(&200.0));
    }

    #[test]
    fn test_increment_uptime() {
        let mut metrics = Metrics::new();
        metrics.increment_uptime(30);
        assert_eq!(metrics.uptime_sec, 30);

        metrics.increment_uptime(30);
        assert_eq!(metrics.uptime_sec, 60);

        metrics.increment_uptime(120);
        assert_eq!(metrics.uptime_sec, 180);
    }

    #[test]
    fn test_update_memory() {
        let mut metrics = Metrics::new();
        metrics.update_memory();
        // Memory should be updated (may be 0 on non-Linux or if proc not available)
        // Just verify it doesn't panic
        #[cfg(target_os = "linux")]
        {
            // On Linux, memory_bytes might be updated
            // We can't assert exact value, just verify it was set
            let _ = metrics.memory_bytes; // always valid for u64
        }
    }

    #[test]
    fn test_metrics_serialization() {
        let mut metrics = Metrics::new();
        metrics.uptime_sec = 3600;
        metrics.record_success();
        metrics.record_success();
        metrics.record_failure();
        metrics.set_custom("test_metric", 123.45);

        let json = serde_json::to_string(&metrics).unwrap();
        let deserialized: Metrics = serde_json::from_str(&json).unwrap();

        assert_eq!(deserialized.uptime_sec, 3600);
        assert_eq!(deserialized.actions_total, 3);
        assert_eq!(deserialized.actions_success, 2);
        assert_eq!(deserialized.actions_failed, 1);
        assert_eq!(deserialized.custom.get("test_metric"), Some(&123.45));
    }
}
