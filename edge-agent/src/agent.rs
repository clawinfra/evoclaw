use std::time::Duration;
use tracing::{error, info, warn};

use crate::config::Config;
use crate::evolution::EvolutionTracker;
use crate::llm::LLMClient;
use crate::metrics::Metrics;
use crate::monitor::Monitor;
use crate::mqtt::{parse_command, MqttClient};
use crate::strategy::StrategyEngine;
use crate::tools::EdgeTools;
use crate::trading::{HyperliquidClient, PnLTracker};

pub struct EdgeAgent {
    pub config: Config,
    pub mqtt: MqttClient,
    pub metrics: Metrics,
    pub trading_client: Option<HyperliquidClient>,
    pub pnl_tracker: PnLTracker,
    pub monitor: Option<Monitor>,
    pub strategy_engine: StrategyEngine,
    pub evolution_tracker: EvolutionTracker,
    pub llm_client: Option<LLMClient>,
    pub tools: EdgeTools,
}

impl EdgeAgent {
    /// Create a new edge agent
    pub async fn new(
        config: Config,
    ) -> Result<(Self, rumqttc::EventLoop), Box<dyn std::error::Error>> {
        let agent_id = config.agent_id.clone();
        let agent_type = config.agent_type.clone();

        let (mqtt, eventloop) = MqttClient::new(&config.mqtt, agent_id, agent_type)?;

        // Initialize trading client if this is a trader agent
        let trading_client = config
            .trading
            .as_ref()
            .map(|tc| HyperliquidClient::new(tc.clone()));

        // Initialize monitor if this is a monitor agent
        let monitor = config.monitor.as_ref().map(|mc| Monitor::new(mc.clone()));

        // Initialize strategy engine (for trader agents)
        let strategy_engine = StrategyEngine::new();

        // Initialize evolution tracker
        let evolution_tracker = EvolutionTracker::default();

        // Initialize LLM client from environment variables
        let llm_client = LLMClient::from_env();
        if llm_client.is_some() {
            info!("LLM client initialized from environment");
        }

        // Initialize edge device tools
        let tools = EdgeTools::new();
        info!(tool_count = tools.get_tool_definitions().len(), "edge tools initialized");

        let agent = Self {
            config,
            mqtt,
            metrics: Metrics::new(),
            trading_client,
            pnl_tracker: PnLTracker::new(),
            monitor,
            strategy_engine,
            evolution_tracker,
            llm_client,
            tools,
        };

        Ok((agent, eventloop))
    }

    /// Subscribe to MQTT topics
    pub async fn subscribe(&self) -> Result<(), Box<dyn std::error::Error>> {
        self.mqtt.subscribe().await
    }

    /// Advertise capabilities to the orchestrator.
    /// Uses config.capabilities if set, otherwise derives a default from agent_type.
    pub async fn advertise_capabilities(&self) -> Result<(), Box<dyn std::error::Error>> {
        let caps = self.config.capabilities.clone().unwrap_or_else(|| {
            match self.config.agent_type.as_str() {
                "sensor" | "monitor" => format!(
                    "{} sensor node — temperature, CPU/memory/disk stats, process monitoring, camera snapshot",
                    self.config.agent_id
                ),
                "trader" => format!(
                    "{} trading agent — market data, order execution, position management",
                    self.config.agent_id
                ),
                "governance" => format!(
                    "{} governance agent — on-chain voting, proposal management",
                    self.config.agent_id
                ),
                _ => format!("{} edge agent", self.config.agent_id),
            }
        });
        self.mqtt.advertise_capabilities(&caps).await
    }

    /// Send heartbeat with metrics
    pub async fn heartbeat(&mut self) -> Result<(), Box<dyn std::error::Error>> {
        self.metrics.increment_uptime(30); // Called every 30s
        self.metrics.update_memory();

        let metrics_json = serde_json::to_value(&self.metrics)?;
        self.mqtt.report("heartbeat", metrics_json).await?;

        Ok(())
    }

