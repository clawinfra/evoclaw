// Package cloud provides E2B sandbox integration for EvoClaw.
//
// E2B (https://e2b.dev) provides Firecracker microVM sandboxes with ~100ms
// cold start times. This package implements a Go client for the E2B REST API,
// enabling the orchestrator to spawn, manage, and terminate edge agents as
// cloud sandboxes.
package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	// DefaultE2BBaseURL is the E2B API endpoint.
	DefaultE2BBaseURL = "https://api.e2b.dev"

	// DefaultTimeout for sandbox creation (seconds).
	DefaultSandboxTimeoutSec = 300

	// DefaultKeepAliveSec is how often to ping sandboxes.
	DefaultKeepAliveSec = 60

	// SandboxStateRunning indicates the sandbox is active.
	SandboxStateRunning = "running"

	// SandboxStatePaused indicates the sandbox is paused.
	SandboxStatePaused = "paused"
)

// AgentConfig holds the configuration needed to spawn a cloud agent.
type AgentConfig struct {
	// TemplateID is the E2B template to use (e.g., "evoclaw-agent").
	TemplateID string `json:"template_id"`

	// AgentID is the unique identifier for this agent instance.
	AgentID string `json:"agent_id"`

	// AgentType is the agent role: "trader", "monitor", "sensor", etc.
	AgentType string `json:"agent_type"`

	// MQTTBroker is the MQTT broker address the agent connects to.
	MQTTBroker string `json:"mqtt_broker"`

	// MQTTPort is the MQTT broker port.
	MQTTPort int `json:"mqtt_port"`

	// OrchestratorURL is the URL of the orchestrator API.
	OrchestratorURL string `json:"orchestrator_url"`

	// TimeoutSec is how long the sandbox stays alive (default 300).
	TimeoutSec int `json:"timeout_sec,omitempty"`

	// Environment variables injected into the sandbox.
	EnvVars map[string]string `json:"env_vars,omitempty"`

	// Metadata key-value pairs for sandbox tagging/filtering.
	Metadata map[string]string `json:"metadata,omitempty"`

	// Genome is the JSON-encoded strategy parameters.
	Genome string `json:"genome,omitempty"`

	// UserID is the owning user (for SaaS mode).
	UserID string `json:"user_id,omitempty"`
}

// Sandbox represents a running E2B sandbox instance.
type Sandbox struct {
	// SandboxID is the E2B-assigned sandbox identifier.
	SandboxID string `json:"sandbox_id"`

	// TemplateID is the template this sandbox was created from.
	TemplateID string `json:"template_id"`

	// AgentID is the EvoClaw agent running in this sandbox.
	AgentID string `json:"agent_id"`

	// ClientID is the host portion of the sandbox URL.
	ClientID string `json:"client_id"`

	// State is the current sandbox state ("running" or "paused").
	State string `json:"state"`

	// StartedAt is when the sandbox was created.
	StartedAt time.Time `json:"started_at"`

	// EndsAt is when the sandbox will auto-terminate.
	EndsAt time.Time `json:"ends_at"`

	// Metadata holds user-defined key-value pairs.
	Metadata map[string]string `json:"metadata,omitempty"`

	// UserID is the owning user (SaaS mode).
	UserID string `json:"user_id,omitempty"`
}

// Status represents the health status of a sandbox agent.
type Status struct {
	SandboxID string `json:"sandbox_id"`
	AgentID   string `json:"agent_id"`
	State     string `json:"state"`
	Healthy   bool   `json:"healthy"`
	UptimeSec int64  `json:"uptime_sec"`
	EndsAt    time.Time `json:"ends_at"`
}

// Command represents a command to execute in a sandbox.
type Command struct {
	Cmd     string `json:"cmd"`
	Args    []string `json:"args,omitempty"`
	Timeout int    `json:"timeout,omitempty"` // seconds
}

// CommandResponse holds the result of executing a command in a sandbox.
type CommandResponse struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// E2BClient is a Go client for the E2B REST API.
type E2BClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	mu         sync.RWMutex
	// Track running sandboxes locally for fast lookups.
	sandboxes map[string]*Sandbox
}

// HTTPClient interface for testing.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// NewE2BClient creates a new E2B API client.
func NewE2BClient(apiKey string) *E2BClient {
	return &E2BClient{
		apiKey:  apiKey,
		baseURL: DefaultE2BBaseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		sandboxes: make(map[string]*Sandbox),
	}
}

