use serde_json::Value;
use tracing::{info, warn};

use crate::agent::EdgeAgent;
use crate::mqtt::AgentCommand;

/// Command handler result
pub type CommandResult = Result<Value, Box<dyn std::error::Error>>;

impl EdgeAgent {
    /// Handle an incoming command from the orchestrator
    pub async fn handle_command(&mut self, cmd: AgentCommand) {
        info!(
            command = %cmd.command,
            request_id = %cmd.request_id,
            "received command"
        );

        let result = match cmd.command.as_str() {
            "ping" => self.handle_ping(&cmd).await,
            "execute" => self.handle_execute(&cmd).await,
            "update_strategy" => self.handle_update_strategy(&cmd).await,
            "get_metrics" => self.handle_get_metrics(&cmd).await,
            "shutdown" => self.handle_shutdown(&cmd).await,
            _ => {
                warn!(command = %cmd.command, "unknown command");
                Err(format!("unknown command: {}", cmd.command).into())
            }
        };

        // Report result or error
        match result {
            Ok(response) => {
                self.metrics.record_success();
                let _ = self.mqtt.report("result", response).await;
            }
            Err(e) => {
                self.metrics.record_failure();
                let error_payload = serde_json::json!({
                    "error": e.to_string(),
                    "request_id": cmd.request_id
                });
                let _ = self.mqtt.report("error", error_payload).await;
            }
        }
    }

    async fn handle_ping(&self, _cmd: &AgentCommand) -> CommandResult {
        Ok(serde_json::json!({"pong": true}))
    }

    async fn handle_execute(&mut self, cmd: &AgentCommand) -> CommandResult {
        match self.config.agent_type.as_str() {
            "trader" => {
                // Execute trading task
                if let Some(ref client) = self.trading_client {
                    let action = cmd.payload.get("action").and_then(|v| v.as_str());

                    match action {
                        Some("get_prices") => {
                            let prices = client.get_prices().await?;
                            Ok(serde_json::json!({
                                "status": "success",
                                "prices": prices
                            }))
                        }
                        Some("get_positions") => {
                            let positions = client.get_positions().await?;
                            Ok(serde_json::json!({
                                "status": "success",
                                "positions": positions
                            }))
                        }
                        Some("place_order") => {
                            let asset =
                                cmd.payload
                                    .get("asset")
                                    .and_then(|v| v.as_u64())
                                    .ok_or("missing asset")? as u32;
                            let is_buy = cmd
                                .payload
                                .get("is_buy")
                                .and_then(|v| v.as_bool())
                                .ok_or("missing is_buy")?;
                            let price = cmd
                                .payload
                                .get("price")
                                .and_then(|v| v.as_f64())
                                .ok_or("missing price")?;
                            let size = cmd
                                .payload
                                .get("size")
                                .and_then(|v| v.as_f64())
                                .ok_or("missing size")?;

                            let response = client
                                .place_limit_order(asset, is_buy, price, size, false)
                                .await?;
                            Ok(serde_json::json!({
                                "status": "success",
                                "order_response": response
                            }))
                        }
                        Some("monitor_positions") => {
                            client.monitor_positions(&mut self.pnl_tracker).await?;
                            Ok(serde_json::json!({
                                "status": "success",
                                "pnl": self.pnl_tracker
                            }))
                        }
                        _ => Ok(serde_json::json!({
                            "status": "executed",
                            "type": "trade",
                            "agent_type": "trader",
                            "note": "specify action: get_prices, get_positions, place_order, monitor_positions"
                        })),
                    }
                } else {
                    Err("trading client not initialized".into())
                }
            }
            "monitor" => {
                // Execute monitoring task
                if let Some(ref mut monitor) = self.monitor {
                    let action = cmd.payload.get("action").and_then(|v| v.as_str());

                    match action {
                        Some("add_price_alert") => {
                            let coin = cmd
                                .payload
                                .get("coin")
                                .and_then(|v| v.as_str())
                                .ok_or("missing coin")?
                                .to_string();
                            let target_price = cmd
                                .payload
                                .get("target_price")
                                .and_then(|v| v.as_f64())
                                .ok_or("missing target_price")?;
                            let alert_type_str = cmd
                                .payload
                                .get("alert_type")
                                .and_then(|v| v.as_str())
                                .ok_or("missing alert_type")?;

                            let alert_type = match alert_type_str {
                                "above" => crate::monitor::AlertType::Above,
                                "below" => crate::monitor::AlertType::Below,
                                _ => {
                                    return Err("invalid alert_type (use 'above' or 'below')".into())
                                }
                            };

                            monitor.add_price_alert(coin.clone(), target_price, alert_type);

                            Ok(serde_json::json!({
                                "status": "success",
                                "coin": coin,
                                "target_price": target_price
                            }))
                        }
                        Some("status") => {
                            let status = monitor.status();
                            Ok(serde_json::json!({
                                "status": "success",
                                "monitor_status": status
                            }))
                        }
                        Some("reset_alerts") => {
                            monitor.reset_alerts();
                            Ok(serde_json::json!({
                                "status": "success",
                                "action": "alerts_reset"
                            }))
                        }
                        Some("clear_alerts") => {
                            monitor.clear_alerts();
                            Ok(serde_json::json!({
                                "status": "success",
                                "action": "alerts_cleared"
                            }))
                        }
                        _ => Ok(serde_json::json!({
                            "status": "executed",
                            "type": "monitor",
                            "agent_type": "monitor",
                            "note": "specify action: add_price_alert, status, reset_alerts, clear_alerts"
                        })),
                    }
                } else {
                    Err("monitor not initialized".into())
                }
            }
            "sensor" => Ok(serde_json::json!({
                "status": "executed",
                "type": "sensor",
                "agent_type": "sensor"
            })),
            "governance" => Ok(serde_json::json!({
                "status": "executed",
                "type": "governance",
                "agent_type": "governance"
            })),
            _ => Err(format!("unknown agent type: {}", self.config.agent_type).into()),
        }
    }

    async fn handle_update_strategy(&mut self, cmd: &AgentCommand) -> CommandResult {
        info!("strategy update received - applying");
        // TODO: Apply new strategy from evolution engine
        Ok(serde_json::json!({
            "status": "strategy_updated",
            "payload": cmd.payload
        }))
    }

    async fn handle_get_metrics(&self, _cmd: &AgentCommand) -> CommandResult {
        let metrics_json = serde_json::to_value(&self.metrics)?;
        Ok(metrics_json)
    }

    async fn handle_shutdown(&self, _cmd: &AgentCommand) -> CommandResult {
        warn!("shutdown command received");
        std::process::exit(0);
    }
}
