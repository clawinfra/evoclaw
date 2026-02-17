package router

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ModelState represents the health state of a model.
type ModelState string

const (
	StateHealthy  ModelState = "healthy"
	StateDegraded ModelState = "degraded"
	StateUnknown  ModelState = "unknown"
)

// Common error types for classification
const (
	ErrQuotaExhausted  = "quota_exhausted"
	ErrRateLimited     = "rate_limited"
	ErrTimeout         = "timeout"
	ErrServerError     = "server_error"
	ErrAuthError       = "auth_error"
	ErrModelNotFound   = "model_not_found"
	ErrContextTooLong  = "context_too_long"
	ErrUnknown         = "unknown"
)

// ModelHealth tracks the health status of a single model.
type ModelHealth struct {
	State               ModelState     `json:"state"`
	ConsecutiveFailures int            `json:"consecutive_failures"`
	LastFailure         *time.Time     `json:"last_failure,omitempty"`
	LastSuccess         *time.Time     `json:"last_success,omitempty"`
	DegradedAt          *time.Time     `json:"degraded_at,omitempty"`
	TotalRequests       int64          `json:"total_requests"`
	TotalFailures       int64          `json:"total_failures"`
	SuccessRate         float64        `json:"success_rate"`
	ErrorTypes          map[string]int `json:"error_types"`
	LastErrorType       string         `json:"last_error_type,omitempty"`
}

// HealthConfig configures the health registry behavior.
type HealthConfig struct {
	FailureThreshold int           `json:"failure_threshold"` // Failures before degraded (default: 3)
	CooldownPeriod   time.Duration `json:"cooldown_period"`   // Time before retry degraded model (default: 5min)
	PersistPath      string        `json:"persist_path"`      // Path to persist state
	AutoRecover      bool          `json:"auto_recover"`      // Auto-recover after cooldown (default: true)
}

// DefaultHealthConfig returns sensible defaults.
func DefaultHealthConfig() HealthConfig {
	homeDir, _ := os.UserHomeDir()
	return HealthConfig{
		FailureThreshold: 3,
		CooldownPeriod:   5 * time.Minute,
		PersistPath:      filepath.Join(homeDir, ".evoclaw", "model_health.json"),
		AutoRecover:      true,
	}
}

// HealthRegistry manages health state for all models.
type HealthRegistry struct {
	mu      sync.RWMutex
	models  map[string]*ModelHealth
	cfg     HealthConfig
	logger  *slog.Logger
	dirty   bool // Track if state needs persisting
}

// HealthSnapshot is the persisted state format.
type HealthSnapshot struct {
	Models      map[string]*ModelHealth `json:"models"`
	LastUpdated time.Time               `json:"last_updated"`
	Version     string                  `json:"version"`
}

// NewHealthRegistry creates a new health registry.
func NewHealthRegistry(cfg HealthConfig, logger *slog.Logger) (*HealthRegistry, error) {
	if logger == nil {
		logger = slog.Default()
	}

	hr := &HealthRegistry{
		models: make(map[string]*ModelHealth),
		cfg:    cfg,
		logger: logger.With("component", "health-registry"),
	}

	// Try to load existing state
	if err := hr.load(); err != nil {
		// Not fatal - start fresh
		hr.logger.Debug("no existing health state, starting fresh", "error", err)
	}

	return hr, nil
}

// getOrCreate gets existing health record or creates new one.
func (hr *HealthRegistry) getOrCreate(modelID string) *ModelHealth {
	if h, ok := hr.models[modelID]; ok {
		return h
	}
	h := &ModelHealth{
		State:      StateUnknown,
		ErrorTypes: make(map[string]int),
	}
	hr.models[modelID] = h
	return h
}

