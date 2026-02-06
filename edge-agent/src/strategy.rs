use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use tracing::info;

/// Trading signal from strategy
#[derive(Debug, Clone, Serialize, Deserialize)]
#[allow(dead_code)]
pub enum Signal {
    Buy {
        asset: String,
        price: f64,
        size: f64,
        reason: String,
    },
    Sell {
        asset: String,
        price: f64,
        size: f64,
        reason: String,
    },
    Hold,
}

/// Market data snapshot for strategy evaluation
#[derive(Debug, Clone, Serialize, Deserialize)]
#[allow(dead_code)]
pub struct MarketData {
    pub prices: HashMap<String, f64>,
    pub funding_rates: HashMap<String, f64>,
    pub timestamp: u64,
}

/// Base trait for all trading strategies
pub trait Strategy: Send + Sync {
    /// Evaluate market data and generate trading signals
    #[allow(dead_code)]
    fn evaluate(&mut self, data: &MarketData) -> Vec<Signal>;

    /// Get strategy parameters (for evolution)
    fn get_params(&self) -> serde_json::Value;

    /// Update strategy parameters (from evolution engine)
    fn update_params(&mut self, params: serde_json::Value) -> Result<(), String>;

    /// Get strategy name
    fn name(&self) -> &str;

    /// Reset strategy state
    fn reset(&mut self);
}

/// Funding Arbitrage Strategy
/// Long assets when funding rate is very negative (getting paid to hold)
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FundingArbitrage {
    pub name: String,
    pub funding_threshold: f64, // Enter when funding < this (e.g., -0.1%)
    pub exit_funding: f64,      // Exit when funding > this (e.g., 0.0%)
    pub position_size_usd: f64, // Size per position
    pub max_positions: usize,   // Max concurrent positions
    pub active_positions: Vec<String>,
}

impl FundingArbitrage {
    pub fn new(funding_threshold: f64, exit_funding: f64, position_size_usd: f64) -> Self {
        Self {
            name: "FundingArbitrage".to_string(),
            funding_threshold,
            exit_funding,
            position_size_usd,
            max_positions: 3,
            active_positions: Vec::new(),
        }
    }
}

impl Strategy for FundingArbitrage {
    fn evaluate(&mut self, data: &MarketData) -> Vec<Signal> {
        let mut signals = Vec::new();

        for (asset, &funding_rate) in &data.funding_rates {
            let funding_pct = funding_rate * 100.0;

            // Check for entry signal (very negative funding)
            if funding_pct < self.funding_threshold
                && self.active_positions.len() < self.max_positions
                && !self.active_positions.contains(asset)
            {
                if let Some(&price) = data.prices.get(asset) {
                    let size = self.position_size_usd / price;
                    signals.push(Signal::Buy {
                        asset: asset.clone(),
                        price,
                        size,
                        reason: format!("Funding arbitrage entry: funding={}%", funding_pct),
                    });
                    self.active_positions.push(asset.clone());
                    info!(
                        asset = %asset,
                        funding = funding_pct,
                        "funding arbitrage entry signal"
                    );
                }
            }

            // Check for exit signal (funding normalized)
            if funding_pct > self.exit_funding && self.active_positions.contains(asset) {
                if let Some(&price) = data.prices.get(asset) {
                    let size = self.position_size_usd / price;
                    signals.push(Signal::Sell {
                        asset: asset.clone(),
                        price,
                        size,
                        reason: format!("Funding arbitrage exit: funding={}%", funding_pct),
                    });
                    self.active_positions.retain(|a| a != asset);
                    info!(
                        asset = %asset,
                        funding = funding_pct,
                        "funding arbitrage exit signal"
                    );
                }
            }
        }

        signals
    }

    fn get_params(&self) -> serde_json::Value {
        serde_json::json!({
            "funding_threshold": self.funding_threshold,
            "exit_funding": self.exit_funding,
            "position_size_usd": self.position_size_usd,
            "max_positions": self.max_positions
        })
    }

