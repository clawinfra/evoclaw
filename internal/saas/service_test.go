package saas

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/clawinfra/evoclaw/internal/cloud"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func newMockE2BServer() *httptest.Server {
	var sandboxCounter atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("POST /sandboxes", func(w http.ResponseWriter, r *http.Request) {
		idx := sandboxCounter.Add(1)
		now := time.Now()
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"sandboxID": fmt.Sprintf("sb-saas-%d", idx),
			"startedAt": now.Format(time.RFC3339),
			"endAt":     now.Add(5 * time.Minute).Format(time.RFC3339),
		})
	})
	mux.HandleFunc("GET /sandboxes", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]interface{}{})
	})
	mux.HandleFunc("DELETE /sandboxes/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /sandboxes/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	return httptest.NewServer(mux)
}

func newTestService(t *testing.T) (*Service, *httptest.Server) {
	t.Helper()

	mockE2B := newMockE2BServer()

	config := cloud.DefaultManagerConfig()
	config.E2BAPIKey = "test-key"
	config.MaxAgents = 20
	config.HealthCheckIntervalSec = 3600
	config.KeepAliveIntervalSec = 3600

	client := cloud.NewE2BClient(config.E2BAPIKey)
	client.SetBaseURL(mockE2B.URL)
	mgr := cloud.NewManagerWithClient(config, client, newTestLogger())

	store := NewTenantStore()
	svc := NewService(store, mgr, newTestLogger())

	return svc, mockE2B
}

