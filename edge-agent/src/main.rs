use clap::Parser;
use rumqttc::{AsyncClient, Event, EventLoop, MqttOptions, Packet, QoS};
use serde::{Deserialize, Serialize};
use std::time::Duration;
use tracing::{error, info, warn};

/// EvoClaw Edge Agent - lightweight agent runtime for edge devices
#[derive(Parser, Debug)]
#[command(name = "evoclaw-agent", version, about)]
struct Args {
    /// Agent ID (unique identifier)
    #[arg(short, long)]
    id: String,

    /// Agent type (trader, monitor, sensor, governance)
    #[arg(short = 't', long, default_value = "monitor")]
    agent_type: String,

    /// MQTT broker address
    #[arg(short, long, default_value = "localhost")]
    broker: String,

    /// MQTT broker port
    #[arg(short, long, default_value_t = 1883)]
    port: u16,

    /// Orchestrator API URL
    #[arg(short, long, default_value = "http://localhost:8420")]
    orchestrator: String,
}

/// Message from orchestrator to agent
#[derive(Debug, Serialize, Deserialize)]
struct AgentCommand {
    command: String,
    payload: serde_json::Value,
    request_id: String,
}

/// Message from agent to orchestrator
#[derive(Debug, Serialize, Deserialize)]
struct AgentReport {
    agent_id: String,
    agent_type: String,
    report_type: String, // "metric", "result", "error", "heartbeat"
    payload: serde_json::Value,
    timestamp: u64,
}

/// Agent metrics for evolution engine
#[derive(Debug, Serialize, Deserialize, Default)]
struct Metrics {
    uptime_sec: u64,
    actions_total: u64,
    actions_success: u64,
    actions_failed: u64,
    memory_bytes: u64,
    custom: std::collections::HashMap<String, f64>,
}

struct EdgeAgent {
    id: String,
    agent_type: String,
    mqtt_client: AsyncClient,
    metrics: Metrics,
    orchestrator_url: String,
}

impl EdgeAgent {
    async fn new(args: &Args) -> Result<(Self, EventLoop), Box<dyn std::error::Error>> {
        let mut mqttoptions =
            MqttOptions::new(format!("evoclaw-{}", args.id), &args.broker, args.port);
        mqttoptions.set_keep_alive(Duration::from_secs(30));

        let (client, eventloop) = AsyncClient::new(mqttoptions, 100);

        let agent = Self {
            id: args.id.clone(),
            agent_type: args.agent_type.clone(),
            mqtt_client: client,
            metrics: Metrics::default(),
            orchestrator_url: args.orchestrator.clone(),
        };

        Ok((agent, eventloop))
    }

    /// Subscribe to relevant MQTT topics
    async fn subscribe(&self) -> Result<(), Box<dyn std::error::Error>> {
        // Subscribe to commands for this agent
        self.mqtt_client
            .subscribe(
                format!("evoclaw/agents/{}/commands", self.id),
                QoS::AtLeastOnce,
            )
            .await?;

        // Subscribe to broadcast commands
        self.mqtt_client
            .subscribe("evoclaw/broadcast".to_string(), QoS::AtLeastOnce)
            .await?;

        // Subscribe to strategy updates from evolution engine
        self.mqtt_client
            .subscribe(
                format!("evoclaw/agents/{}/strategy", self.id),
                QoS::AtLeastOnce,
            )
            .await?;

        info!(agent_id = %self.id, "subscribed to MQTT topics");
        Ok(())
    }

    /// Publish a report to the orchestrator
    async fn report(
        &self,
        report_type: &str,
        payload: serde_json::Value,
    ) -> Result<(), Box<dyn std::error::Error>> {
        let report = AgentReport {
            agent_id: self.id.clone(),
            agent_type: self.agent_type.clone(),
            report_type: report_type.to_string(),
            payload,
            timestamp: std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)?
                .as_secs(),
        };

        let payload = serde_json::to_vec(&report)?;
        self.mqtt_client
            .publish(
                format!("evoclaw/agents/{}/reports", self.id),
                QoS::AtLeastOnce,
                false,
                payload,
            )
            .await?;

