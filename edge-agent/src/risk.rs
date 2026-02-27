//! Risk management module for the EvoClaw edge agent.
//!
//! Enforces position limits, daily loss limits, cooldown periods, and provides
//! an emergency stop mechanism (triggered via MQTT).

use serde::{Deserialize, Serialize};
use std::time::{Duration, Instant};
use tracing::{info, warn};

use crate::config::RiskConfig;

/// Risk check result
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum RiskDecision {
    /// Trade is allowed
    Allowed,
    /// Trade is rejected with a reason
    Rejected(String),
}

impl RiskDecision {
    pub fn is_allowed(&self) -> bool {
        matches!(self, RiskDecision::Allowed)
    }
}

/// Event logged by the risk manager
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RiskEvent {
    pub timestamp: u64,
    pub event_type: String,
    pub details: String,
}

/// Risk manager state
pub struct RiskManager {
    config: RiskConfig,
    /// Daily realized P&L (reset at midnight UTC)
    daily_pnl: f64,
    /// Current date string for daily reset tracking
    daily_date: String,
    /// Count of current open positions
    open_position_count: usize,
    /// Count of consecutive losses
    consecutive_losses: u32,
    /// When cooldown started (None if not in cooldown)
    cooldown_until: Option<Instant>,
    /// Emergency stop flag (set via MQTT)
    emergency_stop: bool,
    /// Log of risk events
    events: Vec<RiskEvent>,
}

impl RiskManager {
    pub fn new(config: RiskConfig) -> Self {
        let today = current_date_string();
        Self {
            config,
            daily_pnl: 0.0,
            daily_date: today,
            open_position_count: 0,
            consecutive_losses: 0,
            cooldown_until: None,
            emergency_stop: false,
            events: Vec::new(),
        }
    }

    /// Check if a new order is allowed
    pub fn check_order(
        &mut self,
        position_size_usd: f64,
        is_new_position: bool,
    ) -> RiskDecision {
        self.maybe_reset_daily();

        // Emergency stop
        if self.emergency_stop {
            let reason = "emergency stop is active".to_string();
            self.log_event("order_rejected", &reason);
            return RiskDecision::Rejected(reason);
        }

        // Cooldown check
        if let Some(until) = self.cooldown_until {
            if Instant::now() < until {
                let remaining = until.duration_since(Instant::now());
                let reason = format!(
                    "in cooldown after {} consecutive losses ({:.0}s remaining)",
                    self.consecutive_losses,
                    remaining.as_secs_f64()
                );
                self.log_event("order_rejected", &reason);
                return RiskDecision::Rejected(reason);
            } else {
                // Cooldown expired
                self.cooldown_until = None;
                self.consecutive_losses = 0;
                self.log_event("cooldown_expired", "cooldown period ended");
            }
        }

        // Position size limit
        if position_size_usd > self.config.max_position_size_usd {
            let reason = format!(
                "position size ${:.2} exceeds max ${:.2}",
                position_size_usd, self.config.max_position_size_usd
            );
            self.log_event("order_rejected", &reason);
            return RiskDecision::Rejected(reason);
        }

        // Max open positions (only for new positions, not modifications)
        if is_new_position && self.open_position_count >= self.config.max_open_positions {
            let reason = format!(
                "max open positions ({}) reached",
                self.config.max_open_positions
            );
            self.log_event("order_rejected", &reason);
            return RiskDecision::Rejected(reason);
        }

        // Daily loss limit
        if self.daily_pnl < -self.config.max_daily_loss_usd {
            let reason = format!(
                "daily loss ${:.2} exceeds limit ${:.2}",
                self.daily_pnl.abs(),
                self.config.max_daily_loss_usd
            );
            self.log_event("order_rejected", &reason);
            return RiskDecision::Rejected(reason);
        }

        RiskDecision::Allowed
    }

