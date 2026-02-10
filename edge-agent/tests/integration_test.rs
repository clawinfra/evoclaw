use evoclaw_agent::*;
use evoclaw_agent::strategy::Strategy;
use serde_json::json;
use std::collections::HashMap;

/// Test full command flow: parse → execute → response
#[tokio::test]
async fn test_command_flow_ping() {
    let config = config::Config::default_for_type("test_agent".to_string(), "trader".to_string());
    let (mut agent, _) = agent::EdgeAgent::new(config).await.unwrap();
    
    let cmd = mqtt::AgentCommand {
        command: "ping".to_string(),
        payload: json!({}),
        request_id: "req_integration_1".to_string(),
    };
    
    // Simulate command handling
    agent.handle_command(cmd).await;
    
    // Verify metrics were updated
    assert!(agent.metrics.actions_total > 0);
}

/// Test strategy → trading signal → order flow
#[test]
fn test_strategy_signal_generation() {
    let mut strategy = strategy::FundingArbitrage::new(-0.1, 0.0, 1000.0);
    
    let mut prices = HashMap::new();
    prices.insert("BTC".to_string(), 50000.0);
    prices.insert("ETH".to_string(), 3000.0);
    
    let mut funding_rates = HashMap::new();
    funding_rates.insert("BTC".to_string(), -0.0015); // -0.15%, triggers entry
    funding_rates.insert("ETH".to_string(), 0.0005);
    
    let market_data = strategy::MarketData {
        prices,
        funding_rates,
        timestamp: 1234567890,
    };
    
    let signals = strategy.evaluate(&market_data);
    
    // Should generate buy signal for BTC
    assert!(!signals.is_empty());
    match &signals[0] {
        strategy::Signal::Buy { asset, price, size, .. } => {
            assert_eq!(asset, "BTC");
            assert_eq!(price, &50000.0);
            assert!(size > &0.0);
        }
        _ => panic!("Expected Buy signal"),
    }
}

/// Test complete evolution cycle
#[test]
fn test_evolution_cycle() {
    let mut tracker = evolution::EvolutionTracker::new(100);
    
    // Simulate a series of trades
    let trades = vec![
        (100.0, "BTC"),
        (50.0, "ETH"),
        (-30.0, "BTC"),
        (75.0, "SOL"),
        (-20.0, "ETH"),
        (90.0, "BTC"),
    ];
    
    for (pnl, asset) in trades {
        let trade = evolution::TradeRecord {
            timestamp: 1234567890,
            asset: asset.to_string(),
            entry_price: 50000.0,
            exit_price: 50000.0 + pnl,
            size: 1.0,
            pnl,
        };
        tracker.record_trade(trade);
    }
    
    // Verify metrics
    let metrics = tracker.get_metrics();
    assert_eq!(metrics.total_trades, 6);
    assert_eq!(metrics.total_pnl, 265.0);
    assert_eq!(metrics.win_rate, 4.0 / 6.0 * 100.0); // 4 wins out of 6
    
    // Verify fitness score is calculated
    let fitness = tracker.fitness_score();
    assert!(fitness > 0.0);
    assert!(fitness <= 100.0);
}

/// Test monitor alert flow
#[test]
fn test_monitor_alert_flow() {
    let config = config::MonitorConfig {
        price_alert_threshold_pct: 5.0,
        funding_rate_threshold_pct: 0.1,
        check_interval_secs: 60,
    };
    
    let mut monitor = monitor::Monitor::new(config);
    
    // Add alerts
    monitor.add_price_alert("BTC".to_string(), 50000.0, monitor::AlertType::Above);
    monitor.add_price_alert("ETH".to_string(), 3000.0, monitor::AlertType::Below);
    
    // Check with prices that trigger alerts
    let mut prices = HashMap::new();
    prices.insert("BTC".to_string(), 51000.0); // Above threshold
    prices.insert("ETH".to_string(), 2900.0);  // Below threshold
    
    let triggered = monitor.check_price_alerts(&prices);
    
    assert_eq!(triggered.len(), 2);
    
    // Verify status
    let status = monitor.status();
    assert_eq!(status.total_alerts, 2);
    assert_eq!(status.triggered_alerts, 2);
    
    // Reset and verify
    monitor.reset_alerts();
    let status_after_reset = monitor.status();
    assert_eq!(status_after_reset.triggered_alerts, 0);
}

/// Test strategy engine with multiple strategies
#[test]
fn test_strategy_engine_multi_strategy() {
    let mut engine = strategy::StrategyEngine::new();
    
    // Add multiple strategies
    let funding_arb = strategy::FundingArbitrage::new(-0.1, 0.0, 1000.0);
    let mean_rev = strategy::MeanReversion::new(3.0, 3.0, 1500.0);
    
    engine.add_strategy(Box::new(funding_arb));
    engine.add_strategy(Box::new(mean_rev));
    
    assert_eq!(engine.strategy_count(), 2);
    
    // Get all params
    let all_params = engine.get_all_params();
    assert_eq!(all_params.len(), 2);
    
    // Update one strategy
    let new_params = json!({
        "funding_threshold": -0.2,
        "position_size_usd": 2000.0
    });
    
    let result = engine.update_strategy_params("FundingArbitrage", new_params);
    assert!(result.is_ok());
    
    // Verify params were updated
    let updated_params = engine.get_all_params();
    assert_eq!(updated_params[0]["funding_threshold"], -0.2);
}

