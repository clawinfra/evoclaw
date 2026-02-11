package evolution

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/clawinfra/evoclaw/internal/config"
)

// CircuitBreakerState represents the state of a circuit breaker.
type CircuitBreakerState string

const (
	CircuitClosed   CircuitBreakerState = "closed"
	CircuitOpen     CircuitBreakerState = "open"
	CircuitHalfOpen CircuitBreakerState = "half-open"
)

// FirewallConfig holds configurable parameters for the evolution firewall.
type FirewallConfig struct {
	Enabled              bool          `json:"enabled"`
	MaxMutationsPerHour  int           `json:"max_mutations_per_hour"`
	FitnessDropThreshold float64       `json:"fitness_drop_threshold"` // e.g. 0.30 = 30%
	CooldownPeriod       time.Duration `json:"cooldown_period"`
	MaxSnapshots         int           `json:"max_snapshots"`
}

// DefaultFirewallConfig returns sensible defaults.
func DefaultFirewallConfig() FirewallConfig {
	return FirewallConfig{
		Enabled:              true,
		MaxMutationsPerHour:  10,
		FitnessDropThreshold: 0.30,
		CooldownPeriod:       1 * time.Hour,
		MaxSnapshots:         10,
	}
}

// ---- Rate Limiter ----

type mutationRecord struct {
	Timestamps []time.Time `json:"timestamps"`
}

// MutationRateLimiter tracks mutations per agent per time window.
type MutationRateLimiter struct {
	mu         sync.Mutex
	records    map[string]*mutationRecord
	maxPerHour int
}

// NewMutationRateLimiter creates a rate limiter.
func NewMutationRateLimiter(maxPerHour int) *MutationRateLimiter {
	return &MutationRateLimiter{
		records:    make(map[string]*mutationRecord),
		maxPerHour: maxPerHour,
	}
}

// AllowMutation returns true if the agent hasn't exceeded the rate limit.
func (rl *MutationRateLimiter) AllowMutation(agentID string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-1 * time.Hour)

	rec, ok := rl.records[agentID]
	if !ok {
		rec = &mutationRecord{}
		rl.records[agentID] = rec
	}

	// Prune old timestamps
	valid := rec.Timestamps[:0]
	for _, t := range rec.Timestamps {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	rec.Timestamps = valid

	if len(rec.Timestamps) >= rl.maxPerHour {
		return false
	}

	rec.Timestamps = append(rec.Timestamps, now)
	return true
}

// Remaining returns mutations remaining in the current window.
func (rl *MutationRateLimiter) Remaining(agentID string) int {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-1 * time.Hour)
	rec := rl.records[agentID]
	if rec == nil {
		return rl.maxPerHour
	}

	count := 0
	for _, t := range rec.Timestamps {
		if t.After(cutoff) {
			count++
		}
	}
	rem := rl.maxPerHour - count
	if rem < 0 {
		rem = 0
	}
	return rem
}

// ---- Circuit Breaker ----

type agentCircuit struct {
	State       CircuitBreakerState `json:"state"`
	OpenedAt    time.Time           `json:"opened_at,omitempty"`
	LastFitness float64             `json:"last_fitness"`
}

// CircuitBreaker monitors fitness after mutations and blocks if things go wrong.
type CircuitBreaker struct {
	mu               sync.Mutex
	agents           map[string]*agentCircuit
	fitnessThreshold float64 // fractional drop, e.g. 0.30
	cooldown         time.Duration
}

// NewCircuitBreaker creates a circuit breaker.
func NewCircuitBreaker(fitnessDropThreshold float64, cooldown time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		agents:           make(map[string]*agentCircuit),
		fitnessThreshold: fitnessDropThreshold,
		cooldown:         cooldown,
	}
}

// ShouldAllowMutation checks if mutations are allowed for the agent.
func (cb *CircuitBreaker) ShouldAllowMutation(agentID string) (bool, string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	ac, ok := cb.agents[agentID]
	if !ok {
		return true, "no circuit state"
	}

	switch ac.State {
	case CircuitClosed:
		return true, "circuit closed"
	case CircuitOpen:
		if time.Since(ac.OpenedAt) >= cb.cooldown {
			ac.State = CircuitHalfOpen
			return true, "circuit half-open (cooldown elapsed)"
		}
		return false, fmt.Sprintf("circuit open since %s", ac.OpenedAt.Format(time.RFC3339))
	case CircuitHalfOpen:
		return true, "circuit half-open (test mutation)"
	}
	return true, ""
}

