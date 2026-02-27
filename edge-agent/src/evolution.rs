#![allow(dead_code)]

use serde::{Deserialize, Serialize};
use std::collections::VecDeque;
use tracing::{info, warn};

/// Performance metrics for evolution engine
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PerformanceMetrics {
    pub win_rate: f64,
    pub total_pnl: f64,
    pub sharpe_ratio: f64,
    pub max_drawdown: f64,
    pub total_trades: u64,
    pub avg_profit_per_trade: f64,
}

/// Trade record for performance tracking
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TradeRecord {
    pub timestamp: u64,
    pub asset: String,
    pub entry_price: f64,
    pub exit_price: f64,
    pub size: f64,
    pub pnl: f64,
}

/// Evolution tracker for strategy performance
pub struct EvolutionTracker {
    trade_history: VecDeque<TradeRecord>,
    max_history_size: usize,
    current_drawdown: f64,
    peak_equity: f64,
    max_drawdown: f64,
    returns: Vec<f64>,
}

impl EvolutionTracker {
    pub fn new(max_history_size: usize) -> Self {
        Self {
            trade_history: VecDeque::new(),
            max_history_size,
            current_drawdown: 0.0,
            peak_equity: 0.0,
            max_drawdown: 0.0,
            returns: Vec::new(),
        }
    }

    /// Record a completed trade
    pub fn record_trade(&mut self, trade: TradeRecord) {
        info!(
            asset = %trade.asset,
            pnl = trade.pnl,
            "recording trade for evolution"
        );

        self.trade_history.push_back(trade.clone());
        if self.trade_history.len() > self.max_history_size {
            self.trade_history.pop_front();
        }

        // Update drawdown tracking
        let total_pnl = self.calculate_total_pnl();
        if total_pnl > self.peak_equity {
            self.peak_equity = total_pnl;
            self.current_drawdown = 0.0;
        } else {
            self.current_drawdown = self.peak_equity - total_pnl;
            if self.current_drawdown > self.max_drawdown {
                self.max_drawdown = self.current_drawdown;
            }
        }

        // Calculate return for Sharpe ratio
        if let Some(last_trade) = self.trade_history.iter().rev().nth(1) {
            let return_pct = (trade.pnl / last_trade.pnl.abs().max(1.0)) * 100.0;
            self.returns.push(return_pct);
        }
    }

    /// Calculate total P&L from trade history
    fn calculate_total_pnl(&self) -> f64 {
        self.trade_history.iter().map(|t| t.pnl).sum()
    }

    /// Calculate win rate
    fn calculate_win_rate(&self) -> f64 {
        if self.trade_history.is_empty() {
            return 0.0;
        }
        let winning_trades = self.trade_history.iter().filter(|t| t.pnl > 0.0).count();
        (winning_trades as f64 / self.trade_history.len() as f64) * 100.0
    }

    /// Calculate Sharpe ratio (simplified, assumes risk-free rate = 0)
    fn calculate_sharpe_ratio(&self) -> f64 {
        if self.returns.len() < 2 {
            return 0.0;
        }

        let mean_return: f64 = self.returns.iter().sum::<f64>() / self.returns.len() as f64;
        let variance: f64 = self
            .returns
            .iter()
            .map(|r| (r - mean_return).powi(2))
            .sum::<f64>()
            / (self.returns.len() - 1) as f64;
        let std_dev = variance.sqrt();

        if std_dev == 0.0 {
            return 0.0;
        }

        mean_return / std_dev
    }

    /// Get current performance metrics
    pub fn get_metrics(&self) -> PerformanceMetrics {
        let total_pnl = self.calculate_total_pnl();
        let win_rate = self.calculate_win_rate();
        let sharpe = self.calculate_sharpe_ratio();
        let total_trades = self.trade_history.len() as u64;
        let avg_profit = if total_trades > 0 {
            total_pnl / total_trades as f64
        } else {
            0.0
        };

        PerformanceMetrics {
            win_rate,
            total_pnl,
            sharpe_ratio: sharpe,
            max_drawdown: self.max_drawdown,
            total_trades,
            avg_profit_per_trade: avg_profit,
        }
    }

