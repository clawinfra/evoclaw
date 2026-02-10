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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_command_valid() {
        let json = r#"{
            "command": "ping",
            "payload": {"test": true},
            "request_id": "req123"
        }"#;

        let cmd = parse_command(json.as_bytes()).unwrap();
        assert_eq!(cmd.command, "ping");
        assert_eq!(cmd.request_id, "req123");
        assert!(cmd.payload.get("test").unwrap().as_bool().unwrap());
    }

    #[test]
    fn test_parse_command_complex_payload() {
        let json = r#"{
            "command": "execute",
            "payload": {
                "action": "place_order",
                "asset": 0,
                "price": 50000.0,
                "size": 0.1
            },
            "request_id": "req456"
        }"#;

        let cmd = parse_command(json.as_bytes()).unwrap();
        assert_eq!(cmd.command, "execute");
        assert_eq!(cmd.payload.get("action").unwrap().as_str().unwrap(), "place_order");
        assert_eq!(cmd.payload.get("price").unwrap().as_f64().unwrap(), 50000.0);
    }

    #[test]
    fn test_parse_command_empty_payload() {
        let json = r#"{
            "command": "status",
            "payload": {},
            "request_id": "req789"
        }"#;

        let cmd = parse_command(json.as_bytes()).unwrap();
        assert_eq!(cmd.command, "status");
        assert!(cmd.payload.as_object().unwrap().is_empty());
    }

    #[test]
    fn test_parse_command_invalid_json() {
        let invalid_json = b"not valid json {[}";
        let result = parse_command(invalid_json);
        assert!(result.is_err());
    }

    #[test]
    fn test_parse_command_missing_field() {
        let json = r#"{
            "command": "ping",
            "payload": {}
        }"#;
        // Missing request_id
        let result = parse_command(json.as_bytes());
        assert!(result.is_err());
    }

    #[test]
    fn test_agent_command_serialization() {
        let cmd = AgentCommand {
            command: "test".to_string(),
            payload: serde_json::json!({"key": "value"}),
            request_id: "req001".to_string(),
        };

        let json = serde_json::to_string(&cmd).unwrap();
        let deserialized: AgentCommand = serde_json::from_str(&json).unwrap();

        assert_eq!(deserialized.command, "test");
        assert_eq!(deserialized.request_id, "req001");
    }

    #[test]
    fn test_agent_report_serialization() {
        let report = AgentReport {
            agent_id: "agent1".to_string(),
            agent_type: "trader".to_string(),
            report_type: "heartbeat".to_string(),
            payload: serde_json::json!({"uptime": 3600}),
            timestamp: 1234567890,
        };

        let json = serde_json::to_string(&report).unwrap();
        let deserialized: AgentReport = serde_json::from_str(&json).unwrap();

        assert_eq!(deserialized.agent_id, "agent1");
        assert_eq!(deserialized.agent_type, "trader");
        assert_eq!(deserialized.report_type, "heartbeat");
        assert_eq!(deserialized.timestamp, 1234567890);
    }

    #[test]
    fn test_mqtt_client_new() {
        let config = MqttConfig {
            broker: "localhost".to_string(),
            port: 1883,
            keep_alive_secs: 30,
        };

        let result = MqttClient::new(&config, "test_agent".to_string(), "trader".to_string());
        assert!(result.is_ok());
        let (client, _eventloop) = result.unwrap();
        assert_eq!(client.agent_id, "test_agent");
        assert_eq!(client.agent_type, "trader");
    }

    #[test]
    fn test_mqtt_topics_format() {
        // Test that topic formats are consistent
        let agent_id = "agent123";
        let command_topic = format!("evoclaw/agents/{}/commands", agent_id);
        let report_topic = format!("evoclaw/agents/{}/reports", agent_id);
        let strategy_topic = format!("evoclaw/agents/{}/strategy", agent_id);

        assert_eq!(command_topic, "evoclaw/agents/agent123/commands");
        assert_eq!(report_topic, "evoclaw/agents/agent123/reports");
        assert_eq!(strategy_topic, "evoclaw/agents/agent123/strategy");
    }

    #[test]
    fn test_parse_command_with_nulls() {
        let json = r#"{
            "command": "test",
            "payload": {"key": null, "value": 42},
            "request_id": "req999"
        }"#;

        let cmd = parse_command(json.as_bytes()).unwrap();
        assert_eq!(cmd.command, "test");
        assert!(cmd.payload.get("key").unwrap().is_null());
        assert_eq!(cmd.payload.get("value").unwrap().as_i64().unwrap(), 42);
    }

    #[test]
    fn test_parse_command_nested_payload() {
        let json = r#"{
            "command": "complex",
            "payload": {
                "level1": {
                    "level2": {
                        "data": "deep"
                    }
                }
            },
            "request_id": "req888"
        }"#;

        let cmd = parse_command(json.as_bytes()).unwrap();
        assert_eq!(cmd.command, "complex");
        let nested = cmd.payload.get("level1").unwrap()
            .get("level2").unwrap()
            .get("data").unwrap().as_str().unwrap();
        assert_eq!(nested, "deep");
    }

    #[test]
    fn test_agent_report_timestamp() {
        let report = AgentReport {
            agent_id: "agent_time".to_string(),
            agent_type: "trader".to_string(),
            report_type: "test".to_string(),
            payload: serde_json::json!({}),
            timestamp: 1700000000,
        };

        assert!(report.timestamp > 0);
        assert_eq!(report.timestamp, 1700000000);
    }

    #[test]
    fn test_mqtt_config_different_ports() {
        let configs = vec![
            (MqttConfig { broker: "broker1".to_string(), port: 1883, keep_alive_secs: 30 }, 1883),
            (MqttConfig { broker: "broker2".to_string(), port: 8883, keep_alive_secs: 30 }, 8883),
            (MqttConfig { broker: "broker3".to_string(), port: 9001, keep_alive_secs: 30 }, 9001),
        ];

        for (config, expected_port) in configs {
            let result = MqttClient::new(&config, "agent".to_string(), "trader".to_string());
            assert!(result.is_ok());
            assert_eq!(config.port, expected_port);
        }
    }

    #[test]
    fn test_parse_command_array_payload() {
        let json = r#"{
            "command": "batch",
            "payload": {
                "items": [1, 2, 3, 4, 5]
            },
            "request_id": "req777"
        }"#;

        let cmd = parse_command(json.as_bytes()).unwrap();
        assert_eq!(cmd.command, "batch");
        let items = cmd.payload.get("items").unwrap().as_array().unwrap();
        assert_eq!(items.len(), 5);
        assert_eq!(items[0].as_i64().unwrap(), 1);
    }

    #[test]
    fn test_agent_command_clone() {
        let cmd = AgentCommand {
            command: "test".to_string(),
            payload: serde_json::json!({"data": 123}),
            request_id: "req666".to_string(),
        };

        let cloned = cmd.clone();
        assert_eq!(cmd.command, cloned.command);
        assert_eq!(cmd.request_id, cloned.request_id);
    }

    #[test]
    fn test_agent_report_clone() {
        let report = AgentReport {
            agent_id: "agent_clone".to_string(),
            agent_type: "monitor".to_string(),
            report_type: "heartbeat".to_string(),
            payload: serde_json::json!({"uptime": 100}),
            timestamp: 9999,
        };

        let cloned = report.clone();
        assert_eq!(report.agent_id, cloned.agent_id);
        assert_eq!(report.timestamp, cloned.timestamp);
    }
}
