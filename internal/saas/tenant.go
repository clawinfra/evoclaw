// Package saas provides multi-tenant Agent-as-a-Service functionality.
//
// Each user (tenant) can register, spawn their own E2B sandboxes, and manage
// their agents independently. Sandbox isolation ensures no cross-tenant access.
package saas

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// User represents a registered SaaS tenant.
type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	APIKey    string    `json:"api_key"`
	CreatedAt time.Time `json:"created_at"`

	// Agent configuration
	MaxAgents int `json:"max_agents"` // per-user agent limit

	// Trading credentials (encrypted at rest in production)
	HyperliquidAPIKey    string `json:"hyperliquid_api_key,omitempty"`
	HyperliquidAPISecret string `json:"hyperliquid_api_secret,omitempty"`

	// Default strategy genome
	DefaultGenome string `json:"default_genome,omitempty"`

	// Usage tracking
	TotalSandboxes int64   `json:"total_sandboxes"`
	TotalUptimeSec int64   `json:"total_uptime_sec"`
	TotalCostUSD   float64 `json:"total_cost_usd"`
	CreditLimitUSD float64 `json:"credit_limit_usd"`
}

// UserAgent tracks a user's cloud agent.
type UserAgent struct {
	SandboxID string    `json:"sandbox_id"`
	AgentID   string    `json:"agent_id"`
	UserID    string    `json:"user_id"`
	AgentType string    `json:"agent_type"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	Mode      string    `json:"mode"` // "on-demand", "scheduled", "burst"
}

// UsageReport is a per-user usage summary.
type UsageReport struct {
	UserID          string  `json:"user_id"`
	ActiveAgents    int     `json:"active_agents"`
	TotalSandboxes  int64   `json:"total_sandboxes"`
	TotalUptimeSec  int64   `json:"total_uptime_sec"`
	EstimatedCost   float64 `json:"estimated_cost_usd"`
	CreditLimit     float64 `json:"credit_limit_usd"`
	CreditRemaining float64 `json:"credit_remaining_usd"`
}

// SpawnRequest represents a user's request to spawn an agent.
type SpawnRequest struct {
	AgentID   string `json:"agent_id,omitempty"` // auto-generated if empty
	AgentType string `json:"agent_type"`
	Genome    string `json:"genome,omitempty"` // JSON strategy parameters
	Mode      string `json:"mode"`             // "on-demand", "scheduled", "burst"
	Count     int    `json:"count,omitempty"`  // for burst mode
}

// RegisterRequest represents a new user registration.
type RegisterRequest struct {
	Email                string  `json:"email"`
	MaxAgents            int     `json:"max_agents,omitempty"`
	CreditLimitUSD       float64 `json:"credit_limit_usd,omitempty"`
	HyperliquidAPIKey    string  `json:"hyperliquid_api_key,omitempty"`
	HyperliquidAPISecret string  `json:"hyperliquid_api_secret,omitempty"`
}

// TenantStore manages user registrations and agent mappings.
type TenantStore struct {
	mu     sync.RWMutex
	users  map[string]*User       // keyed by user ID
	emails map[string]string      // email → user ID
	apiKeys map[string]string     // API key → user ID
	agents map[string]*UserAgent  // sandbox ID → UserAgent
}

// NewTenantStore creates a new tenant store.
func NewTenantStore() *TenantStore {
	return &TenantStore{
		users:   make(map[string]*User),
		emails:  make(map[string]string),
		apiKeys: make(map[string]string),
		agents:  make(map[string]*UserAgent),
	}
}

// Register creates a new user account.
func (s *TenantStore) Register(req RegisterRequest) (*User, error) {
	if req.Email == "" {
		return nil, fmt.Errorf("email is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check for duplicate email
	if _, exists := s.emails[req.Email]; exists {
		return nil, fmt.Errorf("email already registered: %s", req.Email)
	}

	userID := generateID("user")
	apiKey := generateAPIKey()

	maxAgents := req.MaxAgents
	if maxAgents <= 0 {
		maxAgents = 3 // default
	}

	creditLimit := req.CreditLimitUSD
	if creditLimit <= 0 {
		creditLimit = 10.0 // default $10
	}

	user := &User{
		ID:                   userID,
		Email:                req.Email,
		APIKey:               apiKey,
		CreatedAt:            time.Now(),
		MaxAgents:            maxAgents,
		HyperliquidAPIKey:    req.HyperliquidAPIKey,
		HyperliquidAPISecret: req.HyperliquidAPISecret,
		CreditLimitUSD:       creditLimit,
	}

	s.users[userID] = user
	s.emails[req.Email] = userID
	s.apiKeys[apiKey] = userID

	return user, nil
}

// GetUser retrieves a user by ID.
func (s *TenantStore) GetUser(userID string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[userID]
	if !ok {
		return nil, fmt.Errorf("user not found: %s", userID)
	}
	return user, nil
}

// GetUserByAPIKey looks up a user by their API key.
func (s *TenantStore) GetUserByAPIKey(apiKey string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	userID, ok := s.apiKeys[apiKey]
	if !ok {
		return nil, fmt.Errorf("invalid api key")
	}

	user, ok := s.users[userID]
	if !ok {
		return nil, fmt.Errorf("user not found for api key")
	}
	return user, nil
}

// TrackAgent records that a user has spawned an agent.
func (s *TenantStore) TrackAgent(agent UserAgent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.agents[agent.SandboxID] = &agent

	if user, ok := s.users[agent.UserID]; ok {
		user.TotalSandboxes++
	}
}

// RemoveAgent removes an agent tracking entry.
func (s *TenantStore) RemoveAgent(sandboxID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.agents, sandboxID)
}

// GetUserAgents returns all agents belonging to a user.
func (s *TenantStore) GetUserAgents(userID string) []UserAgent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var agents []UserAgent
	for _, a := range s.agents {
		if a.UserID == userID {
			agents = append(agents, *a)
		}
	}
	return agents
}

// UserAgentCount returns how many active agents a user has.
func (s *TenantStore) UserAgentCount(userID string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, a := range s.agents {
		if a.UserID == userID {
			count++
		}
	}
	return count
}

// GetUsage returns usage report for a user.
func (s *TenantStore) GetUsage(userID string) (*UsageReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[userID]
	if !ok {
		return nil, fmt.Errorf("user not found: %s", userID)
	}

	activeCount := 0
	for _, a := range s.agents {
		if a.UserID == userID {
			activeCount++
		}
	}

	return &UsageReport{
		UserID:          userID,
		ActiveAgents:    activeCount,
		TotalSandboxes:  user.TotalSandboxes,
		TotalUptimeSec:  user.TotalUptimeSec,
		EstimatedCost:   user.TotalCostUSD,
		CreditLimit:     user.CreditLimitUSD,
		CreditRemaining: user.CreditLimitUSD - user.TotalCostUSD,
	}, nil
}

// IsUserOverLimit checks if a user has exceeded their agent limit.
func (s *TenantStore) IsUserOverLimit(userID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[userID]
	if !ok {
		return true // unknown user is over limit
	}

	count := 0
	for _, a := range s.agents {
		if a.UserID == userID {
			count++
		}
	}

	return count >= user.MaxAgents
}

// IsUserOverBudget checks if a user has exceeded their credit limit.
func (s *TenantStore) IsUserOverBudget(userID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[userID]
	if !ok {
		return true
	}

	return user.TotalCostUSD >= user.CreditLimitUSD
}

// UpdateUserCost adds cost to a user's running total.
func (s *TenantStore) UpdateUserCost(userID string, costUSD float64, uptimeSec int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if user, ok := s.users[userID]; ok {
		user.TotalCostUSD += costUSD
		user.TotalUptimeSec += uptimeSec
	}
}

// ListUsers returns all registered users.
func (s *TenantStore) ListUsers() []User {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]User, 0, len(s.users))
	for _, u := range s.users {
		users = append(users, *u)
	}
	return users
}

// UserCount returns the total number of registered users.
func (s *TenantStore) UserCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.users)
}

// generateID creates a prefixed random hex ID.
func generateID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(b))
}

// generateAPIKey creates a random API key.
func generateAPIKey() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("evo_%d", time.Now().UnixNano())
	}
	return "evo_" + hex.EncodeToString(b)
}
