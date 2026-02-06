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