// NewE2BClientWithHTTP creates a client with a custom HTTP client (for testing).
func NewE2BClientWithHTTP(apiKey string, httpClient *http.Client) *E2BClient {
	c := NewE2BClient(apiKey)
	c.httpClient = httpClient
	return c
}

// SetBaseURL overrides the E2B API base URL (useful for testing).
func (c *E2BClient) SetBaseURL(url string) {
	c.baseURL = url
}

// e2bCreateRequest is the request body for creating a sandbox.
type e2bCreateRequest struct {
	TemplateID string            `json:"templateID"`
	Timeout    int               `json:"timeout,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	EnvVars    map[string]string `json:"envVars,omitempty"`
}

// e2bSandboxResponse is the E2B API response for sandbox operations.
type e2bSandboxResponse struct {
	SandboxID  string            `json:"sandboxID"`
	TemplateID string            `json:"templateID"`
	ClientID   string            `json:"clientID"`
	StartedAt  string            `json:"startedAt"`
	EndAt      string            `json:"endAt"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// e2bListResponse wraps the list of sandboxes returned by E2B.
type e2bListResponse []e2bSandboxResponse

// e2bProcessRequest is the request body for executing a process in a sandbox.
type e2bProcessRequest struct {
	Cmd  string   `json:"cmd"`
	Args []string `json:"args,omitempty"`
}

// e2bProcessResponse is the E2B response for process execution.
type e2bProcessResponse struct {
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// e2bErrorResponse represents an E2B API error.
type e2bErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// SpawnAgent creates a new E2B sandbox running an EvoClaw edge agent.
func (c *E2BClient) SpawnAgent(ctx context.Context, config AgentConfig) (*Sandbox, error) {
	if config.TemplateID == "" {
		return nil, fmt.Errorf("template_id is required")
	}
	if config.AgentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}

	timeout := config.TimeoutSec
	if timeout <= 0 {
		timeout = DefaultSandboxTimeoutSec
	}

	// Build environment variables
	envVars := make(map[string]string)
	for k, v := range config.EnvVars {
		envVars[k] = v
	}
	envVars["EVOCLAW_AGENT_ID"] = config.AgentID
	envVars["EVOCLAW_AGENT_TYPE"] = config.AgentType
	if config.MQTTBroker != "" {
		envVars["MQTT_BROKER"] = config.MQTTBroker
	}
	if config.MQTTPort > 0 {
		envVars["MQTT_PORT"] = fmt.Sprintf("%d", config.MQTTPort)
	}
	if config.OrchestratorURL != "" {
		envVars["ORCHESTRATOR_URL"] = config.OrchestratorURL
	}
	if config.Genome != "" {
		envVars["EVOCLAW_GENOME"] = config.Genome
	}

	// Build metadata
	metadata := make(map[string]string)
	for k, v := range config.Metadata {
		metadata[k] = v
	}
	metadata["evoclaw_agent_id"] = config.AgentID
	metadata["evoclaw_agent_type"] = config.AgentType
	if config.UserID != "" {
		metadata["evoclaw_user_id"] = config.UserID
	}

	reqBody := e2bCreateRequest{
		TemplateID: config.TemplateID,
		Timeout:    timeout,
		Metadata:   metadata,
		EnvVars:    envVars,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/sandboxes", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("e2b api call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, c.parseError(resp)
	}

	var e2bResp e2bSandboxResponse
	if err := json.NewDecoder(resp.Body).Decode(&e2bResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	sandbox := c.toSandbox(e2bResp, config.AgentID, config.UserID)

	// Track locally
	c.mu.Lock()
	c.sandboxes[sandbox.SandboxID] = sandbox
	c.mu.Unlock()

	return sandbox, nil
}

// KillAgent terminates a running sandbox.
func (c *E2BClient) KillAgent(ctx context.Context, sandboxID string) error {
	if sandboxID == "" {
		return fmt.Errorf("sandbox_id is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		c.baseURL+"/sandboxes/"+sandboxID, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("e2b api call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return c.parseError(resp)
	}

	// Remove from local tracking
	c.mu.Lock()
	delete(c.sandboxes, sandboxID)
	c.mu.Unlock()

	return nil
}

// ListAgents returns all running E2B sandboxes tagged as EvoClaw agents.
func (c *E2BClient) ListAgents(ctx context.Context) ([]Sandbox, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/sandboxes", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("e2b api call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var e2bResp e2bListResponse
	if err := json.NewDecoder(resp.Body).Decode(&e2bResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	var sandboxes []Sandbox
	for _, s := range e2bResp {
		agentID := s.Metadata["evoclaw_agent_id"]
		userID := s.Metadata["evoclaw_user_id"]
		sandbox := c.toSandbox(s, agentID, userID)
		sandboxes = append(sandboxes, *sandbox)

		// Update local cache
		c.mu.Lock()
		c.sandboxes[sandbox.SandboxID] = sandbox
		c.mu.Unlock()
	}

	return sandboxes, nil
}

// GetAgentStatus returns the health status of a sandbox agent.
func (c *E2BClient) GetAgentStatus(ctx context.Context, sandboxID string) (*Status, error) {
	if sandboxID == "" {
		return nil, fmt.Errorf("sandbox_id is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/sandboxes/"+sandboxID, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("e2b api call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var e2bResp e2bSandboxResponse
	if err := json.NewDecoder(resp.Body).Decode(&e2bResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	startedAt, _ := time.Parse(time.RFC3339, e2bResp.StartedAt)
	endsAt, _ := time.Parse(time.RFC3339, e2bResp.EndAt)
	agentID := e2bResp.Metadata["evoclaw_agent_id"]

	status := &Status{
		SandboxID: e2bResp.SandboxID,
		AgentID:   agentID,
		State:     SandboxStateRunning,
		Healthy:   true,
		UptimeSec: int64(time.Since(startedAt).Seconds()),
		EndsAt:    endsAt,
	}

	return status, nil
}

// SendCommand executes a command inside a running sandbox.
func (c *E2BClient) SendCommand(ctx context.Context, sandboxID string, cmd Command) (*CommandResponse, error) {
	if sandboxID == "" {
		return nil, fmt.Errorf("sandbox_id is required")
	}
	if cmd.Cmd == "" {
		return nil, fmt.Errorf("cmd is required")
	}

	reqBody := e2bProcessRequest{
		Cmd:  cmd.Cmd,
		Args: cmd.Args,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/sandboxes/"+sandboxID+"/process", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("e2b api call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var e2bResp e2bProcessResponse
	if err := json.NewDecoder(resp.Body).Decode(&e2bResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &CommandResponse{
		ExitCode: e2bResp.ExitCode,
		Stdout:   e2bResp.Stdout,
		Stderr:   e2bResp.Stderr,
	}, nil
}

// SetTimeout extends or changes the sandbox timeout.
func (c *E2BClient) SetTimeout(ctx context.Context, sandboxID string, timeoutSec int) error {
	if sandboxID == "" {
		return fmt.Errorf("sandbox_id is required")
	}

	body, err := json.Marshal(map[string]int{"timeout": timeoutSec})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/sandboxes/"+sandboxID+"/timeout", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("e2b api call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return c.parseError(resp)
	}

	return nil
}

// GetLocalSandbox retrieves a sandbox from local cache without an API call.
func (c *E2BClient) GetLocalSandbox(sandboxID string) (*Sandbox, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s, ok := c.sandboxes[sandboxID]
	return s, ok
}

// LocalSandboxCount returns the number of locally tracked sandboxes.
func (c *E2BClient) LocalSandboxCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.sandboxes)
}

// setHeaders adds common headers to E2B API requests.
func (c *E2BClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.apiKey)
}

// parseError extracts an error message from an E2B API error response.
func (c *E2BClient) parseError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	var e2bErr e2bErrorResponse
	if err := json.Unmarshal(body, &e2bErr); err == nil && e2bErr.Message != "" {
		return fmt.Errorf("e2b api error (HTTP %d): %s", resp.StatusCode, e2bErr.Message)
	}

	return fmt.Errorf("e2b api error (HTTP %d): %s", resp.StatusCode, string(body))
}

// toSandbox converts an E2B API response to our Sandbox type.
func (c *E2BClient) toSandbox(resp e2bSandboxResponse, agentID, userID string) *Sandbox {
	startedAt, _ := time.Parse(time.RFC3339, resp.StartedAt)
	endsAt, _ := time.Parse(time.RFC3339, resp.EndAt)

	return &Sandbox{
		SandboxID:  resp.SandboxID,
		TemplateID: resp.TemplateID,
		AgentID:    agentID,
		ClientID:   resp.ClientID,
		State:      SandboxStateRunning,
		StartedAt:  startedAt,
		EndsAt:     endsAt,
		Metadata:   resp.Metadata,
		UserID:     userID,
	}
}