// RecordResult records the result of a mutation and transitions state.
func (cb *CircuitBreaker) RecordResult(agentID string, oldFitness, newFitness float64) (tripped bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	ac, ok := cb.agents[agentID]
	if !ok {
		ac = &agentCircuit{State: CircuitClosed}
		cb.agents[agentID] = ac
	}

	ac.LastFitness = newFitness

	if oldFitness <= 0 {
		return false
	}

	drop := (oldFitness - newFitness) / oldFitness
	if drop > cb.fitnessThreshold {
		ac.State = CircuitOpen
		ac.OpenedAt = time.Now()
		return true
	}

	// If half-open and fitness improved or held, close
	if ac.State == CircuitHalfOpen {
		if newFitness >= oldFitness {
			ac.State = CircuitClosed
		} else {
			ac.State = CircuitOpen
			ac.OpenedAt = time.Now()
			return true
		}
	}

	return false
}

// GetState returns the current circuit breaker state for an agent.
func (cb *CircuitBreaker) GetState(agentID string) CircuitBreakerState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	ac := cb.agents[agentID]
	if ac == nil {
		return CircuitClosed
	}
	// Check for auto-transition
	if ac.State == CircuitOpen && time.Since(ac.OpenedAt) >= cb.cooldown {
		ac.State = CircuitHalfOpen
	}
	return ac.State
}

// Reset forces the circuit breaker to closed for an agent.
func (cb *CircuitBreaker) Reset(agentID string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	delete(cb.agents, agentID)
}

// ---- Genome Snapshots / Auto-Rollback ----

// GenomeSnapshot stores a complete genome state.
type GenomeSnapshot struct {
	Timestamp time.Time      `json:"timestamp"`
	Genome    *config.Genome `json:"genome"`
	Fitness   float64        `json:"fitness"`
}

type agentSnapshots struct {
	Snapshots []GenomeSnapshot `json:"snapshots"`
	Max       int              `json:"-"`
}

func (as *agentSnapshots) push(snap GenomeSnapshot) {
	as.Snapshots = append(as.Snapshots, snap)
	if len(as.Snapshots) > as.Max {
		as.Snapshots = as.Snapshots[len(as.Snapshots)-as.Max:]
	}
}

func (as *agentSnapshots) latest() *GenomeSnapshot {
	if len(as.Snapshots) == 0 {
		return nil
	}
	return &as.Snapshots[len(as.Snapshots)-1]
}

// SnapshotStore manages genome snapshots per agent (ring buffer).
type SnapshotStore struct {
	mu       sync.Mutex
	agents   map[string]*agentSnapshots
	maxSnaps int
}

// NewSnapshotStore creates a snapshot store.
func NewSnapshotStore(maxSnapshots int) *SnapshotStore {
	return &SnapshotStore{
		agents:   make(map[string]*agentSnapshots),
		maxSnaps: maxSnapshots,
	}
}

// TakeSnapshot saves a genome snapshot for the agent.
func (ss *SnapshotStore) TakeSnapshot(agentID string, genome *config.Genome, fitness float64) error {
	if genome == nil {
		return fmt.Errorf("genome is nil")
	}

	// Deep copy genome via JSON round-trip
	data, err := json.Marshal(genome)
	if err != nil {
		return fmt.Errorf("marshal genome for snapshot: %w", err)
	}
	var cloned config.Genome
	if err := json.Unmarshal(data, &cloned); err != nil {
		return fmt.Errorf("unmarshal genome clone: %w", err)
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()

	as, ok := ss.agents[agentID]
	if !ok {
		as = &agentSnapshots{Max: ss.maxSnaps}
		ss.agents[agentID] = as
	}

	as.push(GenomeSnapshot{
		Timestamp: time.Now(),
		Genome:    &cloned,
		Fitness:   fitness,
	})

	return nil
}

// Rollback returns the last known good genome for the agent.
func (ss *SnapshotStore) Rollback(agentID string) (*config.Genome, error) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	as, ok := ss.agents[agentID]
	if !ok || len(as.Snapshots) == 0 {
		return nil, fmt.Errorf("no snapshots for agent %s", agentID)
	}

	snap := as.latest()
	if snap == nil {
		return nil, fmt.Errorf("no snapshots for agent %s", agentID)
	}

	return snap.Genome, nil
}

