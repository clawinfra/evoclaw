mod agent;
mod commands;
mod config;
mod evolution;
mod firewall;
mod genome;
mod join;
mod llm;
mod metrics;
mod monitor;
mod mqtt;
mod paper;
mod risk;
mod security;
mod session;
mod signing;
mod skills;
mod strategy;
mod tools;
mod trading;
mod wal;

use std::path::PathBuf;

use clap::{Parser, Subcommand};
use tracing::info;

use agent::EdgeAgent;
use config::Config;
use join::{print_join_banner, print_started_banner, run_join, JoinOptions};

/// EvoClaw Edge Agent - lightweight agent runtime for edge devices
#[derive(Parser, Debug)]
#[command(name = "evoclaw-agent", version, about)]
struct Cli {
    #[command(subcommand)]
    command: Option<Commands>,

    /// Agent ID (unique identifier) — used when running directly (no subcommand)
    #[arg(short, long, global = false)]
    id: Option<String>,

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

#[derive(Subcommand, Debug)]
enum Commands {
    /// Join an EvoClaw hub — auto-configure and connect this device as an edge agent
    Join {
        /// Hub IP address or hostname
        hub: String,

        /// Custom agent ID (default: auto-generated from hostname)
        #[arg(long)]
        id: Option<String>,

        /// Agent type: monitor, trader, sensor
        #[arg(long, default_value = "monitor")]
        r#type: String,

        /// Hub API port
        #[arg(long, default_value_t = 8420)]
        port: u16,

        /// MQTT broker port
        #[arg(long, default_value_t = 1883)]
        mqtt_port: u16,

        /// Config directory
        #[arg(long)]
        config_dir: Option<PathBuf>,

        /// Just generate config, don't start the agent
        #[arg(long, default_value_t = false)]
        no_start: bool,
    },
}

#[tokio::main(flavor = "current_thread")] // Single-threaded for edge devices
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Initialize tracing
    tracing_subscriber::fmt()
        .with_target(false)
        .with_level(true)
        .init();

    let cli = Cli::parse();

    match cli.command {
        Some(Commands::Join {
            hub,
            id,
            r#type,
            port,
            mqtt_port,
            config_dir,
            no_start,
        }) => {
            // Resolve config directory
            let config_dir = config_dir.unwrap_or_else(|| dirs_or_home().join(".evoclaw"));

            let opts = JoinOptions {
                hub: hub.clone(),
                id,
                agent_type: r#type,
                port,
                mqtt_port,
                config_dir,
                no_start,
            };

            // Run the join flow
            let result = run_join(&opts).await?;

            // Print the banner
            print_join_banner(&result, &hub, port);

            if no_start {
                println!("\n  --no-start: Agent not started. Run manually:");
                println!("  evoclaw-agent --config {}", result.config_path.display());
                println!();
                return Ok(());
            }

            // Start the agent
            let config = join::load_generated_config(&result.config_path)?;

            let (agent, eventloop) = EdgeAgent::new(config).await?;

            print_started_banner(&hub, port);

            agent.run(eventloop).await?;
        }
        None => {
            // Legacy direct-run mode (original behavior)
            let agent_id = cli.id.unwrap_or_else(|| {
                eprintln!("Error: --id is required when running directly. Use `evoclaw-agent join <HUB>` for easy setup.");
                std::process::exit(1);
            });

            let config = if let Some(config_path) = cli.config {
                Config::from_file(config_path)?
            } else {
                let mut config = Config::default_for_type(agent_id, cli.agent_type);
                config.mqtt.broker = cli.broker;
                config.mqtt.port = cli.port;
                config.orchestrator.url = cli.orchestrator;
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
        }
    }

    Ok(())
}

/// Get the user's home directory
fn dirs_or_home() -> PathBuf {
    std::env::var("HOME")
        .map(PathBuf::from)
        .unwrap_or_else(|_| PathBuf::from("."))
}
