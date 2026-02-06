use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use tracing::{info, warn};

use crate::config::MonitorConfig;

/// Price alert configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PriceAlert {
    pub coin: String,
    pub target_price: f64,
    pub alert_type: AlertType,
    pub triggered: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum AlertType {
    Above,
    Below,
}

/// Funding rate alert
#[derive(Debug, Clone, Serialize, Deserialize)]
#[allow(dead_code)]
pub struct FundingAlert {
    pub coin: String,
    pub funding_rate: f64,
    pub threshold: f64,
    pub alert_type: AlertType,
}

/// Monitor agent for price and funding rate alerts
pub struct Monitor {
    config: MonitorConfig,
    price_alerts: Vec<PriceAlert>,
    last_prices: HashMap<String, f64>,
    #[allow(dead_code)]
    last_funding_rates: HashMap<String, f64>,
}

impl Monitor {
    pub fn new(config: MonitorConfig) -> Self {
        Self {
            config,
            price_alerts: Vec::new(),
            last_prices: HashMap::new(),
            last_funding_rates: HashMap::new(),
        }
    }

    /// Add a price alert
    pub fn add_price_alert(&mut self, coin: String, target_price: f64, alert_type: AlertType) {
        self.price_alerts.push(PriceAlert {
            coin,
            target_price,
            alert_type,
            triggered: false,
        });
    }

    /// Check prices against configured alerts
    #[allow(dead_code)]
    pub fn check_price_alerts(&mut self, prices: &HashMap<String, f64>) -> Vec<PriceAlert> {
        let mut triggered_alerts = Vec::new();

        for alert in &mut self.price_alerts {
            if alert.triggered {
                continue;
            }

            if let Some(&current_price) = prices.get(&alert.coin) {
                let should_trigger = match alert.alert_type {
                    AlertType::Above => current_price >= alert.target_price,
                    AlertType::Below => current_price <= alert.target_price,
                };

                if should_trigger {
                    info!(
                        coin = %alert.coin,
                        current = current_price,
                        target = alert.target_price,
                        alert_type = ?alert.alert_type,
                        "price alert triggered"
                    );
                    alert.triggered = true;
                    triggered_alerts.push(alert.clone());
                }

                // Update last known price
                self.last_prices.insert(alert.coin.clone(), current_price);
            }
        }

        triggered_alerts
    }

    /// Check for significant price movements
    #[allow(dead_code)]
    pub fn check_price_movements(&mut self, prices: &HashMap<String, f64>) -> Vec<PriceMovement> {
        let mut movements = Vec::new();

        for (coin, &current_price) in prices {
            if let Some(&last_price) = self.last_prices.get(coin) {
                let change_pct = ((current_price - last_price) / last_price) * 100.0;

                if change_pct.abs() >= self.config.price_alert_threshold_pct {
                    info!(
                        coin = %coin,
                        last = last_price,
                        current = current_price,
                        change_pct = change_pct,
                        "significant price movement detected"
                    );

                    movements.push(PriceMovement {
                        coin: coin.clone(),
                        from_price: last_price,
                        to_price: current_price,
                        change_pct,
                    });
                }
            }

            self.last_prices.insert(coin.clone(), current_price);
        }

        movements
    }

    /// Check funding rates for extreme values
    #[allow(dead_code)]
    pub fn check_funding_rates(
        &mut self,
        funding_rates: &HashMap<String, f64>,
    ) -> Vec<FundingAlert> {
        let mut alerts = Vec::new();

        for (coin, &rate) in funding_rates {
            let rate_pct = rate * 100.0;

            if rate_pct.abs() >= self.config.funding_rate_threshold_pct {
                warn!(
                    coin = %coin,
                    funding_rate = rate_pct,
                    threshold = self.config.funding_rate_threshold_pct,
                    "extreme funding rate detected"
                );

                let alert_type = if rate_pct > 0.0 {
                    AlertType::Above
                } else {
                    AlertType::Below
                };

                alerts.push(FundingAlert {
                    coin: coin.clone(),
                    funding_rate: rate_pct,
                    threshold: self.config.funding_rate_threshold_pct,
                    alert_type,
                });
            }

            self.last_funding_rates.insert(coin.clone(), rate);
        }

        alerts
    }

    /// Get current monitor status
    pub fn status(&self) -> MonitorStatus {
        MonitorStatus {
            total_alerts: self.price_alerts.len(),
            triggered_alerts: self.price_alerts.iter().filter(|a| a.triggered).count(),
            tracked_coins: self.last_prices.len(),
            last_check_interval_secs: self.config.check_interval_secs,
        }
    }

    /// Reset all triggered alerts
    pub fn reset_alerts(&mut self) {
        for alert in &mut self.price_alerts {
            alert.triggered = false;
        }
    }

    /// Clear all alerts
    pub fn clear_alerts(&mut self) {
        self.price_alerts.clear();
    }
}