/// Test config loading from TOML
#[test]
fn test_config_roundtrip() {
    use std::io::Write;
    use tempfile::NamedTempFile;
    
    let toml_content = r#"
agent_id = "integration_test_agent"
agent_type = "trader"

[mqtt]
broker = "mqtt.example.com"
port = 8883
keep_alive_secs = 45

[orchestrator]
url = "http://orchestrator.example.com:9000"

[trading]
hyperliquid_api = "https://api.test.com"
wallet_address = "0xintegration"
private_key_path = "keys/integration.key"
max_position_size_usd = 3000.0
max_leverage = 4.0
    "#;
    
    let mut temp_file = NamedTempFile::new().unwrap();
    temp_file.write_all(toml_content.as_bytes()).unwrap();
    
    let config = config::Config::from_file(temp_file.path()).unwrap();
    
    assert_eq!(config.agent_id, "integration_test_agent");
    assert_eq!(config.agent_type, "trader");
    assert_eq!(config.mqtt.broker, "mqtt.example.com");
    assert_eq!(config.mqtt.port, 8883);
    assert_eq!(config.mqtt.keep_alive_secs, 45);
    
    let trading = config.trading.unwrap();
    assert_eq!(trading.wallet_address, "0xintegration");
    assert_eq!(trading.max_position_size_usd, 3000.0);
    assert_eq!(trading.max_leverage, 4.0);
}

/// Test PnL tracking with trading
#[test]
fn test_pnl_tracking_flow() {
    let mut tracker = trading::PnLTracker::new();
    
    // Simulate a trading session
    tracker.record_trade(150.0);  // Win
    tracker.record_trade(-50.0);  // Loss
    tracker.record_trade(200.0);  // Win
    tracker.record_trade(-30.0);  // Loss
    tracker.update_unrealized(75.0); // Current unrealized gain
    
    assert_eq!(tracker.total_trades, 4);
    assert_eq!(tracker.win_count, 2);
    assert_eq!(tracker.loss_count, 2);
    assert_eq!(tracker.win_rate(), 50.0);
    assert_eq!(tracker.realized_pnl, 270.0);
    assert_eq!(tracker.unrealized_pnl, 75.0);
    assert_eq!(tracker.total_pnl, 345.0);
}

/// Test MQTT message parsing
#[test]
fn test_mqtt_message_parsing() {
    let json = json!({
        "command": "execute",
        "payload": {
            "action": "get_positions",
            "detailed": true
        },
        "request_id": "req_integration_final"
    });
    
    let json_str = serde_json::to_string(&json).unwrap();
    let cmd = mqtt::parse_command(json_str.as_bytes()).unwrap();
    
    assert_eq!(cmd.command, "execute");
    assert_eq!(cmd.request_id, "req_integration_final");
    assert_eq!(cmd.payload["action"], "get_positions");
    assert_eq!(cmd.payload["detailed"], true);
}

/// Test metrics tracking
#[test]
fn test_metrics_tracking_flow() {
    let mut metrics = metrics::Metrics::new();
    
    // Simulate agent operations
    metrics.record_success();
    metrics.record_success();
    metrics.record_failure();
    metrics.record_success();
    metrics.increment_uptime(30);
    metrics.increment_uptime(30);
    metrics.set_custom("latency_ms", 42.5);
    metrics.set_custom("cpu_percent", 35.0);
    
    assert_eq!(metrics.actions_total, 4);
    assert_eq!(metrics.actions_success, 3);
    assert_eq!(metrics.actions_failed, 1);
    assert_eq!(metrics.success_rate(), 75.0);
    assert_eq!(metrics.uptime_sec, 60);
    assert_eq!(metrics.custom.get("latency_ms"), Some(&42.5));
}

/// Test complete agent lifecycle
#[tokio::test]
async fn test_agent_lifecycle() {
    let config = config::Config::default_for_type("lifecycle_agent".to_string(), "monitor".to_string());
    let (mut agent, _eventloop) = agent::EdgeAgent::new(config).await.unwrap();
    
    // Initial state
    assert_eq!(agent.metrics.actions_total, 0);
    assert_eq!(agent.evolution_tracker.trade_count(), 0);
    
    // Simulate heartbeat
    agent.metrics.increment_uptime(30);
    assert_eq!(agent.metrics.uptime_sec, 30);
    
    // Simulate successful operations
    agent.metrics.record_success();
    agent.metrics.record_success();
    
    assert_eq!(agent.metrics.success_rate(), 100.0);
}
