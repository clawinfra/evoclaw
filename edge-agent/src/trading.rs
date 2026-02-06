use reqwest::Client;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::process::Command;
use tracing::info;

use crate::config::TradingConfig;

/// Hyperliquid API client
pub struct HyperliquidClient {
    config: TradingConfig,
    client: Client,
}

/// Market prices response from allMids endpoint
#[derive(Debug, Deserialize)]
pub struct AllMidsResponse {
    pub mids: HashMap<String, String>,
}

/// Position from clearinghouseState endpoint
#[derive(Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Position {
    pub coin: String,
    pub szi: String,              // Size (signed)
    pub entry_px: Option<String>, // Entry price
    pub position_value: String,
    pub unrealized_pnl: String,
    pub return_on_equity: String,
}

/// Clearinghouse state response
#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ClearinghouseState {
    pub asset_positions: Vec<Position>,
    #[allow(dead_code)]
    pub margin_summary: MarginSummary,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MarginSummary {
    #[allow(dead_code)]
    pub account_value: String,
    #[allow(dead_code)]
    pub total_margin_used: String,
}

/// Funding rate information
#[derive(Debug, Deserialize)]
#[allow(dead_code)]
pub struct FundingRate {
    pub coin: String,
    pub funding_rate: String,
    pub premium: String,
}

/// Order request
#[derive(Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct OrderRequest {
    pub asset: u32,
    pub is_buy: bool,
    pub limit_px: u64, // Price in basis points (e.g., 50000 = $50.000)
    pub sz: u64,       // Size in basis points
    pub reduce_only: bool,
    pub timestamp: u64,
}

/// Signature from Python helper
#[derive(Debug, Serialize, Deserialize)]
pub struct Signature {
    pub r: String,
    pub s: String,
    pub v: u8,
}

/// Order response
#[derive(Debug, Serialize, Deserialize)]
pub struct OrderResponse {
    pub status: String,
    pub response: serde_json::Value,
}

/// P&L tracker for trading performance
#[derive(Debug, Default, Serialize, Deserialize)]
pub struct PnLTracker {
    pub total_pnl: f64,
    pub realized_pnl: f64,
    pub unrealized_pnl: f64,
    pub win_count: u64,
    pub loss_count: u64,
    pub total_trades: u64,
}

impl PnLTracker {
    pub fn new() -> Self {
        Self::default()
    }

    #[allow(dead_code)]
    pub fn record_trade(&mut self, pnl: f64) {
        self.total_trades += 1;
        self.realized_pnl += pnl;
        self.total_pnl += pnl;

        if pnl > 0.0 {
            self.win_count += 1;
        } else {
            self.loss_count += 1;
        }
    }

    pub fn update_unrealized(&mut self, unrealized: f64) {
        self.unrealized_pnl = unrealized;
        self.total_pnl = self.realized_pnl + self.unrealized_pnl;
    }

    #[allow(dead_code)]
    pub fn win_rate(&self) -> f64 {
        if self.total_trades == 0 {
            return 0.0;
        }
        (self.win_count as f64 / self.total_trades as f64) * 100.0
    }
}

impl HyperliquidClient {
    pub fn new(config: TradingConfig) -> Self {
        Self {
            config,
            client: Client::new(),
        }
    }

    /// Get current market prices for all assets
    pub async fn get_prices(&self) -> Result<HashMap<String, f64>, Box<dyn std::error::Error>> {
        let url = format!("{}/info", self.config.hyperliquid_api);
        let response: AllMidsResponse = self
            .client
            .post(&url)
            .json(&serde_json::json!({
                "type": "allMids"
            }))
            .send()
            .await?
            .json()
            .await?;

        let mut prices = HashMap::new();
        for (coin, price_str) in response.mids {
            if let Ok(price) = price_str.parse::<f64>() {
                prices.insert(coin, price);
            }
        }

        Ok(prices)
    }

    /// Get current positions
    pub async fn get_positions(&self) -> Result<Vec<Position>, Box<dyn std::error::Error>> {
        let url = format!("{}/info", self.config.hyperliquid_api);
        let response: ClearinghouseState = self
            .client
            .post(&url)
            .json(&serde_json::json!({
                "type": "clearinghouseState",
                "user": self.config.wallet_address
            }))
            .send()
            .await?
            .json()
            .await?;

        Ok(response.asset_positions)
    }