    /// Record a trade result (called after a fill)
    pub fn record_trade(&mut self, pnl: f64) {
        self.maybe_reset_daily();
        self.daily_pnl += pnl;

        if pnl < 0.0 {
            self.consecutive_losses += 1;
            info!(
                consecutive_losses = self.consecutive_losses,
                daily_pnl = self.daily_pnl,
                "loss recorded"
            );

            // Check if we need to enter cooldown
            if self.consecutive_losses >= self.config.consecutive_loss_limit {
                let cooldown_duration =
                    Duration::from_secs(self.config.cooldown_after_losses_secs);
                self.cooldown_until = Some(Instant::now() + cooldown_duration);
                let msg = format!(
                    "entering cooldown for {}s after {} consecutive losses",
                    self.config.cooldown_after_losses_secs, self.consecutive_losses
                );
                warn!("{}", msg);
                self.log_event("cooldown_started", &msg);
            }
        } else {
            self.consecutive_losses = 0;
        }

        // Check daily loss limit
        if self.daily_pnl < -self.config.max_daily_loss_usd {
            let msg = format!(
                "daily loss limit breached: ${:.2} (limit: ${:.2})",
                self.daily_pnl.abs(),
                self.config.max_daily_loss_usd
            );
            warn!("{}", msg);
            self.log_event("daily_loss_limit_breached", &msg);
        }
    }

    /// Update the count of open positions
    pub fn set_open_positions(&mut self, count: usize) {
        self.open_position_count = count;
    }

    /// Activate emergency stop
    pub fn emergency_stop(&mut self) {
        self.emergency_stop = true;
        warn!("EMERGENCY STOP activated");
        self.log_event("emergency_stop", "emergency stop activated via command");
    }

    /// Deactivate emergency stop
    pub fn clear_emergency_stop(&mut self) {
        self.emergency_stop = false;
        info!("emergency stop cleared");
        self.log_event("emergency_stop_cleared", "emergency stop deactivated");
    }

    /// Check if emergency stop is active
    pub fn is_emergency_stopped(&self) -> bool {
        self.emergency_stop
    }

    /// Get current daily P&L
    pub fn daily_pnl(&self) -> f64 {
        self.daily_pnl
    }

    /// Get consecutive loss count
    pub fn consecutive_losses(&self) -> u32 {
        self.consecutive_losses
    }

    /// Get risk events
    pub fn events(&self) -> &[RiskEvent] {
        &self.events
    }

    /// Get a summary of current risk state
    pub fn status(&self) -> RiskStatus {
        RiskStatus {
            daily_pnl: self.daily_pnl,
            open_positions: self.open_position_count,
            consecutive_losses: self.consecutive_losses,
            in_cooldown: self.cooldown_until.map_or(false, |u| Instant::now() < u),
            emergency_stop: self.emergency_stop,
            daily_loss_limit: self.config.max_daily_loss_usd,
            max_position_size: self.config.max_position_size_usd,
            max_open_positions: self.config.max_open_positions,
        }
    }

    // -----------------------------------------------------------------------
    // Internal
    // -----------------------------------------------------------------------

    /// Reset daily tracking if the date has changed
    fn maybe_reset_daily(&mut self) {
        let today = current_date_string();
        if today != self.daily_date {
            info!(
                old_date = %self.daily_date,
                new_date = %today,
                final_daily_pnl = self.daily_pnl,
                "daily risk counters reset"
            );
            self.daily_pnl = 0.0;
            self.daily_date = today;
        }
    }

    fn log_event(&mut self, event_type: &str, details: &str) {
        let event = RiskEvent {
            timestamp: current_timestamp(),
            event_type: event_type.to_string(),
            details: details.to_string(),
        };
        self.events.push(event);
    }
}

/// Risk status summary
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RiskStatus {
    pub daily_pnl: f64,
    pub open_positions: usize,
    pub consecutive_losses: u32,
    pub in_cooldown: bool,
    pub emergency_stop: bool,
    pub daily_loss_limit: f64,
    pub max_position_size: f64,
    pub max_open_positions: usize,
}

fn current_timestamp() -> u64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs()
}

