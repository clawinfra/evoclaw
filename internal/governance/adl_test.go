package governance

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestNewADL(t *testing.T) {
	tmpDir := t.TempDir()
	var logger *slog.Logger

	adl, err := NewADL(tmpDir, logger)
	if err != nil {
		t.Fatalf("NewADL failed: %v", err)
	}

	if adl == nil {
		t.Fatal("expected non-nil ADL")
	}

	// Check directory was created
	adlDir := filepath.Join(tmpDir, "adl")
	if _, err := os.Stat(adlDir); os.IsNotExist(err) {
		t.Error("adl directory not created")
	}
}

func TestADLAnalyze(t *testing.T) {
	tmpDir := t.TempDir()
	adl, _ := NewADL(tmpDir, nil)

	tests := []struct {
		name              string
		text              string
		wantAntiPatterns  bool
		wantPersonaSignals bool
	}{
		{
			name:              "sycophancy",
			text:              "I'd be happy to help with that!",
			wantAntiPatterns:  true,
			wantPersonaSignals: false,
		},
		{
			name:              "passivity",
			text:              "Would you like me to do this?",
			wantAntiPatterns:  true,
			wantPersonaSignals: false,
		},
		{
			name:              "direct",
			text:              "Done. Fixed it.",
			wantAntiPatterns:  false,
			wantPersonaSignals: true,
		},
		{
			name:              "opinionated",
			text:              "I'd argue the better approach is...",
			wantAntiPatterns:  false,
			wantPersonaSignals: true,
		},
		{
			name:              "action-oriented",
			text:              "Spawning sub-agent now.",
			wantAntiPatterns:  false,
			wantPersonaSignals: true,
		},
		{
			name:              "mixed",
			text:              "Great question! Done.",
			wantAntiPatterns:  true,
			wantPersonaSignals: true,
		},
		{
			name:              "neutral",
			text:              "The file is located at /path/to/file",
			wantAntiPatterns:  false,
			wantPersonaSignals: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signals, err := adl.Analyze(tt.text)
			if err != nil {
				t.Fatalf("Analyze failed: %v", err)
			}

			hasAntiPattern := false
			hasPersonaSignal := false

			for _, s := range signals {
				if !s.Positive {
					hasAntiPattern = true
				} else {
					hasPersonaSignal = true
				}
			}

			if hasAntiPattern != tt.wantAntiPatterns {
				t.Errorf("expected anti-patterns %v, got %v", tt.wantAntiPatterns, hasAntiPattern)
			}

			if hasPersonaSignal != tt.wantPersonaSignals {
				t.Errorf("expected persona signals %v, got %v", tt.wantPersonaSignals, hasPersonaSignal)
			}
		})
	}
}

func TestADLCheckDrift(t *testing.T) {
	tmpDir := t.TempDir()
	adl, _ := NewADL(tmpDir, nil)
	agentID := "test-agent"

	// Analyze text with anti-patterns
	text := "I'd be happy to help! Would you like me to?"
	score, err := adl.CheckDrift(agentID, text)
	if err != nil {
		t.Fatalf("CheckDrift failed: %v", err)
	}

	// Score should be > 0 due to anti-patterns
	if score <= 0 {
		t.Errorf("expected positive drift score, got %.2f", score)
	}

	// Analyze text with persona signals
	text2 := "Done. Fixed it."
	score2, err := adl.CheckDrift(agentID, text2)
	if err != nil {
		t.Fatalf("CheckDrift failed: %v", err)
	}

	// Score should decrease with persona signals
	if score2 >= score {
		t.Errorf("expected score to decrease with persona signals, %.2f >= %.2f", score2, score)
	}
}

func TestADLLog(t *testing.T) {
	tmpDir := t.TempDir()
	adl, _ := NewADL(tmpDir, nil)
	agentID := "test-agent"

	// Log anti-pattern signal
	err := adl.Log(agentID, SignalAntiSycophancy, "I'd be happy to help", false)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Log persona signal
	err = adl.Log(agentID, SignalPersonaDirect, "Done", true)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Check stats
	stats, err := adl.Stats(agentID)
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if stats.TotalSignals != 2 {
		t.Errorf("expected 2 signals, got %d", stats.TotalSignals)
	}

	if stats.AntiPatterns != 1 {
		t.Errorf("expected 1 anti-pattern, got %d", stats.AntiPatterns)
	}

	if stats.PersonaSignals != 1 {
		t.Errorf("expected 1 persona signal, got %d", stats.PersonaSignals)
	}
}

