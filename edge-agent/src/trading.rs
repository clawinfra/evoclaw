use reqwest::Client;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::sync::Mutex;
use tracing::{info, warn};

use crate::config::TradingConfig;
use crate::signing::{self, EthSignature};

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum TimeInForce { Gtc, Alo, Ioc }

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PlaceOrderRequest {
    pub coin: String, pub is_buy: bool, pub price: String, pub size: String,
    pub reduce_only: bool, pub tif: TimeInForce, pub cloid: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CancelOrderRequest { pub coin: String, pub oid: u64 }

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ModifyOrderRequest {
    pub oid: u64, pub coin: String, pub is_buy: bool, pub price: String,
    pub size: String, pub reduce_only: bool, pub tif: TimeInForce, pub cloid: Option<String>,
}

#[derive(Debug, Deserialize)]
pub struct AllMidsResponse { pub mids: HashMap<String, String> }

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Position {
    pub coin: String, pub szi: String, pub entry_px: Option<String>,
    pub position_value: String, pub unrealized_pnl: String, pub return_on_equity: String,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ClearinghouseState {
    pub asset_positions: Vec<AssetPosition>,
    pub margin_summary: MarginSummary,
}

#[derive(Debug, Deserialize)]
pub struct AssetPosition {
    pub position: Position,
    #[serde(rename = "type")]
    pub position_type: String,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct MarginSummary { pub account_value: String, pub total_margin_used: String }

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OpenOrder {
    pub coin: String, pub oid: u64, #[serde(rename = "limitPx")] pub limit_px: String,
    pub sz: String, pub side: String, pub timestamp: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Fill {
    pub coin: String, pub px: String, pub sz: String, pub side: String,
    pub time: u64, pub fee: String, pub oid: u64,
    #[serde(rename = "closedPnl")] pub closed_pnl: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OrderResponse { pub status: String, pub response: serde_json::Value }

#[derive(Debug, Clone, Deserialize)]
pub struct MetaResponse { pub universe: Vec<AssetMeta> }

#[derive(Debug, Clone, Deserialize)]
pub struct AssetMeta { pub name: String, #[serde(rename = "szDecimals")] pub sz_decimals: u32 }

#[derive(Debug, Default, Serialize, Deserialize)]
pub struct PnLTracker {
    pub total_pnl: f64, pub realized_pnl: f64, pub unrealized_pnl: f64,
    pub win_count: u64, pub loss_count: u64, pub total_trades: u64,
}

impl PnLTracker {
    pub fn new() -> Self { Self::default() }
    pub fn record_trade(&mut self, pnl: f64) {
        self.total_trades += 1; self.realized_pnl += pnl;
        self.total_pnl = self.realized_pnl + self.unrealized_pnl;
        if pnl > 0.0 { self.win_count += 1; } else { self.loss_count += 1; }
    }
    pub fn update_unrealized(&mut self, unrealized: f64) {
        self.unrealized_pnl = unrealized; self.total_pnl = self.realized_pnl + self.unrealized_pnl;
    }
    pub fn win_rate(&self) -> f64 {
        if self.total_trades == 0 { return 0.0; }
        (self.win_count as f64 / self.total_trades as f64) * 100.0
    }
}

#[derive(Debug, Clone)]
pub struct RateLimiter { inner: Arc<Mutex<RateLimiterInner>> }
#[derive(Debug)]
struct RateLimiterInner { max_requests: u32, window: Duration, timestamps: Vec<Instant> }

impl RateLimiter {
    pub fn new(max_requests: u32, window: Duration) -> Self {
        Self { inner: Arc::new(Mutex::new(RateLimiterInner { max_requests, window, timestamps: Vec::new() })) }
    }
    pub async fn acquire(&self) {
        loop {
            let mut inner = self.inner.lock().await;
            let now = Instant::now();
            let window = inner.window;
            inner.timestamps.retain(|&ts| now.duration_since(ts) < window);
            if (inner.timestamps.len() as u32) < inner.max_requests { inner.timestamps.push(now); return; }
            let oldest = inner.timestamps[0];
            let wait = window - now.duration_since(oldest);
            drop(inner);
            tokio::time::sleep(wait).await;
        }
    }
}

pub struct HyperliquidClient {
    config: TradingConfig, client: Client, private_key: Option<String>,
    rate_limiter: RateLimiter, asset_index: Arc<Mutex<HashMap<String, u32>>>,
}

impl HyperliquidClient {
    pub fn new(config: TradingConfig) -> Self {
        let private_key = signing::load_private_key(&config.private_key_path).ok();
        let rate_limiter = RateLimiter::new(100, Duration::from_secs(10));
        Self { config, client: Client::new(), private_key, rate_limiter, asset_index: Arc::new(Mutex::new(HashMap::new())) }
    }

    async fn resolve_asset(&self, coin: &str) -> Result<u32, Box<dyn std::error::Error>> {
        { let idx = self.asset_index.lock().await; if let Some(&i) = idx.get(coin) { return Ok(i); } }
        self.refresh_asset_index().await?;
        self.asset_index.lock().await.get(coin).copied().ok_or_else(|| format!("unknown asset: {}", coin).into())
    }

    async fn refresh_asset_index(&self) -> Result<(), Box<dyn std::error::Error>> {
        let url = format!("{}/info", self.config.effective_api_url());
        self.rate_limiter.acquire().await;
        let resp: MetaResponse = self.client.post(&url).json(&serde_json::json!({"type": "meta"})).send().await?.json().await?;
        let mut idx = self.asset_index.lock().await;
        for (i, a) in resp.universe.iter().enumerate() { idx.insert(a.name.clone(), i as u32); }
        Ok(())
    }

    fn api_url(&self) -> &str { self.config.effective_api_url() }
    fn require_private_key(&self) -> Result<&str, Box<dyn std::error::Error>> {
        self.private_key.as_deref().ok_or_else(|| "private key not loaded".into())
    }

    pub async fn get_prices(&self) -> Result<HashMap<String, f64>, Box<dyn std::error::Error>> {
        let url = format!("{}/info", self.api_url()); self.rate_limiter.acquire().await;
        let r: AllMidsResponse = self.client.post(&url).json(&serde_json::json!({"type": "allMids"})).send().await?.json().await?;
        Ok(r.mids.into_iter().filter_map(|(c, p)| p.parse().ok().map(|v| (c, v))).collect())
    }

    pub async fn get_positions(&self) -> Result<Vec<Position>, Box<dyn std::error::Error>> {
        let url = format!("{}/info", self.api_url()); self.rate_limiter.acquire().await;
        let r: ClearinghouseState = self.client.post(&url).json(&serde_json::json!({"type": "clearinghouseState", "user": self.config.wallet_address})).send().await?.json().await?;
        Ok(r.asset_positions.into_iter().map(|ap| ap.position).collect())
    }

    pub async fn get_account_balance(&self) -> Result<f64, Box<dyn std::error::Error>> {
        let url = format!("{}/info", self.api_url()); self.rate_limiter.acquire().await;
        let r: ClearinghouseState = self.client.post(&url).json(&serde_json::json!({"type": "clearinghouseState", "user": self.config.wallet_address})).send().await?.json().await?;
        Ok(r.margin_summary.account_value.parse()?)
    }

    pub async fn get_open_orders(&self) -> Result<Vec<OpenOrder>, Box<dyn std::error::Error>> {
        let url = format!("{}/info", self.api_url()); self.rate_limiter.acquire().await;
        Ok(self.client.post(&url).json(&serde_json::json!({"type": "openOrders", "user": self.config.wallet_address})).send().await?.json().await?)
    }

    pub async fn get_fills(&self) -> Result<Vec<Fill>, Box<dyn std::error::Error>> {
        let url = format!("{}/info", self.api_url()); self.rate_limiter.acquire().await;
        Ok(self.client.post(&url).json(&serde_json::json!({"type": "userFills", "user": self.config.wallet_address})).send().await?.json().await?)
    }

    pub async fn place_order(&self, req: &PlaceOrderRequest) -> Result<OrderResponse, Box<dyn std::error::Error>> {
        let key = self.require_private_key()?;
        let asset = self.resolve_asset(&req.coin).await?;
        let tif_s = match req.tif { TimeInForce::Gtc => "Gtc", TimeInForce::Alo => "Alo", TimeInForce::Ioc => "Ioc" };
        let mut ow = serde_json::json!({"a": asset, "b": req.is_buy, "p": req.price, "s": req.size, "r": req.reduce_only, "t": {"limit": {"tif": tif_s}}});
        if let Some(ref c) = req.cloid { ow["c"] = serde_json::json!(c); }
        let action = serde_json::json!({"type": "order", "orders": [ow], "grouping": "na"});
        let nonce = current_timestamp_ms();
        let sig = signing::sign_l1_action(key, &action, None, nonce, None, self.config.network_mode).await?;
        self.send_exchange_request(&action, nonce, &sig).await
    }

    pub async fn cancel_order(&self, req: &CancelOrderRequest) -> Result<OrderResponse, Box<dyn std::error::Error>> {
        let key = self.require_private_key()?;
        let asset = self.resolve_asset(&req.coin).await?;
        let action = serde_json::json!({"type": "cancel", "cancels": [{"a": asset, "o": req.oid}]});
        let nonce = current_timestamp_ms();
        let sig = signing::sign_l1_action(key, &action, None, nonce, None, self.config.network_mode).await?;
        self.send_exchange_request(&action, nonce, &sig).await
    }

    pub async fn modify_order(&self, req: &ModifyOrderRequest) -> Result<OrderResponse, Box<dyn std::error::Error>> {
        let key = self.require_private_key()?;
        let asset = self.resolve_asset(&req.coin).await?;
        let tif_s = match req.tif { TimeInForce::Gtc => "Gtc", TimeInForce::Alo => "Alo", TimeInForce::Ioc => "Ioc" };
        let mut ow = serde_json::json!({"a": asset, "b": req.is_buy, "p": req.price, "s": req.size, "r": req.reduce_only, "t": {"limit": {"tif": tif_s}}});
        if let Some(ref c) = req.cloid { ow["c"] = serde_json::json!(c); }
        let action = serde_json::json!({"type": "modify", "oid": req.oid, "order": ow});
        let nonce = current_timestamp_ms();
        let sig = signing::sign_l1_action(key, &action, None, nonce, None, self.config.network_mode).await?;
        self.send_exchange_request(&action, nonce, &sig).await
    }

    pub async fn cancel_all_orders(&self) -> Result<Vec<OrderResponse>, Box<dyn std::error::Error>> {
        let orders = self.get_open_orders().await?;
        let mut results = Vec::new();
        for order in orders {
            match self.cancel_order(&CancelOrderRequest { coin: order.coin.clone(), oid: order.oid }).await {
                Ok(r) => results.push(r),
                Err(e) => { warn!(oid = order.oid, error = %e, "cancel failed"); }
            }
        }
        Ok(results)
    }

    async fn send_exchange_request(&self, action: &serde_json::Value, nonce: u64, sig: &EthSignature) -> Result<OrderResponse, Box<dyn std::error::Error>> {
        let url = format!("{}/exchange", self.api_url());
        let body = serde_json::json!({"action": action, "nonce": nonce, "signature": {"r": sig.r, "s": sig.s, "v": sig.v}});
        for attempt in 0..3u32 {
            self.rate_limiter.acquire().await;
            match self.client.post(&url).json(&body).send().await {
                Ok(resp) => {
                    let st = resp.status(); let text = resp.text().await?;
                    if st.is_success() {
                        return Ok(serde_json::from_str(&text).unwrap_or(OrderResponse { status: "ok".to_string(), response: serde_json::json!({"raw": text}) }));
                    }
                    if st.as_u16() == 429 { tokio::time::sleep(Duration::from_millis(500 * 2u64.pow(attempt))).await; continue; }
                    return Err(format!("exchange request failed ({}): {}", st, text).into());
                }
                Err(e) => { tokio::time::sleep(Duration::from_millis(500 * 2u64.pow(attempt))).await; if attempt == 2 { return Err(e.into()); } }
            }
        }
        Err("max retries".into())
    }

    pub async fn monitor_positions(&self, tracker: &mut PnLTracker) -> Result<(), Box<dyn std::error::Error>> {
        let positions = self.get_positions().await?;
        let mut total = 0.0;
        for p in positions { if let Ok(u) = p.unrealized_pnl.parse::<f64>() { total += u; info!(coin = %p.coin, size = %p.szi, unrealized_pnl = u, "position update"); } }
        tracker.update_unrealized(total);
        Ok(())
    }
}

fn current_timestamp_ms() -> u64 { std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).unwrap().as_millis() as u64 }
pub fn format_price(price: f64) -> String { if price >= 10000.0 { format!("{:.1}", price) } else if price >= 1000.0 { format!("{:.2}", price) } else if price >= 1.0 { format!("{:.4}", price) } else { format!("{:.6}", price) } }
pub fn format_size(size: f64, decimals: u32) -> String { format!("{:.prec$}", size, prec = decimals as usize) }

#[cfg(test)]
mod tests {
    use super::*;
    fn create_test_config() -> TradingConfig {
        TradingConfig { hyperliquid_api: "https://api.test.com".to_string(), wallet_address: "0x1234".to_string(), private_key_path: "test.key".to_string(), max_position_size_usd: 5000.0, max_leverage: 5.0, network_mode: crate::config::NetworkMode::Testnet, trading_mode: crate::config::TradingMode::Paper, paper_log_path: "t.jsonl".to_string() }
    }
    #[test] fn test_pnl_tracker_new() { let t = PnLTracker::new(); assert_eq!(t.total_pnl, 0.0); assert_eq!(t.total_trades, 0); }
    #[test] fn test_pnl_tracker_default() { assert_eq!(PnLTracker::default().total_pnl, 0.0); }
    #[test] fn test_record_trade_winning() { let mut t = PnLTracker::new(); t.record_trade(100.0); assert_eq!(t.win_count, 1); assert_eq!(t.realized_pnl, 100.0); }
    #[test] fn test_record_trade_losing() { let mut t = PnLTracker::new(); t.record_trade(-50.0); assert_eq!(t.loss_count, 1); assert_eq!(t.realized_pnl, -50.0); }
    #[test] fn test_record_trade_multiple() { let mut t = PnLTracker::new(); t.record_trade(100.0); t.record_trade(-30.0); t.record_trade(50.0); t.record_trade(-20.0); assert_eq!(t.total_trades, 4); assert_eq!(t.win_count, 2); assert_eq!(t.realized_pnl, 100.0); }
    #[test] fn test_update_unrealized() { let mut t = PnLTracker::new(); t.record_trade(100.0); t.update_unrealized(50.0); assert_eq!(t.total_pnl, 150.0); }
    #[test] fn test_update_unrealized_negative() { let mut t = PnLTracker::new(); t.record_trade(100.0); t.update_unrealized(-30.0); assert_eq!(t.total_pnl, 70.0); }
    #[test] fn test_win_rate_no_trades() { assert_eq!(PnLTracker::new().win_rate(), 0.0); }
    #[test] fn test_win_rate_all_wins() { let mut t = PnLTracker::new(); t.record_trade(100.0); t.record_trade(50.0); t.record_trade(75.0); assert_eq!(t.win_rate(), 100.0); }
    #[test] fn test_win_rate_all_losses() { let mut t = PnLTracker::new(); t.record_trade(-100.0); t.record_trade(-50.0); assert_eq!(t.win_rate(), 0.0); }
    #[test] fn test_win_rate_mixed() { let mut t = PnLTracker::new(); t.record_trade(100.0); t.record_trade(-50.0); t.record_trade(75.0); t.record_trade(-25.0); assert_eq!(t.win_rate(), 50.0); }
    #[test] fn test_client_new() { let c = HyperliquidClient::new(create_test_config()); assert_eq!(c.config.wallet_address, "0x1234"); }
    #[test] fn test_place_order_req() { let r = PlaceOrderRequest { coin: "BTC".to_string(), is_buy: true, price: "50000".to_string(), size: "0.01".to_string(), reduce_only: false, tif: TimeInForce::Gtc, cloid: None }; assert_eq!(r.coin, "BTC"); }
    #[test] fn test_cancel_order_req() { let r = CancelOrderRequest { coin: "ETH".to_string(), oid: 12345 }; assert_eq!(r.oid, 12345); }
    #[test] fn test_modify_order_req() { let r = ModifyOrderRequest { oid: 999, coin: "BTC".to_string(), is_buy: false, price: "48000".to_string(), size: "0.02".to_string(), reduce_only: true, tif: TimeInForce::Alo, cloid: None }; assert!(r.reduce_only); }
    #[test] fn test_order_response() { let r: OrderResponse = serde_json::from_str(r#"{"status":"ok","response":{"oid":1}}"#).unwrap(); assert_eq!(r.status, "ok"); }
    #[test] fn test_position_deser() { let p: Position = serde_json::from_str(r#"{"coin":"BTC","szi":"0.5","entryPx":"50000","positionValue":"25000","unrealizedPnl":"500","returnOnEquity":"0.02"}"#).unwrap(); assert_eq!(p.coin, "BTC"); }
    #[test] fn test_pnl_serde() { let mut t = PnLTracker::new(); t.record_trade(100.0); t.record_trade(-30.0); t.update_unrealized(50.0); let j = serde_json::to_string(&t).unwrap(); let d: PnLTracker = serde_json::from_str(&j).unwrap(); assert_eq!(d.total_pnl, 120.0); }
    #[test] fn test_all_mids() { let r: AllMidsResponse = serde_json::from_str(r#"{"mids":{"BTC":"50000","ETH":"3000"}}"#).unwrap(); assert_eq!(r.mids.len(), 2); }
    #[test] fn test_format_price() { assert_eq!(format_price(50000.0), "50000.0"); assert_eq!(format_price(3000.5), "3000.50"); }
    #[test] fn test_format_size() { assert_eq!(format_size(0.01, 2), "0.01"); }
    #[test] fn test_tif_serde() { let j = serde_json::to_string(&TimeInForce::Gtc).unwrap(); assert_eq!(serde_json::from_str::<TimeInForce>(&j).unwrap(), TimeInForce::Gtc); }
    #[test] fn test_open_order() { let o: OpenOrder = serde_json::from_str(r#"{"coin":"BTC","oid":1,"limitPx":"50000","sz":"0.01","side":"B","timestamp":1700000000}"#).unwrap(); assert_eq!(o.oid, 1); }
    #[test] fn test_fill() { let f: Fill = serde_json::from_str(r#"{"coin":"BTC","px":"50000","sz":"0.01","side":"B","time":1700000000,"fee":"0.5","oid":1,"closedPnl":"100"}"#).unwrap(); assert_eq!(f.closed_pnl, "100"); }
    #[tokio::test] async fn test_rate_limiter() { let l = RateLimiter::new(5, Duration::from_secs(1)); for _ in 0..5 { l.acquire().await; } }
    #[tokio::test] async fn test_rate_limiter_blocks() { let l = RateLimiter::new(2, Duration::from_millis(100)); let s = Instant::now(); for _ in 0..3 { l.acquire().await; } assert!(s.elapsed() >= Duration::from_millis(80)); }
    #[test] fn test_timestamp() { assert!(current_timestamp_ms() > 1_577_836_800_000); }
    #[test] fn test_meta() { let m: MetaResponse = serde_json::from_str(r#"{"universe":[{"name":"BTC","szDecimals":5}]}"#).unwrap(); assert_eq!(m.universe[0].name, "BTC"); }
}