func TestNewService(t *testing.T) {
	svc, mockE2B := newTestService(t)
	defer mockE2B.Close()

	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestServiceRegister(t *testing.T) {
	svc, mockE2B := newTestService(t)
	defer mockE2B.Close()

	user, err := svc.Register(RegisterRequest{
		Email:     "svc@test.com",
		MaxAgents: 5,
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if user.Email != "svc@test.com" {
		t.Errorf("expected email 'svc@test.com', got '%s'", user.Email)
	}
}

func TestServiceRegister_Error(t *testing.T) {
	svc, mockE2B := newTestService(t)
	defer mockE2B.Close()

	_, err := svc.Register(RegisterRequest{})
	if err == nil {
		t.Fatal("expected error for empty email")
	}
}

func TestServiceSpawnAgent(t *testing.T) {
	svc, mockE2B := newTestService(t)
	defer mockE2B.Close()

	user, _ := svc.Register(RegisterRequest{
		Email:     "spawn@test.com",
		MaxAgents: 5,
	})

	agent, err := svc.SpawnAgent(context.Background(), user.ID, SpawnRequest{
		AgentType: "trader",
		Mode:      "on-demand",
	})
	if err != nil {
		t.Fatalf("SpawnAgent failed: %v", err)
	}

	if agent.UserID != user.ID {
		t.Errorf("expected user ID '%s', got '%s'", user.ID, agent.UserID)
	}
	if agent.AgentType != "trader" {
		t.Errorf("expected agent type 'trader', got '%s'", agent.AgentType)
	}
	if agent.Mode != "on-demand" {
		t.Errorf("expected mode 'on-demand', got '%s'", agent.Mode)
	}
}

func TestServiceSpawnAgent_WithCredentials(t *testing.T) {
	svc, mockE2B := newTestService(t)
	defer mockE2B.Close()

	user, _ := svc.Register(RegisterRequest{
		Email:                "creds@test.com",
		HyperliquidAPIKey:    "hl-key",
		HyperliquidAPISecret: "hl-secret",
	})

	agent, err := svc.SpawnAgent(context.Background(), user.ID, SpawnRequest{
		AgentType: "trader",
		Genome:    `{"type":"momentum"}`,
	})
	if err != nil {
		t.Fatalf("SpawnAgent failed: %v", err)
	}

	if agent.AgentID == "" {
		t.Error("expected auto-generated agent ID")
	}
}

func TestServiceSpawnAgent_DefaultGenome(t *testing.T) {
	svc, mockE2B := newTestService(t)
	defer mockE2B.Close()

	user, _ := svc.Register(RegisterRequest{Email: "genome@test.com"})
	// Set default genome via direct store access
	svc.store.mu.Lock()
	svc.store.users[user.ID].DefaultGenome = `{"type":"mean_revert"}`
	svc.store.mu.Unlock()

	agent, err := svc.SpawnAgent(context.Background(), user.ID, SpawnRequest{
		AgentType: "trader",
	})
	if err != nil {
		t.Fatalf("SpawnAgent failed: %v", err)
	}
	if agent.SandboxID == "" {
		t.Error("expected non-empty sandbox ID")
	}
}

func TestServiceSpawnAgent_UserNotFound(t *testing.T) {
	svc, mockE2B := newTestService(t)
	defer mockE2B.Close()

	_, err := svc.SpawnAgent(context.Background(), "nonexistent", SpawnRequest{})
	if err == nil {
		t.Fatal("expected error for nonexistent user")
	}
}

func TestServiceSpawnAgent_OverLimit(t *testing.T) {
	svc, mockE2B := newTestService(t)
	defer mockE2B.Close()

	user, _ := svc.Register(RegisterRequest{
		Email:     "limit@test.com",
		MaxAgents: 1,
	})

	// Spawn one agent
	_, err := svc.SpawnAgent(context.Background(), user.ID, SpawnRequest{AgentType: "trader"})
	if err != nil {
		t.Fatalf("First spawn failed: %v", err)
	}

	// Second should fail
	_, err = svc.SpawnAgent(context.Background(), user.ID, SpawnRequest{AgentType: "trader"})
	if err == nil {
		t.Fatal("expected error for exceeding limit")
	}
}

func TestServiceSpawnAgent_OverBudget(t *testing.T) {
	svc, mockE2B := newTestService(t)
	defer mockE2B.Close()

	user, _ := svc.Register(RegisterRequest{
		Email:          "budget@test.com",
		CreditLimitUSD: 1.0,
	})

	svc.store.UpdateUserCost(user.ID, 1.0, 3600)

	_, err := svc.SpawnAgent(context.Background(), user.ID, SpawnRequest{AgentType: "trader"})
	if err == nil {
		t.Fatal("expected error for exceeded budget")
	}
}

func TestServiceSpawnBurst(t *testing.T) {
	svc, mockE2B := newTestService(t)
	defer mockE2B.Close()

	user, _ := svc.Register(RegisterRequest{
		Email:     "burst@test.com",
		MaxAgents: 5,
	})

	agent, err := svc.SpawnAgent(context.Background(), user.ID, SpawnRequest{
		AgentType: "trader",
		Mode:      "burst",
		Count:     3,
	})
	if err != nil {
		t.Fatalf("SpawnBurst failed: %v", err)
	}

	if agent.Mode != "burst" {
		t.Errorf("expected mode 'burst', got '%s'", agent.Mode)
	}

	// Should have 3 agents tracked
	agents, _ := svc.ListAgents(user.ID)
	if len(agents) != 3 {
		t.Errorf("expected 3 agents, got %d", len(agents))
	}
}

func TestServiceSpawnBurst_ExceedsLimit(t *testing.T) {
	svc, mockE2B := newTestService(t)
	defer mockE2B.Close()

	user, _ := svc.Register(RegisterRequest{
		Email:     "burst-limit@test.com",
		MaxAgents: 2,
	})

	// Request 5 but only 2 available
	agent, err := svc.SpawnAgent(context.Background(), user.ID, SpawnRequest{
		Mode:  "burst",
		Count: 5,
	})
	if err != nil {
		t.Fatalf("Burst spawn failed: %v", err)
	}
	if agent == nil {
		t.Fatal("expected non-nil agent")
	}

	agents, _ := svc.ListAgents(user.ID)
	if len(agents) != 2 {
		t.Errorf("expected 2 agents (limited), got %d", len(agents))
	}
}

func TestServiceSpawnBurst_NoSlots(t *testing.T) {
	svc, mockE2B := newTestService(t)
	defer mockE2B.Close()

	user, _ := svc.Register(RegisterRequest{
		Email:     "no-slots@test.com",
		MaxAgents: 1,
	})

	// Fill the slot
	svc.SpawnAgent(context.Background(), user.ID, SpawnRequest{AgentType: "trader"})

	// Burst should fail
	_, err := svc.SpawnAgent(context.Background(), user.ID, SpawnRequest{
		Mode:  "burst",
		Count: 2,
	})
	if err == nil {
		t.Fatal("expected error when no slots available")
	}
}

func TestServiceKillAgent(t *testing.T) {
	svc, mockE2B := newTestService(t)
	defer mockE2B.Close()

	user, _ := svc.Register(RegisterRequest{Email: "kill@test.com"})
	agent, _ := svc.SpawnAgent(context.Background(), user.ID, SpawnRequest{AgentType: "trader"})

	err := svc.KillAgent(context.Background(), user.ID, agent.SandboxID)
	if err != nil {
		t.Fatalf("KillAgent failed: %v", err)
	}

	agents, _ := svc.ListAgents(user.ID)
	if len(agents) != 0 {
		t.Errorf("expected 0 agents after kill, got %d", len(agents))
	}
}

func TestServiceKillAgent_NotOwned(t *testing.T) {
	svc, mockE2B := newTestService(t)
	defer mockE2B.Close()

	user1, _ := svc.Register(RegisterRequest{Email: "owner@test.com"})
	user2, _ := svc.Register(RegisterRequest{Email: "other@test.com"})

	agent, _ := svc.SpawnAgent(context.Background(), user1.ID, SpawnRequest{AgentType: "trader"})

	// User2 should not be able to kill user1's agent
	err := svc.KillAgent(context.Background(), user2.ID, agent.SandboxID)
	if err == nil {
		t.Fatal("expected error for non-owned agent")
	}
}

func TestServiceListAgents(t *testing.T) {
	svc, mockE2B := newTestService(t)
	defer mockE2B.Close()

	user, _ := svc.Register(RegisterRequest{Email: "list@test.com", MaxAgents: 5})

	svc.SpawnAgent(context.Background(), user.ID, SpawnRequest{AgentType: "trader"})
	svc.SpawnAgent(context.Background(), user.ID, SpawnRequest{AgentType: "monitor"})

	agents, err := svc.ListAgents(user.ID)
	if err != nil {
		t.Fatalf("ListAgents failed: %v", err)
	}
	if len(agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(agents))
	}
}

func TestServiceListAgents_NotFound(t *testing.T) {
	svc, mockE2B := newTestService(t)
	defer mockE2B.Close()

	_, err := svc.ListAgents("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent user")
	}
}

func TestServiceGetUsage(t *testing.T) {
	svc, mockE2B := newTestService(t)
	defer mockE2B.Close()

	user, _ := svc.Register(RegisterRequest{Email: "usage@test.com"})

	usage, err := svc.GetUsage(user.ID)
	if err != nil {
		t.Fatalf("GetUsage failed: %v", err)
	}
	if usage.UserID != user.ID {
		t.Errorf("expected user ID '%s', got '%s'", user.ID, usage.UserID)
	}
}

func TestServiceAuthenticateAPIKey(t *testing.T) {
	svc, mockE2B := newTestService(t)
	defer mockE2B.Close()

	user, _ := svc.Register(RegisterRequest{Email: "auth@test.com"})

	authed, err := svc.AuthenticateAPIKey(user.APIKey)
	if err != nil {
		t.Fatalf("AuthenticateAPIKey failed: %v", err)
	}
	if authed.ID != user.ID {
		t.Errorf("expected user ID '%s', got '%s'", user.ID, authed.ID)
	}
}

func TestServiceAuthenticateAPIKey_Invalid(t *testing.T) {
	svc, mockE2B := newTestService(t)
	defer mockE2B.Close()

	_, err := svc.AuthenticateAPIKey("invalid")
	if err == nil {
		t.Fatal("expected error for invalid API key")
	}
}

func TestServiceSpawnAgent_DefaultType(t *testing.T) {
	svc, mockE2B := newTestService(t)
	defer mockE2B.Close()

	user, _ := svc.Register(RegisterRequest{Email: "default-type@test.com"})

	agent, err := svc.SpawnAgent(context.Background(), user.ID, SpawnRequest{})
	if err != nil {
		t.Fatalf("SpawnAgent failed: %v", err)
	}

	if agent.AgentType != "trader" {
		t.Errorf("expected default type 'trader', got '%s'", agent.AgentType)
	}
	if agent.Mode != "on-demand" {
		t.Errorf("expected default mode 'on-demand', got '%s'", agent.Mode)
	}
}
