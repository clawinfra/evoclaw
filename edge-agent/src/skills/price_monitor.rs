use std::collections::{HashMap, VecDeque};

use async_trait::async_trait;
use serde_json::Value;
use tracing::{info, warn};

use super::{Skill, SkillReport};

/// A single price reading
#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct PriceReading {
    pub timestamp: u64,
    pub symbol: String,
    pub price: f64,
}

/// A price alert configuration
#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct PriceAlertConfig {
    pub symbol: String,
    pub target_price: f64,
    pub direction: String, // "above" or "below"
    pub triggered: bool,
}

/// Price Monitor Skill — monitors crypto/asset prices
pub struct PriceMonitorSkill {
    symbols: Vec<String>,
    threshold_pct: f64,
    tick_interval: u64,
    last_prices: HashMap<String, f64>,
    price_history: VecDeque<PriceReading>,
    max_history: usize,
    alerts: Vec<PriceAlertConfig>,
    http_client: Option<reqwest::Client>,
}

impl PriceMonitorSkill {
    pub fn new(symbols: Vec<String>, threshold_pct: f64, tick_interval: u64) -> Self {
        Self {
            symbols,
            threshold_pct,
            tick_interval,
            last_prices: HashMap::new(),
            price_history: VecDeque::new(),
            max_history: 100,
            alerts: Vec::new(),
            http_client: None,
        }
    }

    /// Fetch prices from CoinGecko API (free, no key required)
    async fn fetch_prices(&self) -> Result<HashMap<String, f64>, Box<dyn std::error::Error + Send + Sync>> {
        let client = match &self.http_client {
            Some(c) => c.clone(),
            None => return Ok(HashMap::new()),
        };

        // Map common symbols to CoinGecko IDs
        let id_map: HashMap<&str, &str> = [
            ("BTC", "bitcoin"),
            ("ETH", "ethereum"),
            ("SOL", "solana"),
            ("AVAX", "avalanche-2"),
            ("DOGE", "dogecoin"),
            ("ADA", "cardano"),
            ("DOT", "polkadot"),
            ("MATIC", "matic-network"),
            ("LINK", "chainlink"),
            ("UNI", "uniswap"),
        ]
        .into_iter()
        .collect();

        let ids: Vec<&str> = self
            .symbols
            .iter()
            .filter_map(|s| id_map.get(s.as_str()).copied())
            .collect();

        if ids.is_empty() {
            return Ok(HashMap::new());
        }

        let ids_str = ids.join(",");
        let url = format!(
            "https://api.coingecko.com/api/v3/simple/price?ids={}&vs_currencies=usd",
            ids_str
        );

        let resp = client
            .get(&url)
            .timeout(std::time::Duration::from_secs(10))
            .send()
            .await?;

        let data: HashMap<String, HashMap<String, f64>> = resp.json().await?;

        // Reverse-map CoinGecko IDs back to symbols
        let reverse_map: HashMap<&str, &str> = id_map.iter().map(|(k, v)| (*v, *k)).collect();

        let mut prices = HashMap::new();
        for (gecko_id, price_map) in &data {
            if let (Some(&symbol), Some(&usd_price)) =
                (reverse_map.get(gecko_id.as_str()), price_map.get("usd"))
            {
                prices.insert(symbol.to_string(), usd_price);
            }
        }

        Ok(prices)
    }

    /// Check for threshold-crossing price movements
    fn check_price_movements(&self, prices: &HashMap<String, f64>) -> Vec<SkillReport> {
        let mut reports = Vec::new();

        for (symbol, &current_price) in prices {
            if let Some(&last_price) = self.last_prices.get(symbol) {
                if last_price == 0.0 {
                    continue;
                }
                let change_pct = ((current_price - last_price) / last_price) * 100.0;

                if change_pct.abs() >= self.threshold_pct {
                    reports.push(SkillReport {
                        skill: "price_monitor".to_string(),
                        report_type: "alert".to_string(),
                        payload: serde_json::json!({
                            "alert": "price_movement",
                            "symbol": symbol,
                            "from_price": last_price,
                            "to_price": current_price,
                            "change_pct": change_pct,
                            "threshold_pct": self.threshold_pct,
                            "message": format!("{} moved {:.1}% ({:.2} → {:.2})", symbol, change_pct, last_price, current_price)
                        }),
                    });
                }
            }
        }

        reports
    }

