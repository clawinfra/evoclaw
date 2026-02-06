use serde_json::Value;
use tracing::{info, warn};

use crate::agent::EdgeAgent;
use crate::evolution::TradeRecord;
use crate::mqtt::AgentCommand;
use crate::strategy::{FundingArbitrage, MeanReversion};

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
            "evolution" => self.handle_evolution(&cmd).await,
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

        let action = cmd.payload.get("action").and_then(|v| v.as_str());

        match action {
            Some("add_funding_arbitrage") => {
                let threshold = cmd
                    .payload
                    .get("funding_threshold")
                    .and_then(|v| v.as_f64())
                    .unwrap_or(-0.1);
                let exit = cmd
                    .payload
                    .get("exit_funding")
                    .and_then(|v| v.as_f64())
                    .unwrap_or(0.0);
                let size = cmd
                    .payload
                    .get("position_size_usd")
                    .and_then(|v| v.as_f64())
                    .unwrap_or(1000.0);

                let strategy = FundingArbitrage::new(threshold, exit, size);
                self.strategy_engine.add_strategy(Box::new(strategy));

                Ok(serde_json::json!({
                    "status": "strategy_added",
                    "strategy": "FundingArbitrage",
                    "params": {
                        "funding_threshold": threshold,
                        "exit_funding": exit,
                        "position_size_usd": size
                    }
                }))
            }
            Some("add_mean_reversion") => {
                let support = cmd
                    .payload
                    .get("support_level")
                    .and_then(|v| v.as_f64())
                    .unwrap_or(2.0);
                let resistance = cmd
                    .payload
                    .get("resistance_level")
                    .and_then(|v| v.as_f64())
                    .unwrap_or(2.0);
                let size = cmd
                    .payload
                    .get("position_size_usd")
                    .and_then(|v| v.as_f64())
                    .unwrap_or(1000.0);

                let strategy = MeanReversion::new(support, resistance, size);
                self.strategy_engine.add_strategy(Box::new(strategy));

                Ok(serde_json::json!({
                    "status": "strategy_added",
                    "strategy": "MeanReversion",
                    "params": {
                        "support_level": support,
                        "resistance_level": resistance,
                        "position_size_usd": size
                    }
                }))
            }
            Some("update_params") => {
                let strategy_name = cmd
                    .payload
                    .get("strategy")
                    .and_then(|v| v.as_str())
                    .ok_or("missing strategy name")?;
                let params = cmd.payload.get("params").ok_or("missing params")?.clone();

                self.strategy_engine
                    .update_strategy_params(strategy_name, params.clone())?;

                Ok(serde_json::json!({
                    "status": "strategy_updated",
                    "strategy": strategy_name,
                    "new_params": params
                }))
            }
            Some("get_params") => {
                let all_params = self.strategy_engine.get_all_params();
                Ok(serde_json::json!({
                    "status": "success",
                    "strategies": all_params,
                    "count": self.strategy_engine.strategy_count()
                }))
            }
            Some("reset") => {
                self.strategy_engine.reset_all();
                Ok(serde_json::json!({
                    "status": "strategies_reset"
                }))
            }
            _ => Ok(serde_json::json!({
                "status": "strategy_update_received",
                "payload": cmd.payload,
                "note": "specify action: add_funding_arbitrage, add_mean_reversion, update_params, get_params, reset"
            })),
        }
    }

    async fn handle_get_metrics(&self, _cmd: &AgentCommand) -> CommandResult {
        let metrics_json = serde_json::to_value(&self.metrics)?;
        let evolution_metrics = self.evolution_tracker.get_metrics();
        let fitness = self.evolution_tracker.fitness_score();

        Ok(serde_json::json!({
            "agent_metrics": metrics_json,
            "evolution_metrics": evolution_metrics,
            "fitness_score": fitness
        }))
    }

    async fn handle_evolution(&mut self, cmd: &AgentCommand) -> CommandResult {
        let action = cmd.payload.get("action").and_then(|v| v.as_str());

        match action {
            Some("record_trade") => {
                let asset = cmd
                    .payload
                    .get("asset")
                    .and_then(|v| v.as_str())
                    .ok_or("missing asset")?
                    .to_string();
                let entry_price = cmd
                    .payload
                    .get("entry_price")
                    .and_then(|v| v.as_f64())
                    .ok_or("missing entry_price")?;
                let exit_price = cmd
                    .payload
                    .get("exit_price")
                    .and_then(|v| v.as_f64())
                    .ok_or("missing exit_price")?;
                let size = cmd
                    .payload
                    .get("size")
                    .and_then(|v| v.as_f64())
                    .ok_or("missing size")?;

                let pnl = (exit_price - entry_price) * size;
                let timestamp = std::time::SystemTime::now()
                    .duration_since(std::time::UNIX_EPOCH)?
                    .as_secs();

                let trade = TradeRecord {
                    timestamp,
                    asset: asset.clone(),
                    entry_price,
                    exit_price,
                    size,
                    pnl,
                };

                self.evolution_tracker.record_trade(trade);

                Ok(serde_json::json!({
                    "status": "trade_recorded",
                    "asset": asset,
                    "pnl": pnl
                }))
            }
            Some("get_performance") => {
                let metrics = self.evolution_tracker.get_metrics();
                let fitness = self.evolution_tracker.fitness_score();

                Ok(serde_json::json!({
                    "status": "success",
                    "performance": metrics,
                    "fitness_score": fitness
                }))
            }
            Some("get_trade_history") => {
                let history = self.evolution_tracker.get_trade_history();
                Ok(serde_json::json!({
                    "status": "success",
                    "trade_count": history.len(),
                    "trades": history
                }))
            }
            Some("reset") => {
                self.evolution_tracker.reset();
                Ok(serde_json::json!({
                    "status": "evolution_tracker_reset"
                }))
            }
            _ => Ok(serde_json::json!({
                "status": "evolution_command_received",
                "note": "specify action: record_trade, get_performance, get_trade_history, reset"
            })),
        }
    }

    async fn handle_shutdown(&self, _cmd: &AgentCommand) -> CommandResult {
        warn!("shutdown command received");
        std::process::exit(0);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::{Config, MonitorConfig, TradingConfig};
    use crate::mqtt::AgentCommand;

    fn create_test_agent_config(agent_type: &str) -> Config {
        let mut config = Config::default_for_type("test_agent".to_string(), agent_type.to_string());
        
        if agent_type == "trader" {
            config.trading = Some(TradingConfig {
                hyperliquid_api: "https://api.test.com".to_string(),
                wallet_address: "0xtest".to_string(),
                private_key_path: "test.key".to_string(),
                max_position_size_usd: 1000.0,
                max_leverage: 3.0,
            });
        } else if agent_type == "monitor" {
            config.monitor = Some(MonitorConfig {
                price_alert_threshold_pct: 5.0,
                funding_rate_threshold_pct: 0.1,
                check_interval_secs: 60,
            });
        }
        
        config
    }

    #[tokio::test]
    async fn test_handle_ping() {
        let config = create_test_agent_config("trader");
        let (agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "ping".to_string(),
            payload: serde_json::json!({}),
            request_id: "req1".to_string(),
        };
        
        let result = agent.handle_ping(&cmd).await;
        assert!(result.is_ok());
        assert_eq!(result.unwrap()["pong"], true);
    }

    #[tokio::test]
    async fn test_handle_get_metrics() {
        let config = create_test_agent_config("trader");
        let (agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "get_metrics".to_string(),
            payload: serde_json::json!({}),
            request_id: "req2".to_string(),
        };
        
        let result = agent.handle_get_metrics(&cmd).await;
        assert!(result.is_ok());
        
        let response = result.unwrap();
        assert!(response.get("agent_metrics").is_some());
        assert!(response.get("evolution_metrics").is_some());
        assert!(response.get("fitness_score").is_some());
    }

    #[tokio::test]
    async fn test_handle_execute_trader_no_action() {
        let config = create_test_agent_config("trader");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "execute".to_string(),
            payload: serde_json::json!({}),
            request_id: "req3".to_string(),
        };
        
        let result = agent.handle_execute(&cmd).await;
        assert!(result.is_ok());
        assert_eq!(result.unwrap()["agent_type"], "trader");
    }

    #[tokio::test]
    async fn test_handle_execute_monitor_add_alert() {
        let config = create_test_agent_config("monitor");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "execute".to_string(),
            payload: serde_json::json!({
                "action": "add_price_alert",
                "coin": "BTC",
                "target_price": 50000.0,
                "alert_type": "above"
            }),
            request_id: "req4".to_string(),
        };
        
        let result = agent.handle_execute(&cmd).await;
        assert!(result.is_ok());
        assert_eq!(result.unwrap()["status"], "success");
    }

    #[tokio::test]
    async fn test_handle_execute_monitor_status() {
        let config = create_test_agent_config("monitor");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "execute".to_string(),
            payload: serde_json::json!({
                "action": "status"
            }),
            request_id: "req5".to_string(),
        };
        
        let result = agent.handle_execute(&cmd).await;
        assert!(result.is_ok());
        let response = result.unwrap();
        assert_eq!(response["status"], "success");
        assert!(response.get("monitor_status").is_some());
    }

    #[tokio::test]
    async fn test_handle_execute_sensor() {
        let config = create_test_agent_config("sensor");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "execute".to_string(),
            payload: serde_json::json!({}),
            request_id: "req6".to_string(),
        };
        
        let result = agent.handle_execute(&cmd).await;
        assert!(result.is_ok());
        assert_eq!(result.unwrap()["agent_type"], "sensor");
    }

    #[tokio::test]
    async fn test_handle_update_strategy_add_funding_arbitrage() {
        let config = create_test_agent_config("trader");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "update_strategy".to_string(),
            payload: serde_json::json!({
                "action": "add_funding_arbitrage",
                "funding_threshold": -0.15,
                "exit_funding": 0.05,
                "position_size_usd": 2000.0
            }),
            request_id: "req7".to_string(),
        };
        
        let result = agent.handle_update_strategy(&cmd).await;
        assert!(result.is_ok());
        let response = result.unwrap();
        assert_eq!(response["status"], "strategy_added");
        assert_eq!(response["strategy"], "FundingArbitrage");
    }

    #[tokio::test]
    async fn test_handle_update_strategy_add_mean_reversion() {
        let config = create_test_agent_config("trader");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "update_strategy".to_string(),
            payload: serde_json::json!({
                "action": "add_mean_reversion",
                "support_level": 3.0,
                "resistance_level": 4.0,
                "position_size_usd": 1500.0
            }),
            request_id: "req8".to_string(),
        };
        
        let result = agent.handle_update_strategy(&cmd).await;
        assert!(result.is_ok());
        let response = result.unwrap();
        assert_eq!(response["status"], "strategy_added");
        assert_eq!(response["strategy"], "MeanReversion");
    }

    #[tokio::test]
    async fn test_handle_update_strategy_get_params() {
        let config = create_test_agent_config("trader");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        // Add a strategy first
        let add_cmd = AgentCommand {
            command: "update_strategy".to_string(),
            payload: serde_json::json!({
                "action": "add_funding_arbitrage"
            }),
            request_id: "req9a".to_string(),
        };
        agent.handle_update_strategy(&add_cmd).await.unwrap();
        
        // Now get params
        let cmd = AgentCommand {
            command: "update_strategy".to_string(),
            payload: serde_json::json!({
                "action": "get_params"
            }),
            request_id: "req9".to_string(),
        };
        
        let result = agent.handle_update_strategy(&cmd).await;
        assert!(result.is_ok());
        let response = result.unwrap();
        assert_eq!(response["status"], "success");
        assert!(response["strategies"].is_array());
        assert_eq!(response["count"], 1);
    }

    #[tokio::test]
    async fn test_handle_evolution_record_trade() {
        let config = create_test_agent_config("trader");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "evolution".to_string(),
            payload: serde_json::json!({
                "action": "record_trade",
                "asset": "BTC",
                "entry_price": 50000.0,
                "exit_price": 51000.0,
                "size": 0.1
            }),
            request_id: "req10".to_string(),
        };
        
        let result = agent.handle_evolution(&cmd).await;
        assert!(result.is_ok());
        let response = result.unwrap();
        assert_eq!(response["status"], "trade_recorded");
        assert_eq!(response["asset"], "BTC");
    }

    #[tokio::test]
    async fn test_handle_evolution_get_performance() {
        let config = create_test_agent_config("trader");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "evolution".to_string(),
            payload: serde_json::json!({
                "action": "get_performance"
            }),
            request_id: "req11".to_string(),
        };
        
        let result = agent.handle_evolution(&cmd).await;
        assert!(result.is_ok());
        let response = result.unwrap();
        assert_eq!(response["status"], "success");
        assert!(response.get("performance").is_some());
        assert!(response.get("fitness_score").is_some());
    }

    #[tokio::test]
    async fn test_handle_evolution_reset() {
        let config = create_test_agent_config("trader");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "evolution".to_string(),
            payload: serde_json::json!({
                "action": "reset"
            }),
            request_id: "req12".to_string(),
        };
        
        let result = agent.handle_evolution(&cmd).await;
        assert!(result.is_ok());
        assert_eq!(result.unwrap()["status"], "evolution_tracker_reset");
    }

    #[tokio::test]
    async fn test_handle_execute_monitor_invalid_alert_type() {
        let config = create_test_agent_config("monitor");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "execute".to_string(),
            payload: serde_json::json!({
                "action": "add_price_alert",
                "coin": "BTC",
                "target_price": 50000.0,
                "alert_type": "invalid"
            }),
            request_id: "req13".to_string(),
        };
        
        let result = agent.handle_execute(&cmd).await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_handle_execute_monitor_missing_coin() {
        let config = create_test_agent_config("monitor");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "execute".to_string(),
            payload: serde_json::json!({
                "action": "add_price_alert",
                "target_price": 50000.0,
                "alert_type": "above"
            }),
            request_id: "req14".to_string(),
        };
        
        let result = agent.handle_execute(&cmd).await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_handle_evolution_missing_fields() {
        let config = create_test_agent_config("trader");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "evolution".to_string(),
            payload: serde_json::json!({
                "action": "record_trade",
                "asset": "BTC"
                // Missing required fields
            }),
            request_id: "req15".to_string(),
        };
        
        let result = agent.handle_evolution(&cmd).await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_handle_evolution_get_trade_history() {
        let config = create_test_agent_config("trader");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        // Record a trade first
        let record_cmd = AgentCommand {
            command: "evolution".to_string(),
            payload: serde_json::json!({
                "action": "record_trade",
                "asset": "ETH",
                "entry_price": 3000.0,
                "exit_price": 3100.0,
                "size": 1.0
            }),
            request_id: "req16a".to_string(),
        };
        agent.handle_evolution(&record_cmd).await.unwrap();
        
        // Now get trade history
        let cmd = AgentCommand {
            command: "evolution".to_string(),
            payload: serde_json::json!({
                "action": "get_trade_history"
            }),
            request_id: "req16".to_string(),
        };
        
        let result = agent.handle_evolution(&cmd).await;
        assert!(result.is_ok());
        let response = result.unwrap();
        assert_eq!(response["status"], "success");
        assert_eq!(response["trade_count"], 1);
        assert!(response.get("trades").is_some());
    }

    #[tokio::test]
    async fn test_handle_execute_monitor_reset_alerts() {
        let config = create_test_agent_config("monitor");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "execute".to_string(),
            payload: serde_json::json!({
                "action": "reset_alerts"
            }),
            request_id: "req17".to_string(),
        };
        
        let result = agent.handle_execute(&cmd).await;
        assert!(result.is_ok());
        assert_eq!(result.unwrap()["action"], "alerts_reset");
    }

    #[tokio::test]
    async fn test_handle_execute_monitor_clear_alerts() {
        let config = create_test_agent_config("monitor");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "execute".to_string(),
            payload: serde_json::json!({
                "action": "clear_alerts"
            }),
            request_id: "req18".to_string(),
        };
        
        let result = agent.handle_execute(&cmd).await;
        assert!(result.is_ok());
        assert_eq!(result.unwrap()["action"], "alerts_cleared");
    }

    #[tokio::test]
    async fn test_handle_execute_governance() {
        let config = create_test_agent_config("governance");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "execute".to_string(),
            payload: serde_json::json!({}),
            request_id: "req19".to_string(),
        };
        
        let result = agent.handle_execute(&cmd).await;
        assert!(result.is_ok());
        assert_eq!(result.unwrap()["agent_type"], "governance");
    }

    #[tokio::test]
    async fn test_handle_execute_unknown_agent_type() {
        let config = create_test_agent_config("trader");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        agent.config.agent_type = "unknown".to_string();
        
        let cmd = AgentCommand {
            command: "execute".to_string(),
            payload: serde_json::json!({}),
            request_id: "req20".to_string(),
        };
        
        let result = agent.handle_execute(&cmd).await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_handle_update_strategy_update_params() {
        let config = create_test_agent_config("trader");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        // Add a strategy first
        let add_cmd = AgentCommand {
            command: "update_strategy".to_string(),
            payload: serde_json::json!({
                "action": "add_funding_arbitrage"
            }),
            request_id: "req21a".to_string(),
        };
        agent.handle_update_strategy(&add_cmd).await.unwrap();
        
        // Now update its params
        let cmd = AgentCommand {
            command: "update_strategy".to_string(),
            payload: serde_json::json!({
                "action": "update_params",
                "strategy": "FundingArbitrage",
                "params": {
                    "funding_threshold": -0.2
                }
            }),
            request_id: "req21".to_string(),
        };
        
        let result = agent.handle_update_strategy(&cmd).await;
        assert!(result.is_ok());
        let response = result.unwrap();
        assert_eq!(response["status"], "strategy_updated");
        assert_eq!(response["strategy"], "FundingArbitrage");
    }

    #[tokio::test]
    async fn test_handle_update_strategy_update_nonexistent() {
        let config = create_test_agent_config("trader");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "update_strategy".to_string(),
            payload: serde_json::json!({
                "action": "update_params",
                "strategy": "NonExistent",
                "params": {}
            }),
            request_id: "req22".to_string(),
        };
        
        let result = agent.handle_update_strategy(&cmd).await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_handle_update_strategy_reset() {
        let config = create_test_agent_config("trader");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "update_strategy".to_string(),
            payload: serde_json::json!({
                "action": "reset"
            }),
            request_id: "req23".to_string(),
        };
        
        let result = agent.handle_update_strategy(&cmd).await;
        assert!(result.is_ok());
        assert_eq!(result.unwrap()["status"], "strategies_reset");
    }

    #[tokio::test]
    async fn test_handle_unknown_command() {
        let config = create_test_agent_config("trader");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "unknown_command".to_string(),
            payload: serde_json::json!({}),
            request_id: "req24".to_string(),
        };
        
        // handle_command is called internally, so we'll test the dispatch
        // Instead, we can call handle_command through the agent
        agent.handle_command(cmd).await;
        
        // Verify that metrics recorded a failure
        assert_eq!(agent.metrics.actions_failed, 1);
    }

    #[tokio::test]
    async fn test_handle_command_success_records_metric() {
        let config = create_test_agent_config("trader");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "ping".to_string(),
            payload: serde_json::json!({}),
            request_id: "req25".to_string(),
        };
        
        agent.handle_command(cmd).await;
        
        // Verify that metrics recorded a success
        assert_eq!(agent.metrics.actions_success, 1);
        assert_eq!(agent.metrics.actions_total, 1);
    }

    #[tokio::test]
    async fn test_handle_evolution_no_action() {
        let config = create_test_agent_config("trader");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "evolution".to_string(),
            payload: serde_json::json!({}),
            request_id: "req26".to_string(),
        };
        
        let result = agent.handle_evolution(&cmd).await;
        assert!(result.is_ok());
        let response = result.unwrap();
        assert_eq!(response["status"], "evolution_command_received");
    }

    #[tokio::test]
    async fn test_handle_update_strategy_no_action() {
        let config = create_test_agent_config("trader");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "update_strategy".to_string(),
            payload: serde_json::json!({}),
            request_id: "req27".to_string(),
        };
        
        let result = agent.handle_update_strategy(&cmd).await;
        assert!(result.is_ok());
        let response = result.unwrap();
        assert_eq!(response["status"], "strategy_update_received");
    }

    #[tokio::test]
    async fn test_handle_execute_trader_no_client() {
        let mut config = create_test_agent_config("trader");
        config.trading = None; // Remove trading config
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "execute".to_string(),
            payload: serde_json::json!({
                "action": "get_prices"
            }),
            request_id: "req28".to_string(),
        };
        
        let result = agent.handle_execute(&cmd).await;
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("trading client not initialized"));
    }

    #[tokio::test]
    async fn test_handle_execute_monitor_no_monitor() {
        let mut config = create_test_agent_config("monitor");
        config.monitor = None; // Remove monitor config
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "execute".to_string(),
            payload: serde_json::json!({
                "action": "status"
            }),
            request_id: "req29".to_string(),
        };
        
        let result = agent.handle_execute(&cmd).await;
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("monitor not initialized"));
    }

    #[tokio::test]
    async fn test_handle_execute_monitor_missing_target_price() {
        let config = create_test_agent_config("monitor");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "execute".to_string(),
            payload: serde_json::json!({
                "action": "add_price_alert",
                "coin": "BTC",
                "alert_type": "above"
                // Missing target_price
            }),
            request_id: "req30".to_string(),
        };
        
        let result = agent.handle_execute(&cmd).await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_handle_execute_monitor_missing_alert_type() {
        let config = create_test_agent_config("monitor");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "execute".to_string(),
            payload: serde_json::json!({
                "action": "add_price_alert",
                "coin": "BTC",
                "target_price": 50000.0
                // Missing alert_type
            }),
            request_id: "req31".to_string(),
        };
        
        let result = agent.handle_execute(&cmd).await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_handle_update_strategy_update_params_missing_strategy() {
        let config = create_test_agent_config("trader");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "update_strategy".to_string(),
            payload: serde_json::json!({
                "action": "update_params",
                "params": {"key": "value"}
                // Missing strategy name
            }),
            request_id: "req32".to_string(),
        };
        
        let result = agent.handle_update_strategy(&cmd).await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_handle_update_strategy_update_params_missing_params() {
        let config = create_test_agent_config("trader");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "update_strategy".to_string(),
            payload: serde_json::json!({
                "action": "update_params",
                "strategy": "FundingArbitrage"
                // Missing params
            }),
            request_id: "req33".to_string(),
        };
        
        let result = agent.handle_update_strategy(&cmd).await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_handle_evolution_record_trade_missing_entry_price() {
        let config = create_test_agent_config("trader");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "evolution".to_string(),
            payload: serde_json::json!({
                "action": "record_trade",
                "asset": "BTC",
                "exit_price": 51000.0,
                "size": 0.1
                // Missing entry_price
            }),
            request_id: "req34".to_string(),
        };
        
        let result = agent.handle_evolution(&cmd).await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_handle_evolution_record_trade_missing_exit_price() {
        let config = create_test_agent_config("trader");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "evolution".to_string(),
            payload: serde_json::json!({
                "action": "record_trade",
                "asset": "BTC",
                "entry_price": 50000.0,
                "size": 0.1
                // Missing exit_price
            }),
            request_id: "req35".to_string(),
        };
        
        let result = agent.handle_evolution(&cmd).await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_handle_evolution_record_trade_missing_size() {
        let config = create_test_agent_config("trader");
        let (mut agent, _) = EdgeAgent::new(config).await.unwrap();
        
        let cmd = AgentCommand {
            command: "evolution".to_string(),
            payload: serde_json::json!({
                "action": "record_trade",
                "asset": "BTC",
                "entry_price": 50000.0,
                "exit_price": 51000.0
                // Missing size
            }),
            request_id: "req36".to_string(),
        };
        
        let result = agent.handle_evolution(&cmd).await;
        assert!(result.is_err());
    }
}