/// Price movement event
#[derive(Debug, Clone, Serialize, Deserialize)]
#[allow(dead_code)]
pub struct PriceMovement {
    pub coin: String,
    pub from_price: f64,
    pub to_price: f64,
    pub change_pct: f64,
}

/// Monitor status summary
#[derive(Debug, Serialize, Deserialize)]
pub struct MonitorStatus {
    pub total_alerts: usize,
    pub triggered_alerts: usize,
    pub tracked_coins: usize,
    pub last_check_interval_secs: u64,
}

#[cfg(test)]
mod tests {
    use super::*;

    fn create_test_config() -> MonitorConfig {
        MonitorConfig {
            price_alert_threshold_pct: 5.0,
            funding_rate_threshold_pct: 0.1,
            check_interval_secs: 60,
        }
    }

    #[test]
    fn test_monitor_new() {
        let config = create_test_config();
        let monitor = Monitor::new(config.clone());
        
        assert_eq!(monitor.config.price_alert_threshold_pct, 5.0);
        assert_eq!(monitor.config.funding_rate_threshold_pct, 0.1);
        assert!(monitor.price_alerts.is_empty());
        assert!(monitor.last_prices.is_empty());
    }

    #[test]
    fn test_add_price_alert() {
        let config = create_test_config();
        let mut monitor = Monitor::new(config);
        
        monitor.add_price_alert("BTC".to_string(), 50000.0, AlertType::Above);
        monitor.add_price_alert("ETH".to_string(), 3000.0, AlertType::Below);
        
        assert_eq!(monitor.price_alerts.len(), 2);
        assert_eq!(monitor.price_alerts[0].coin, "BTC");
        assert_eq!(monitor.price_alerts[0].target_price, 50000.0);
        assert!(!monitor.price_alerts[0].triggered);
        assert_eq!(monitor.price_alerts[1].coin, "ETH");
    }

    #[test]
    fn test_check_price_alerts_above_triggered() {
        let config = create_test_config();
        let mut monitor = Monitor::new(config);
        
        monitor.add_price_alert("BTC".to_string(), 50000.0, AlertType::Above);
        
        let mut prices = HashMap::new();
        prices.insert("BTC".to_string(), 51000.0);
        
        let triggered = monitor.check_price_alerts(&prices);
        
        assert_eq!(triggered.len(), 1);
        assert_eq!(triggered[0].coin, "BTC");
        assert!(monitor.price_alerts[0].triggered);
    }

    #[test]
    fn test_check_price_alerts_below_triggered() {
        let config = create_test_config();
        let mut monitor = Monitor::new(config);
        
        monitor.add_price_alert("ETH".to_string(), 3000.0, AlertType::Below);
        
        let mut prices = HashMap::new();
        prices.insert("ETH".to_string(), 2900.0);
        
        let triggered = monitor.check_price_alerts(&prices);
        
        assert_eq!(triggered.len(), 1);
        assert_eq!(triggered[0].coin, "ETH");
        assert!(triggered[0].triggered);
    }

    #[test]
    fn test_check_price_alerts_not_triggered() {
        let config = create_test_config();
        let mut monitor = Monitor::new(config);
        
        monitor.add_price_alert("BTC".to_string(), 50000.0, AlertType::Above);
        
        let mut prices = HashMap::new();
        prices.insert("BTC".to_string(), 49000.0);
        
        let triggered = monitor.check_price_alerts(&prices);
        
        assert_eq!(triggered.len(), 0);
        assert!(!monitor.price_alerts[0].triggered);
    }

    #[test]
    fn test_check_price_alerts_already_triggered() {
        let config = create_test_config();
        let mut monitor = Monitor::new(config);
        
        monitor.add_price_alert("BTC".to_string(), 50000.0, AlertType::Above);
        
        let mut prices = HashMap::new();
        prices.insert("BTC".to_string(), 51000.0);
        
        // First check - triggers
        let triggered1 = monitor.check_price_alerts(&prices);
        assert_eq!(triggered1.len(), 1);
        
        // Second check - should not trigger again
        prices.insert("BTC".to_string(), 52000.0);
        let triggered2 = monitor.check_price_alerts(&prices);
        assert_eq!(triggered2.len(), 0);
    }

    #[test]
    fn test_check_price_alerts_exact_threshold() {
        let config = create_test_config();
        let mut monitor = Monitor::new(config);
        
        monitor.add_price_alert("BTC".to_string(), 50000.0, AlertType::Above);
        
        let mut prices = HashMap::new();
        prices.insert("BTC".to_string(), 50000.0);
        
        let triggered = monitor.check_price_alerts(&prices);
        assert_eq!(triggered.len(), 1); // Exact match should trigger
    }

    #[test]
    fn test_check_price_movements() {
        let config = create_test_config();
        let mut monitor = Monitor::new(config);
        
        // Set initial prices
        let mut prices = HashMap::new();
        prices.insert("BTC".to_string(), 50000.0);
        monitor.check_price_movements(&prices);
        
        // Move price by 6% (above 5% threshold)
        prices.insert("BTC".to_string(), 53000.0);
        let movements = monitor.check_price_movements(&prices);
        
        assert_eq!(movements.len(), 1);
        assert_eq!(movements[0].coin, "BTC");
        assert_eq!(movements[0].from_price, 50000.0);
        assert_eq!(movements[0].to_price, 53000.0);
        assert!((movements[0].change_pct - 6.0).abs() < 0.01);
    }

