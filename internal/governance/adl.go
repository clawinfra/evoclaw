package governance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SignalType categorises an ADL behaviour signal.
type SignalType string

const (
	SignalAntiSycophancy SignalType = "anti_sycophancy" // "Great question!", "I'd be happy to…"
	SignalAntiPassivity  SignalType = "anti_passivity"  // "Would you like me to…", "Should I…"
	SignalPersonaDirect  SignalType = "persona_direct"  // "Done.", "Fixed."
	SignalPersonaAction  SignalType = "persona_action"  // "Spawning…", "Executing…"
)

// ADLSignal is a single detected behaviour signal.
type ADLSignal struct {
	Type     SignalType `json:"type"`
	Excerpt  string     `json:"excerpt"`
	Positive bool       `json:"positive"` // true = good persona alignment
}

// ADLStats summarises logged signals for an agent.
type ADLStats struct {
	TotalSignals   int     `json:"total_signals"`
	AntiPatterns   int     `json:"anti_patterns"`
	PersonaSignals int     `json:"persona_signals"`
	// DivergenceScore is (AntiPatterns - PersonaSignals) / TotalSignals in [-1, 1].
	// Negative means persona-aligned; positive means drifting toward anti-patterns.
	DivergenceScore float64 `json:"divergence_score"`
}

// adlLogEntry is persisted to disk per agent.
type adlLogEntry struct {
	AgentID  string    `json:"agent_id"`
	Signal   SignalType `json:"signal"`
	Excerpt  string    `json:"excerpt"`
	Positive bool      `json:"positive"`
	At       time.Time `json:"at"`
}

// ADLBaseline represents the baseline persona from SOUL.md.
type ADLBaseline struct {
	Hash       string    `json:"hash"`        // SHA256 of SOUL.md content
	Keywords   []string  `json:"keywords"`    // Key identity markers
	Boundaries []string  `json:"boundaries"`  // Behavioral boundaries
	LoadedAt   time.Time `json:"loaded_at"`
}

// ADLCheckResult represents the result of a drift check.
type ADLCheckResult struct {
	AgentID       string    `json:"agent_id"`
	DriftScore    float64   `json:"drift_score"`    // 0.0 = no drift, 1.0 = complete divergence
	ViolatedRules []string  `json:"violated_rules"` // Boundaries that may be violated
	Timestamp     time.Time `json:"timestamp"`
}

// ADL implements Anti-Divergence Limit protocol.
type ADL struct {
	baseDir   string
	logger    *slog.Logger
	mu        sync.RWMutex
	baselines map[string]*ADLBaseline
}

// NewADL creates a new ADL instance.
func NewADL(baseDir string, logger *slog.Logger) (*ADL, error) {
	adlDir := filepath.Join(baseDir, "adl")
	if err := os.MkdirAll(adlDir, 0755); err != nil {
		return nil, fmt.Errorf("create ADL directory: %w", err)
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ADL{
		baseDir:   adlDir,
		logger:    logger.With("component", "adl"),
		baselines: make(map[string]*ADLBaseline),
	}, nil
}

func (a *ADL) baselinePath(agentID string) string {
	return filepath.Join(a.baseDir, agentID+"_baseline.json")
}

// LoadBaseline loads or creates a baseline from SOUL.md.
func (a *ADL) LoadBaseline(agentID, soulPath string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	content, err := os.ReadFile(soulPath)
	if err != nil {
		return fmt.Errorf("read SOUL.md: %w", err)
	}

	hash := sha256.Sum256(content)
	baseline := &ADLBaseline{
		Hash:       hex.EncodeToString(hash[:]),
		Keywords:   extractKeywords(string(content)),
		Boundaries: extractBoundaries(string(content)),
		LoadedAt:   time.Now(),
	}

	a.baselines[agentID] = baseline

	// Persist baseline
	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal baseline: %w", err)
	}

	if err := os.WriteFile(a.baselinePath(agentID), data, 0644); err != nil {
		return fmt.Errorf("write baseline: %w", err)
	}

	a.logger.Info("ADL baseline loaded", "agent", agentID, "keywords", len(baseline.Keywords), "boundaries", len(baseline.Boundaries))
	return nil
}