    /// Get funding rates for all assets
    #[allow(dead_code)]
    pub async fn get_funding_rates(&self) -> Result<Vec<FundingRate>, Box<dyn std::error::Error>> {
        let url = format!("{}/info", self.config.hyperliquid_api);
        let response: Vec<FundingRate> = self
            .client
            .post(&url)
            .json(&serde_json::json!({
                "type": "fundingHistory",
                "coin": "BTC" // Get for main assets
            }))
            .send()
            .await?
            .json()
            .await?;

        Ok(response)
    }

    /// Place a limit order
    pub async fn place_limit_order(
        &self,
        asset: u32,
        is_buy: bool,
        price: f64,
        size: f64,
        reduce_only: bool,
    ) -> Result<OrderResponse, Box<dyn std::error::Error>> {
        let timestamp = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)?
            .as_millis() as u64;

        // Convert to basis points
        let limit_px = (price * 1000.0) as u64;
        let sz = (size * 1000.0) as u64;

        let order = OrderRequest {
            asset,
            is_buy,
            limit_px,
            sz,
            reduce_only,
            timestamp,
        };

        // Sign the order using Python helper
        let signature = self.sign_order(&order)?;

        // Submit the order
        let url = format!("{}/exchange", self.config.hyperliquid_api);
        let response: OrderResponse = self
            .client
            .post(&url)
            .json(&serde_json::json!({
                "type": "order",
                "order": order,
                "signature": signature
            }))
            .send()
            .await?
            .json()
            .await?;

        info!(
            asset = asset,
            is_buy = is_buy,
            price = price,
            size = size,
            status = %response.status,
            "order placed"
        );