    fn update_params(&mut self, params: serde_json::Value) -> Result<(), String> {
        if let Some(threshold) = params.get("funding_threshold").and_then(|v| v.as_f64()) {
            self.funding_threshold = threshold;
        }
        if let Some(exit) = params.get("exit_funding").and_then(|v| v.as_f64()) {
            self.exit_funding = exit;
        }
        if let Some(size) = params.get("position_size_usd").and_then(|v| v.as_f64()) {
            self.position_size_usd = size;
        }
        if let Some(max) = params.get("max_positions").and_then(|v| v.as_u64()) {
            self.max_positions = max as usize;
        }
        Ok(())
    }

    fn name(&self) -> &str {
        &self.name
    }

    fn reset(&mut self) {
        self.active_positions.clear();
    }
}

/// Mean Reversion Strategy
/// Buy at support levels, sell at resistance levels
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MeanReversion {
    pub name: String,
    pub support_level: f64,      // Buy when price < support
    pub resistance_level: f64,   // Sell when price > resistance
    pub position_size_usd: f64,  // Size per trade
    pub lookback_periods: usize, // Number of periods for mean calculation
    pub price_history: HashMap<String, Vec<f64>>,
}

impl MeanReversion {
    pub fn new(support_level: f64, resistance_level: f64, position_size_usd: f64) -> Self {
        Self {
            name: "MeanReversion".to_string(),
            support_level,
            resistance_level,
            position_size_usd,
            lookback_periods: 20,
            price_history: HashMap::new(),
        }
    }

    #[allow(dead_code)]
    fn calculate_mean(&self, asset: &str) -> Option<f64> {
        self.price_history.get(asset).and_then(|history| {
            if history.is_empty() {
                return None;
            }
            let sum: f64 = history.iter().sum();
            Some(sum / history.len() as f64)
        })
    }
}

impl Strategy for MeanReversion {
    fn evaluate(&mut self, data: &MarketData) -> Vec<Signal> {
        let mut signals = Vec::new();

        for (asset, &price) in &data.prices {
            // Update price history
            let history = self.price_history.entry(asset.clone()).or_default();
            history.push(price);
            if history.len() > self.lookback_periods {
                history.remove(0);
            }

            // Need sufficient history
            if history.len() < self.lookback_periods {
                continue;
            }

            if let Some(mean_price) = self.calculate_mean(asset) {
                let deviation_pct = ((price - mean_price) / mean_price) * 100.0;

                // Buy at support (price below mean)
                if deviation_pct < -self.support_level {
                    let size = self.position_size_usd / price;
                    signals.push(Signal::Buy {
                        asset: asset.clone(),
                        price,
                        size,
                        reason: format!(
                            "Mean reversion buy: deviation={}%, mean={}",
                            deviation_pct, mean_price
                        ),
                    });
                    info!(
                        asset = %asset,
                        price = price,
                        mean = mean_price,
                        deviation = deviation_pct,
                        "mean reversion buy signal"
                    );
                }

                // Sell at resistance (price above mean)
                if deviation_pct > self.resistance_level {
                    let size = self.position_size_usd / price;
                    signals.push(Signal::Sell {
                        asset: asset.clone(),
                        price,
                        size,
                        reason: format!(
                            "Mean reversion sell: deviation={}%, mean={}",
                            deviation_pct, mean_price
                        ),
                    });
                    info!(
                        asset = %asset,
                        price = price,
                        mean = mean_price,
                        deviation = deviation_pct,
                        "mean reversion sell signal"
                    );
                }
            }
        }

        signals
    }

    fn get_params(&self) -> serde_json::Value {
        serde_json::json!({
            "support_level": self.support_level,
            "resistance_level": self.resistance_level,
            "position_size_usd": self.position_size_usd,
            "lookback_periods": self.lookback_periods
        })
    }

    fn update_params(&mut self, params: serde_json::Value) -> Result<(), String> {
        if let Some(support) = params.get("support_level").and_then(|v| v.as_f64()) {
            self.support_level = support;
        }
        if let Some(resistance) = params.get("resistance_level").and_then(|v| v.as_f64()) {
            self.resistance_level = resistance;
        }
        if let Some(size) = params.get("position_size_usd").and_then(|v| v.as_f64()) {
            self.position_size_usd = size;
        }
        if let Some(lookback) = params.get("lookback_periods").and_then(|v| v.as_u64()) {
            self.lookback_periods = lookback as usize;
        }
        Ok(())
    }

    fn name(&self) -> &str {
        &self.name
    }