        Ok(())
    }

    /// Send heartbeat with metrics
    async fn heartbeat(&mut self) -> Result<(), Box<dyn std::error::Error>> {
        self.metrics.uptime_sec += 30; // Called every 30s

        // Get memory usage
        #[cfg(target_os = "linux")]
        {
            if let Ok(status) = std::fs::read_to_string("/proc/self/status") {
                for line in status.lines() {
                    if line.starts_with("VmRSS:") {
                        if let Some(kb) = line.split_whitespace().nth(1) {
                            if let Ok(kb) = kb.parse::<u64>() {
                                self.metrics.memory_bytes = kb * 1024;
                            }
                        }
                    }
                }
            }
        }

        let metrics_json = serde_json::to_value(&self.metrics)?;
        self.report("heartbeat", metrics_json).await?;

        Ok(())
    }

    /// Handle an incoming command from the orchestrator
    async fn handle_command(&mut self, cmd: AgentCommand) {
        info!(command = %cmd.command, request_id = %cmd.request_id, "received command");

        match cmd.command.as_str() {
            "ping" => {
                let _ = self
                    .report("result", serde_json::json!({"pong": true}))
                    .await;
            }
            "execute" => {
                self.metrics.actions_total += 1;
                // TODO: Execute task based on agent type
                match self.execute_task(&cmd.payload).await {
                    Ok(result) => {
                        self.metrics.actions_success += 1;
                        let _ = self.report("result", result).await;
                    }
                    Err(e) => {
                        self.metrics.actions_failed += 1;
                        let _ = self
                            .report("error", serde_json::json!({"error": e.to_string()}))
                            .await;
                    }
                }
            }
            "update_strategy" => {
                info!("strategy update received - applying");
                // TODO: Apply new strategy from evolution engine
            }
            "shutdown" => {
                warn!("shutdown command received");
                std::process::exit(0);
            }
            _ => {
                warn!(command = %cmd.command, "unknown command");
            }
        }
    }

    /// Execute a task based on agent type
    async fn execute_task(
        &self,
        _payload: &serde_json::Value,
    ) -> Result<serde_json::Value, Box<dyn std::error::Error>> {
        match self.agent_type.as_str() {
            "trader" => {
                // TODO: Trading logic (Hyperliquid API calls)
                Ok(serde_json::json!({"status": "executed", "type": "trade"}))
            }
            "monitor" => {
                // TODO: Monitoring logic (KOL tweets, price alerts)
                Ok(serde_json::json!({"status": "executed", "type": "monitor"}))
            }
            "sensor" => {
                // TODO: Sensor reading logic
                Ok(serde_json::json!({"status": "executed", "type": "sensor"}))
            }
            "governance" => {
                // TODO: ClawChain governance logic
                Ok(serde_json::json!({"status": "executed", "type": "governance"}))
            }
            _ => Err(format!("unknown agent type: {}", self.agent_type).into()),
        }
    }
}

#[tokio::main(flavor = "current_thread")] // Single-threaded for edge devices
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Initialize tracing
    tracing_subscriber::fmt()
        .with_target(false)
        .with_level(true)
        .init();

    let args = Args::parse();

    info!(
        agent_id = %args.id,
        agent_type = %args.agent_type,
        broker = %args.broker,
        "🧬 EvoClaw Edge Agent starting"
    );

    let (mut agent, mut eventloop) = EdgeAgent::new(&args).await?;
    agent.subscribe().await?;

    info!("agent ready, entering main loop");

    // Heartbeat timer
    let mut heartbeat_interval = tokio::time::interval(Duration::from_secs(30));

    loop {
        tokio::select! {
            // Process MQTT events
            event = eventloop.poll() => {
                match event {
                    Ok(Event::Incoming(Packet::Publish(publish))) => {
                        let topic = publish.topic.clone();
                        match serde_json::from_slice::<AgentCommand>(&publish.payload) {
                            Ok(cmd) => agent.handle_command(cmd).await,
                            Err(e) => warn!(topic = %topic, error = %e, "failed to parse command"),
                        }
                    }
                    Ok(Event::Incoming(Packet::ConnAck(_))) => {
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
                if let Err(e) = agent.heartbeat().await {
                    warn!(error = %e, "failed to send heartbeat");
                }
            }
        }
    }
}