// RecordSuccess records a successful model call.
func (hr *HealthRegistry) RecordSuccess(modelID string) {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	h := hr.getOrCreate(modelID)
	now := time.Now()

	h.LastSuccess = &now
	h.ConsecutiveFailures = 0
	h.TotalRequests++

	// Recover from degraded state
	switch h.State {
	case StateDegraded:
		h.State = StateHealthy
		h.DegradedAt = nil
		hr.logger.Info("model recovered", "model", modelID)
	case StateUnknown:
		h.State = StateHealthy
	}

	// Update success rate
	if h.TotalRequests > 0 {
		h.SuccessRate = float64(h.TotalRequests-h.TotalFailures) / float64(h.TotalRequests)
	}

	hr.dirty = true
}

// RecordFailure records a failed model call.
func (hr *HealthRegistry) RecordFailure(modelID string, errType string) {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	h := hr.getOrCreate(modelID)
	now := time.Now()

	h.LastFailure = &now
	h.LastErrorType = errType
	h.ConsecutiveFailures++
	h.TotalRequests++
	h.TotalFailures++

	// Track error types
	if h.ErrorTypes == nil {
		h.ErrorTypes = make(map[string]int)
	}
	h.ErrorTypes[errType]++

	// Update success rate
	if h.TotalRequests > 0 {
		h.SuccessRate = float64(h.TotalRequests-h.TotalFailures) / float64(h.TotalRequests)
	}

	// Check if should degrade
	if h.ConsecutiveFailures >= hr.cfg.FailureThreshold && h.State != StateDegraded {
		h.State = StateDegraded
		h.DegradedAt = &now
		hr.logger.Warn("model degraded",
			"model", modelID,
			"consecutive_failures", h.ConsecutiveFailures,
			"error_type", errType,
		)
	}

	hr.dirty = true
}

// IsHealthy checks if a model is healthy (not degraded).
func (hr *HealthRegistry) IsHealthy(modelID string) bool {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	h, ok := hr.models[modelID]
	if !ok {
		return true // Unknown models are assumed healthy
	}

	// Check if degraded but cooldown expired (auto-recovery)
	if h.State == StateDegraded && hr.cfg.AutoRecover && h.DegradedAt != nil {
		if time.Since(*h.DegradedAt) > hr.cfg.CooldownPeriod {
			return true // Allow retry
		}
		return false
	}

	return h.State != StateDegraded
}

// GetHealthyModel returns the best healthy model from preferred + fallbacks.
func (hr *HealthRegistry) GetHealthyModel(preferred string, fallbacks []string) string {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	// Check preferred first
	if hr.isHealthyLocked(preferred) {
		return preferred
	}

	// Try fallbacks in order
	for _, fb := range fallbacks {
		if hr.isHealthyLocked(fb) {
			hr.logger.Info("using fallback model",
				"preferred", preferred,
				"fallback", fb,
			)
			return fb
		}
	}

	// All degraded - return the one with best success rate
	return hr.bestSuccessRateLocked(preferred, fallbacks)
}

// isHealthyLocked checks health without acquiring lock (caller must hold lock).
func (hr *HealthRegistry) isHealthyLocked(modelID string) bool {
	h, ok := hr.models[modelID]
	if !ok {
		return true // Unknown = healthy
	}

	if h.State == StateDegraded {
		// Check cooldown
		if hr.cfg.AutoRecover && h.DegradedAt != nil {
			if time.Since(*h.DegradedAt) > hr.cfg.CooldownPeriod {
				return true // Allow retry after cooldown
			}
		}
		return false
	}

	return true
}

// bestSuccessRateLocked returns model with highest success rate.
func (hr *HealthRegistry) bestSuccessRateLocked(preferred string, fallbacks []string) string {
	best := preferred
	bestRate := float64(-1)

	// Check preferred
	if h, ok := hr.models[preferred]; ok {
		bestRate = h.SuccessRate
	}

	// Check fallbacks
	for _, fb := range fallbacks {
		if h, ok := hr.models[fb]; ok {
			if h.SuccessRate > bestRate {
				best = fb
				bestRate = h.SuccessRate
			}
		}
	}

	return best
}