// CheckDrift evaluates behavioral drift from the baseline.
// Returns a drift score between 0.0 (no drift) and 1.0 (complete divergence).
func (a *ADL) CheckDrift(agentID, currentBehavior string) (float64, error) {
	a.mu.RLock()
	baseline, ok := a.baselines[agentID]
	a.mu.RUnlock()

	if !ok {
		// Try to load from disk
		data, err := os.ReadFile(a.baselinePath(agentID))
		if err != nil {
			// No baseline — fall back to signal-based analysis only
			signals, _ := a.Analyze(currentBehavior)
			antiPatterns := 0
			for _, s := range signals {
				if !s.Positive {
					antiPatterns++
				}
			}
			total := len(signals)
			if total == 0 {
				return 0, nil
			}
			return float64(antiPatterns) / float64(total), nil
		}
		baseline = &ADLBaseline{}
		if err := json.Unmarshal(data, baseline); err != nil {
			return 0, fmt.Errorf("parse baseline: %w", err)
		}
		a.mu.Lock()
		a.baselines[agentID] = baseline
		a.mu.Unlock()
	}

	// Calculate keyword alignment
	keywordScore := calculateKeywordAlignment(currentBehavior, baseline.Keywords)

	// Check boundary violations
	violations := checkBoundaryViolations(currentBehavior, baseline.Boundaries)

	// Drift score: combination of keyword divergence and boundary violations
	driftScore := (1.0-keywordScore)*0.5 + float64(len(violations))*0.1
	if driftScore > 1.0 {
		driftScore = 1.0
	}

	a.logger.Debug("ADL drift check",
		"agent", agentID,
		"drift_score", driftScore,
		"keyword_alignment", keywordScore,
		"violations", len(violations))

	return driftScore, nil
}

// GetViolations returns any boundary violations in the current behavior.
func (a *ADL) GetViolations(agentID, currentBehavior string) ([]string, error) {
	a.mu.RLock()
	baseline, ok := a.baselines[agentID]
	a.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no baseline for agent: %s", agentID)
	}

	return checkBoundaryViolations(currentBehavior, baseline.Boundaries), nil
}

// extractKeywords pulls identity markers from SOUL.md content.
func extractKeywords(content string) []string {
	// Look for patterns like "Name:", "Nature:", "Role:", values in headers
	var keywords []string
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Extract values after colons in key lines
		if strings.Contains(line, "**Name:**") ||
			strings.Contains(line, "**Nature:**") ||
			strings.Contains(line, "**Role:**") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				keywords = append(keywords, strings.TrimSpace(parts[1]))
			}
		}
		// Extract emphasized words
		if strings.HasPrefix(line, "**") && strings.HasSuffix(line, "**") {
			word := strings.Trim(line, "*")
			if len(word) > 0 && len(word) < 50 {
				keywords = append(keywords, word)
			}
		}
	}

	return keywords
}

// extractBoundaries pulls behavioral boundaries from SOUL.md.
func extractBoundaries(content string) []string {
	var boundaries []string
	lines := strings.Split(content, "\n")
	inBoundaries := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.Contains(strings.ToLower(line), "boundaries") ||
			strings.Contains(strings.ToLower(line), "never") ||
			strings.Contains(strings.ToLower(line), "don't") {
			inBoundaries = true
		}

		if inBoundaries && strings.HasPrefix(line, "-") {
			boundary := strings.TrimPrefix(line, "-")
			boundary = strings.TrimSpace(boundary)
			if len(boundary) > 0 {
				boundaries = append(boundaries, boundary)
			}
		}

		// Reset on new section
		if strings.HasPrefix(line, "##") && !strings.Contains(strings.ToLower(line), "boundaries") {
			inBoundaries = false
		}
	}

	return boundaries
}

