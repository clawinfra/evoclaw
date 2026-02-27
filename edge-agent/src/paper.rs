//! Paper trading module — simulates trading locally with no real orders.
//!
//! Implements the same interface as live trading so strategies can be tested
//! without risking real money. All trades are logged to a JSONL file for
//! backtesting analysis.

use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::fs::OpenOptions;
use std::io::Write;
use tracing::info;

/// A simulated paper position
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PaperPosition {
    pub coin: String,
    pub size: f64,        // Signed: positive = long, negative = short
    pub entry_price: f64, // Average entry price
    pub notional: f64,    // Current notional value
    pub unrealized_pnl: f64,
}

/// A simulated paper order
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PaperOrder {
    pub oid: u64,
    pub coin: String,
    pub is_buy: bool,
    pub price: f64,
    pub size: f64,
    pub reduce_only: bool,
    pub timestamp: u64,
    pub status: PaperOrderStatus,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum PaperOrderStatus {
    Open,
    Filled,
    Canceled,
}

/// A paper trade fill record
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PaperFill {
    pub timestamp: u64,
    pub coin: String,
    pub is_buy: bool,
    pub price: f64,
    pub size: f64,
    pub pnl: f64,
    pub fee: f64,
}

/// Paper trading engine
pub struct PaperTrader {
    positions: HashMap<String, PaperPosition>,
    orders: Vec<PaperOrder>,
    fills: Vec<PaperFill>,
    next_oid: u64,
    balance: f64,
    initial_balance: f64,
    log_path: String,
    total_fees: f64,
}

impl PaperTrader {
    /// Create a new paper trader with the given starting balance
    pub fn new(initial_balance: f64, log_path: String) -> Self {
        Self {
            positions: HashMap::new(),
            orders: Vec::new(),
            fills: Vec::new(),
            next_oid: 1,
            balance: initial_balance,
            initial_balance,
            log_path,
            total_fees: 0.0,
        }
    }

    /// Place a paper limit order
    pub fn place_order(
        &mut self,
        coin: &str,
        is_buy: bool,
        price: f64,
        size: f64,
        reduce_only: bool,
    ) -> u64 {
        let oid = self.next_oid;
        self.next_oid += 1;

        let order = PaperOrder {
            oid,
            coin: coin.to_string(),
            is_buy,
            price,
            size,
            reduce_only,
            timestamp: current_timestamp_ms(),
            status: PaperOrderStatus::Open,
        };

        info!(
            oid = oid,
            coin = coin,
            is_buy = is_buy,
            price = price,
            size = size,
            "[PAPER] order placed"
        );

        self.orders.push(order);
        oid
    }

    /// Place a market order (fills immediately at current price)
    pub fn place_market_order(
        &mut self,
        coin: &str,
        is_buy: bool,
        size: f64,
        current_price: f64,
    ) -> u64 {
        let oid = self.place_order(coin, is_buy, current_price, size, false);
        self.fill_order(oid, current_price);
        oid
    }

    /// Cancel an open order
    pub fn cancel_order(&mut self, oid: u64) -> bool {
        for order in &mut self.orders {
            if order.oid == oid && order.status == PaperOrderStatus::Open {
                order.status = PaperOrderStatus::Canceled;
                info!(oid = oid, "[PAPER] order canceled");
                return true;
            }
        }
        false
    }

    /// Check if any limit orders should fill at the current prices
    pub fn check_fills(&mut self, prices: &HashMap<String, f64>) {
        let orders_to_fill: Vec<(u64, f64)> = self
            .orders
            .iter()
            .filter(|o| o.status == PaperOrderStatus::Open)
            .filter_map(|o| {
                if let Some(&price) = prices.get(&o.coin) {
                    let should_fill = if o.is_buy {
                        price <= o.price
                    } else {
                        price >= o.price
                    };
                    if should_fill {
                        Some((o.oid, price))
                    } else {
                        None
                    }
                } else {
                    None
                }
            })
            .collect();

        for (oid, fill_price) in orders_to_fill {
            self.fill_order(oid, fill_price);
        }
    }

    /// Fill an order at the given price
    fn fill_order(&mut self, oid: u64, fill_price: f64) {
        // Extract order data first to avoid borrow conflicts
        let order_data = self
            .orders
            .iter()
            .find(|o| o.oid == oid)
            .map(|o| (o.status, o.coin.clone(), o.is_buy, o.size));

        let (_status, coin, is_buy, size) = match order_data {
            Some(data) if data.0 == PaperOrderStatus::Open => data,
            _ => return,
        };

        // Mark as filled
        if let Some(order) = self.orders.iter_mut().find(|o| o.oid == oid) {
            order.status = PaperOrderStatus::Filled;
        }

        let signed_size = if is_buy { size } else { -size };

        // Calculate P&L from closing portion
        let pnl = self.update_position(&coin, signed_size, fill_price);

        // Fee: 0.035% taker fee (HL typical)
        let fee = fill_price * size.abs() * 0.00035;
        self.balance -= fee;
        self.total_fees += fee;

        let fill = PaperFill {
            timestamp: current_timestamp_ms(),
            coin: coin.clone(),
            is_buy,
            price: fill_price,
            size,
            pnl,
            fee,
        };

        info!(
            oid = oid,
            coin = %fill.coin,
            is_buy = fill.is_buy,
            price = fill_price,
            size = fill.size,
            pnl = pnl,
            fee = fee,
            "[PAPER] order filled"
        );

        self.fills.push(fill.clone());
        self.log_fill(&fill);
    }

    /// Update position for a coin. Returns realized PnL from any closing portion.
    fn update_position(&mut self, coin: &str, signed_size: f64, price: f64) -> f64 {
        let mut realized_pnl = 0.0;

        if let Some(pos) = self.positions.get_mut(coin) {
            let old_size = pos.size;

            // Check if this is reducing or flipping the position
            if (old_size > 0.0 && signed_size < 0.0) || (old_size < 0.0 && signed_size > 0.0) {
                // Closing (at least partially)
                let closing_size = signed_size.abs().min(old_size.abs());
                if old_size > 0.0 {
                    // Was long, selling: pnl = (exit - entry) * size
                    realized_pnl = (price - pos.entry_price) * closing_size;
                } else {
                    // Was short, buying: pnl = (entry - exit) * size
                    realized_pnl = (pos.entry_price - price) * closing_size;
                }
                self.balance += realized_pnl;
            }

            let new_size = old_size + signed_size;

            if new_size.abs() < 1e-10 {
                // Position fully closed
                self.positions.remove(coin);
            } else {
                // Update entry price for any newly opened portion
                if (old_size >= 0.0 && signed_size > 0.0) || (old_size <= 0.0 && signed_size < 0.0)
                {
                    // Adding to same direction: weighted average entry
                    let old_notional = pos.entry_price * old_size.abs();
                    let new_notional = price * signed_size.abs();
                    pos.entry_price = (old_notional + new_notional) / new_size.abs();
                }
                // If partially closing + opening opposite, entry for new direction = price
                if (old_size > 0.0 && new_size < 0.0) || (old_size < 0.0 && new_size > 0.0) {
                    pos.entry_price = price;
                }
                pos.size = new_size;
                pos.notional = price * new_size.abs();
            }
        } else if signed_size.abs() > 1e-10 {
            // New position
            self.positions.insert(
                coin.to_string(),
                PaperPosition {
                    coin: coin.to_string(),
                    size: signed_size,
                    entry_price: price,
                    notional: price * signed_size.abs(),
                    unrealized_pnl: 0.0,
                },
            );
        }

        realized_pnl
    }

    /// Update unrealized P&L for all positions at current prices
    pub fn update_unrealized(&mut self, prices: &HashMap<String, f64>) {
        for pos in self.positions.values_mut() {
            if let Some(&price) = prices.get(&pos.coin) {
                pos.unrealized_pnl = if pos.size > 0.0 {
                    (price - pos.entry_price) * pos.size
                } else {
                    (pos.entry_price - price) * pos.size.abs()
                };
                pos.notional = price * pos.size.abs();
            }
        }
    }

    /// Get all open positions
    pub fn get_positions(&self) -> Vec<&PaperPosition> {
        self.positions.values().collect()
    }

    /// Get all open orders
    pub fn get_open_orders(&self) -> Vec<&PaperOrder> {
        self.orders
            .iter()
            .filter(|o| o.status == PaperOrderStatus::Open)
            .collect()
    }

    /// Get all fills
    pub fn get_fills(&self) -> &[PaperFill] {
        &self.fills
    }

    /// Get current balance
    pub fn balance(&self) -> f64 {
        self.balance
    }

    /// Get total P&L (balance change + unrealized)
    pub fn total_pnl(&self) -> f64 {
        let unrealized: f64 = self.positions.values().map(|p| p.unrealized_pnl).sum();
        (self.balance - self.initial_balance) + unrealized
    }

    /// Get number of fills
    pub fn fill_count(&self) -> usize {
        self.fills.len()
    }

    /// Cancel all open orders
    pub fn cancel_all_orders(&mut self) -> usize {
        let mut canceled = 0;
        for order in &mut self.orders {
            if order.status == PaperOrderStatus::Open {
                order.status = PaperOrderStatus::Canceled;
                canceled += 1;
            }
        }
        canceled
    }

    /// Log a fill to the JSONL file
    fn log_fill(&self, fill: &PaperFill) {
        if let Ok(mut file) = OpenOptions::new()
            .create(true)
            .append(true)
            .open(&self.log_path)
        {
            if let Ok(json) = serde_json::to_string(fill) {
                let _ = writeln!(file, "{}", json);
            }
        }
    }
}

fn current_timestamp_ms() -> u64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as u64
}

