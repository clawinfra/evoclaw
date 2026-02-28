package saas

import (
	"strings"
	"testing"
)

func TestNewTenantStore(t *testing.T) {
	store := NewTenantStore()
	if store == nil {
		t.Fatal("expected non-nil store")
	}
	if store.UserCount() != 0 {
		t.Errorf("expected 0 users, got %d", store.UserCount())
	}
}

func TestRegister(t *testing.T) {
	store := NewTenantStore()

	user, err := store.Register(RegisterRequest{
		Email:     "test@example.com",
		MaxAgents: 5,
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if user.ID == "" {
		t.Error("expected non-empty user ID")
	}
	if user.Email != "test@example.com" {
		t.Errorf("expected email 'test@example.com', got '%s'", user.Email)
	}
	if user.APIKey == "" {
		t.Error("expected non-empty API key")
	}
	if !strings.HasPrefix(user.APIKey, "evo_") {
		t.Errorf("expected API key to start with 'evo_', got '%s'", user.APIKey)
	}
	if user.MaxAgents != 5 {
		t.Errorf("expected max agents 5, got %d", user.MaxAgents)
	}
	if store.UserCount() != 1 {
		t.Errorf("expected 1 user, got %d", store.UserCount())
	}
}

func TestRegister_DefaultLimits(t *testing.T) {
	store := NewTenantStore()

	user, err := store.Register(RegisterRequest{
		Email: "defaults@test.com",
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if user.MaxAgents != 3 {
		t.Errorf("expected default max agents 3, got %d", user.MaxAgents)
	}
	if user.CreditLimitUSD != 10.0 {
		t.Errorf("expected default credit limit 10.0, got %f", user.CreditLimitUSD)
	}
}

func TestRegister_EmptyEmail(t *testing.T) {
	store := NewTenantStore()

	_, err := store.Register(RegisterRequest{})
	if err == nil {
		t.Fatal("expected error for empty email")
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	store := NewTenantStore()

	_, err := store.Register(RegisterRequest{Email: "dupe@test.com"})
	if err != nil {
		t.Fatalf("First register failed: %v", err)
	}

	_, err = store.Register(RegisterRequest{Email: "dupe@test.com"})
	if err == nil {
		t.Fatal("expected error for duplicate email")
	}
}

func TestGetUser(t *testing.T) {
	store := NewTenantStore()

	user, _ := store.Register(RegisterRequest{Email: "get@test.com"})

	fetched, err := store.GetUser(user.ID)
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}
	if fetched.Email != "get@test.com" {
		t.Errorf("expected email 'get@test.com', got '%s'", fetched.Email)
	}
}

func TestGetUser_NotFound(t *testing.T) {
	store := NewTenantStore()

	_, err := store.GetUser("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent user")
	}
}

func TestGetUserByAPIKey(t *testing.T) {
	store := NewTenantStore()

	user, _ := store.Register(RegisterRequest{Email: "apikey@test.com"})

	fetched, err := store.GetUserByAPIKey(user.APIKey)
	if err != nil {
		t.Fatalf("GetUserByAPIKey failed: %v", err)
	}
	if fetched.ID != user.ID {
		t.Errorf("expected user ID '%s', got '%s'", user.ID, fetched.ID)
	}
}

func TestGetUserByAPIKey_Invalid(t *testing.T) {
	store := NewTenantStore()

	_, err := store.GetUserByAPIKey("invalid-key")
	if err == nil {
		t.Fatal("expected error for invalid API key")
	}
}

func TestTrackAgent(t *testing.T) {
	store := NewTenantStore()

	user, _ := store.Register(RegisterRequest{Email: "track@test.com"})

	agent := UserAgent{
		SandboxID: "sb-1",
		AgentID:   "agent-1",
		UserID:    user.ID,
		AgentType: "trader",
		Status:    "running",
		Mode:      "on-demand",
	}
	store.TrackAgent(agent)

	agents := store.GetUserAgents(user.ID)
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].SandboxID != "sb-1" {
		t.Errorf("expected sandbox ID 'sb-1', got '%s'", agents[0].SandboxID)
	}

	// Check total sandboxes incremented
	updated, _ := store.GetUser(user.ID)
	if updated.TotalSandboxes != 1 {
		t.Errorf("expected 1 total sandbox, got %d", updated.TotalSandboxes)
	}
}

func TestRemoveAgent(t *testing.T) {
	store := NewTenantStore()

	user, _ := store.Register(RegisterRequest{Email: "remove@test.com"})
	store.TrackAgent(UserAgent{SandboxID: "sb-rm", UserID: user.ID})

	store.RemoveAgent("sb-rm")

	agents := store.GetUserAgents(user.ID)
	if len(agents) != 0 {
		t.Errorf("expected 0 agents after removal, got %d", len(agents))
	}
}

func TestGetUserAgents_Empty(t *testing.T) {
	store := NewTenantStore()

	agents := store.GetUserAgents("nonexistent-user")
	if len(agents) > 0 {
		t.Error("expected nil/empty agents for nonexistent user")
	}
}

func TestUserAgentCount(t *testing.T) {
	store := NewTenantStore()

	user, _ := store.Register(RegisterRequest{Email: "count@test.com"})

	if store.UserAgentCount(user.ID) != 0 {
		t.Error("expected 0 agents initially")
	}

	store.TrackAgent(UserAgent{SandboxID: "sb-c1", UserID: user.ID})
	store.TrackAgent(UserAgent{SandboxID: "sb-c2", UserID: user.ID})

	if store.UserAgentCount(user.ID) != 2 {
		t.Errorf("expected 2 agents, got %d", store.UserAgentCount(user.ID))
	}
}

func TestGetUsage(t *testing.T) {
	store := NewTenantStore()

	user, _ := store.Register(RegisterRequest{
		Email:          "usage@test.com",
		CreditLimitUSD: 20.0,
	})

	store.TrackAgent(UserAgent{SandboxID: "sb-u1", UserID: user.ID})

	usage, err := store.GetUsage(user.ID)
	if err != nil {
		t.Fatalf("GetUsage failed: %v", err)
	}
	if usage.ActiveAgents != 1 {
		t.Errorf("expected 1 active agent, got %d", usage.ActiveAgents)
	}
	if usage.CreditLimit != 20.0 {
		t.Errorf("expected credit limit 20.0, got %f", usage.CreditLimit)
	}
	if usage.CreditRemaining != 20.0 {
		t.Errorf("expected 20.0 remaining, got %f", usage.CreditRemaining)
	}
}

func TestGetUsage_NotFound(t *testing.T) {
	store := NewTenantStore()

	_, err := store.GetUsage("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent user")
	}
}

func TestIsUserOverLimit(t *testing.T) {
	store := NewTenantStore()

	user, _ := store.Register(RegisterRequest{
		Email:     "limit@test.com",
		MaxAgents: 2,
	})

	if store.IsUserOverLimit(user.ID) {
		t.Error("should not be over limit initially")
	}

	store.TrackAgent(UserAgent{SandboxID: "sb-l1", UserID: user.ID})
	store.TrackAgent(UserAgent{SandboxID: "sb-l2", UserID: user.ID})

	if !store.IsUserOverLimit(user.ID) {
		t.Error("should be over limit with 2/2 agents")
	}
}

func TestIsUserOverLimit_UnknownUser(t *testing.T) {
	store := NewTenantStore()

	if !store.IsUserOverLimit("unknown") {
		t.Error("unknown user should be over limit")
	}
}

func TestIsUserOverBudget(t *testing.T) {
	store := NewTenantStore()

	user, _ := store.Register(RegisterRequest{
		Email:          "budget@test.com",
		CreditLimitUSD: 5.0,
	})

	if store.IsUserOverBudget(user.ID) {
		t.Error("should not be over budget initially")
	}

	store.UpdateUserCost(user.ID, 5.0, 3600)

	if !store.IsUserOverBudget(user.ID) {
		t.Error("should be over budget after $5 spent")
	}
}

func TestIsUserOverBudget_UnknownUser(t *testing.T) {
	store := NewTenantStore()

	if !store.IsUserOverBudget("unknown") {
		t.Error("unknown user should be over budget")
	}
}

func TestUpdateUserCost(t *testing.T) {
	store := NewTenantStore()

	user, _ := store.Register(RegisterRequest{Email: "cost@test.com"})

	store.UpdateUserCost(user.ID, 1.5, 1800)
	store.UpdateUserCost(user.ID, 0.5, 600)

	updated, _ := store.GetUser(user.ID)
	if updated.TotalCostUSD != 2.0 {
		t.Errorf("expected cost 2.0, got %f", updated.TotalCostUSD)
	}
	if updated.TotalUptimeSec != 2400 {
		t.Errorf("expected uptime 2400, got %d", updated.TotalUptimeSec)
	}
}

func TestUpdateUserCost_UnknownUser(t *testing.T) {
	store := NewTenantStore()
	// Should not panic
	store.UpdateUserCost("unknown", 1.0, 60)
}

func TestListUsers(t *testing.T) {
	store := NewTenantStore()

	store.Register(RegisterRequest{Email: "list1@test.com"})
	store.Register(RegisterRequest{Email: "list2@test.com"})

	users := store.ListUsers()
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}
}

