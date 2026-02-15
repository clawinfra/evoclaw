mod agent;
mod commands;
mod config;
mod evolution;
mod genome;
mod llm;
mod metrics;
mod monitor;
mod mqtt;
mod strategy;
mod tools;
mod trading;

use clap::Parser;
use tracing::info;

use agent::EdgeAgent;
use config::Config;

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

    /// Configuration file (optional, overrides CLI args)
    #[arg(short, long)]
    config: Option<String>,
}

#[tokio::main(flavor = "current_thread")] // Single-threaded for edge devices
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Initialize tracing
    tracing_subscriber::fmt()
        .with_target(false)
        .with_level(true)
        .init();

    let args = Args::parse();

    // Load configuration from file or use CLI args
    let config = if let Some(config_path) = args.config {
        Config::from_file(config_path)?
    } else {
        let mut config = Config::default_for_type(args.id.clone(), args.agent_type.clone());
        config.mqtt.broker = args.broker;
        config.mqtt.port = args.port;
        config.orchestrator.url = args.orchestrator;
        config
    };

    info!(
        agent_id = %config.agent_id,
        agent_type = %config.agent_type,
        broker = %config.mqtt.broker,
        "🧬 EvoClaw Edge Agent starting"
    );

    let (agent, eventloop) = EdgeAgent::new(config).await?;
    agent.run(eventloop).await?;

    Ok(())
}