#[cfg(test)]
mod tests {
    use super::*;

    fn create_paper_trader() -> PaperTrader {
        let dir = tempfile::tempdir().unwrap();
        let log_path = dir.path().join("test_paper.jsonl");
        PaperTrader::new(10000.0, log_path.to_str().unwrap().to_string())
    }

    #[test]
    fn test_paper_trader_new() {
        let trader = create_paper_trader();
        assert_eq!(trader.balance(), 10000.0);
        assert_eq!(trader.total_pnl(), 0.0);
        assert!(trader.get_positions().is_empty());
        assert!(trader.get_open_orders().is_empty());
        assert_eq!(trader.fill_count(), 0);
    }

    #[test]
    fn test_place_order() {
        let mut trader = create_paper_trader();
        let oid = trader.place_order("BTC", true, 50000.0, 0.1, false);

        assert_eq!(oid, 1);
        assert_eq!(trader.get_open_orders().len(), 1);
        assert_eq!(trader.get_open_orders()[0].coin, "BTC");
        assert_eq!(trader.get_open_orders()[0].price, 50000.0);
    }

    #[test]
    fn test_place_market_order_long() {
        let mut trader = create_paper_trader();
        let oid = trader.place_market_order("BTC", true, 0.1, 50000.0);

        assert_eq!(oid, 1);
        assert_eq!(trader.fill_count(), 1);
        assert!(trader.get_open_orders().is_empty()); // Market order should be filled immediately

        let positions = trader.get_positions();
        assert_eq!(positions.len(), 1);
        assert_eq!(positions[0].coin, "BTC");
        assert!((positions[0].size - 0.1).abs() < 1e-10);
        assert!((positions[0].entry_price - 50000.0).abs() < 1e-10);
    }