// LastSnapshotTime returns the timestamp of the last snapshot, or zero time.
func (ss *SnapshotStore) LastSnapshotTime(agentID string) time.Time {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	as := ss.agents[agentID]
	if as == nil {
		return time.Time{}
	}
	snap := as.latest()
	if snap == nil {
		return time.Time{}
	}
	return snap.Timestamp
}

// SnapshotCount returns the number of stored snapshots for an agent.
func (ss *SnapshotStore) SnapshotCount(agentID string) int {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	as := ss.agents[agentID]
	if as == nil {
		return 0
	}
	return len(as.Snapshots)
}

// Save persists all snapshots to disk.
func (ss *SnapshotStore) Save(dir string) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	data, err := json.MarshalIndent(ss.agents, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "firewall-snapshots.json"), data, 0640)
}

// Load restores snapshots from disk.
func (ss *SnapshotStore) Load(dir string) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	data, err := os.ReadFile(filepath.Join(dir, "firewall-snapshots.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	agents := make(map[string]*agentSnapshots)
	if err := json.Unmarshal(data, &agents); err != nil {
		return err
	}
	for _, as := range agents {
		as.Max = ss.maxSnaps
	}
	ss.agents = agents
	return nil
}

// ---- Evolution Firewall (combines all three) ----

// FirewallStatus represents the current state of the firewall for an agent.
type FirewallStatus struct {
	Enabled             bool                `json:"enabled"`
	RateLimitRemaining  int                 `json:"rate_limit_remaining"`
	MaxMutationsPerHour int                 `json:"max_mutations_per_hour"`
	CircuitBreakerState CircuitBreakerState `json:"circuit_breaker_state"`
	LastSnapshotTime    *time.Time          `json:"last_snapshot_time,omitempty"`
	SnapshotCount       int                 `json:"snapshot_count"`
}

// EvolutionFirewall wraps rate limiter, circuit breaker, and snapshot store.
type EvolutionFirewall struct {
	Config    FirewallConfig
	Limiter   *MutationRateLimiter
	Breaker   *CircuitBreaker
	Snapshots *SnapshotStore
}

// NewEvolutionFirewall creates a new firewall with the given config.
func NewEvolutionFirewall(cfg FirewallConfig) *EvolutionFirewall {
	return &EvolutionFirewall{
		Config:    cfg,
		Limiter:   NewMutationRateLimiter(cfg.MaxMutationsPerHour),
		Breaker:   NewCircuitBreaker(cfg.FitnessDropThreshold, cfg.CooldownPeriod),
		Snapshots: NewSnapshotStore(cfg.MaxSnapshots),
	}
}

// PreMutationCheck performs rate limit and circuit breaker checks.
// Returns (allowed, reason, error).
func (fw *EvolutionFirewall) PreMutationCheck(agentID string) (bool, string, error) {
	if !fw.Config.Enabled {
		return true, "firewall disabled", nil
	}

	// Circuit breaker check first
	allowed, reason := fw.Breaker.ShouldAllowMutation(agentID)
	if !allowed {
		return false, "circuit breaker: " + reason, nil
	}

	// Rate limit check
	if !fw.Limiter.AllowMutation(agentID) {
		return false, "rate limit exceeded", nil
	}

	return true, reason, nil
}

// PostMutationCheck evaluates mutation result and triggers rollback if needed.
func (fw *EvolutionFirewall) PostMutationCheck(agentID string, oldFitness, newFitness float64) error {
	if !fw.Config.Enabled {
		return nil
	}

	tripped := fw.Breaker.RecordResult(agentID, oldFitness, newFitness)
	if tripped {
		// Circuit breaker tripped — auto-rollback is signaled
		return fmt.Errorf("circuit breaker tripped: fitness dropped from %.4f to %.4f (>%.0f%% drop)",
			oldFitness, newFitness, fw.Config.FitnessDropThreshold*100)
	}
	return nil
}

// GetFirewallStatus returns current firewall state for an agent.
func (fw *EvolutionFirewall) GetFirewallStatus(agentID string) FirewallStatus {
	status := FirewallStatus{
		Enabled:             fw.Config.Enabled,
		RateLimitRemaining:  fw.Limiter.Remaining(agentID),
		MaxMutationsPerHour: fw.Config.MaxMutationsPerHour,
		CircuitBreakerState: fw.Breaker.GetState(agentID),
		SnapshotCount:       fw.Snapshots.SnapshotCount(agentID),
	}
	t := fw.Snapshots.LastSnapshotTime(agentID)
	if !t.IsZero() {
		status.LastSnapshotTime = &t
	}
	return status
}
