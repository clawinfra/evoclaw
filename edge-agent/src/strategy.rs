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
