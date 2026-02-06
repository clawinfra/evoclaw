use serde::{Deserialize, Serialize};
use std::path::Path;
use tracing::info;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Config {
    pub agent_id: String,
    pub agent_type: String,
    pub mqtt: MqttConfig,
    pub orchestrator: OrchestratorConfig,
    #[serde(default)]
    pub trading: Option<TradingConfig>,
    #[serde(default)]
    pub monitor: Option<MonitorConfig>,
    #[serde(default)]
    pub risk: Option<RiskConfig>,
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

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum NetworkMode {
    Mainnet,
    Testnet,
}

impl Default for NetworkMode {
    fn default() -> Self { Self::Testnet }
}

impl NetworkMode {
    pub fn api_url(&self) -> &str {
        match self {
            Self::Mainnet => "https://api.hyperliquid.xyz",
            Self::Testnet => "https://api.hyperliquid-testnet.xyz",
        }
    }
    pub fn is_mainnet(&self) -> bool { matches!(self, Self::Mainnet) }
    pub fn source_id(&self) -> &str {
        match self { Self::Mainnet => "a", Self::Testnet => "b" }
    }
    pub fn chain_string(&self) -> &str {
        match self { Self::Mainnet => "Mainnet", Self::Testnet => "Testnet" }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum TradingMode {
    Live,
    Paper,
}

impl Default for TradingMode {
    fn default() -> Self { Self::Paper }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TradingConfig {
    #[serde(default)]
    pub hyperliquid_api: String,
    pub wallet_address: String,
    pub private_key_path: String,
    #[serde(default = "default_max_position_size")]
    pub max_position_size_usd: f64,
    #[serde(default = "default_max_leverage")]
    pub max_leverage: f64,
    #[serde(default)]
    pub network_mode: NetworkMode,
    #[serde(default)]
    pub trading_mode: TradingMode,
    #[serde(default = "default_paper_log_path")]
    pub paper_log_path: String,
}

impl TradingConfig {
    pub fn effective_api_url(&self) -> &str {
        if self.hyperliquid_api.is_empty() { self.network_mode.api_url() } else { &self.hyperliquid_api }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MonitorConfig {
    pub price_alert_threshold_pct: f64,
    pub funding_rate_threshold_pct: f64,
    pub check_interval_secs: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RiskConfig {
    #[serde(default = "default_risk_max_position")]
    pub max_position_size_usd: f64,
    #[serde(default = "default_risk_max_daily_loss")]
    pub max_daily_loss_usd: f64,
    #[serde(default = "default_risk_max_open_positions")]
    pub max_open_positions: usize,
    #[serde(default = "default_risk_cooldown_secs")]
    pub cooldown_after_losses_secs: u64,
    #[serde(default = "default_risk_consecutive_losses")]
    pub consecutive_loss_limit: u32,
}

impl Default for RiskConfig {
    fn default() -> Self {
        Self {
            max_position_size_usd: 5000.0,
            max_daily_loss_usd: 500.0,
            max_open_positions: 5,
            cooldown_after_losses_secs: 300,
            consecutive_loss_limit: 3,
        }
    }
}

fn default_keep_alive() -> u64 { 30 }
fn default_max_position_size() -> f64 { 1000.0 }
fn default_max_leverage() -> f64 { 3.0 }
fn default_paper_log_path() -> String { "paper_trades.jsonl".to_string() }
fn default_risk_max_position() -> f64 { 5000.0 }
fn default_risk_max_daily_loss() -> f64 { 500.0 }
fn default_risk_max_open_positions() -> usize { 5 }
fn default_risk_cooldown_secs() -> u64 { 300 }
fn default_risk_consecutive_losses() -> u32 { 3 }

impl Config {
    pub fn from_file<P: AsRef<Path>>(path: P) -> Result<Self, Box<dyn std::error::Error>> {
        let content = std::fs::read_to_string(path)?;
        let config: Config = toml::from_str(&content)?;
        info!(agent_id = %config.agent_id, agent_type = %config.agent_type, "configuration loaded");
        Ok(config)
    }

    pub fn default_for_type(agent_id: String, agent_type: String) -> Self {
        Self {
            agent_id,
            agent_type: agent_type.clone(),
            mqtt: MqttConfig { broker: "localhost".to_string(), port: 1883, keep_alive_secs: 30 },
            orchestrator: OrchestratorConfig { url: "http://localhost:8420".to_string() },
            trading: if agent_type == "trader" {
                Some(TradingConfig {
                    hyperliquid_api: String::new(), wallet_address: String::new(),
                    private_key_path: "keys/private.key".to_string(), max_position_size_usd: 1000.0,
                    max_leverage: 3.0, network_mode: NetworkMode::Testnet, trading_mode: TradingMode::Paper,
                    paper_log_path: default_paper_log_path(),
                })
            } else { None },
            monitor: if agent_type == "monitor" {
                Some(MonitorConfig { price_alert_threshold_pct: 5.0, funding_rate_threshold_pct: 0.1, check_interval_secs: 60 })
            } else { None },
            risk: if agent_type == "trader" { Some(RiskConfig::default()) } else { None },
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
        let c = Config::default_for_type("a1".to_string(), "trader".to_string());
        assert_eq!(c.agent_id, "a1");
        assert!(c.trading.is_some());
        assert!(c.risk.is_some());
        assert!(c.monitor.is_none());
        let t = c.trading.unwrap();
        assert_eq!(t.network_mode, NetworkMode::Testnet);
        assert_eq!(t.trading_mode, TradingMode::Paper);
        assert_eq!(t.max_position_size_usd, 1000.0);
        assert_eq!(t.max_leverage, 3.0);
        assert_eq!(c.mqtt.broker, "localhost");
        assert_eq!(c.mqtt.port, 1883);
        assert_eq!(c.mqtt.keep_alive_secs, 30);
    }

    #[test]
    fn test_default_config_monitor() {
        let c = Config::default_for_type("a2".to_string(), "monitor".to_string());
        assert!(c.trading.is_none());
        assert!(c.monitor.is_some());
        assert!(c.risk.is_none());
        let m = c.monitor.unwrap();
        assert_eq!(m.price_alert_threshold_pct, 5.0);
    }

    #[test]
    fn test_default_config_sensor() {
        let c = Config::default_for_type("a3".to_string(), "sensor".to_string());
        assert!(c.trading.is_none());
        assert!(c.monitor.is_none());
    }

    #[test]
    fn test_config_from_file_valid() {
        let t = r#"
agent_id = "test_agent"
agent_type = "trader"
[mqtt]
broker = "mqtt.example.com"
port = 8883
keep_alive_secs = 60
[orchestrator]
url = "http://orch:9000"
[trading]
hyperliquid_api = "https://api.test.com"
wallet_address = "0x1234567890abcdef"
private_key_path = "keys/test.key"
max_position_size_usd = 5000.0
max_leverage = 5.0
network_mode = "testnet"
trading_mode = "paper"
        "#;
        let mut f = NamedTempFile::new().unwrap();
        f.write_all(t.as_bytes()).unwrap();
        let c = Config::from_file(f.path()).unwrap();
        assert_eq!(c.agent_id, "test_agent");
        let tr = c.trading.unwrap();
        assert_eq!(tr.wallet_address, "0x1234567890abcdef");
        assert_eq!(tr.max_position_size_usd, 5000.0);
        assert_eq!(tr.network_mode, NetworkMode::Testnet);
    }

    #[test]
    fn test_config_from_file_minimal() {
        let t = "agent_id = \"min\"\nagent_type = \"monitor\"\n[mqtt]\nbroker = \"localhost\"\nport = 1883\n[orchestrator]\nurl = \"http://localhost:8420\"";
        let mut f = NamedTempFile::new().unwrap();
        f.write_all(t.as_bytes()).unwrap();
        let c = Config::from_file(f.path()).unwrap();
        assert_eq!(c.mqtt.keep_alive_secs, 30);
    }

    #[test]
    fn test_config_from_file_missing_required_field() {
        let t = "agent_type = \"trader\"\n[mqtt]\nbroker=\"x\"\nport=1883\n[orchestrator]\nurl=\"x\"";
        let mut f = NamedTempFile::new().unwrap();
        f.write_all(t.as_bytes()).unwrap();
        assert!(Config::from_file(f.path()).is_err());
    }

    #[test]
    fn test_config_from_file_invalid_toml() {
        let mut f = NamedTempFile::new().unwrap();
        f.write_all(b"bad {{[}}").unwrap();
        assert!(Config::from_file(f.path()).is_err());
    }

    #[test]
    fn test_config_from_file_nonexistent() { assert!(Config::from_file("/nope").is_err()); }

    #[test]
    fn test_mqtt_config_defaults() { assert_eq!(default_keep_alive(), 30); }

    #[test]
    fn test_trading_config_defaults() { assert_eq!(default_max_position_size(), 1000.0); assert_eq!(default_max_leverage(), 3.0); }

    #[test]
    fn test_network_mode_api_urls() { assert_eq!(NetworkMode::Mainnet.api_url(), "https://api.hyperliquid.xyz"); assert_eq!(NetworkMode::Testnet.api_url(), "https://api.hyperliquid-testnet.xyz"); }

    #[test]
    fn test_network_mode_is_mainnet() { assert!(NetworkMode::Mainnet.is_mainnet()); assert!(!NetworkMode::Testnet.is_mainnet()); }

    #[test]
    fn test_network_mode_source_id() { assert_eq!(NetworkMode::Mainnet.source_id(), "a"); assert_eq!(NetworkMode::Testnet.source_id(), "b"); }

    #[test]
    fn test_effective_api_url_explicit() {
        let c = TradingConfig { hyperliquid_api: "https://custom".to_string(), wallet_address: "x".to_string(), private_key_path: "x".to_string(), max_position_size_usd: 0.0, max_leverage: 0.0, network_mode: NetworkMode::Testnet, trading_mode: TradingMode::Paper, paper_log_path: "x".to_string() };
        assert_eq!(c.effective_api_url(), "https://custom");
    }

    #[test]
    fn test_effective_api_url_derived() {
        let c = TradingConfig { hyperliquid_api: String::new(), wallet_address: "x".to_string(), private_key_path: "x".to_string(), max_position_size_usd: 0.0, max_leverage: 0.0, network_mode: NetworkMode::Testnet, trading_mode: TradingMode::Paper, paper_log_path: "x".to_string() };
        assert_eq!(c.effective_api_url(), "https://api.hyperliquid-testnet.xyz");
    }

    #[test]
    fn test_risk_config_default() { let r = RiskConfig::default(); assert_eq!(r.max_position_size_usd, 5000.0); assert_eq!(r.max_daily_loss_usd, 500.0); assert_eq!(r.max_open_positions, 5); }

    #[test]
    fn test_network_mode_serialization() { assert_eq!(serde_json::to_string(&NetworkMode::Mainnet).unwrap(), "\"mainnet\""); assert_eq!(serde_json::to_string(&NetworkMode::Testnet).unwrap(), "\"testnet\""); }

    #[test]
    fn test_trading_mode_serialization() { assert_eq!(serde_json::to_string(&TradingMode::Live).unwrap(), "\"live\""); assert_eq!(serde_json::to_string(&TradingMode::Paper).unwrap(), "\"paper\""); }
}