    #[test]
    fn test_place_market_order_short() {
        let mut trader = create_paper_trader();
        trader.place_market_order("ETH", false, 1.0, 3000.0);

        let positions = trader.get_positions();
        assert_eq!(positions.len(), 1);
        assert!((positions[0].size + 1.0).abs() < 1e-10); // Negative = short
    }

    #[test]
    fn test_cancel_order() {
        let mut trader = create_paper_trader();
        let oid = trader.place_order("BTC", true, 49000.0, 0.1, false);

        assert_eq!(trader.get_open_orders().len(), 1);
        assert!(trader.cancel_order(oid));
        assert!(trader.get_open_orders().is_empty());
    }

    #[test]
    fn test_cancel_nonexistent_order() {
        let mut trader = create_paper_trader();
        assert!(!trader.cancel_order(999));
    }

    #[test]
    fn test_cancel_all_orders() {
        let mut trader = create_paper_trader();
        trader.place_order("BTC", true, 49000.0, 0.1, false);
        trader.place_order("ETH", false, 3200.0, 1.0, false);

        assert_eq!(trader.get_open_orders().len(), 2);
        let canceled = trader.cancel_all_orders();
        assert_eq!(canceled, 2);
        assert!(trader.get_open_orders().is_empty());
    }

    #[test]
    fn test_check_fills_limit_buy() {
        let mut trader = create_paper_trader();
        trader.place_order("BTC", true, 49000.0, 0.1, false);

        // Price doesn't trigger
        let mut prices = HashMap::new();
        prices.insert("BTC".to_string(), 50000.0);
        trader.check_fills(&prices);
        assert_eq!(trader.fill_count(), 0);

        // Price drops below limit → fill
        prices.insert("BTC".to_string(), 48500.0);
        trader.check_fills(&prices);
        assert_eq!(trader.fill_count(), 1);
    }

