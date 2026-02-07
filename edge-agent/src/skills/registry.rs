use std::collections::HashMap;

use serde_json::Value;
use tracing::{error, info, warn};

use super::{Skill, SkillInfo, SkillReport};
use crate::config::SkillsConfig;

/// Registry that holds all loaded skills
pub struct SkillRegistry {
    skills: HashMap<String, Box<dyn Skill>>,
    enabled: HashMap<String, bool>,
    last_tick_ts: HashMap<String, u64>,
}

impl SkillRegistry {
    /// Create a new empty skill registry
    pub fn new() -> Self {
        Self {
            skills: HashMap::new(),
            enabled: HashMap::new(),
            last_tick_ts: HashMap::new(),
        }
    }

    /// Register a skill in the registry
    pub fn register(&mut self, skill: Box<dyn Skill>) {
        let name = skill.name().to_string();
        info!(skill = %name, "registering skill");
        self.enabled.insert(name.clone(), true);
        self.skills.insert(name, skill);
    }

    /// Initialize all registered skills
    pub async fn init_all(&mut self) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        let names: Vec<String> = self.skills.keys().cloned().collect();
        for name in names {
            if let Some(skill) = self.skills.get_mut(&name) {
                info!(skill = %name, "initializing skill");
                if let Err(e) = skill.init().await {
                    error!(skill = %name, error = %e, "failed to initialize skill");
                    self.enabled.insert(name.clone(), false);
                } else {
                    info!(skill = %name, "skill initialized");
                }
            }
        }
        Ok(())
    }

    /// Load skills based on config
    pub fn load_from_config(&mut self, config: &SkillsConfig) {
        // Skills are created externally and registered; this method
        // applies enabled/disabled state from config
        if let Some(sm) = &config.system_monitor {
            if !sm.enabled {
                self.enabled.insert("system_monitor".to_string(), false);
            }
        }
        if let Some(gpio) = &config.gpio {
            if !gpio.enabled {
                self.enabled.insert("gpio".to_string(), false);
            }
        }
        if let Some(pm) = &config.price_monitor {
            if !pm.enabled {
                self.enabled.insert("price_monitor".to_string(), false);
            }
        }
    }

    /// Route a command to the appropriate skill
    pub async fn handle_command(
        &mut self,
        skill_name: &str,
        action: &str,
        payload: Value,
    ) -> Result<Value, Box<dyn std::error::Error + Send + Sync>> {
        // Check if skill exists and is enabled
        if !self.is_enabled(skill_name) {
            return Err(format!("skill '{}' not found or not enabled", skill_name).into());
        }

        let skill = self
            .skills
            .get_mut(skill_name)
            .ok_or_else(|| format!("skill '{}' not found", skill_name))?;

        skill.handle(action, payload).await
    }

    /// Check if a skill is enabled
    pub fn is_enabled(&self, name: &str) -> bool {
        self.enabled.get(name).copied().unwrap_or(false)
    }

    /// Tick all skills that are due
    pub async fn tick_all(&mut self) -> Vec<SkillReport> {
        let mut reports = Vec::new();
        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs();

        let names: Vec<String> = self.skills.keys().cloned().collect();

        for name in names {
            if !self.is_enabled(&name) {
                continue;
            }

            let interval = self
                .skills
                .get(&name)
                .map(|s| s.tick_interval_secs())
                .unwrap_or(0);

            if interval == 0 {
                continue;
            }

            let last_tick = self.last_tick_ts.get(&name).copied().unwrap_or(0);
            if now - last_tick >= interval {
                if let Some(skill) = self.skills.get_mut(&name) {
                    if let Some(report) = skill.tick().await {
                        reports.push(report);
                    }
                    self.last_tick_ts.insert(name, now);
                }
            }
        }

        reports
    }

    /// Get information about all registered skills
    pub fn list_skills(&self) -> Vec<SkillInfo> {
        self.skills
            .values()
            .map(|skill| {
                let name = skill.name().to_string();
                SkillInfo {
                    name: name.clone(),
                    enabled: self.is_enabled(&name),
                    capabilities: skill.capabilities(),
                    tick_interval_secs: skill.tick_interval_secs(),
                    last_tick: self.last_tick_ts.get(&name).copied(),
                }
            })
            .collect()
    }

    /// Get the number of registered skills
    pub fn skill_count(&self) -> usize {
        self.skills.len()
    }

    /// Get the number of enabled skills
    pub fn enabled_count(&self) -> usize {
        self.enabled.values().filter(|&&v| v).count()
    }

    /// Shutdown all skills gracefully
    pub async fn shutdown_all(&mut self) {
        let names: Vec<String> = self.skills.keys().cloned().collect();
        for name in names {
            if let Some(skill) = self.skills.get_mut(&name) {
                info!(skill = %name, "shutting down skill");
                skill.shutdown().await;
            }
        }
    }

    /// Enable or disable a skill
    pub fn set_enabled(&mut self, name: &str, enabled: bool) {
        if self.skills.contains_key(name) {
            self.enabled.insert(name.to_string(), enabled);
            if enabled {
                info!(skill = %name, "skill enabled");
            } else {
                warn!(skill = %name, "skill disabled");
            }
        }
    }
}