func TestADLStats(t *testing.T) {
	tmpDir := t.TempDir()
	adl, _ := NewADL(tmpDir, nil)
	agentID := "test-agent-stats"

	// Log various signals
	_ = adl.Log(agentID, SignalAntiSycophancy, "Happy to help", false)
	_ = adl.Log(agentID, SignalAntiPassivity, "Would you like", false)
	_ = adl.Log(agentID, SignalPersonaDirect, "Done", true)
	_ = adl.Log(agentID, SignalPersonaAction, "Spawning", true)

	// Get stats
	stats, err := adl.Stats(agentID)
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if stats.TotalSignals != 4 {
		t.Errorf("expected 4 signals, got %d", stats.TotalSignals)
	}

	if stats.AntiPatterns != 2 {
		t.Errorf("expected 2 anti-patterns, got %d", stats.AntiPatterns)
	}

	if stats.PersonaSignals != 2 {
		t.Errorf("expected 2 persona signals, got %d", stats.PersonaSignals)
	}

	// Score should be 0 since balanced (2 - 2) / 4 = 0
	if stats.DivergenceScore != 0.0 {
		t.Errorf("expected divergence score 0.0, got %.2f", stats.DivergenceScore)
	}
}

func TestADLReset(t *testing.T) {
	tmpDir := t.TempDir()
	adl, _ := NewADL(tmpDir, nil)
	agentID := "test-agent-reset"

	// Log some signals
	_ = adl.Log(agentID, SignalAntiSycophancy, "Test", false)
	_ = adl.Log(agentID, SignalPersonaDirect, "Test", true)

	// Verify signals exist
	stats1, _ := adl.Stats(agentID)
	if stats1.TotalSignals != 2 {
		t.Errorf("expected 2 signals before reset, got %d", stats1.TotalSignals)
	}

	// Reset
	err := adl.Reset(agentID)
	if err != nil {
		t.Fatalf("Reset failed: %v", err)
	}

	// Verify signals cleared
	stats2, _ := adl.Stats(agentID)
	if stats2.TotalSignals != 0 {
		t.Errorf("expected 0 signals after reset, got %d", stats2.TotalSignals)
	}
}

func TestADLCheck(t *testing.T) {
	tmpDir := t.TempDir()
	adl, _ := NewADL(tmpDir, nil)
	agentID := "test-agent-check"
	threshold := 0.5

	// Initially should not exceed threshold
	drifted, err := adl.Check(agentID, threshold)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if drifted {
		t.Error("expected not drifted initially")
	}

	// Add anti-patterns
	for i := 0; i < 10; i++ {
		_ = adl.Log(agentID, SignalAntiSycophancy, "Happy to help", false)
	}

	// Should now exceed threshold
	drifted, err = adl.Check(agentID, threshold)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if !drifted {
		t.Error("expected drifted after adding anti-patterns")
	}
}

func TestADLAgentIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	adl, _ := NewADL(tmpDir, nil)

	agent1 := "agent-1"
	agent2 := "agent-2"

	_ = adl.Log(agent1, SignalAntiSycophancy, "Test 1", false)
	_ = adl.Log(agent2, SignalPersonaDirect, "Test 2", true)

	// Check isolation
	stats1, _ := adl.Stats(agent1)
	stats2, _ := adl.Stats(agent2)

	if stats1.TotalSignals != 1 {
		t.Errorf("agent1: expected 1 signal, got %d", stats1.TotalSignals)
	}

	if stats2.TotalSignals != 1 {
		t.Errorf("agent2: expected 1 signal, got %d", stats2.TotalSignals)
	}

	if stats1.AntiPatterns != 1 {
		t.Errorf("agent1: expected 1 anti-pattern, got %d", stats1.AntiPatterns)
	}

	if stats2.PersonaSignals != 1 {
		t.Errorf("agent2: expected 1 persona signal, got %d", stats2.PersonaSignals)
	}
}
