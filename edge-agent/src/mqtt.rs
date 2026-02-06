use rumqttc::{AsyncClient, EventLoop, MqttOptions, QoS};
use serde::{Deserialize, Serialize};
use std::time::Duration;
use tracing::info;

use crate::config::MqttConfig;

/// Message from orchestrator to agent
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AgentCommand {
    pub command: String,
    pub payload: serde_json::Value,
    pub request_id: String,
}

/// Message from agent to orchestrator
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AgentReport {
    pub agent_id: String,
    pub agent_type: String,
    pub report_type: String, // "metric", "result", "error", "heartbeat"
    pub payload: serde_json::Value,
    pub timestamp: u64,
}

pub struct MqttClient {
    client: AsyncClient,
    agent_id: String,
    agent_type: String,
}

impl MqttClient {
    /// Create a new MQTT client
    pub fn new(
        config: &MqttConfig,
        agent_id: String,
        agent_type: String,
    ) -> Result<(Self, EventLoop), Box<dyn std::error::Error>> {
        let mut mqttoptions =
            MqttOptions::new(format!("evoclaw-{}", agent_id), &config.broker, config.port);
        mqttoptions.set_keep_alive(Duration::from_secs(config.keep_alive_secs));

        let (client, eventloop) = AsyncClient::new(mqttoptions, 100);

        Ok((
            Self {
                client,
                agent_id,
                agent_type,
            },
            eventloop,
        ))
    }

    /// Subscribe to relevant MQTT topics
    pub async fn subscribe(&self) -> Result<(), Box<dyn std::error::Error>> {
        // Subscribe to commands for this agent
        self.client
            .subscribe(
                format!("evoclaw/agents/{}/commands", self.agent_id),
                QoS::AtLeastOnce,
            )
            .await?;

        // Subscribe to broadcast commands
        self.client
            .subscribe("evoclaw/broadcast".to_string(), QoS::AtLeastOnce)
            .await?;

        // Subscribe to strategy updates from evolution engine
        self.client
            .subscribe(
                format!("evoclaw/agents/{}/strategy", self.agent_id),
                QoS::AtLeastOnce,
            )
            .await?;

        info!(agent_id = %self.agent_id, "subscribed to MQTT topics");
        Ok(())
    }

    /// Publish a report to the orchestrator
    pub async fn report(
        &self,
        report_type: &str,
        payload: serde_json::Value,
    ) -> Result<(), Box<dyn std::error::Error>> {
        let report = AgentReport {
            agent_id: self.agent_id.clone(),
            agent_type: self.agent_type.clone(),
            report_type: report_type.to_string(),
            payload,
            timestamp: std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)?
                .as_secs(),
        };

        let payload = serde_json::to_vec(&report)?;
        self.client
            .publish(
                format!("evoclaw/agents/{}/reports", self.agent_id),
                QoS::AtLeastOnce,
                false,
                payload,
            )
            .await?;

        Ok(())
    }

    /// Get the underlying async client (for custom operations)
    #[allow(dead_code)]
    pub fn client(&self) -> &AsyncClient {
        &self.client
    }
}

/// Parse an incoming MQTT message as an AgentCommand
pub fn parse_command(payload: &[u8]) -> Result<AgentCommand, serde_json::Error> {
    serde_json::from_slice(payload)
}
