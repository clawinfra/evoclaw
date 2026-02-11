//! Evolution Firewall for edge agents (Security Layer 3).
//!
//! Provides local rate limiting and circuit breaker for edge agents that
//! cannot rely on hub connectivity. Simpler than the hub-side Go implementation:
//! rate limit + fitness-based circuit breaker, no full snapshot system.

use std::collections::HashMap;
use std::sync::Mutex;
use std::time::{Duration, Instant};

/// Configuration for the edge firewall.
#[derive(Debug, Clone)]
pub struct FirewallConfig {
    pub enabled: bool,
    pub max_mutations_per_hour: usize,
    pub fitness_drop_threshold: f64,
    pub cooldown: Duration,
}

impl Default for FirewallConfig {
    fn default() -> Self {
        Self {
            enabled: true,
            max_mutations_per_hour: 10,
            fitness_drop_threshold: 0.30,
            cooldown: Duration::from_secs(3600),
        }
    }
}

/// Circuit breaker state.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CircuitState {
    Closed,
    Open,
    HalfOpen,
}

impl std::fmt::Display for CircuitState {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Closed => write!(f, "closed"),
            Self::Open => write!(f, "open"),
            Self::HalfOpen => write!(f, "half-open"),
        }
    }
}

struct AgentState {
    mutation_timestamps: Vec<Instant>,
    circuit: CircuitState,
    opened_at: Option<Instant>,
    last_fitness: f64,
}

impl AgentState {
    fn new() -> Self {
        Self {
            mutation_timestamps: Vec::new(),
            circuit: CircuitState::Closed,
            opened_at: None,
            last_fitness: 0.0,
        }
    }
}

/// Firewall status returned for monitoring.
#[derive(Debug, Clone)]
pub struct FirewallStatus {
    pub enabled: bool,
    pub rate_limit_remaining: usize,
    pub circuit_state: CircuitState,
}

/// Edge evolution firewall combining rate limiter and circuit breaker.
pub struct EdgeFirewall {
    config: FirewallConfig,
    agents: Mutex<HashMap<String, AgentState>>,
}

impl EdgeFirewall {
    /// Create a new edge firewall.
    pub fn new(config: FirewallConfig) -> Self {
        Self {
            config,
            agents: Mutex::new(HashMap::new()),
        }
    }

    /// Check if a mutation is allowed (rate limit + circuit breaker).
    /// Returns `Ok(())` if allowed, `Err(reason)` if blocked.
    pub fn pre_mutation_check(&self, agent_id: &str) -> Result<(), String> {
        if !self.config.enabled {
            return Ok(());
        }

        let mut agents = self.agents.lock().unwrap();
        let state = agents
            .entry(agent_id.to_string())
            .or_insert_with(AgentState::new);

        // Circuit breaker check
        match state.circuit {
            CircuitState::Open => {
                if let Some(opened) = state.opened_at {
                    if opened.elapsed() >= self.config.cooldown {
                        state.circuit = CircuitState::HalfOpen;
                    } else {
                        return Err(format!("circuit breaker open"));
                    }
                } else {
                    return Err("circuit breaker open".to_string());
                }
            }
            CircuitState::HalfOpen | CircuitState::Closed => {}
        }

        // Rate limit check
        let cutoff = Instant::now() - Duration::from_secs(3600);
        state.mutation_timestamps.retain(|t| *t > cutoff);
        if state.mutation_timestamps.len() >= self.config.max_mutations_per_hour {
            return Err("rate limit exceeded".to_string());
        }
        state.mutation_timestamps.push(Instant::now());

        Ok(())
    }

    /// Record the result of a mutation. Returns true if circuit breaker tripped.
    pub fn post_mutation_check(&self, agent_id: &str, old_fitness: f64, new_fitness: f64) -> bool {
        if !self.config.enabled {
            return false;
        }

        let mut agents = self.agents.lock().unwrap();
        let state = agents
            .entry(agent_id.to_string())
            .or_insert_with(AgentState::new);

        state.last_fitness = new_fitness;

        if old_fitness <= 0.0 {
            return false;
        }

        let drop = (old_fitness - new_fitness) / old_fitness;
        if drop > self.config.fitness_drop_threshold {
            state.circuit = CircuitState::Open;
            state.opened_at = Some(Instant::now());
            return true;
        }

        // Half-open → close on good result
        if state.circuit == CircuitState::HalfOpen {
            if new_fitness >= old_fitness {
                state.circuit = CircuitState::Closed;
                state.opened_at = None;
            } else {
                state.circuit = CircuitState::Open;
                state.opened_at = Some(Instant::now());
                return true;
            }
        }

        false
    }

    /// Reset the circuit breaker for an agent.
    pub fn reset(&self, agent_id: &str) {
        let mut agents = self.agents.lock().unwrap();
        agents.remove(agent_id);
    }

    /// Get firewall status for an agent.
    pub fn status(&self, agent_id: &str) -> FirewallStatus {
        let agents = self.agents.lock().unwrap();
        match agents.get(agent_id) {
            None => FirewallStatus {
                enabled: self.config.enabled,
                rate_limit_remaining: self.config.max_mutations_per_hour,
                circuit_state: CircuitState::Closed,
            },
            Some(state) => {
                let cutoff = Instant::now() - Duration::from_secs(3600);
                let used = state
                    .mutation_timestamps
                    .iter()
                    .filter(|t| **t > cutoff)
                    .count();
                let remaining = self.config.max_mutations_per_hour.saturating_sub(used);
                FirewallStatus {
                    enabled: self.config.enabled,
                    rate_limit_remaining: remaining,
                    circuit_state: state.circuit,
                }
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_rate_limit() {
        let cfg = FirewallConfig {
            max_mutations_per_hour: 3,
            ..Default::default()
        };
        let fw = EdgeFirewall::new(cfg);

        for _ in 0..3 {
            assert!(fw.pre_mutation_check("a1").is_ok());
        }
        assert!(fw.pre_mutation_check("a1").is_err());
    }

    #[test]
    fn test_circuit_breaker_trips() {
        let fw = EdgeFirewall::new(Default::default());
        let tripped = fw.post_mutation_check("a1", 1.0, 0.5);
        assert!(tripped);
        assert!(fw.pre_mutation_check("a1").is_err());
    }

    #[test]
    fn test_circuit_breaker_reset() {
        let fw = EdgeFirewall::new(Default::default());
        fw.post_mutation_check("a1", 1.0, 0.5);
        fw.reset("a1");
        assert!(fw.pre_mutation_check("a1").is_ok());
    }

    #[test]
    fn test_disabled_firewall() {
        let cfg = FirewallConfig {
            enabled: false,
            ..Default::default()
        };
        let fw = EdgeFirewall::new(cfg);
        // Should always allow
        for _ in 0..20 {
            assert!(fw.pre_mutation_check("a1").is_ok());
        }
    }

    #[test]
    fn test_status() {
        let cfg = FirewallConfig {
            max_mutations_per_hour: 5,
            ..Default::default()
        };
        let fw = EdgeFirewall::new(cfg);
        let s = fw.status("a1");
        assert_eq!(s.rate_limit_remaining, 5);
        assert_eq!(s.circuit_state, CircuitState::Closed);
    }
}