    /// Main event loop
    pub async fn run(
        mut self,
        mut eventloop: rumqttc::EventLoop,
    ) -> Result<(), Box<dyn std::error::Error>> {
        self.subscribe().await?;
        self.advertise_capabilities().await?;
        info!("agent ready, entering main loop");

        // Heartbeat timer
        let mut heartbeat_interval = tokio::time::interval(Duration::from_secs(30));

        loop {
            tokio::select! {
                // Process MQTT events
                event = eventloop.poll() => {
                    match event {
                        Ok(rumqttc::Event::Incoming(rumqttc::Packet::Publish(publish))) => {
                            let topic = publish.topic.clone();
                            match parse_command(&publish.payload) {
                                Ok(cmd) => self.handle_command(cmd).await,
                                Err(e) => warn!(topic = %topic, error = %e, "failed to parse command"),
                            }
                        }
                        Ok(rumqttc::Event::Incoming(rumqttc::Packet::ConnAck(_))) => {
                            info!("connected to MQTT broker");
                        }
                        Err(e) => {
                            error!(error = %e, "MQTT error, reconnecting...");
                            tokio::time::sleep(Duration::from_secs(5)).await;
                        }
                        _ => {}
                    }
                }
                // Send heartbeat
                _ = heartbeat_interval.tick() => {
                    if let Err(e) = self.heartbeat().await {
                        warn!(error = %e, "failed to send heartbeat");
                    }
                }
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_edge_agent_new() {
        let config = Config::default_for_type("test_agent".to_string(), "trader".to_string());
        let result = EdgeAgent::new(config).await;
        assert!(result.is_ok());

        let (agent, _eventloop) = result.unwrap();
        assert_eq!(agent.config.agent_id, "test_agent");
        assert_eq!(agent.config.agent_type, "trader");
        assert_eq!(agent.metrics.actions_total, 0);
    }

    #[tokio::test]
    async fn test_edge_agent_new_monitor() {
        let config = Config::default_for_type("monitor1".to_string(), "monitor".to_string());
        let result = EdgeAgent::new(config).await;
        assert!(result.is_ok());

        let (agent, _) = result.unwrap();
        assert!(agent.monitor.is_some());
        assert!(agent.trading_client.is_none());
    }

    #[tokio::test]
    async fn test_heartbeat() {
        let config = Config::default_for_type("test_agent".to_string(), "trader".to_string());
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();

        let initial_uptime = agent.metrics.uptime_sec;

        // Heartbeat should increment uptime
        let _result = agent.heartbeat().await;
        // May fail if MQTT not running, but shouldn't panic

        // Uptime should be incremented regardless
        assert!(agent.metrics.uptime_sec > initial_uptime);
    }

    #[tokio::test]
    async fn test_edge_agent_trader_with_trading_client() {
        use crate::config::TradingConfig;
        let mut config = Config::default_for_type("trader1".to_string(), "trader".to_string());
        config.trading = Some(TradingConfig {
            hyperliquid_api: "https://api.test.com".to_string(),
            wallet_address: "0xtest".to_string(),
            private_key_path: "test.key".to_string(),
            max_position_size_usd: 5000.0,
            max_leverage: 5.0,
        });

        let result = EdgeAgent::new(config).await;
        assert!(result.is_ok());

        let (agent, _) = result.unwrap();
        assert!(agent.trading_client.is_some());
        assert!(agent.monitor.is_none());
    }

    #[tokio::test]
    async fn test_edge_agent_monitor_with_monitor() {
        use crate::config::MonitorConfig;
        let mut config = Config::default_for_type("monitor1".to_string(), "monitor".to_string());
        config.monitor = Some(MonitorConfig {
            price_alert_threshold_pct: 5.0,
            funding_rate_threshold_pct: 0.1,
            check_interval_secs: 60,
        });

        let result = EdgeAgent::new(config).await;
        assert!(result.is_ok());

        let (agent, _) = result.unwrap();
        assert!(agent.monitor.is_some());
        assert!(agent.trading_client.is_none());
    }

    #[tokio::test]
    async fn test_heartbeat_multiple_calls() {
        let config = Config::default_for_type("test_agent".to_string(), "trader".to_string());
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();

        let initial_uptime = agent.metrics.uptime_sec;

        // Call heartbeat multiple times
        let _ = agent.heartbeat().await;
        let _ = agent.heartbeat().await;
        let _ = agent.heartbeat().await;

        // Uptime should have increased by 90 seconds (3 calls * 30s each)
        assert_eq!(agent.metrics.uptime_sec, initial_uptime + 90);
    }

    #[tokio::test]
    async fn test_edge_agent_initializes_all_components() {
        let config = Config::default_for_type("full_agent".to_string(), "trader".to_string());
        let result = EdgeAgent::new(config).await;
        assert!(result.is_ok());

        let (agent, _) = result.unwrap();

        // Verify all components initialized
        assert_eq!(agent.metrics.actions_total, 0);
        assert_eq!(agent.pnl_tracker.total_trades, 0);
        assert_eq!(agent.strategy_engine.strategy_count(), 0);
        assert_eq!(agent.evolution_tracker.trade_count(), 0);
    }
}