    /// Calculate fitness score for evolution engine
    pub fn fitness_score(&self) -> f64 {
        let metrics = self.get_metrics();

        // Weighted fitness function:
        // - 40% Sharpe ratio (risk-adjusted returns)
        // - 30% Win rate
        // - 20% Total P&L (normalized)
        // - 10% Drawdown penalty

        let sharpe_score = (metrics.sharpe_ratio.max(0.0) / 3.0).min(1.0) * 40.0;
        let win_rate_score = (metrics.win_rate / 100.0) * 30.0;
        let pnl_score = (metrics.total_pnl / 10000.0).clamp(0.0, 1.0) * 20.0;
        let drawdown_penalty = (1.0 - (metrics.max_drawdown / 5000.0).min(1.0)) * 10.0;

        let total_score = sharpe_score + win_rate_score + pnl_score + drawdown_penalty;

        info!(
            sharpe_score = sharpe_score,
            win_rate_score = win_rate_score,
            pnl_score = pnl_score,
            drawdown_penalty = drawdown_penalty,
            total_fitness = total_score,
            "calculated fitness score"
        );

        total_score
    }

    /// Reset all tracking data (for strategy hot-swap)
    pub fn reset(&mut self) {
        warn!("resetting evolution tracker");
        self.trade_history.clear();
        self.current_drawdown = 0.0;
        self.peak_equity = 0.0;
        self.max_drawdown = 0.0;
        self.returns.clear();
    }

    /// Get trade history
    pub fn get_trade_history(&self) -> Vec<TradeRecord> {
        self.trade_history.iter().cloned().collect()
    }

    /// Get number of trades tracked
    #[allow(dead_code)]
    pub fn trade_count(&self) -> usize {
        self.trade_history.len()
    }
}

impl Default for EvolutionTracker {
    fn default() -> Self {
        Self::new(1000) // Track last 1000 trades by default
    }
}

/// Strategy update from evolution engine
#[derive(Debug, Clone, Serialize, Deserialize)]
#[allow(dead_code)]
pub struct StrategyUpdate {
    pub strategy_name: String,
    pub new_params: serde_json::Value,
    pub generation: u64,
    pub reason: String,
}