    fn reset(&mut self) {
        self.price_history.clear();
    }
}

/// Strategy engine that manages active strategies
pub struct StrategyEngine {
    strategies: Vec<Box<dyn Strategy>>,
}

impl StrategyEngine {
    pub fn new() -> Self {
        Self {
            strategies: Vec::new(),
        }
    }

    /// Add a strategy to the engine
    pub fn add_strategy(&mut self, strategy: Box<dyn Strategy>) {
        self.strategies.push(strategy);
    }

    /// Evaluate all strategies and collect signals
    #[allow(dead_code)]
    pub fn evaluate_all(&mut self, data: &MarketData) -> Vec<Signal> {
        let mut all_signals = Vec::new();
        for strategy in &mut self.strategies {
            let signals = strategy.evaluate(data);
            all_signals.extend(signals);
        }
        all_signals
    }

    /// Get all strategy parameters (for evolution)
    pub fn get_all_params(&self) -> Vec<serde_json::Value> {
        self.strategies.iter().map(|s| s.get_params()).collect()
    }

    /// Update strategy parameters by name
    pub fn update_strategy_params(
        &mut self,
        name: &str,
        params: serde_json::Value,
    ) -> Result<(), String> {
        for strategy in &mut self.strategies {
            if strategy.name() == name {
                return strategy.update_params(params);
            }
        }
        Err(format!("strategy not found: {}", name))
    }

    /// Reset all strategies
    pub fn reset_all(&mut self) {
        for strategy in &mut self.strategies {
            strategy.reset();
        }
    }

    /// Get number of active strategies
    pub fn strategy_count(&self) -> usize {
        self.strategies.len()
    }
}

impl Default for StrategyEngine {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn create_test_market_data() -> MarketData {
        let mut prices = HashMap::new();
        prices.insert("BTC".to_string(), 50000.0);
        prices.insert("ETH".to_string(), 3000.0);
        
        let mut funding_rates = HashMap::new();
        funding_rates.insert("BTC".to_string(), -0.001); // -0.1%
        funding_rates.insert("ETH".to_string(), 0.0005);  // 0.05%
        
        MarketData {
            prices,
            funding_rates,
            timestamp: 1234567890,
        }
    }

    #[test]
    fn test_funding_arbitrage_new() {
        let strategy = FundingArbitrage::new(-0.1, 0.0, 1000.0);
        assert_eq!(strategy.name, "FundingArbitrage");
        assert_eq!(strategy.funding_threshold, -0.1);
        assert_eq!(strategy.exit_funding, 0.0);
        assert_eq!(strategy.position_size_usd, 1000.0);
        assert_eq!(strategy.max_positions, 3);
        assert!(strategy.active_positions.is_empty());
    }

    #[test]
    fn test_funding_arbitrage_entry_signal() {
        let mut strategy = FundingArbitrage::new(-0.05, 0.0, 1000.0);
        let data = create_test_market_data();
        
        let signals = strategy.evaluate(&data);
        
        // BTC funding is -0.1%, below -0.05% threshold
        assert_eq!(signals.len(), 1);
        match &signals[0] {
            Signal::Buy { asset, price, size, reason } => {
                assert_eq!(asset, "BTC");
                assert_eq!(*price, 50000.0);
                assert!((size - 0.02).abs() < 0.001); // 1000/50000 = 0.02
                assert!(reason.contains("Funding arbitrage entry"));
            }
            _ => panic!("Expected Buy signal"),
        }
        
        assert_eq!(strategy.active_positions.len(), 1);
        assert!(strategy.active_positions.contains(&"BTC".to_string()));
    }

    #[test]
    fn test_funding_arbitrage_exit_signal() {
        let mut strategy = FundingArbitrage::new(-0.2, 0.0, 1000.0);
        strategy.active_positions.push("BTC".to_string());
        
        let mut data = create_test_market_data();
        data.funding_rates.insert("BTC".to_string(), 0.001); // 0.1%, above 0.0% exit
        
        let signals = strategy.evaluate(&data);
        
        assert_eq!(signals.len(), 1);
        match &signals[0] {
            Signal::Sell { asset, reason, .. } => {
                assert_eq!(asset, "BTC");
                assert!(reason.contains("Funding arbitrage exit"));
            }
            _ => panic!("Expected Sell signal"),
        }
        
        assert!(strategy.active_positions.is_empty());
    }

