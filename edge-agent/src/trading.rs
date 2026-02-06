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