    /// Check price alerts
    fn check_alerts(&mut self, prices: &HashMap<String, f64>) -> Vec<SkillReport> {
        let mut reports = Vec::new();

        for alert in &mut self.alerts {
            if alert.triggered {
                continue;
            }

            if let Some(&price) = prices.get(&alert.symbol) {
                let should_trigger = match alert.direction.as_str() {
                    "above" => price >= alert.target_price,
                    "below" => price <= alert.target_price,
                    _ => false,
                };

                if should_trigger {
                    alert.triggered = true;
                    reports.push(SkillReport {
                        skill: "price_monitor".to_string(),
                        report_type: "alert".to_string(),
                        payload: serde_json::json!({
                            "alert": "price_target",
                            "symbol": alert.symbol,
                            "target_price": alert.target_price,
                            "current_price": price,
                            "direction": alert.direction,
                            "message": format!("{} hit {} target {:.2} (current: {:.2})",
                                alert.symbol, alert.direction, alert.target_price, price)
                        }),
                    });
                }
            }
        }

        reports
    }
}

#[async_trait]
impl Skill for PriceMonitorSkill {
    fn name(&self) -> &str {
        "price_monitor"
    }

    fn capabilities(&self) -> Vec<String> {
        vec![
            "price.check".to_string(),
            "price.alert".to_string(),
            "price.history".to_string(),
        ]
    }