// GetStatus returns health status for all models.
func (hr *HealthRegistry) GetStatus() map[string]*ModelHealth {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	// Return a copy
	result := make(map[string]*ModelHealth, len(hr.models))
	for k, v := range hr.models {
		copy := *v
		result[k] = &copy
	}
	return result
}

// GetModelStatus returns health status for a specific model.
func (hr *HealthRegistry) GetModelStatus(modelID string) (*ModelHealth, bool) {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	h, ok := hr.models[modelID]
	if !ok {
		return nil, false
	}
	copy := *h
	return &copy, true
}

// DegradedModels returns list of currently degraded models.
func (hr *HealthRegistry) DegradedModels() []string {
	hr.mu.RLock()
	defer hr.mu.RUnlock()

	var degraded []string
	for id, h := range hr.models {
		if h.State == StateDegraded {
			degraded = append(degraded, id)
		}
	}
	return degraded
}

// ResetModel manually resets a model to healthy state.
func (hr *HealthRegistry) ResetModel(modelID string) {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	if h, ok := hr.models[modelID]; ok {
		h.State = StateHealthy
		h.ConsecutiveFailures = 0
		h.DegradedAt = nil
		hr.logger.Info("model manually reset", "model", modelID)
		hr.dirty = true
	}
}

// Persist saves state to disk.
func (hr *HealthRegistry) Persist() error {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	if !hr.dirty {
		return nil
	}

	snapshot := HealthSnapshot{
		Models:      hr.models,
		LastUpdated: time.Now(),
		Version:     "1.0",
	}

	// Ensure directory exists
	dir := filepath.Dir(hr.cfg.PersistPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create health dir: %w", err)
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal health state: %w", err)
	}

	if err := os.WriteFile(hr.cfg.PersistPath, data, 0644); err != nil {
		return fmt.Errorf("write health state: %w", err)
	}

	hr.dirty = false
	hr.logger.Debug("health state persisted", "path", hr.cfg.PersistPath)
	return nil
}

// load reads state from disk.
func (hr *HealthRegistry) load() error {
	data, err := os.ReadFile(hr.cfg.PersistPath)
	if err != nil {
		return err
	}

	var snapshot HealthSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return fmt.Errorf("parse health state: %w", err)
	}

	hr.models = snapshot.Models
	if hr.models == nil {
		hr.models = make(map[string]*ModelHealth)
	}

	hr.logger.Debug("health state loaded",
		"path", hr.cfg.PersistPath,
		"models", len(hr.models),
	)
	return nil
}

// ClassifyError categorizes an error for tracking.
func ClassifyError(err error) string {
	if err == nil {
		return ""
	}

	errStr := err.Error()

	// Check for common patterns
	patterns := map[string][]string{
		ErrQuotaExhausted: {"quota", "exhausted", "limit exceeded", "resource package"},
		ErrRateLimited:    {"rate limit", "too many requests", "429"},
		ErrTimeout:        {"timeout", "deadline exceeded", "context canceled"},
		ErrServerError:    {"500", "502", "503", "504", "internal server error"},
		ErrAuthError:      {"401", "403", "unauthorized", "forbidden", "invalid api key"},
		ErrModelNotFound:  {"model not found", "does not exist", "404"},
		ErrContextTooLong: {"context length", "too long", "max tokens"},
	}

	for errType, keywords := range patterns {
		for _, kw := range keywords {
			if containsIgnoreCase(errStr, kw) {
				return errType
			}
		}
	}

	return ErrUnknown
}

func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			len(s) > 0 && len(substr) > 0 &&
				(s[0]|0x20) >= 'a' && (s[0]|0x20) <= 'z' &&
				containsIgnoreCaseSlow(s, substr))
}

func containsIgnoreCaseSlow(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			sc := s[i+j]
			pc := substr[j]
			if sc != pc && sc != pc^0x20 && (sc < 'A' || sc > 'z' || pc < 'A' || pc > 'z') {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