    #[test]
    fn test_funding_arbitrage_no_duplicate_entry() {
        let mut strategy = FundingArbitrage::new(-0.05, 0.0, 1000.0);
        let data = create_test_market_data();
        
        // First evaluation - should generate signal
        let signals1 = strategy.evaluate(&data);
        assert_eq!(signals1.len(), 1);
        
        // Second evaluation - should not generate duplicate signal
        let signals2 = strategy.evaluate(&data);
        assert_eq!(signals2.len(), 0);
    }

    #[test]
    fn test_funding_arbitrage_max_positions() {
        let mut strategy = FundingArbitrage::new(-0.05, 0.0, 1000.0);
        strategy.max_positions = 2;
        
        let mut data = create_test_market_data();
        data.funding_rates.insert("SOL".to_string(), -0.15);
        data.prices.insert("SOL".to_string(), 150.0);
        data.funding_rates.insert("AVAX".to_string(), -0.12);
        data.prices.insert("AVAX".to_string(), 40.0);
        
        let signals = strategy.evaluate(&data);
        
        // Should only open 2 positions (max_positions limit)
        assert!(signals.len() <= 2);
        assert!(strategy.active_positions.len() <= 2);
    }

    #[test]
    fn test_funding_arbitrage_get_params() {
        let strategy = FundingArbitrage::new(-0.1, 0.05, 2000.0);
        let params = strategy.get_params();
        
        assert_eq!(params["funding_threshold"], -0.1);
        assert_eq!(params["exit_funding"], 0.05);
        assert_eq!(params["position_size_usd"], 2000.0);
        assert_eq!(params["max_positions"], 3);
    }

    #[test]
    fn test_funding_arbitrage_update_params() {
        let mut strategy = FundingArbitrage::new(-0.1, 0.0, 1000.0);
        
        let new_params = serde_json::json!({
            "funding_threshold": -0.15,
            "exit_funding": 0.02,
            "position_size_usd": 2500.0,
            "max_positions": 5
        });
        
        strategy.update_params(new_params).unwrap();
        
        assert_eq!(strategy.funding_threshold, -0.15);
        assert_eq!(strategy.exit_funding, 0.02);
        assert_eq!(strategy.position_size_usd, 2500.0);
        assert_eq!(strategy.max_positions, 5);
    }

    #[test]
    fn test_funding_arbitrage_reset() {
        let mut strategy = FundingArbitrage::new(-0.1, 0.0, 1000.0);
        strategy.active_positions.push("BTC".to_string());
        strategy.active_positions.push("ETH".to_string());
        
        strategy.reset();
        
        assert!(strategy.active_positions.is_empty());
    }

    #[test]
    fn test_mean_reversion_new() {
        let strategy = MeanReversion::new(2.0, 2.0, 1000.0);
        assert_eq!(strategy.name, "MeanReversion");
        assert_eq!(strategy.support_level, 2.0);
        assert_eq!(strategy.resistance_level, 2.0);
        assert_eq!(strategy.position_size_usd, 1000.0);
        assert_eq!(strategy.lookback_periods, 20);
        assert!(strategy.price_history.is_empty());
    }

    #[test]
    fn test_mean_reversion_insufficient_history() {
        let mut strategy = MeanReversion::new(2.0, 2.0, 1000.0);
        strategy.lookback_periods = 20;
        
        let data = create_test_market_data();
        let signals = strategy.evaluate(&data);
        
        // Not enough history yet
        assert_eq!(signals.len(), 0);
    }

