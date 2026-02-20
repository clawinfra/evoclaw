pub mod agent;
pub mod commands;
pub mod config;
pub mod evolution;
pub mod firewall;
pub mod genome;
pub mod llm;
pub mod metrics;
pub mod monitor;
pub mod mqtt;
pub mod security;
/// JSONL tree session store — pi-inspired append-only branching format.
/// See docs/PI-INTEGRATION.md for the format spec and design rationale.
pub mod session;
pub mod skills;
pub mod strategy;
pub mod tools;
pub mod trading;
pub mod wal;