// calculateKeywordAlignment checks how many baseline keywords appear in behavior.
func calculateKeywordAlignment(behavior string, keywords []string) float64 {
	if len(keywords) == 0 {
		return 1.0 // No keywords to check
	}

	behaviorLower := strings.ToLower(behavior)
	matches := 0

	for _, kw := range keywords {
		if strings.Contains(behaviorLower, strings.ToLower(kw)) {
			matches++
		}
	}

	return float64(matches) / float64(len(keywords))
}

// Analyze detects ADL signals in text without requiring a baseline.
func (a *ADL) Analyze(text string) ([]ADLSignal, error) {
	lower := strings.ToLower(text)
	var signals []ADLSignal

	antiPatterns := map[SignalType][]string{
		SignalAntiSycophancy: {"great question", "i'd be happy", "certainly!", "of course!", "absolutely!"},
		SignalAntiPassivity:  {"would you like me to", "should i", "do you want me to", "shall i"},
	}
	personaPatterns := map[SignalType][]string{
		SignalPersonaDirect: {"done.", "done!", "fixed.", "fixed!", "shipped."},
		SignalPersonaAction: {"spawning", "executing", "deploying", "building", "i'd argue"},
	}

	for sigType, patterns := range antiPatterns {
		for _, p := range patterns {
			if strings.Contains(lower, p) {
				signals = append(signals, ADLSignal{Type: sigType, Excerpt: p, Positive: false})
				break
			}
		}
	}
	for sigType, patterns := range personaPatterns {
		for _, p := range patterns {
			if strings.Contains(lower, p) {
				signals = append(signals, ADLSignal{Type: sigType, Excerpt: p, Positive: true})
				break
			}
		}
	}
	return signals, nil
}

// Log persists a single ADL signal for an agent.
func (a *ADL) Log(agentID string, signal SignalType, excerpt string, positive bool) error {
	entry := adlLogEntry{
		AgentID:  agentID,
		Signal:   signal,
		Excerpt:  excerpt,
		Positive: positive,
		At:       time.Now(),
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal log entry: %w", err)
	}

	logPath := filepath.Join(a.baseDir, agentID+"_signals.jsonl")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open signal log: %w", err)
	}
	defer f.Close()

	_, err = fmt.Fprintln(f, string(data))
	return err
}

// Stats returns aggregated signal statistics for an agent.
func (a *ADL) Stats(agentID string) (*ADLStats, error) {
	logPath := filepath.Join(a.baseDir, agentID+"_signals.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &ADLStats{}, nil
		}
		return nil, fmt.Errorf("read signal log: %w", err)
	}

	stats := &ADLStats{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var entry adlLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		stats.TotalSignals++
		if entry.Positive {
			stats.PersonaSignals++
		} else {
			stats.AntiPatterns++
		}
	}
	if stats.TotalSignals > 0 {
		stats.DivergenceScore = float64(stats.AntiPatterns-stats.PersonaSignals) / float64(stats.TotalSignals)
	}
	return stats, nil
}

// Reset clears all logged signals for an agent.
func (a *ADL) Reset(agentID string) error {
	logPath := filepath.Join(a.baseDir, agentID+"_signals.jsonl")
	if err := os.Remove(logPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reset signal log: %w", err)
	}
	return nil
}

// Check returns true if the agent's divergence score exceeds the given threshold.
func (a *ADL) Check(agentID string, threshold float64) (bool, error) {
	stats, err := a.Stats(agentID)
	if err != nil {
		return false, err
	}
	if stats.TotalSignals == 0 {
		return false, nil
	}
	return stats.DivergenceScore > threshold, nil
}

// checkBoundaryViolations looks for boundary violations in behavior.
func checkBoundaryViolations(behavior string, boundaries []string) []string {
	var violations []string
	behaviorLower := strings.ToLower(behavior)

	// Look for negative patterns that might indicate boundary violations
	violationIndicators := []string{
		"exfiltrate", "leak", "share private", "ignore safety",
		"bypass", "override", "without permission",
	}

	for _, indicator := range violationIndicators {
		if strings.Contains(behaviorLower, indicator) {
			violations = append(violations, fmt.Sprintf("potential violation: %s", indicator))
		}
	}

	return violations
}