impl StrategyUpdate {
    #[allow(dead_code)]
    pub fn new(
        strategy_name: String,
        new_params: serde_json::Value,
        generation: u64,
        reason: String,
    ) -> Self {
        Self {
            strategy_name,
            new_params,
            generation,
            reason,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn create_test_trade(pnl: f64) -> TradeRecord {
        TradeRecord {
            timestamp: 1234567890,
            asset: "BTC".to_string(),
            entry_price: 50000.0,
            exit_price: 50000.0 + pnl,
            size: 1.0,
            pnl,
        }
    }

    #[test]
    fn test_evolution_tracker_new() {
        let tracker = EvolutionTracker::new(100);
        assert_eq!(tracker.max_history_size, 100);
        assert_eq!(tracker.trade_history.len(), 0);
        assert_eq!(tracker.current_drawdown, 0.0);
        assert_eq!(tracker.peak_equity, 0.0);
        assert_eq!(tracker.max_drawdown, 0.0);
    }

    #[test]
    fn test_evolution_tracker_default() {
        let tracker = EvolutionTracker::default();
        assert_eq!(tracker.max_history_size, 1000);
    }

    #[test]
    fn test_record_trade_single() {
        let mut tracker = EvolutionTracker::new(100);
        let trade = create_test_trade(100.0);

        tracker.record_trade(trade.clone());

        assert_eq!(tracker.trade_history.len(), 1);
        assert_eq!(tracker.trade_history[0].pnl, 100.0);
        assert_eq!(tracker.peak_equity, 100.0);
        assert_eq!(tracker.current_drawdown, 0.0);
    }

    #[test]
    fn test_record_trade_multiple() {
        let mut tracker = EvolutionTracker::new(100);

        tracker.record_trade(create_test_trade(100.0));
        tracker.record_trade(create_test_trade(50.0));
        tracker.record_trade(create_test_trade(-30.0));

        assert_eq!(tracker.trade_history.len(), 3);
        let total_pnl: f64 = tracker.trade_history.iter().map(|t| t.pnl).sum();
        assert_eq!(total_pnl, 120.0);
    }

    #[test]
    fn test_record_trade_max_history() {
        let mut tracker = EvolutionTracker::new(5);

        for i in 0..10 {
            tracker.record_trade(create_test_trade(i as f64));
        }

        assert_eq!(tracker.trade_history.len(), 5);
        // Should keep most recent trades
        assert_eq!(tracker.trade_history[0].pnl, 5.0);
        assert_eq!(tracker.trade_history[4].pnl, 9.0);
    }

    #[test]
    fn test_drawdown_tracking_winning_streak() {
        let mut tracker = EvolutionTracker::new(100);

        tracker.record_trade(create_test_trade(100.0));
        tracker.record_trade(create_test_trade(50.0));
        tracker.record_trade(create_test_trade(75.0));

        assert_eq!(tracker.peak_equity, 225.0);
        assert_eq!(tracker.current_drawdown, 0.0);
        assert_eq!(tracker.max_drawdown, 0.0);
    }

    #[test]
    fn test_drawdown_tracking_losing_after_wins() {
        let mut tracker = EvolutionTracker::new(100);

        tracker.record_trade(create_test_trade(100.0));
        tracker.record_trade(create_test_trade(50.0));
        // Peak at 150
        tracker.record_trade(create_test_trade(-30.0));
        // Now at 120

        assert_eq!(tracker.peak_equity, 150.0);
        assert_eq!(tracker.current_drawdown, 30.0);
        assert_eq!(tracker.max_drawdown, 30.0);
    }

    #[test]
    fn test_drawdown_tracking_recovery() {
        let mut tracker = EvolutionTracker::new(100);

        tracker.record_trade(create_test_trade(100.0));
        tracker.record_trade(create_test_trade(-50.0));
        tracker.record_trade(create_test_trade(100.0)); // Recovers above old peak

        assert_eq!(tracker.peak_equity, 150.0);
        assert_eq!(tracker.current_drawdown, 0.0);
    }

    #[test]
    fn test_calculate_total_pnl() {
        let mut tracker = EvolutionTracker::new(100);

        tracker.record_trade(create_test_trade(100.0));
        tracker.record_trade(create_test_trade(-30.0));
        tracker.record_trade(create_test_trade(50.0));
        tracker.record_trade(create_test_trade(-20.0));

        let total = tracker.calculate_total_pnl();
        assert_eq!(total, 100.0);
    }

    #[test]
    fn test_calculate_win_rate_no_trades() {
        let tracker = EvolutionTracker::new(100);
        assert_eq!(tracker.calculate_win_rate(), 0.0);
    }

    #[test]
    fn test_calculate_win_rate_all_wins() {
        let mut tracker = EvolutionTracker::new(100);

        tracker.record_trade(create_test_trade(100.0));
        tracker.record_trade(create_test_trade(50.0));
        tracker.record_trade(create_test_trade(75.0));

        assert_eq!(tracker.calculate_win_rate(), 100.0);
    }

    #[test]
    fn test_calculate_win_rate_all_losses() {
        let mut tracker = EvolutionTracker::new(100);

        tracker.record_trade(create_test_trade(-100.0));
        tracker.record_trade(create_test_trade(-50.0));

        assert_eq!(tracker.calculate_win_rate(), 0.0);
    }

    #[test]
    fn test_calculate_win_rate_mixed() {
        let mut tracker = EvolutionTracker::new(100);

        tracker.record_trade(create_test_trade(100.0));
        tracker.record_trade(create_test_trade(-50.0));
        tracker.record_trade(create_test_trade(75.0));
        tracker.record_trade(create_test_trade(-25.0));

        // 2 wins out of 4 = 50%
        assert_eq!(tracker.calculate_win_rate(), 50.0);
    }

    #[test]
    fn test_calculate_sharpe_ratio_insufficient_data() {
        let tracker = EvolutionTracker::new(100);
        assert_eq!(tracker.calculate_sharpe_ratio(), 0.0);

        let mut tracker2 = EvolutionTracker::new(100);
        tracker2.record_trade(create_test_trade(100.0));
        assert_eq!(tracker2.calculate_sharpe_ratio(), 0.0); // Need at least 2 trades
    }

    #[test]
    fn test_calculate_sharpe_ratio_zero_volatility() {
        let mut tracker = EvolutionTracker::new(100);

        // Manually set identical returns (zero volatility)
        tracker.returns = vec![10.0, 10.0, 10.0, 10.0];

        let sharpe = tracker.calculate_sharpe_ratio();
        assert_eq!(sharpe, 0.0); // Zero volatility = zero Sharpe
    }

    #[test]
    fn test_get_metrics() {
        let mut tracker = EvolutionTracker::new(100);

        tracker.record_trade(create_test_trade(100.0));
        tracker.record_trade(create_test_trade(-30.0));
        tracker.record_trade(create_test_trade(50.0));

        let metrics = tracker.get_metrics();

        assert_eq!(metrics.total_trades, 3);
        assert_eq!(metrics.total_pnl, 120.0);
        assert!((metrics.win_rate - 66.67).abs() < 0.1); // 2 wins out of 3
        assert_eq!(metrics.avg_profit_per_trade, 40.0); // 120/3
    }

    #[test]
    fn test_get_metrics_no_trades() {
        let tracker = EvolutionTracker::new(100);
        let metrics = tracker.get_metrics();

        assert_eq!(metrics.total_trades, 0);
        assert_eq!(metrics.total_pnl, 0.0);
        assert_eq!(metrics.win_rate, 0.0);
        assert_eq!(metrics.avg_profit_per_trade, 0.0);
    }

    #[test]
    fn test_fitness_score_all_positive() {
        let mut tracker = EvolutionTracker::new(100);

        // Build good performance
        for _ in 0..10 {
            tracker.record_trade(create_test_trade(100.0));
        }

        let fitness = tracker.fitness_score();

        // Should have a positive fitness score (all wins, positive PnL, no drawdown)
        // Actual value depends on Sharpe calculation with limited data
        assert!(fitness > 0.0);
        assert!(fitness <= 100.0);
    }

    #[test]
    fn test_fitness_score_poor_performance() {
        let mut tracker = EvolutionTracker::new(100);

        // Build poor performance
        for _ in 0..10 {
            tracker.record_trade(create_test_trade(-100.0));
        }

        let fitness = tracker.fitness_score();

        // Should have a low fitness score (all losses, negative PnL)
        assert!(fitness < 50.0);
    }

    #[test]
    fn test_fitness_score_mixed() {
        let mut tracker = EvolutionTracker::new(100);

        tracker.record_trade(create_test_trade(100.0));
        tracker.record_trade(create_test_trade(-50.0));
        tracker.record_trade(create_test_trade(75.0));

        let fitness = tracker.fitness_score();

        // Should be moderate (mixed results)
        assert!(fitness > 0.0);
        assert!(fitness < 100.0);
    }

    #[test]
    fn test_reset() {
        let mut tracker = EvolutionTracker::new(100);

        tracker.record_trade(create_test_trade(100.0));
        tracker.record_trade(create_test_trade(-50.0));

        assert_eq!(tracker.trade_history.len(), 2);
        assert!(tracker.peak_equity > 0.0);

        tracker.reset();

        assert_eq!(tracker.trade_history.len(), 0);
        assert_eq!(tracker.peak_equity, 0.0);
        assert_eq!(tracker.current_drawdown, 0.0);
        assert_eq!(tracker.max_drawdown, 0.0);
        assert_eq!(tracker.returns.len(), 0);
    }

    #[test]
    fn test_get_trade_history() {
        let mut tracker = EvolutionTracker::new(100);

        tracker.record_trade(create_test_trade(100.0));
        tracker.record_trade(create_test_trade(-50.0));

        let history = tracker.get_trade_history();

        assert_eq!(history.len(), 2);
        assert_eq!(history[0].pnl, 100.0);
        assert_eq!(history[1].pnl, -50.0);
    }

    #[test]
    fn test_trade_count() {
        let mut tracker = EvolutionTracker::new(100);

        assert_eq!(tracker.trade_count(), 0);

        tracker.record_trade(create_test_trade(100.0));
        assert_eq!(tracker.trade_count(), 1);

        tracker.record_trade(create_test_trade(-50.0));
        assert_eq!(tracker.trade_count(), 2);
    }

    #[test]
    fn test_trade_record_serialization() {
        let trade = TradeRecord {
            timestamp: 1234567890,
            asset: "BTC".to_string(),
            entry_price: 50000.0,
            exit_price: 51000.0,
            size: 0.5,
            pnl: 500.0,
        };

        let json = serde_json::to_string(&trade).unwrap();
        let deserialized: TradeRecord = serde_json::from_str(&json).unwrap();

        assert_eq!(deserialized.asset, "BTC");
        assert_eq!(deserialized.entry_price, 50000.0);
        assert_eq!(deserialized.exit_price, 51000.0);
        assert_eq!(deserialized.pnl, 500.0);
    }

    #[test]
    fn test_performance_metrics_serialization() {
        let metrics = PerformanceMetrics {
            win_rate: 75.0,
            total_pnl: 1000.0,
            sharpe_ratio: 1.5,
            max_drawdown: 200.0,
            total_trades: 10,
            avg_profit_per_trade: 100.0,
        };

        let json = serde_json::to_string(&metrics).unwrap();
        let deserialized: PerformanceMetrics = serde_json::from_str(&json).unwrap();

        assert_eq!(deserialized.win_rate, 75.0);
        assert_eq!(deserialized.total_pnl, 1000.0);
        assert_eq!(deserialized.sharpe_ratio, 1.5);
    }

    #[test]
    fn test_strategy_update_new() {
        let update = StrategyUpdate::new(
            "FundingArbitrage".to_string(),
            serde_json::json!({"threshold": -0.1}),
            5,
            "Improved performance".to_string(),
        );

        assert_eq!(update.strategy_name, "FundingArbitrage");
        assert_eq!(update.generation, 5);
        assert_eq!(update.reason, "Improved performance");
    }
}