fn current_date_string() -> String {
    let ts = current_timestamp();
    let secs_per_day = 86400u64;
    let days = ts / secs_per_day;
    // Simple date: just use the day number. For proper dates we'd use chrono
    // but for daily reset tracking this is sufficient.
    format!("day-{}", days)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn create_test_config() -> RiskConfig {
        RiskConfig {
            max_position_size_usd: 5000.0,
            max_daily_loss_usd: 500.0,
            max_open_positions: 3,
            cooldown_after_losses_secs: 60,
            consecutive_loss_limit: 3,
        }
    }

    #[test]
    fn test_risk_manager_new() {
        let config = create_test_config();
        let rm = RiskManager::new(config);

        assert_eq!(rm.daily_pnl(), 0.0);
        assert_eq!(rm.consecutive_losses(), 0);
        assert!(!rm.is_emergency_stopped());
        assert!(rm.events().is_empty());
    }

    #[test]
    fn test_check_order_allowed() {
        let config = create_test_config();
        let mut rm = RiskManager::new(config);

        let decision = rm.check_order(1000.0, true);
        assert!(decision.is_allowed());
    }

    #[test]
    fn test_check_order_position_size_exceeded() {
        let config = create_test_config();
        let mut rm = RiskManager::new(config);

        let decision = rm.check_order(6000.0, true);
        assert!(!decision.is_allowed());
        match decision {
            RiskDecision::Rejected(reason) => {
                assert!(reason.contains("position size"));
                assert!(reason.contains("exceeds max"));
            }
            _ => panic!("expected rejection"),
        }
    }

    #[test]
    fn test_check_order_max_positions_reached() {
        let config = create_test_config();
        let mut rm = RiskManager::new(config);
        rm.set_open_positions(3);

        let decision = rm.check_order(1000.0, true);
        assert!(!decision.is_allowed());
        match decision {
            RiskDecision::Rejected(reason) => {
                assert!(reason.contains("max open positions"));
            }
            _ => panic!("expected rejection"),
        }
    }

    #[test]
    fn test_check_order_max_positions_ok_for_modification() {
        let config = create_test_config();
        let mut rm = RiskManager::new(config);
        rm.set_open_positions(3);

        // Not a new position → should be allowed
        let decision = rm.check_order(1000.0, false);
        assert!(decision.is_allowed());
    }

    #[test]
    fn test_check_order_daily_loss_exceeded() {
        let mut config = create_test_config();
        config.consecutive_loss_limit = 100; // Prevent cooldown from triggering first
        let mut rm = RiskManager::new(config);

        // Record losses exceeding the daily limit
        rm.record_trade(-200.0);
        rm.record_trade(-200.0);
        rm.record_trade(-200.0); // Total: -600, limit is 500

        let decision = rm.check_order(1000.0, true);
        assert!(!decision.is_allowed());
        match decision {
            RiskDecision::Rejected(reason) => {
                assert!(reason.contains("daily loss"));
            }
            _ => panic!("expected rejection"),
        }
    }

    #[test]
    fn test_record_trade_winning_resets_consecutive() {
        let config = create_test_config();
        let mut rm = RiskManager::new(config);

        rm.record_trade(-10.0);
        rm.record_trade(-10.0);
        assert_eq!(rm.consecutive_losses(), 2);

        rm.record_trade(20.0); // Win resets streak
        assert_eq!(rm.consecutive_losses(), 0);
    }

    #[test]
    fn test_cooldown_after_consecutive_losses() {
        let mut config = create_test_config();
        config.consecutive_loss_limit = 3;
        config.cooldown_after_losses_secs = 60;
        let mut rm = RiskManager::new(config);

        rm.record_trade(-10.0);
        rm.record_trade(-10.0);
        rm.record_trade(-10.0); // Triggers cooldown

        assert_eq!(rm.consecutive_losses(), 3);

        // Order should be rejected during cooldown
        let decision = rm.check_order(100.0, true);
        assert!(!decision.is_allowed());
        match decision {
            RiskDecision::Rejected(reason) => {
                assert!(reason.contains("cooldown"));
            }
            _ => panic!("expected rejection"),
        }
    }

    #[test]
    fn test_emergency_stop() {
        let config = create_test_config();
        let mut rm = RiskManager::new(config);

        assert!(!rm.is_emergency_stopped());

        rm.emergency_stop();
        assert!(rm.is_emergency_stopped());

        let decision = rm.check_order(100.0, true);
        assert!(!decision.is_allowed());
        match decision {
            RiskDecision::Rejected(reason) => {
                assert!(reason.contains("emergency stop"));
            }
            _ => panic!("expected rejection"),
        }
    }

    #[test]
    fn test_clear_emergency_stop() {
        let config = create_test_config();
        let mut rm = RiskManager::new(config);

        rm.emergency_stop();
        assert!(rm.is_emergency_stopped());

        rm.clear_emergency_stop();
        assert!(!rm.is_emergency_stopped());

        let decision = rm.check_order(100.0, true);
        assert!(decision.is_allowed());
    }

    #[test]
    fn test_risk_events_logged() {
        let config = create_test_config();
        let mut rm = RiskManager::new(config);

        // Trigger position size rejection
        rm.check_order(10000.0, true);

        assert_eq!(rm.events().len(), 1);
        assert_eq!(rm.events()[0].event_type, "order_rejected");
    }

    #[test]
    fn test_status() {
        let config = create_test_config();
        let mut rm = RiskManager::new(config);

        rm.set_open_positions(2);
        rm.record_trade(-100.0);
        rm.record_trade(-50.0);

        let status = rm.status();
        assert_eq!(status.daily_pnl, -150.0);
        assert_eq!(status.open_positions, 2);
        assert_eq!(status.consecutive_losses, 2);
        assert!(!status.in_cooldown);
        assert!(!status.emergency_stop);
        assert_eq!(status.daily_loss_limit, 500.0);
        assert_eq!(status.max_position_size, 5000.0);
        assert_eq!(status.max_open_positions, 3);
    }

    #[test]
    fn test_risk_status_serialization() {
        let status = RiskStatus {
            daily_pnl: -100.0,
            open_positions: 2,
            consecutive_losses: 1,
            in_cooldown: false,
            emergency_stop: false,
            daily_loss_limit: 500.0,
            max_position_size: 5000.0,
            max_open_positions: 5,
        };

        let json = serde_json::to_string(&status).unwrap();
        let deserialized: RiskStatus = serde_json::from_str(&json).unwrap();
        assert_eq!(deserialized.daily_pnl, -100.0);
        assert_eq!(deserialized.open_positions, 2);
    }

    #[test]
    fn test_risk_event_serialization() {
        let event = RiskEvent {
            timestamp: 1700000000,
            event_type: "test".to_string(),
            details: "test event".to_string(),
        };

        let json = serde_json::to_string(&event).unwrap();
        let deserialized: RiskEvent = serde_json::from_str(&json).unwrap();
        assert_eq!(deserialized.event_type, "test");
    }

    #[test]
    fn test_risk_decision_display() {
        let allowed = RiskDecision::Allowed;
        let rejected = RiskDecision::Rejected("too risky".to_string());

        assert!(allowed.is_allowed());
        assert!(!rejected.is_allowed());
    }

    #[test]
    fn test_daily_pnl_tracking() {
        let config = create_test_config();
        let mut rm = RiskManager::new(config);

        rm.record_trade(100.0);
        assert_eq!(rm.daily_pnl(), 100.0);

        rm.record_trade(-50.0);
        assert_eq!(rm.daily_pnl(), 50.0);

        rm.record_trade(-200.0);
        assert_eq!(rm.daily_pnl(), -150.0);
    }

    #[test]
    fn test_set_open_positions() {
        let config = create_test_config();
        let mut rm = RiskManager::new(config);

        rm.set_open_positions(0);
        assert!(rm.check_order(1000.0, true).is_allowed());

        rm.set_open_positions(3);
        assert!(!rm.check_order(1000.0, true).is_allowed());

        rm.set_open_positions(2);
        assert!(rm.check_order(1000.0, true).is_allowed());
    }

    #[test]
    fn test_emergency_stop_events() {
        let config = create_test_config();
        let mut rm = RiskManager::new(config);

        rm.emergency_stop();
        rm.clear_emergency_stop();

        assert_eq!(rm.events().len(), 2);
        assert_eq!(rm.events()[0].event_type, "emergency_stop");
        assert_eq!(rm.events()[1].event_type, "emergency_stop_cleared");
    }

    #[test]
    fn test_cooldown_events() {
        let mut config = create_test_config();
        config.consecutive_loss_limit = 2;
        let mut rm = RiskManager::new(config);

        rm.record_trade(-10.0);
        rm.record_trade(-10.0); // Triggers cooldown

        let cooldown_events: Vec<_> = rm
            .events()
            .iter()
            .filter(|e| e.event_type == "cooldown_started")
            .collect();
        assert_eq!(cooldown_events.len(), 1);
    }
}
