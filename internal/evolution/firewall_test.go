package evolution

import (
	"sync"
	"testing"
	"time"

	"github.com/clawinfra/evoclaw/internal/config"
)

func TestRateLimiter_AllowUpToLimit(t *testing.T) {
	rl := NewMutationRateLimiter(3)

	for i := 0; i < 3; i++ {
		if !rl.AllowMutation("agent-1") {
			t.Fatalf("mutation %d should be allowed", i+1)
		}
	}

	if rl.AllowMutation("agent-1") {
		t.Fatal("4th mutation should be rejected")
	}
}

func TestRateLimiter_DifferentAgents(t *testing.T) {
	rl := NewMutationRateLimiter(1)

	if !rl.AllowMutation("agent-1") {
		t.Fatal("agent-1 first mutation should be allowed")
	}
	if !rl.AllowMutation("agent-2") {
		t.Fatal("agent-2 first mutation should be allowed")
	}
	if rl.AllowMutation("agent-1") {
		t.Fatal("agent-1 second mutation should be rejected")
	}
}

func TestRateLimiter_Remaining(t *testing.T) {
	rl := NewMutationRateLimiter(5)
	if rem := rl.Remaining("agent-1"); rem != 5 {
		t.Fatalf("expected 5 remaining, got %d", rem)
	}

	rl.AllowMutation("agent-1")
	rl.AllowMutation("agent-1")
	if rem := rl.Remaining("agent-1"); rem != 3 {
		t.Fatalf("expected 3 remaining, got %d", rem)
	}
}

func TestCircuitBreaker_StaysClosedOnGoodMutations(t *testing.T) {
	cb := NewCircuitBreaker(0.30, 1*time.Hour)

	tripped := cb.RecordResult("agent-1", 1.0, 0.9) // 10% drop, under threshold
	if tripped {
		t.Fatal("should not trip on 10% drop")
	}

	state := cb.GetState("agent-1")
	if state != CircuitClosed {
		t.Fatalf("expected closed, got %s", state)
	}
}

func TestCircuitBreaker_OpensOnFitnessDrop(t *testing.T) {
	cb := NewCircuitBreaker(0.30, 1*time.Hour)

	tripped := cb.RecordResult("agent-1", 1.0, 0.5) // 50% drop
	if !tripped {
		t.Fatal("should trip on 50% drop")
	}

	state := cb.GetState("agent-1")
	if state != CircuitOpen {
		t.Fatalf("expected open, got %s", state)
	}

	allowed, _ := cb.ShouldAllowMutation("agent-1")
	if allowed {
		t.Fatal("should not allow mutation when circuit is open")
	}
}

func TestCircuitBreaker_HalfOpenTransition(t *testing.T) {
	cb := NewCircuitBreaker(0.30, 1*time.Millisecond) // tiny cooldown for testing

	cb.RecordResult("agent-1", 1.0, 0.5) // trip

	time.Sleep(5 * time.Millisecond)

	allowed, reason := cb.ShouldAllowMutation("agent-1")
	if !allowed {
		t.Fatalf("should allow after cooldown, reason: %s", reason)
	}

	state := cb.GetState("agent-1")
	if state != CircuitHalfOpen {
		t.Fatalf("expected half-open, got %s", state)
	}

	// Good mutation in half-open → close
	cb.RecordResult("agent-1", 0.5, 0.6)
	state = cb.GetState("agent-1")
	if state != CircuitClosed {
		t.Fatalf("expected closed after good half-open mutation, got %s", state)
	}
}

func TestCircuitBreaker_HalfOpenReopens(t *testing.T) {
	cb := NewCircuitBreaker(0.30, 1*time.Millisecond)

	cb.RecordResult("agent-1", 1.0, 0.5) // trip
	time.Sleep(5 * time.Millisecond)

	cb.ShouldAllowMutation("agent-1") // transition to half-open

	// Bad mutation in half-open → reopen
	tripped := cb.RecordResult("agent-1", 0.5, 0.3)
	if !tripped {
		t.Fatal("should trip on bad half-open mutation")
	}

	state := cb.GetState("agent-1")
	if state != CircuitOpen {
		t.Fatalf("expected open after bad half-open, got %s", state)
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(0.30, 1*time.Hour)
	cb.RecordResult("agent-1", 1.0, 0.5) // trip

	cb.Reset("agent-1")
	state := cb.GetState("agent-1")
	if state != CircuitClosed {
		t.Fatalf("expected closed after reset, got %s", state)
	}
}

func TestSnapshotStore_TakeAndRollback(t *testing.T) {
	ss := NewSnapshotStore(10)

	g := &config.Genome{
		Identity: config.GenomeIdentity{Name: "test-agent"},
		Skills:   map[string]config.SkillGenome{"s1": {Enabled: true, Fitness: 0.8}},
	}

	if err := ss.TakeSnapshot("agent-1", g, 0.8); err != nil {
		t.Fatalf("TakeSnapshot: %v", err)
	}

	if ss.SnapshotCount("agent-1") != 1 {
		t.Fatalf("expected 1 snapshot, got %d", ss.SnapshotCount("agent-1"))
	}

	restored, err := ss.Rollback("agent-1")
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	if restored.Identity.Name != "test-agent" {
		t.Fatalf("expected test-agent, got %s", restored.Identity.Name)
	}
}

func TestSnapshotStore_RingBuffer(t *testing.T) {
	ss := NewSnapshotStore(3)

	for i := 0; i < 5; i++ {
		g := &config.Genome{Identity: config.GenomeIdentity{Name: "test"}}
		ss.TakeSnapshot("agent-1", g, float64(i))
	}

	if ss.SnapshotCount("agent-1") != 3 {
		t.Fatalf("expected 3 snapshots (ring buffer), got %d", ss.SnapshotCount("agent-1"))
	}
}

func TestSnapshotStore_RollbackNoSnapshots(t *testing.T) {
	ss := NewSnapshotStore(10)
	_, err := ss.Rollback("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent agent")
	}
}