        Ok(response)
    }

    /// Place stop-loss order
    #[allow(dead_code)]
    pub async fn place_stop_loss(
        &self,
        asset: u32,
        stop_price: f64,
        size: f64,
    ) -> Result<OrderResponse, Box<dyn std::error::Error>> {
        info!(
            asset = asset,
            stop_price = stop_price,
            size = size,
            "placing stop-loss"
        );
        // Stop-loss is a reduce-only sell order
        self.place_limit_order(asset, false, stop_price, size, true)
            .await
    }

    /// Place take-profit order
    #[allow(dead_code)]
    pub async fn place_take_profit(
        &self,
        asset: u32,
        target_price: f64,
        size: f64,
    ) -> Result<OrderResponse, Box<dyn std::error::Error>> {
        info!(
            asset = asset,
            target_price = target_price,
            size = size,
            "placing take-profit"
        );
        // Take-profit is a reduce-only sell order
        self.place_limit_order(asset, false, target_price, size, true)
            .await
    }

    /// Monitor positions and calculate P&L
    pub async fn monitor_positions(
        &self,
        tracker: &mut PnLTracker,
    ) -> Result<(), Box<dyn std::error::Error>> {
        let positions = self.get_positions().await?;

        let mut total_unrealized = 0.0;
        for position in positions {
            if let Ok(unrealized) = position.unrealized_pnl.parse::<f64>() {
                total_unrealized += unrealized;
                info!(
                    coin = %position.coin,
                    size = %position.szi,
                    unrealized_pnl = unrealized,
                    "position update"
                );
            }
        }

        tracker.update_unrealized(total_unrealized);
        Ok(())
    }

    /// Sign an order using the Python helper script
    fn sign_order(&self, order: &OrderRequest) -> Result<Signature, Box<dyn std::error::Error>> {
        let private_key = std::fs::read_to_string(&self.config.private_key_path)
            .map_err(|e| format!("failed to read private key: {}", e))?
            .trim()
            .to_string();

        let order_json = serde_json::to_string(&serde_json::json!({
            "type": "order",
            "asset": order.asset,
            "isBuy": order.is_buy,
            "limitPx": order.limit_px,
            "sz": order.sz,
            "reduceOnly": order.reduce_only,
            "timestamp": order.timestamp,
        }))?;

        let output = Command::new("python3")
            .arg("scripts/hl_sign.py")
            .arg(&self.config.wallet_address)
            .arg(&private_key)
            .arg(&order_json)
            .output()?;

        if !output.status.success() {
            let error = String::from_utf8_lossy(&output.stderr);
            return Err(format!("signing failed: {}", error).into());
        }

        let signature_json = String::from_utf8(output.stdout)?;
        let signature: Signature = serde_json::from_str(&signature_json)?;

        Ok(signature)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn create_test_config() -> TradingConfig {
        TradingConfig {
            hyperliquid_api: "https://api.test.com".to_string(),
            wallet_address: "0x1234567890abcdef".to_string(),
            private_key_path: "test.key".to_string(),
            max_position_size_usd: 5000.0,
            max_leverage: 5.0,
        }
    }

    #[test]
    fn test_pnl_tracker_new() {
        let tracker = PnLTracker::new();
        assert_eq!(tracker.total_pnl, 0.0);
        assert_eq!(tracker.realized_pnl, 0.0);
        assert_eq!(tracker.unrealized_pnl, 0.0);
        assert_eq!(tracker.win_count, 0);
        assert_eq!(tracker.loss_count, 0);
        assert_eq!(tracker.total_trades, 0);
    }

    #[test]
    fn test_pnl_tracker_default() {
        let tracker = PnLTracker::default();
        assert_eq!(tracker.total_pnl, 0.0);
    }

    #[test]
    fn test_record_trade_winning() {
        let mut tracker = PnLTracker::new();
        tracker.record_trade(100.0);
        
        assert_eq!(tracker.total_trades, 1);
        assert_eq!(tracker.win_count, 1);
        assert_eq!(tracker.loss_count, 0);
        assert_eq!(tracker.realized_pnl, 100.0);
        assert_eq!(tracker.total_pnl, 100.0);
    }

    #[test]
    fn test_record_trade_losing() {
        let mut tracker = PnLTracker::new();
        tracker.record_trade(-50.0);
        
        assert_eq!(tracker.total_trades, 1);
        assert_eq!(tracker.win_count, 0);
        assert_eq!(tracker.loss_count, 1);
        assert_eq!(tracker.realized_pnl, -50.0);
        assert_eq!(tracker.total_pnl, -50.0);
    }

    #[test]
    fn test_record_trade_multiple() {
        let mut tracker = PnLTracker::new();
        tracker.record_trade(100.0);
        tracker.record_trade(-30.0);
        tracker.record_trade(50.0);
        tracker.record_trade(-20.0);
        
        assert_eq!(tracker.total_trades, 4);
        assert_eq!(tracker.win_count, 2);
        assert_eq!(tracker.loss_count, 2);
        assert_eq!(tracker.realized_pnl, 100.0);
    }

    #[test]
    fn test_update_unrealized() {
        let mut tracker = PnLTracker::new();
        tracker.record_trade(100.0);
        tracker.update_unrealized(50.0);
        
        assert_eq!(tracker.unrealized_pnl, 50.0);
        assert_eq!(tracker.total_pnl, 150.0);
        assert_eq!(tracker.realized_pnl, 100.0);
    }

    #[test]
    fn test_update_unrealized_negative() {
        let mut tracker = PnLTracker::new();
        tracker.record_trade(100.0);
        tracker.update_unrealized(-30.0);
        
        assert_eq!(tracker.unrealized_pnl, -30.0);
        assert_eq!(tracker.total_pnl, 70.0);
    }

    #[test]
    fn test_win_rate_no_trades() {
        let tracker = PnLTracker::new();
        assert_eq!(tracker.win_rate(), 0.0);
    }

    #[test]
    fn test_win_rate_all_wins() {
        let mut tracker = PnLTracker::new();
        tracker.record_trade(100.0);
        tracker.record_trade(50.0);
        tracker.record_trade(75.0);
        
        assert_eq!(tracker.win_rate(), 100.0);
    }

    #[test]
    fn test_win_rate_all_losses() {
        let mut tracker = PnLTracker::new();
        tracker.record_trade(-100.0);
        tracker.record_trade(-50.0);
        
        assert_eq!(tracker.win_rate(), 0.0);
    }

    #[test]
    fn test_win_rate_mixed() {
        let mut tracker = PnLTracker::new();
        tracker.record_trade(100.0);
        tracker.record_trade(-50.0);
        tracker.record_trade(75.0);
        tracker.record_trade(-25.0);
        
        // 2 wins out of 4 = 50%
        assert_eq!(tracker.win_rate(), 50.0);
    }

    #[test]
    fn test_hyperliquid_client_new() {
        let config = create_test_config();
        let client = HyperliquidClient::new(config.clone());
        
        assert_eq!(client.config.wallet_address, "0x1234567890abcdef");
        assert_eq!(client.config.max_position_size_usd, 5000.0);
    }

    #[test]
    fn test_order_request_construction() {
        let order = OrderRequest {
            asset: 0,
            is_buy: true,
            limit_px: 50000000, // $50,000 in basis points
            sz: 100000,         // 0.1 BTC in basis points
            reduce_only: false,
            timestamp: 1234567890,
        };
        
        assert_eq!(order.asset, 0);
        assert!(order.is_buy);
        assert_eq!(order.limit_px, 50000000);
        assert_eq!(order.sz, 100000);
        assert!(!order.reduce_only);
    }

    #[test]
    fn test_order_request_serialization() {
        let order = OrderRequest {
            asset: 0,
            is_buy: false,
            limit_px: 3000000,
            sz: 500000,
            reduce_only: true,
            timestamp: 9876543210,
        };
        
        let json = serde_json::to_string(&order).unwrap();
        let deserialized: OrderRequest = serde_json::from_str(&json).unwrap();
        
        assert_eq!(deserialized.asset, 0);
        assert!(!deserialized.is_buy);
        assert!(deserialized.reduce_only);
    }

    #[test]
    fn test_signature_serialization() {
        let sig = Signature {
            r: "0xabc123".to_string(),
            s: "0xdef456".to_string(),
            v: 27,
        };
        
        let json = serde_json::to_string(&sig).unwrap();
        let deserialized: Signature = serde_json::from_str(&json).unwrap();
        
        assert_eq!(deserialized.r, "0xabc123");
        assert_eq!(deserialized.s, "0xdef456");
        assert_eq!(deserialized.v, 27);
    }

    #[test]
    fn test_position_deserialization() {
        let json = r#"{
            "coin": "BTC",
            "szi": "0.5",
            "entryPx": "50000.0",
            "positionValue": "25000.0",
            "unrealizedPnl": "500.0",
            "returnOnEquity": "0.02"
        }"#;
        
        let position: Position = serde_json::from_str(json).unwrap();
        assert_eq!(position.coin, "BTC");
        assert_eq!(position.szi, "0.5");
        assert_eq!(position.entry_px, Some("50000.0".to_string()));
    }

    #[test]
    fn test_order_response_deserialization() {
        let json = r#"{
            "status": "success",
            "response": {"orderId": 12345}
        }"#;
        
        let response: OrderResponse = serde_json::from_str(json).unwrap();
        assert_eq!(response.status, "success");
    }

    #[test]
    fn test_pnl_tracker_serialization() {
        let mut tracker = PnLTracker::new();
        tracker.record_trade(100.0);
        tracker.record_trade(-30.0);
        tracker.update_unrealized(50.0);
        
        let json = serde_json::to_string(&tracker).unwrap();
        let deserialized: PnLTracker = serde_json::from_str(&json).unwrap();
        
        assert_eq!(deserialized.total_trades, 2);
        assert_eq!(deserialized.win_count, 1);
        assert_eq!(deserialized.loss_count, 1);
        assert_eq!(deserialized.realized_pnl, 70.0);
        assert_eq!(deserialized.unrealized_pnl, 50.0);
        assert_eq!(deserialized.total_pnl, 120.0);
    }

    #[test]
    fn test_all_mids_response_parsing() {
        let json = r#"{
            "mids": {
                "BTC": "50000.0",
                "ETH": "3000.5",
                "SOL": "150.25"
            }
        }"#;
        
        let response: AllMidsResponse = serde_json::from_str(json).unwrap();
        assert_eq!(response.mids.len(), 3);
        assert_eq!(response.mids.get("BTC"), Some(&"50000.0".to_string()));
    }

    #[test]
    fn test_price_conversion() {
        // Test converting price to basis points (used in orders)
        let price = 50000.0;
        let price_bp = (price * 1000.0) as u64;
        assert_eq!(price_bp, 50000000);
        
        let size = 0.1;
        let size_bp = (size * 1000.0) as u64;
        assert_eq!(size_bp, 100);
    }
}