    #[test]
    fn test_check_fills_limit_sell() {
        let mut trader = create_paper_trader();
        // Open a long first
        trader.place_market_order("BTC", true, 0.1, 50000.0);

        // Place a sell limit above current price
        trader.place_order("BTC", false, 51000.0, 0.1, false);

        let mut prices = HashMap::new();
        prices.insert("BTC".to_string(), 51500.0);
        trader.check_fills(&prices);

        // Should have 2 fills: market buy + limit sell
        assert_eq!(trader.fill_count(), 2);
        assert!(trader.get_positions().is_empty()); // Position should be closed
    }

    #[test]
    fn test_close_long_pnl_positive() {
        let mut trader = create_paper_trader();
        trader.place_market_order("BTC", true, 0.1, 50000.0);
        let initial_balance = trader.balance();

        trader.place_market_order("BTC", false, 0.1, 51000.0); // +$100 PnL minus fees

        // PnL should be ~$100 minus fees (~$1.75 per leg * 2 = ~$3.54)
        let pnl = trader.balance() - initial_balance;
        assert!(pnl > 95.0 && pnl < 101.0, "pnl was: {}", pnl);
        assert!(trader.get_positions().is_empty());
    }

    #[test]
    fn test_close_long_pnl_negative() {
        let mut trader = create_paper_trader();
        trader.place_market_order("BTC", true, 0.1, 50000.0);
        let initial_balance = trader.balance();

        trader.place_market_order("BTC", false, 0.1, 49000.0); // -$100 PnL minus fees

        let pnl = trader.balance() - initial_balance;
        assert!(pnl < -99.0 && pnl > -105.0, "pnl was: {}", pnl);
    }

    #[test]
    fn test_close_short_pnl_positive() {
        let mut trader = create_paper_trader();
        trader.place_market_order("ETH", false, 1.0, 3000.0); // Short
        let initial_balance = trader.balance();

        trader.place_market_order("ETH", true, 1.0, 2800.0); // Close short at lower price

        let pnl = trader.balance() - initial_balance;
        assert!(pnl > 199.0 && pnl < 201.0, "pnl was: {}", pnl);
    }

    #[test]
    fn test_add_to_position() {
        let mut trader = create_paper_trader();
        trader.place_market_order("BTC", true, 0.1, 50000.0);
        trader.place_market_order("BTC", true, 0.1, 52000.0);

        let positions = trader.get_positions();
        assert_eq!(positions.len(), 1);
        assert!((positions[0].size - 0.2).abs() < 1e-10);
        // Weighted average: (50000*0.1 + 52000*0.1) / 0.2 = 51000
        assert!((positions[0].entry_price - 51000.0).abs() < 1.0);
    }

    #[test]
    fn test_update_unrealized() {
        let mut trader = create_paper_trader();
        trader.place_market_order("BTC", true, 0.1, 50000.0);

        let mut prices = HashMap::new();
        prices.insert("BTC".to_string(), 51000.0);
        trader.update_unrealized(&prices);

        let positions = trader.get_positions();
        assert_eq!(positions.len(), 1);
        assert!((positions[0].unrealized_pnl - 100.0).abs() < 1.0);
    }

