use std::time::Duration;
use tracing::{error, info, warn};

use crate::config::Config;
use crate::evolution::EvolutionTracker;
use crate::metrics::Metrics;
use crate::monitor::Monitor;
use crate::mqtt::{parse_command, MqttClient};
use crate::strategy::StrategyEngine;
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

        let agent = Self {
            config,
            mqtt,
            metrics: Metrics::new(),
            trading_client,
            pnl_tracker: PnLTracker::new(),
            monitor,
            strategy_engine,
            evolution_tracker,
        };

        Ok((agent, eventloop))
    }

    /// Subscribe to MQTT topics
    pub async fn subscribe(&self) -> Result<(), Box<dyn std::error::Error>> {
        self.mqtt.subscribe().await
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