    async fn init(&mut self) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        self.http_client = Some(reqwest::Client::new());
        info!(symbols = ?self.symbols, threshold = self.threshold_pct, "price monitor initialized");
        Ok(())
    }

    async fn handle(
        &mut self,
        command: &str,
        payload: Value,
    ) -> Result<Value, Box<dyn std::error::Error + Send + Sync>> {
        match command {
            "check" | "status" => {
                let prices = self.fetch_prices().await?;
                let timestamp = std::time::SystemTime::now()
                    .duration_since(std::time::UNIX_EPOCH)
                    .unwrap_or_default()
                    .as_secs();

                // Store readings
                for (symbol, &price) in &prices {
                    self.price_history.push_back(PriceReading {
                        timestamp,
                        symbol: symbol.clone(),
                        price,
                    });
                    self.last_prices.insert(symbol.clone(), price);
                }

                // Trim history
                while self.price_history.len() > self.max_history {
                    self.price_history.pop_front();
                }

                Ok(serde_json::json!({
                    "status": "success",
                    "prices": prices,
                    "timestamp": timestamp
                }))
            }
            "alert" => {
                let symbol = payload
                    .get("symbol")
                    .and_then(|v| v.as_str())
                    .ok_or("missing symbol")?
                    .to_string();
                let target_price = payload
                    .get("target_price")
                    .and_then(|v| v.as_f64())
                    .ok_or("missing target_price")?;
                let direction = payload
                    .get("direction")
                    .and_then(|v| v.as_str())
                    .unwrap_or("above")
                    .to_string();

                if direction != "above" && direction != "below" {
                    return Err("direction must be 'above' or 'below'".into());
                }

                self.alerts.push(PriceAlertConfig {
                    symbol: symbol.clone(),
                    target_price,
                    direction: direction.clone(),
                    triggered: false,
                });

                Ok(serde_json::json!({
                    "status": "alert_created",
                    "symbol": symbol,
                    "target_price": target_price,
                    "direction": direction,
                    "total_alerts": self.alerts.len()
                }))
            }
            "history" => {
                let count = payload
                    .get("count")
                    .and_then(|v| v.as_u64())
                    .unwrap_or(10) as usize;
                let symbol = payload.get("symbol").and_then(|v| v.as_str());

                let history: Vec<&PriceReading> = self
                    .price_history
                    .iter()
                    .rev()
                    .filter(|r| symbol.map_or(true, |s| r.symbol == s))
                    .take(count)
                    .collect();

                Ok(serde_json::json!({
                    "count": history.len(),
                    "readings": history
                }))
            }
            "clear_alerts" => {
                let count = self.alerts.len();
                self.alerts.clear();
                Ok(serde_json::json!({
                    "status": "alerts_cleared",
                    "cleared": count
                }))
            }
            "list_alerts" => {
                Ok(serde_json::json!({
                    "alerts": self.alerts,
                    "count": self.alerts.len()
                }))
            }
            _ => Err(format!("unknown price_monitor command: {}", command).into()),
        }
    }

    async fn tick(&mut self) -> Option<SkillReport> {
        let prices = match self.fetch_prices().await {
            Ok(p) => p,
            Err(e) => {
                warn!(error = %e, "failed to fetch prices");
                return None;
            }
        };

        if prices.is_empty() {
            return None;
        }

        let timestamp = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs();

        // Store readings in history
        for (symbol, &price) in &prices {
            self.price_history.push_back(PriceReading {
                timestamp,
                symbol: symbol.clone(),
                price,
            });
        }
        while self.price_history.len() > self.max_history {
            self.price_history.pop_front();
        }

        // Check for movement alerts
        let _movement_reports = self.check_price_movements(&prices);

        // Check price alerts
        let _alert_reports = self.check_alerts(&prices);

        // Update last prices
        for (symbol, price) in &prices {
            self.last_prices.insert(symbol.clone(), *price);
        }

        Some(SkillReport {
            skill: "price_monitor".to_string(),
            report_type: "metric".to_string(),
            payload: serde_json::json!({
                "prices": prices,
                "timestamp": timestamp,
                "symbols_tracked": self.symbols.len()
            }),
        })
    }

    fn tick_interval_secs(&self) -> u64 {
        self.tick_interval
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_price_monitor_new() {
        let skill = PriceMonitorSkill::new(
            vec!["BTC".to_string(), "ETH".to_string()],
            5.0,
            60,
        );
        assert_eq!(skill.symbols.len(), 2);
        assert_eq!(skill.threshold_pct, 5.0);
        assert_eq!(skill.tick_interval, 60);
        assert!(skill.last_prices.is_empty());
        assert!(skill.price_history.is_empty());
        assert!(skill.alerts.is_empty());
    }

    #[test]
    fn test_skill_name() {
        let skill = PriceMonitorSkill::new(vec![], 5.0, 60);
        assert_eq!(skill.name(), "price_monitor");
    }

    #[test]
    fn test_skill_capabilities() {
        let skill = PriceMonitorSkill::new(vec![], 5.0, 60);
        let caps = skill.capabilities();
        assert_eq!(caps.len(), 3);
        assert!(caps.contains(&"price.check".to_string()));
        assert!(caps.contains(&"price.alert".to_string()));
        assert!(caps.contains(&"price.history".to_string()));
    }

    #[test]
    fn test_tick_interval() {
        let skill = PriceMonitorSkill::new(vec![], 5.0, 120);
        assert_eq!(skill.tick_interval_secs(), 120);
    }

    #[test]
    fn test_check_price_movements_above_threshold() {
        let mut skill = PriceMonitorSkill::new(vec!["BTC".to_string()], 5.0, 60);
        skill.last_prices.insert("BTC".to_string(), 50000.0);

        let mut prices = HashMap::new();
        prices.insert("BTC".to_string(), 53000.0); // +6%

        let reports = skill.check_price_movements(&prices);
        assert_eq!(reports.len(), 1);
        assert_eq!(reports[0].payload["alert"], "price_movement");
        assert_eq!(reports[0].payload["symbol"], "BTC");
    }

    #[test]
    fn test_check_price_movements_below_threshold() {
        let mut skill = PriceMonitorSkill::new(vec!["BTC".to_string()], 5.0, 60);
        skill.last_prices.insert("BTC".to_string(), 50000.0);

        let mut prices = HashMap::new();
        prices.insert("BTC".to_string(), 51000.0); // +2%

        let reports = skill.check_price_movements(&prices);
        assert!(reports.is_empty());
    }

    #[test]
    fn test_check_price_movements_no_previous() {
        let skill = PriceMonitorSkill::new(vec!["BTC".to_string()], 5.0, 60);

        let mut prices = HashMap::new();
        prices.insert("BTC".to_string(), 50000.0);

        let reports = skill.check_price_movements(&prices);
        assert!(reports.is_empty());
    }

    #[test]
    fn test_check_alerts_above_triggered() {
        let mut skill = PriceMonitorSkill::new(vec![], 5.0, 60);
        skill.alerts.push(PriceAlertConfig {
            symbol: "BTC".to_string(),
            target_price: 50000.0,
            direction: "above".to_string(),
            triggered: false,
        });

        let mut prices = HashMap::new();
        prices.insert("BTC".to_string(), 51000.0);

        let reports = skill.check_alerts(&prices);
        assert_eq!(reports.len(), 1);
        assert_eq!(reports[0].payload["alert"], "price_target");
        assert!(skill.alerts[0].triggered);
    }

    #[test]
    fn test_check_alerts_below_triggered() {
        let mut skill = PriceMonitorSkill::new(vec![], 5.0, 60);
        skill.alerts.push(PriceAlertConfig {
            symbol: "ETH".to_string(),
            target_price: 3000.0,
            direction: "below".to_string(),
            triggered: false,
        });

        let mut prices = HashMap::new();
        prices.insert("ETH".to_string(), 2900.0);

        let reports = skill.check_alerts(&prices);
        assert_eq!(reports.len(), 1);
    }

    #[test]
    fn test_check_alerts_not_triggered() {
        let mut skill = PriceMonitorSkill::new(vec![], 5.0, 60);
        skill.alerts.push(PriceAlertConfig {
            symbol: "BTC".to_string(),
            target_price: 50000.0,
            direction: "above".to_string(),
            triggered: false,
        });

        let mut prices = HashMap::new();
        prices.insert("BTC".to_string(), 49000.0);

        let reports = skill.check_alerts(&prices);
        assert!(reports.is_empty());
        assert!(!skill.alerts[0].triggered);
    }

    #[test]
    fn test_check_alerts_already_triggered() {
        let mut skill = PriceMonitorSkill::new(vec![], 5.0, 60);
        skill.alerts.push(PriceAlertConfig {
            symbol: "BTC".to_string(),
            target_price: 50000.0,
            direction: "above".to_string(),
            triggered: true, // Already triggered
        });

        let mut prices = HashMap::new();
        prices.insert("BTC".to_string(), 55000.0);

        let reports = skill.check_alerts(&prices);
        assert!(reports.is_empty());
    }

    #[tokio::test]
    async fn test_init() {
        let mut skill = PriceMonitorSkill::new(vec!["BTC".to_string()], 5.0, 60);
        let result = skill.init().await;
        assert!(result.is_ok());
        assert!(skill.http_client.is_some());
    }

    #[tokio::test]
    async fn test_handle_alert_create() {
        let mut skill = PriceMonitorSkill::new(vec![], 5.0, 60);

        let result = skill
            .handle(
                "alert",
                serde_json::json!({
                    "symbol": "BTC",
                    "target_price": 100000.0,
                    "direction": "above"
                }),
            )
            .await;
        assert!(result.is_ok());
        let val = result.unwrap();
        assert_eq!(val["status"], "alert_created");
        assert_eq!(val["total_alerts"], 1);
        assert_eq!(skill.alerts.len(), 1);
    }

    #[tokio::test]
    async fn test_handle_alert_invalid_direction() {
        let mut skill = PriceMonitorSkill::new(vec![], 5.0, 60);

        let result = skill
            .handle(
                "alert",
                serde_json::json!({
                    "symbol": "BTC",
                    "target_price": 100000.0,
                    "direction": "sideways"
                }),
            )
            .await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_handle_alert_missing_symbol() {
        let mut skill = PriceMonitorSkill::new(vec![], 5.0, 60);

        let result = skill
            .handle("alert", serde_json::json!({"target_price": 100000.0}))
            .await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_handle_history_empty() {
        let mut skill = PriceMonitorSkill::new(vec![], 5.0, 60);

        let result = skill.handle("history", serde_json::json!({})).await;
        assert!(result.is_ok());
        let val = result.unwrap();
        assert_eq!(val["count"], 0);
    }

    #[tokio::test]
    async fn test_handle_history_with_data() {
        let mut skill = PriceMonitorSkill::new(vec![], 5.0, 60);
        skill.price_history.push_back(PriceReading {
            timestamp: 1000,
            symbol: "BTC".to_string(),
            price: 50000.0,
        });
        skill.price_history.push_back(PriceReading {
            timestamp: 1001,
            symbol: "ETH".to_string(),
            price: 3000.0,
        });

        let result = skill.handle("history", serde_json::json!({})).await;
        assert!(result.is_ok());
        let val = result.unwrap();
        assert_eq!(val["count"], 2);
    }

    #[tokio::test]
    async fn test_handle_history_filter_by_symbol() {
        let mut skill = PriceMonitorSkill::new(vec![], 5.0, 60);
        skill.price_history.push_back(PriceReading {
            timestamp: 1000,
            symbol: "BTC".to_string(),
            price: 50000.0,
        });
        skill.price_history.push_back(PriceReading {
            timestamp: 1001,
            symbol: "ETH".to_string(),
            price: 3000.0,
        });

        let result = skill
            .handle("history", serde_json::json!({"symbol": "BTC"}))
            .await;
        assert!(result.is_ok());
        let val = result.unwrap();
        assert_eq!(val["count"], 1);
    }

    #[tokio::test]
    async fn test_handle_clear_alerts() {
        let mut skill = PriceMonitorSkill::new(vec![], 5.0, 60);
        skill.alerts.push(PriceAlertConfig {
            symbol: "BTC".to_string(),
            target_price: 100000.0,
            direction: "above".to_string(),
            triggered: false,
        });

        let result = skill.handle("clear_alerts", serde_json::json!({})).await;
        assert!(result.is_ok());
        let val = result.unwrap();
        assert_eq!(val["cleared"], 1);
        assert!(skill.alerts.is_empty());
    }

    #[tokio::test]
    async fn test_handle_list_alerts() {
        let mut skill = PriceMonitorSkill::new(vec![], 5.0, 60);
        skill.alerts.push(PriceAlertConfig {
            symbol: "BTC".to_string(),
            target_price: 100000.0,
            direction: "above".to_string(),
            triggered: false,
        });

        let result = skill.handle("list_alerts", serde_json::json!({})).await;
        assert!(result.is_ok());
        let val = result.unwrap();
        assert_eq!(val["count"], 1);
    }

    #[tokio::test]
    async fn test_handle_unknown() {
        let mut skill = PriceMonitorSkill::new(vec![], 5.0, 60);
        let result = skill.handle("unknown", serde_json::json!({})).await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_tick_no_client() {
        let mut skill = PriceMonitorSkill::new(vec!["BTC".to_string()], 5.0, 60);
        // No init called, so no http_client
        let report = skill.tick().await;
        // Should return None because fetch returns empty with no client
        assert!(report.is_none());
    }

    #[test]
    fn test_price_reading_serialization() {
        let reading = PriceReading {
            timestamp: 1234567890,
            symbol: "BTC".to_string(),
            price: 50000.0,
        };
        let json = serde_json::to_string(&reading).unwrap();
        let deserialized: PriceReading = serde_json::from_str(&json).unwrap();
        assert_eq!(deserialized.symbol, "BTC");
        assert_eq!(deserialized.price, 50000.0);
    }

    #[test]
    fn test_price_alert_config_serialization() {
        let alert = PriceAlertConfig {
            symbol: "ETH".to_string(),
            target_price: 5000.0,
            direction: "above".to_string(),
            triggered: false,
        };
        let json = serde_json::to_string(&alert).unwrap();
        let deserialized: PriceAlertConfig = serde_json::from_str(&json).unwrap();
        assert_eq!(deserialized.symbol, "ETH");
        assert!(!deserialized.triggered);
    }

    #[test]
    fn test_check_price_movements_zero_last_price() {
        let mut skill = PriceMonitorSkill::new(vec!["BTC".to_string()], 5.0, 60);
        skill.last_prices.insert("BTC".to_string(), 0.0);

        let mut prices = HashMap::new();
        prices.insert("BTC".to_string(), 50000.0);

        let reports = skill.check_price_movements(&prices);
        assert!(reports.is_empty()); // Should not divide by zero
    }
}
