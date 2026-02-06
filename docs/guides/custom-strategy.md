# Write Custom Trading Strategies

This guide covers implementing custom trading strategies for the EvoClaw edge agent.

## Strategy Interface

All strategies implement the `Strategy` trait:

```rust
pub trait Strategy: Send + Sync {
    /// Strategy name
    fn name(&self) -> &str;

    /// Evaluate market data and optionally return a trade signal
    fn evaluate(&self, market_data: &MarketData) -> Option<Signal>;

    /// Get current parameters (for evolution)
    fn parameters(&self) -> HashMap<String, f64>;

    /// Set parameters (from evolution engine)
    fn set_parameters(&mut self, params: HashMap<String, f64>);
}

pub struct MarketData {
    pub prices: HashMap<String, f64>,      // Asset → mid price
    pub funding_rates: HashMap<String, f64>, // Asset → funding rate
    pub positions: Vec<Position>,           // Current positions
    pub timestamp: u64,
}

pub struct Signal {
    pub asset: String,
    pub action: Action,  // Buy, Sell, Close
    pub size: f64,       // Position size in USD
    pub price: f64,      // Limit price
    pub reason: String,  // Human-readable reason
}

pub enum Action {
    Buy,
    Sell,
    Close,
}
```

## Example: Momentum Breakout Strategy

```rust
use std::collections::HashMap;

pub struct MomentumBreakout {
    lookback: usize,
    breakout_threshold: f64,
    position_size_pct: f64,
    price_history: Vec<f64>,
}

impl MomentumBreakout {
    pub fn new() -> Self {
        Self {
            lookback: 20,
            breakout_threshold: 2.0,  // Standard deviations
            position_size_pct: 0.1,
            price_history: Vec::new(),
        }
    }

    fn mean(&self) -> f64 {
        let sum: f64 = self.price_history.iter().sum();
        sum / self.price_history.len() as f64
    }

    fn std_dev(&self) -> f64 {
        let mean = self.mean();
        let variance: f64 = self.price_history.iter()
            .map(|p| (p - mean).powi(2))
            .sum::<f64>() / self.price_history.len() as f64;
        variance.sqrt()
    }
}

impl Strategy for MomentumBreakout {
    fn name(&self) -> &str {
        "MomentumBreakout"
    }

    fn evaluate(&self, data: &MarketData) -> Option<Signal> {
        let price = *data.prices.get("ETH")?;

        // Need enough history
        if self.price_history.len() < self.lookback {
            return None;
        }

        let mean = self.mean();
        let std = self.std_dev();
        let z_score = (price - mean) / std;

        if z_score > self.breakout_threshold {
            // Breakout up → buy
            Some(Signal {
                asset: "ETH-PERP".to_string(),
                action: Action::Buy,
                size: 5000.0 * self.position_size_pct,
                price,
                reason: format!("Momentum breakout: z={:.2}", z_score),
            })
        } else if z_score < -self.breakout_threshold {
            // Breakout down → sell
            Some(Signal {
                asset: "ETH-PERP".to_string(),
                action: Action::Sell,
                size: 5000.0 * self.position_size_pct,
                price,
                reason: format!("Momentum breakdown: z={:.2}", z_score),
            })
        } else {
            None
        }
    }

    fn parameters(&self) -> HashMap<String, f64> {
        let mut params = HashMap::new();
        params.insert("lookback".to_string(), self.lookback as f64);
        params.insert("breakout_threshold".to_string(), self.breakout_threshold);
        params.insert("position_size_pct".to_string(), self.position_size_pct);
        params
    }

    fn set_parameters(&mut self, params: HashMap<String, f64>) {
        if let Some(&v) = params.get("lookback") {
            self.lookback = v.max(5.0) as usize;
        }
        if let Some(&v) = params.get("breakout_threshold") {
            self.breakout_threshold = v.clamp(0.5, 5.0);
        }
        if let Some(&v) = params.get("position_size_pct") {
            self.position_size_pct = v.clamp(0.01, 0.5);
        }
    }
}
```

## Registering Your Strategy

Add your strategy to the agent's strategy engine in `src/strategy.rs`:

```rust
pub fn create_strategies(config: &Config) -> Vec<Box<dyn Strategy>> {
    let mut strategies: Vec<Box<dyn Strategy>> = Vec::new();

    // Built-in strategies
    strategies.push(Box::new(FundingArbitrage::new()));
    strategies.push(Box::new(MeanReversion::new()));

    // Your custom strategy
    strategies.push(Box::new(MomentumBreakout::new()));

    strategies
}
```

## Evolution-Friendly Design

For your strategy to evolve well:

1. **Use named parameters** — Each tunable value should be in the `parameters()` map
2. **Set reasonable bounds** — Use `clamp()` in `set_parameters()` to prevent extreme values
3. **Keep parameters continuous** — The mutation engine works with float values
4. **Document defaults** — What works well before evolution starts?

### Parameter Design Tips

| Good | Bad | Why |
|------|-----|-----|
| `threshold: f64` | `use_threshold: bool` | Booleans can't be gradually mutated |
| `lookback: 20.0` | `lookback: 1000000.0` | Keep scale reasonable |
| Bounded: `0.01..0.5` | Unbounded | Prevents degenerate strategies |

## Testing Strategies

```rust
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_momentum_breakout_signal() {
        let mut strategy = MomentumBreakout::new();
        strategy.lookback = 5;
        strategy.breakout_threshold = 1.5;

        // Build price history: stable around 3000
        strategy.price_history = vec![3000.0, 3010.0, 2990.0, 3005.0, 2995.0];

        let mut prices = HashMap::new();
        prices.insert("ETH".to_string(), 3100.0); // Big breakout

        let data = MarketData {
            prices,
            funding_rates: HashMap::new(),
            positions: vec![],
            timestamp: 0,
        };

        let signal = strategy.evaluate(&data);
        assert!(signal.is_some());
        assert!(matches!(signal.unwrap().action, Action::Buy));
    }
}
```

## See Also

- [Trading Agent Guide](trading-agent.md)
- [Edge Agent Architecture](../architecture/edge-agent.md)
- [Evolution Engine](../architecture/evolution.md)
- [Genome Format](../reference/genome-format.md)