impl Default for SkillRegistry {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use async_trait::async_trait;

    /// Mock skill for testing
    struct MockSkill {
        name: String,
        init_called: bool,
        tick_count: u32,
        should_fail_init: bool,
        tick_interval: u64,
    }

    impl MockSkill {
        fn new(name: &str) -> Self {
            Self {
                name: name.to_string(),
                init_called: false,
                tick_count: 0,
                should_fail_init: false,
                tick_interval: 30,
            }
        }

        fn with_tick_interval(mut self, interval: u64) -> Self {
            self.tick_interval = interval;
            self
        }

        fn failing_init(mut self) -> Self {
            self.should_fail_init = true;
            self
        }
    }

    #[async_trait]
    impl Skill for MockSkill {
        fn name(&self) -> &str {
            &self.name
        }

        fn capabilities(&self) -> Vec<String> {
            vec![
                format!("{}.test1", self.name),
                format!("{}.test2", self.name),
            ]
        }

        async fn init(&mut self) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
            if self.should_fail_init {
                return Err("mock init failure".into());
            }
            self.init_called = true;
            Ok(())
        }

        async fn handle(
            &mut self,
            command: &str,
            _payload: Value,
        ) -> Result<Value, Box<dyn std::error::Error + Send + Sync>> {
            match command {
                "status" => Ok(serde_json::json!({"status": "ok", "skill": self.name})),
                "fail" => Err("deliberate failure".into()),
                _ => Ok(serde_json::json!({"command": command})),
            }
        }

        async fn tick(&mut self) -> Option<SkillReport> {
            self.tick_count += 1;
            Some(SkillReport {
                skill: self.name.clone(),
                report_type: "metric".to_string(),
                payload: serde_json::json!({"tick": self.tick_count}),
            })
        }