    #[test]
    fn test_check_price_movements_below_threshold() {
        let config = create_test_config();
        let mut monitor = Monitor::new(config);
        
        let mut prices = HashMap::new();
        prices.insert("BTC".to_string(), 50000.0);
        monitor.check_price_movements(&prices);
        
        // Move price by 3% (below 5% threshold)
        prices.insert("BTC".to_string(), 51500.0);
        let movements = monitor.check_price_movements(&prices);
        
        assert_eq!(movements.len(), 0);
    }

    #[test]
    fn test_check_price_movements_negative() {
        let config = create_test_config();
        let mut monitor = Monitor::new(config);
        
        let mut prices = HashMap::new();
        prices.insert("ETH".to_string(), 3000.0);
        monitor.check_price_movements(&prices);
        
        // Drop by 6%
        prices.insert("ETH".to_string(), 2820.0);
        let movements = monitor.check_price_movements(&prices);
        
        assert_eq!(movements.len(), 1);
        assert!((movements[0].change_pct + 6.0).abs() < 0.01);
    }

    #[test]
    fn test_check_funding_rates_extreme_positive() {
        let config = create_test_config();
        let mut monitor = Monitor::new(config);
        
        let mut rates = HashMap::new();
        rates.insert("BTC".to_string(), 0.0015); // 0.15%
        
        let alerts = monitor.check_funding_rates(&rates);
        
        assert_eq!(alerts.len(), 1);
        assert_eq!(alerts[0].coin, "BTC");
        assert!((alerts[0].funding_rate - 0.15).abs() < 0.01);
    }

    #[test]
    fn test_check_funding_rates_extreme_negative() {
        let config = create_test_config();
        let mut monitor = Monitor::new(config);
        
        let mut rates = HashMap::new();
        rates.insert("ETH".to_string(), -0.0012); // -0.12%
        
        let alerts = monitor.check_funding_rates(&rates);
        
        assert_eq!(alerts.len(), 1);
        assert!((alerts[0].funding_rate + 0.12).abs() < 0.01);
    }

    #[test]
    fn test_check_funding_rates_normal() {
        let config = create_test_config();
        let mut monitor = Monitor::new(config);
        
        let mut rates = HashMap::new();
        rates.insert("BTC".to_string(), 0.0005); // 0.05%, below threshold
        
        let alerts = monitor.check_funding_rates(&rates);
        assert_eq!(alerts.len(), 0);
    }

    #[test]
    fn test_status() {
        let config = create_test_config();
        let mut monitor = Monitor::new(config);
        
        monitor.add_price_alert("BTC".to_string(), 50000.0, AlertType::Above);
        monitor.add_price_alert("ETH".to_string(), 3000.0, AlertType::Below);
        
        let mut prices = HashMap::new();
        prices.insert("BTC".to_string(), 51000.0);
        monitor.check_price_alerts(&prices);
        
        let status = monitor.status();
        
        assert_eq!(status.total_alerts, 2);
        assert_eq!(status.triggered_alerts, 1);
        assert_eq!(status.last_check_interval_secs, 60);
    }

    #[test]
    fn test_reset_alerts() {
        let config = create_test_config();
        let mut monitor = Monitor::new(config);
        
        monitor.add_price_alert("BTC".to_string(), 50000.0, AlertType::Above);
        
        let mut prices = HashMap::new();
        prices.insert("BTC".to_string(), 51000.0);
        monitor.check_price_alerts(&prices);
        
        assert!(monitor.price_alerts[0].triggered);
        
        monitor.reset_alerts();
        
        assert!(!monitor.price_alerts[0].triggered);
        assert_eq!(monitor.price_alerts.len(), 1); // Alert still exists
    }

    #[test]
    fn test_clear_alerts() {
        let config = create_test_config();
        let mut monitor = Monitor::new(config);
        
        monitor.add_price_alert("BTC".to_string(), 50000.0, AlertType::Above);
        monitor.add_price_alert("ETH".to_string(), 3000.0, AlertType::Below);
        
        assert_eq!(monitor.price_alerts.len(), 2);
        
        monitor.clear_alerts();
        
        assert_eq!(monitor.price_alerts.len(), 0);
    }

    #[test]
    fn test_alert_type_serialization() {
        let alert = PriceAlert {
            coin: "BTC".to_string(),
            target_price: 50000.0,
            alert_type: AlertType::Above,
            triggered: false,
        };
        
        let json = serde_json::to_string(&alert).unwrap();
        let deserialized: PriceAlert = serde_json::from_str(&json).unwrap();
        
        assert_eq!(deserialized.coin, "BTC");
        assert_eq!(deserialized.target_price, 50000.0);
    }
}