func TestEvolutionFirewall_PreMutationCheck(t *testing.T) {
	cfg := DefaultFirewallConfig()
	cfg.MaxMutationsPerHour = 2
	fw := NewEvolutionFirewall(cfg)

	// First two should pass
	for i := 0; i < 2; i++ {
		allowed, _, err := fw.PreMutationCheck("agent-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Fatalf("mutation %d should be allowed", i+1)
		}
	}

	// Third should fail (rate limit)
	allowed, reason, _ := fw.PreMutationCheck("agent-1")
	if allowed {
		t.Fatal("3rd mutation should be rejected")
	}
	if reason != "rate limit exceeded" {
		t.Fatalf("unexpected reason: %s", reason)
	}
}

func TestEvolutionFirewall_PostMutationCheck(t *testing.T) {
	cfg := DefaultFirewallConfig()
	fw := NewEvolutionFirewall(cfg)

	// Good mutation
	err := fw.PostMutationCheck("agent-1", 1.0, 0.9)
	if err != nil {
		t.Fatalf("good mutation should not error: %v", err)
	}

	// Bad mutation (>30% drop)
	err = fw.PostMutationCheck("agent-1", 1.0, 0.5)
	if err == nil {
		t.Fatal("bad mutation should return error")
	}
}

func TestEvolutionFirewall_Disabled(t *testing.T) {
	cfg := DefaultFirewallConfig()
	cfg.Enabled = false
	fw := NewEvolutionFirewall(cfg)

	allowed, _, _ := fw.PreMutationCheck("agent-1")
	if !allowed {
		t.Fatal("disabled firewall should allow all")
	}

	err := fw.PostMutationCheck("agent-1", 1.0, 0.1)
	if err != nil {
		t.Fatal("disabled firewall should not error")
	}
}

func TestEvolutionFirewall_GetStatus(t *testing.T) {
	cfg := DefaultFirewallConfig()
	cfg.MaxMutationsPerHour = 5
	fw := NewEvolutionFirewall(cfg)

	status := fw.GetFirewallStatus("agent-1")
	if !status.Enabled {
		t.Fatal("expected enabled")
	}
	if status.RateLimitRemaining != 5 {
		t.Fatalf("expected 5 remaining, got %d", status.RateLimitRemaining)
	}
	if status.CircuitBreakerState != CircuitClosed {
		t.Fatalf("expected closed, got %s", status.CircuitBreakerState)
	}
}

func TestEvolutionFirewall_CircuitBreakerBlocksPreCheck(t *testing.T) {
	cfg := DefaultFirewallConfig()
	cfg.CooldownPeriod = 1 * time.Hour
	fw := NewEvolutionFirewall(cfg)

	// Trip the circuit breaker
	fw.Breaker.RecordResult("agent-1", 1.0, 0.5)

	allowed, reason, _ := fw.PreMutationCheck("agent-1")
	if allowed {
		t.Fatal("should be blocked by circuit breaker")
	}
	if reason == "" {
		t.Fatal("reason should not be empty")
	}
}

func TestFirewallConcurrentAccess(t *testing.T) {
	cfg := DefaultFirewallConfig()
	cfg.MaxMutationsPerHour = 100
	fw := NewEvolutionFirewall(cfg)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			agentID := "agent-concurrent"
			fw.PreMutationCheck(agentID)
			fw.PostMutationCheck(agentID, 1.0, 0.95)
			fw.GetFirewallStatus(agentID)
			g := &config.Genome{Identity: config.GenomeIdentity{Name: "test"}}
			fw.Snapshots.TakeSnapshot(agentID, g, 0.9)
		}(i)
	}
	wg.Wait()
}
