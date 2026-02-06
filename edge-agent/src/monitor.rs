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