func TestGenerateID(t *testing.T) {
	id1 := generateID("test")
	id2 := generateID("test")

	if id1 == id2 {
		t.Error("expected unique IDs")
	}
	if !strings.HasPrefix(id1, "test-") {
		t.Errorf("expected 'test-' prefix, got '%s'", id1)
	}
}

func TestGenerateAPIKey(t *testing.T) {
	key1 := generateAPIKey()
	key2 := generateAPIKey()

	if key1 == key2 {
		t.Error("expected unique API keys")
	}
	if !strings.HasPrefix(key1, "evo_") {
		t.Errorf("expected 'evo_' prefix, got '%s'", key1)
	}
}

func TestRegister_WithCredentials(t *testing.T) {
	store := NewTenantStore()

	user, err := store.Register(RegisterRequest{
		Email:                "creds@test.com",
		HyperliquidAPIKey:    "hl-key-123",
		HyperliquidAPISecret: "hl-secret-456",
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if user.HyperliquidAPIKey != "hl-key-123" {
		t.Error("expected hyperliquid API key to be set")
	}
	if user.HyperliquidAPISecret != "hl-secret-456" {
		t.Error("expected hyperliquid API secret to be set")
	}
}

func TestTrackAgent_IncrementsTotalSandboxes(t *testing.T) {
	store := NewTenantStore()

	user, _ := store.Register(RegisterRequest{Email: "inc@test.com"})

	for i := 0; i < 3; i++ {
		store.TrackAgent(UserAgent{
			SandboxID: generateID("sb"),
			UserID:    user.ID,
		})
	}

	updated, _ := store.GetUser(user.ID)
	if updated.TotalSandboxes != 3 {
		t.Errorf("expected 3 total sandboxes, got %d", updated.TotalSandboxes)
	}
}

func TestMultipleUsersIsolation(t *testing.T) {
	store := NewTenantStore()

	user1, _ := store.Register(RegisterRequest{Email: "user1@test.com"})
	user2, _ := store.Register(RegisterRequest{Email: "user2@test.com"})

	store.TrackAgent(UserAgent{SandboxID: "sb-u1-1", UserID: user1.ID})
	store.TrackAgent(UserAgent{SandboxID: "sb-u1-2", UserID: user1.ID})
	store.TrackAgent(UserAgent{SandboxID: "sb-u2-1", UserID: user2.ID})

	u1Agents := store.GetUserAgents(user1.ID)
	u2Agents := store.GetUserAgents(user2.ID)

	if len(u1Agents) != 2 {
		t.Errorf("expected 2 agents for user1, got %d", len(u1Agents))
	}
	if len(u2Agents) != 1 {
		t.Errorf("expected 1 agent for user2, got %d", len(u2Agents))
	}
}
