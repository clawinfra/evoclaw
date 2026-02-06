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
        }
    }
}