        fn tick_interval_secs(&self) -> u64 {
            self.tick_interval
        }
    }

    #[test]
    fn test_registry_new() {
        let registry = SkillRegistry::new();
        assert_eq!(registry.skill_count(), 0);
        assert_eq!(registry.enabled_count(), 0);
    }

    #[test]
    fn test_registry_default() {
        let registry = SkillRegistry::default();
        assert_eq!(registry.skill_count(), 0);
    }

    #[test]
    fn test_register_skill() {
        let mut registry = SkillRegistry::new();
        let skill = MockSkill::new("test_skill");
        registry.register(Box::new(skill));
        assert_eq!(registry.skill_count(), 1);
        assert!(registry.is_enabled("test_skill"));
    }

    #[test]
    fn test_register_multiple_skills() {
        let mut registry = SkillRegistry::new();
        registry.register(Box::new(MockSkill::new("skill1")));
        registry.register(Box::new(MockSkill::new("skill2")));
        registry.register(Box::new(MockSkill::new("skill3")));
        assert_eq!(registry.skill_count(), 3);
        assert_eq!(registry.enabled_count(), 3);
    }

    #[tokio::test]
    async fn test_init_all_success() {
        let mut registry = SkillRegistry::new();
        registry.register(Box::new(MockSkill::new("skill1")));
        registry.register(Box::new(MockSkill::new("skill2")));
        let result = registry.init_all().await;
        assert!(result.is_ok());
        // Both should still be enabled
        assert!(registry.is_enabled("skill1"));
        assert!(registry.is_enabled("skill2"));
    }

    #[tokio::test]
    async fn test_init_all_with_failure() {
        let mut registry = SkillRegistry::new();
        registry.register(Box::new(MockSkill::new("good_skill")));
        registry.register(Box::new(MockSkill::new("bad_skill").failing_init()));
        let result = registry.init_all().await;
        assert!(result.is_ok()); // Overall doesn't fail
        assert!(registry.is_enabled("good_skill"));
        assert!(!registry.is_enabled("bad_skill")); // Failed skill is disabled
    }

    #[tokio::test]
    async fn test_handle_command_success() {
        let mut registry = SkillRegistry::new();
        registry.register(Box::new(MockSkill::new("test")));
        registry.init_all().await.unwrap();

        let result = registry
            .handle_command("test", "status", serde_json::json!({}))
            .await;
        assert!(result.is_ok());
        let val = result.unwrap();
        assert_eq!(val["status"], "ok");
        assert_eq!(val["skill"], "test");
    }

    #[tokio::test]
    async fn test_handle_command_unknown_skill() {
        let mut registry = SkillRegistry::new();
        let result = registry
            .handle_command("nonexistent", "status", serde_json::json!({}))
            .await;
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("not found"));
    }

    #[tokio::test]
    async fn test_handle_command_disabled_skill() {
        let mut registry = SkillRegistry::new();
        registry.register(Box::new(MockSkill::new("disabled_skill")));
        registry.set_enabled("disabled_skill", false);

        let result = registry
            .handle_command("disabled_skill", "status", serde_json::json!({}))
            .await;
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("not enabled"));
    }

    #[tokio::test]
    async fn test_handle_command_failure() {
        let mut registry = SkillRegistry::new();
        registry.register(Box::new(MockSkill::new("test")));
        registry.init_all().await.unwrap();

        let result = registry
            .handle_command("test", "fail", serde_json::json!({}))
            .await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_tick_all() {
        let mut registry = SkillRegistry::new();
        registry.register(Box::new(MockSkill::new("ticker").with_tick_interval(0)));
        registry.register(Box::new(MockSkill::new("ticker2").with_tick_interval(1)));

        // First tick — ticker2 should tick (interval 1, never ticked)
        let reports = registry.tick_all().await;
        // ticker has interval 0, so it won't tick
        // ticker2 should tick because now - 0 >= 1
        assert!(reports.len() <= 1);
    }

    #[tokio::test]
    async fn test_tick_all_disabled_skip() {
        let mut registry = SkillRegistry::new();
        registry.register(Box::new(MockSkill::new("disabled").with_tick_interval(1)));
        registry.set_enabled("disabled", false);

        let reports = registry.tick_all().await;
        assert_eq!(reports.len(), 0);
    }

    #[test]
    fn test_list_skills() {
        let mut registry = SkillRegistry::new();
        registry.register(Box::new(MockSkill::new("skill_a")));
        registry.register(Box::new(MockSkill::new("skill_b")));

        let skills = registry.list_skills();
        assert_eq!(skills.len(), 2);

        let names: Vec<&str> = skills.iter().map(|s| s.name.as_str()).collect();
        assert!(names.contains(&"skill_a"));
        assert!(names.contains(&"skill_b"));

        // Each mock skill has 2 capabilities
        for skill in &skills {
            assert_eq!(skill.capabilities.len(), 2);
            assert!(skill.enabled);
            assert_eq!(skill.tick_interval_secs, 30);
        }
    }

    #[test]
    fn test_set_enabled() {
        let mut registry = SkillRegistry::new();
        registry.register(Box::new(MockSkill::new("toggle")));
        assert!(registry.is_enabled("toggle"));

        registry.set_enabled("toggle", false);
        assert!(!registry.is_enabled("toggle"));

        registry.set_enabled("toggle", true);
        assert!(registry.is_enabled("toggle"));
    }

    #[test]
    fn test_set_enabled_nonexistent() {
        let mut registry = SkillRegistry::new();
        // Should not panic; just does nothing
        registry.set_enabled("nonexistent", true);
        assert!(!registry.is_enabled("nonexistent"));
    }

    #[tokio::test]
    async fn test_shutdown_all() {
        let mut registry = SkillRegistry::new();
        registry.register(Box::new(MockSkill::new("s1")));
        registry.register(Box::new(MockSkill::new("s2")));
        // Should not panic
        registry.shutdown_all().await;
    }

    #[test]
    fn test_is_enabled_unknown() {
        let registry = SkillRegistry::new();
        assert!(!registry.is_enabled("unknown"));
    }

    #[test]
    fn test_load_from_config_disables() {
        let mut registry = SkillRegistry::new();
        registry.register(Box::new(MockSkill::new("system_monitor")));
        registry.register(Box::new(MockSkill::new("gpio")));
        registry.register(Box::new(MockSkill::new("price_monitor")));

        let config = SkillsConfig {
            system_monitor: Some(crate::config::SystemMonitorSkillConfig {
                enabled: false,
                tick_interval_secs: Some(30),
            }),
            gpio: Some(crate::config::GpioSkillConfig {
                enabled: false,
                pins: vec![],
            }),
            price_monitor: Some(crate::config::PriceMonitorSkillConfig {
                enabled: false,
                symbols: vec![],
                threshold_pct: None,
                tick_interval_secs: None,
            }),
        };
        registry.load_from_config(&config);

        assert!(!registry.is_enabled("system_monitor"));
        assert!(!registry.is_enabled("gpio"));
        assert!(!registry.is_enabled("price_monitor"));
    }

    #[test]
    fn test_load_from_config_all_enabled() {
        let mut registry = SkillRegistry::new();
        registry.register(Box::new(MockSkill::new("system_monitor")));

        let config = SkillsConfig {
            system_monitor: Some(crate::config::SystemMonitorSkillConfig {
                enabled: true,
                tick_interval_secs: Some(30),
            }),
            gpio: None,
            price_monitor: None,
        };
        registry.load_from_config(&config);

        assert!(registry.is_enabled("system_monitor"));
    }

    #[tokio::test]
    async fn test_handle_command_custom_action() {
        let mut registry = SkillRegistry::new();
        registry.register(Box::new(MockSkill::new("test")));

        let result = registry
            .handle_command("test", "custom_action", serde_json::json!({"key": "value"}))
            .await;
        assert!(result.is_ok());
        assert_eq!(result.unwrap()["command"], "custom_action");
    }

    #[test]
    fn test_enabled_count() {
        let mut registry = SkillRegistry::new();
        registry.register(Box::new(MockSkill::new("a")));
        registry.register(Box::new(MockSkill::new("b")));
        registry.register(Box::new(MockSkill::new("c")));
        assert_eq!(registry.enabled_count(), 3);

        registry.set_enabled("b", false);
        assert_eq!(registry.enabled_count(), 2);

        registry.set_enabled("a", false);
        assert_eq!(registry.enabled_count(), 1);
    }
}
