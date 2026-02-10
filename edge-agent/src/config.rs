use serde::{Deserialize, Serialize};
use std::path::Path;
use tracing::info;

/// Agent configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Config {
    /// Unique agent identifier
    pub agent_id: String,

    /// Agent type (trader, monitor, sensor, governance)
    pub agent_type: String,

    /// MQTT broker configuration
    pub mqtt: MqttConfig,

    /// Orchestrator API configuration
    pub orchestrator: OrchestratorConfig,

    /// Trading configuration (optional)
    #[serde(default)]
    pub trading: Option<TradingConfig>,

    /// Monitor configuration (optional)
    #[serde(default)]
    pub monitor: Option<MonitorConfig>,

    /// Skills configuration (optional)
    #[serde(default)]
    pub skills: Option<SkillsConfig>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MqttConfig {
    pub broker: String,
    pub port: u16,
    #[serde(default = "default_keep_alive")]
    pub keep_alive_secs: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OrchestratorConfig {
    pub url: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TradingConfig {
    pub hyperliquid_api: String,
    pub wallet_address: String,
    pub private_key_path: String,
    #[serde(default = "default_max_position_size")]
    pub max_position_size_usd: f64,
    #[serde(default = "default_max_leverage")]
    pub max_leverage: f64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MonitorConfig {
    pub price_alert_threshold_pct: f64,
    pub funding_rate_threshold_pct: f64,
    pub check_interval_secs: u64,
}

/// Configuration for the skills subsystem
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct SkillsConfig {
    #[serde(default)]
    pub clawchain: Option<ClawChainSkillConfig>,
}

/// Configuration for the ClawChain blockchain skill
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ClawChainSkillConfig {
    #[serde(default = "default_true")]
    pub enabled: bool,
    #[serde(default = "default_clawchain_node_url")]
    pub node_url: String,
    #[serde(default)]
    pub agent_did: Option<String>,
    #[serde(default = "default_clawchain_tick_interval")]
    pub tick_interval_secs: u64,
}

fn default_true() -> bool {
    true
}

fn default_clawchain_node_url() -> String {
    "http://localhost:9933".to_string()
}

fn default_clawchain_tick_interval() -> u64 {
    120
}

fn default_keep_alive() -> u64 {
    30
}

fn default_max_position_size() -> f64 {
    1000.0
}

fn default_max_leverage() -> f64 {
    3.0
}

impl Config {
    /// Load configuration from TOML file
    pub fn from_file<P: AsRef<Path>>(path: P) -> Result<Self, Box<dyn std::error::Error>> {
        let content = std::fs::read_to_string(path)?;
        let config: Config = toml::from_str(&content)?;
        info!(agent_id = %config.agent_id, agent_type = %config.agent_type, "configuration loaded");
        Ok(config)
    }

    /// Create default configuration
    pub fn default_for_type(agent_id: String, agent_type: String) -> Self {
        Self {
            agent_id,
            agent_type: agent_type.clone(),
            mqtt: MqttConfig {
                broker: "localhost".to_string(),
                port: 1883,
                keep_alive_secs: 30,
            },
            orchestrator: OrchestratorConfig {
                url: "http://localhost:8420".to_string(),
            },
            trading: if agent_type == "trader" {
                Some(TradingConfig {
                    hyperliquid_api: "https://api.hyperliquid.xyz".to_string(),
                    wallet_address: String::new(),
                    private_key_path: "keys/private.key".to_string(),
                    max_position_size_usd: 1000.0,
                    max_leverage: 3.0,
                })
            } else {
                None
            },
            monitor: if agent_type == "monitor" {
                Some(MonitorConfig {
                    price_alert_threshold_pct: 5.0,
                    funding_rate_threshold_pct: 0.1,
                    check_interval_secs: 60,
                })
            } else {
                None
            },
            skills: None,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Write;
    use tempfile::NamedTempFile;

    #[test]
    fn test_default_config_trader() {
        let config = Config::default_for_type("agent1".to_string(), "trader".to_string());
        assert_eq!(config.agent_id, "agent1");
        assert_eq!(config.agent_type, "trader");
        assert_eq!(config.mqtt.broker, "localhost");
        assert_eq!(config.mqtt.port, 1883);
        assert_eq!(config.mqtt.keep_alive_secs, 30);
        assert!(config.trading.is_some());
        assert!(config.monitor.is_none());

        let trading = config.trading.unwrap();
        assert_eq!(trading.max_position_size_usd, 1000.0);
        assert_eq!(trading.max_leverage, 3.0);
    }

    #[test]
    fn test_default_config_monitor() {
        let config = Config::default_for_type("agent2".to_string(), "monitor".to_string());
        assert_eq!(config.agent_id, "agent2");
        assert_eq!(config.agent_type, "monitor");
        assert!(config.trading.is_none());
        assert!(config.monitor.is_some());

        let monitor = config.monitor.unwrap();
        assert_eq!(monitor.price_alert_threshold_pct, 5.0);
        assert_eq!(monitor.funding_rate_threshold_pct, 0.1);
        assert_eq!(monitor.check_interval_secs, 60);
    }

    #[test]
    fn test_default_config_sensor() {
        let config = Config::default_for_type("agent3".to_string(), "sensor".to_string());
        assert_eq!(config.agent_type, "sensor");
        assert!(config.trading.is_none());
        assert!(config.monitor.is_none());
    }

    #[test]
    fn test_config_from_file_valid() {
        let toml_content = r#"
agent_id = "test_agent"
agent_type = "trader"

[mqtt]
broker = "mqtt.example.com"
port = 8883
keep_alive_secs = 60

[orchestrator]
url = "http://orchestrator.example.com:9000"

[trading]
hyperliquid_api = "https://api.test.com"
wallet_address = "0x1234567890abcdef"
private_key_path = "keys/test.key"
max_position_size_usd = 5000.0
max_leverage = 5.0
        "#;

        let mut temp_file = NamedTempFile::new().unwrap();
        temp_file.write_all(toml_content.as_bytes()).unwrap();

        let config = Config::from_file(temp_file.path()).unwrap();
        assert_eq!(config.agent_id, "test_agent");
        assert_eq!(config.agent_type, "trader");
        assert_eq!(config.mqtt.broker, "mqtt.example.com");
        assert_eq!(config.mqtt.port, 8883);
        assert_eq!(config.mqtt.keep_alive_secs, 60);
        assert_eq!(
            config.orchestrator.url,
            "http://orchestrator.example.com:9000"
        );

        let trading = config.trading.unwrap();
        assert_eq!(trading.wallet_address, "0x1234567890abcdef");
        assert_eq!(trading.max_position_size_usd, 5000.0);
        assert_eq!(trading.max_leverage, 5.0);
    }

    #[test]
    fn test_config_from_file_minimal() {
        let toml_content = r#"
agent_id = "minimal_agent"
agent_type = "monitor"

[mqtt]
broker = "localhost"
port = 1883

[orchestrator]
url = "http://localhost:8420"
        "#;

        let mut temp_file = NamedTempFile::new().unwrap();
        temp_file.write_all(toml_content.as_bytes()).unwrap();

        let config = Config::from_file(temp_file.path()).unwrap();
        assert_eq!(config.agent_id, "minimal_agent");
        assert_eq!(config.mqtt.keep_alive_secs, 30); // Default value
    }

    #[test]
    fn test_config_from_file_missing_required_field() {
        let toml_content = r#"
agent_type = "trader"

[mqtt]
broker = "localhost"
port = 1883

[orchestrator]
url = "http://localhost:8420"
        "#;

        let mut temp_file = NamedTempFile::new().unwrap();
        temp_file.write_all(toml_content.as_bytes()).unwrap();

        let result = Config::from_file(temp_file.path());
        assert!(result.is_err());
    }

    #[test]
    fn test_config_from_file_invalid_toml() {
        let toml_content = "this is not valid toml {{[}}";

        let mut temp_file = NamedTempFile::new().unwrap();
        temp_file.write_all(toml_content.as_bytes()).unwrap();

        let result = Config::from_file(temp_file.path());
        assert!(result.is_err());
    }

    #[test]
    fn test_config_from_file_nonexistent() {
        let result = Config::from_file("/nonexistent/path/config.toml");
        assert!(result.is_err());
    }

    #[test]
    fn test_mqtt_config_defaults() {
        let config = MqttConfig {
            broker: "test".to_string(),
            port: 1883,
            keep_alive_secs: default_keep_alive(),
        };
        assert_eq!(config.keep_alive_secs, 30);
    }

    #[test]
    fn test_trading_config_defaults() {
        let config = TradingConfig {
            hyperliquid_api: "test".to_string(),
            wallet_address: "test".to_string(),
            private_key_path: "test".to_string(),
            max_position_size_usd: default_max_position_size(),
            max_leverage: default_max_leverage(),
        };
        assert_eq!(config.max_position_size_usd, 1000.0);
        assert_eq!(config.max_leverage, 3.0);
    }

    #[test]
    fn test_clawchain_skill_config_defaults() {
        let config = ClawChainSkillConfig {
            enabled: default_true(),
            node_url: default_clawchain_node_url(),
            agent_did: None,
            tick_interval_secs: default_clawchain_tick_interval(),
        };
        assert!(config.enabled);
        assert_eq!(config.node_url, "http://localhost:9933");
        assert!(config.agent_did.is_none());
        assert_eq!(config.tick_interval_secs, 120);
    }

    #[test]
    fn test_clawchain_skill_config_with_did() {
        let config = ClawChainSkillConfig {
            enabled: true,
            node_url: "http://custom:9933".to_string(),
            agent_did: Some("did:claw:myagent".to_string()),
            tick_interval_secs: 60,
        };
        assert_eq!(config.agent_did, Some("did:claw:myagent".to_string()));
        assert_eq!(config.tick_interval_secs, 60);
    }

    #[test]
    fn test_clawchain_skill_config_serialization() {
        let config = ClawChainSkillConfig {
            enabled: true,
            node_url: "http://localhost:9933".to_string(),
            agent_did: Some("did:claw:test".to_string()),
            tick_interval_secs: 120,
        };
        let json = serde_json::to_string(&config).unwrap();
        let deserialized: ClawChainSkillConfig = serde_json::from_str(&json).unwrap();
        assert!(deserialized.enabled);
        assert_eq!(deserialized.node_url, "http://localhost:9933");
        assert_eq!(deserialized.agent_did, Some("did:claw:test".to_string()));
    }

    #[test]
    fn test_skills_config_with_clawchain() {
        let config = SkillsConfig {
            clawchain: Some(ClawChainSkillConfig {
                enabled: true,
                node_url: "http://localhost:9933".to_string(),
                agent_did: None,
                tick_interval_secs: 120,
            }),
        };
        assert!(config.clawchain.is_some());
        assert!(config.clawchain.unwrap().enabled);
    }

    #[test]
    fn test_config_from_file_with_clawchain() {
        let toml_content = r#"
agent_id = "chain_agent"
agent_type = "sensor"

[mqtt]
broker = "localhost"
port = 1883

[orchestrator]
url = "http://localhost:8420"

[skills.clawchain]
enabled = true
node_url = "http://chain-node:9933"
agent_did = "did:claw:sensor01"
tick_interval_secs = 90
        "#;

        let mut temp_file = NamedTempFile::new().unwrap();
        temp_file.write_all(toml_content.as_bytes()).unwrap();

        let config = Config::from_file(temp_file.path()).unwrap();
        assert_eq!(config.agent_id, "chain_agent");
        let skills = config.skills.unwrap();
        let cc = skills.clawchain.unwrap();
        assert!(cc.enabled);
        assert_eq!(cc.node_url, "http://chain-node:9933");
        assert_eq!(cc.agent_did, Some("did:claw:sensor01".to_string()));
        assert_eq!(cc.tick_interval_secs, 90);
    }
}