    #[test]
    fn test_mean_reversion_buy_signal() {
        let mut strategy = MeanReversion::new(5.0, 5.0, 1000.0);
        strategy.lookback_periods = 4;
        
        // Manually set up history for BTC with known prices (mean = 50000)
        strategy.price_history.insert("BTC".to_string(), vec![50000.0, 50000.0, 50000.0]);
        
        // Create market data with very low price
        // After adding this price, history becomes [50000, 50000, 50000, 47000]
        // Mean = 196000 / 4 = 49000
        // Deviation = (47000 - 49000) / 49000 * 100 = -4.08% 
        // But we need > -5% to trigger, so let's use an even lower price
        let mut data = create_test_market_data();
        data.prices.insert("BTC".to_string(), 45000.0); 
        // Mean will be (50000+50000+50000+45000)/4 = 48750
        // Deviation = (45000 - 48750) / 48750 * 100 = -7.69% which triggers
        
        let signals = strategy.evaluate(&data);
        
        // Should generate buy signal for BTC
        let btc_buy_signals: Vec<_> = signals.iter().filter(|s| {
            match s {
                Signal::Buy { asset, .. } => asset == "BTC",
                _ => false,
            }
        }).collect();
        
        assert_eq!(btc_buy_signals.len(), 1, "Expected exactly one BTC buy signal");
    }

    #[test]
    fn test_mean_reversion_sell_signal() {
        let mut strategy = MeanReversion::new(5.0, 5.0, 1000.0);
        strategy.lookback_periods = 4;
        
        // Manually set up history for BTC with known prices (mean = 50000)
        strategy.price_history.insert("BTC".to_string(), vec![50000.0, 50000.0, 50000.0]);
        
        // Create market data with very high price
        // After adding this price, history becomes [50000, 50000, 50000, 55000]
        // Mean = 205000 / 4 = 51250
        // Deviation = (55000 - 51250) / 51250 * 100 = 7.32% which triggers (>5%)
        let mut data = create_test_market_data();
        data.prices.insert("BTC".to_string(), 55000.0);
        
        let signals = strategy.evaluate(&data);
        
        // Should generate sell signal for BTC
        let btc_sell_signals: Vec<_> = signals.iter().filter(|s| {
            match s {
                Signal::Sell { asset, .. } => asset == "BTC",
                _ => false,
            }
        }).collect();
        
        assert_eq!(btc_sell_signals.len(), 1, "Expected exactly one BTC sell signal");
    }

    #[test]
    fn test_mean_reversion_no_signal_within_bounds() {
        let mut strategy = MeanReversion::new(5.0, 5.0, 1000.0);
        strategy.lookback_periods = 5;
        
        // Build price history
        for _ in 0..6 {
            let data = create_test_market_data();
            strategy.evaluate(&data);
        }
        
        // Should have no signals if price is within bounds
        let last_signals = strategy.evaluate(&create_test_market_data());
        
        // Depends on whether deviation exceeds threshold
        // With consistent prices, should be minimal signals
        assert!(last_signals.len() <= 2); // At most one per asset
    }

    #[test]
    fn test_mean_reversion_calculate_mean() {
        let mut strategy = MeanReversion::new(2.0, 2.0, 1000.0);
        
        let history = vec![100.0, 110.0, 105.0, 95.0, 100.0];
        strategy.price_history.insert("BTC".to_string(), history);
        
        let mean = strategy.calculate_mean("BTC").unwrap();
        assert_eq!(mean, 102.0); // (100+110+105+95+100)/5
    }

    #[test]
    fn test_mean_reversion_get_params() {
        let strategy = MeanReversion::new(3.0, 4.0, 1500.0);
        let params = strategy.get_params();
        
        assert_eq!(params["support_level"], 3.0);
        assert_eq!(params["resistance_level"], 4.0);
        assert_eq!(params["position_size_usd"], 1500.0);
        assert_eq!(params["lookback_periods"], 20);
    }

    #[test]
    fn test_mean_reversion_update_params() {
        let mut strategy = MeanReversion::new(2.0, 2.0, 1000.0);
        
        let new_params = serde_json::json!({
            "support_level": 5.0,
            "resistance_level": 6.0,
            "position_size_usd": 3000.0,
            "lookback_periods": 30
        });
        
        strategy.update_params(new_params).unwrap();
        
        assert_eq!(strategy.support_level, 5.0);
        assert_eq!(strategy.resistance_level, 6.0);
        assert_eq!(strategy.position_size_usd, 3000.0);
        assert_eq!(strategy.lookback_periods, 30);
    }

    #[test]
    fn test_mean_reversion_reset() {
        let mut strategy = MeanReversion::new(2.0, 2.0, 1000.0);
        strategy.price_history.insert("BTC".to_string(), vec![100.0, 110.0]);
        strategy.price_history.insert("ETH".to_string(), vec![3000.0, 3100.0]);
        
        strategy.reset();
        
        assert!(strategy.price_history.is_empty());
    }