    #[test]
    fn test_total_pnl_with_unrealized() {
        let mut trader = create_paper_trader();
        trader.place_market_order("BTC", true, 0.1, 50000.0);

        let mut prices = HashMap::new();
        prices.insert("BTC".to_string(), 51000.0);
        trader.update_unrealized(&prices);

        // Total PnL = balance_change + unrealized
        let total = trader.total_pnl();
        assert!(total > 98.0 && total < 102.0, "total pnl was: {}", total);
    }

    #[test]
    fn test_multiple_positions() {
        let mut trader = create_paper_trader();
        trader.place_market_order("BTC", true, 0.1, 50000.0);
        trader.place_market_order("ETH", false, 1.0, 3000.0);

        assert_eq!(trader.get_positions().len(), 2);
    }

    #[test]
    fn test_fill_logging() {
        let dir = tempfile::tempdir().unwrap();
        let log_path = dir.path().join("test_fills.jsonl");
        let mut trader = PaperTrader::new(10000.0, log_path.to_str().unwrap().to_string());

        trader.place_market_order("BTC", true, 0.1, 50000.0);
        trader.place_market_order("BTC", false, 0.1, 51000.0);

        // Verify the log file was created and has content
        let content = std::fs::read_to_string(&log_path).unwrap();
        let lines: Vec<&str> = content.trim().lines().collect();
        assert_eq!(lines.len(), 2);

        // Parse first line to verify it's valid JSON
        let fill: PaperFill = serde_json::from_str(lines[0]).unwrap();
        assert_eq!(fill.coin, "BTC");
        assert!(fill.is_buy);
    }

    #[test]
    fn test_paper_order_status_serialization() {
        let open = PaperOrderStatus::Open;
        let json = serde_json::to_string(&open).unwrap();
        let deserialized: PaperOrderStatus = serde_json::from_str(&json).unwrap();
        assert_eq!(deserialized, PaperOrderStatus::Open);
    }

    #[test]
    fn test_order_ids_increment() {
        let mut trader = create_paper_trader();
        let oid1 = trader.place_order("BTC", true, 50000.0, 0.1, false);
        let oid2 = trader.place_order("ETH", true, 3000.0, 1.0, false);
        let oid3 = trader.place_order("SOL", true, 100.0, 10.0, false);

        assert_eq!(oid1, 1);
        assert_eq!(oid2, 2);
        assert_eq!(oid3, 3);
    }

    #[test]
    fn test_paper_fill_serialization() {
        let fill = PaperFill {
            timestamp: 1700000000,
            coin: "BTC".to_string(),
            is_buy: true,
            price: 50000.0,
            size: 0.1,
            pnl: 0.0,
            fee: 1.75,
        };

        let json = serde_json::to_string(&fill).unwrap();
        let deserialized: PaperFill = serde_json::from_str(&json).unwrap();
        assert_eq!(deserialized.coin, "BTC");
        assert_eq!(deserialized.fee, 1.75);
    }

    #[test]
    fn test_paper_position_serialization() {
        let pos = PaperPosition {
            coin: "ETH".to_string(),
            size: -2.5,
            entry_price: 3000.0,
            notional: 7500.0,
            unrealized_pnl: -100.0,
        };

        let json = serde_json::to_string(&pos).unwrap();
        let deserialized: PaperPosition = serde_json::from_str(&json).unwrap();
        assert_eq!(deserialized.size, -2.5);
        assert_eq!(deserialized.unrealized_pnl, -100.0);
    }

    #[test]
    fn test_fees_deducted() {
        let mut trader = create_paper_trader();
        let initial = trader.balance();

        trader.place_market_order("BTC", true, 0.1, 50000.0);

        // Fee = 50000 * 0.1 * 0.00035 = 1.75
        let expected_balance = initial - 1.75;
        assert!(
            (trader.balance() - expected_balance).abs() < 0.01,
            "balance: {}, expected: {}",
            trader.balance(),
            expected_balance
        );
    }
}