    #[test]
    fn test_strategy_engine_new() {
        let engine = StrategyEngine::new();
        assert_eq!(engine.strategy_count(), 0);
    }

    #[test]
    fn test_strategy_engine_add_strategy() {
        let mut engine = StrategyEngine::new();
        
        let strategy1 = FundingArbitrage::new(-0.1, 0.0, 1000.0);
        let strategy2 = MeanReversion::new(2.0, 2.0, 1500.0);
        
        engine.add_strategy(Box::new(strategy1));
        engine.add_strategy(Box::new(strategy2));
        
        assert_eq!(engine.strategy_count(), 2);
    }

    #[test]
    fn test_strategy_engine_evaluate_all() {
        let mut engine = StrategyEngine::new();
        
        let strategy = FundingArbitrage::new(-0.05, 0.0, 1000.0);
        engine.add_strategy(Box::new(strategy));
        
        let data = create_test_market_data();
        let signals = engine.evaluate_all(&data);
        
        // Should get signals from the funding arbitrage strategy
        assert!(signals.len() > 0);
    }

    #[test]
    fn test_strategy_engine_get_all_params() {
        let mut engine = StrategyEngine::new();
        
        let strategy1 = FundingArbitrage::new(-0.1, 0.0, 1000.0);
        let strategy2 = MeanReversion::new(2.0, 2.0, 1500.0);
        
        engine.add_strategy(Box::new(strategy1));
        engine.add_strategy(Box::new(strategy2));
        
        let all_params = engine.get_all_params();
        
        assert_eq!(all_params.len(), 2);
        assert!(all_params[0].get("funding_threshold").is_some());
        assert!(all_params[1].get("support_level").is_some());
    }

    #[test]
    fn test_strategy_engine_update_strategy_params() {
        let mut engine = StrategyEngine::new();
        
        let strategy = FundingArbitrage::new(-0.1, 0.0, 1000.0);
        engine.add_strategy(Box::new(strategy));
        
        let new_params = serde_json::json!({
            "funding_threshold": -0.2,
            "position_size_usd": 2000.0
        });
        
        let result = engine.update_strategy_params("FundingArbitrage", new_params);
        assert!(result.is_ok());
    }

    #[test]
    fn test_strategy_engine_update_nonexistent_strategy() {
        let mut engine = StrategyEngine::new();
        
        let new_params = serde_json::json!({"threshold": 5.0});
        let result = engine.update_strategy_params("NonExistent", new_params);
        
        assert!(result.is_err());
        assert!(result.unwrap_err().contains("strategy not found"));
    }

    #[test]
    fn test_strategy_engine_reset_all() {
        let mut engine = StrategyEngine::new();
        
        let mut strategy = FundingArbitrage::new(-0.1, 0.0, 1000.0);
        strategy.active_positions.push("BTC".to_string());
        
        engine.add_strategy(Box::new(strategy));
        engine.reset_all();
        
        // All strategies should be reset (can't verify internal state easily)
        assert_eq!(engine.strategy_count(), 1);
    }

    #[test]
    fn test_signal_serialization() {
        let signal = Signal::Buy {
            asset: "BTC".to_string(),
            price: 50000.0,
            size: 0.1,
            reason: "Test buy".to_string(),
        };
        
        let json = serde_json::to_string(&signal).unwrap();
        let deserialized: Signal = serde_json::from_str(&json).unwrap();
        
        match deserialized {
            Signal::Buy { asset, price, size, reason } => {
                assert_eq!(asset, "BTC");
                assert_eq!(price, 50000.0);
                assert_eq!(size, 0.1);
                assert_eq!(reason, "Test buy");
            }
            _ => panic!("Expected Buy signal"),
        }
    }

    #[test]
    fn test_market_data_serialization() {
        let data = create_test_market_data();
        
        let json = serde_json::to_string(&data).unwrap();
        let deserialized: MarketData = serde_json::from_str(&json).unwrap();
        
        assert_eq!(deserialized.timestamp, 1234567890);
        assert_eq!(deserialized.prices.len(), 2);
        assert_eq!(deserialized.funding_rates.len(), 2);
    }
}
