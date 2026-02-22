package rsi

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Analyzer scans outcomes and detects recurring patterns.
type Analyzer struct {
	observer *Observer
	cfg      Config
	logger   *slog.Logger
	// Track patterns across analysis cycles
	knownPatterns map[string]*Pattern
}

// NewAnalyzer creates a new Analyzer.
func NewAnalyzer(observer *Observer, cfg Config, logger *slog.Logger) *Analyzer {
	return &Analyzer{
		observer:      observer,
		cfg:           cfg,
		logger:        logger,
		knownPatterns: make(map[string]*Pattern),
	}
}

// Analyze scans recent outcomes and detects patterns.
func (a *Analyzer) Analyze() ([]Pattern, error) {
	outcomes := a.observer.Outcomes(a.cfg.AnalysisWindow)
	if len(outcomes) == 0 {
		return nil, nil
	}

	var patterns []Pattern

	// Group by (task_type, issue)
	groups := a.groupByTaskAndIssue(outcomes)
	for key, group := range groups {
		if len(group) < a.cfg.RecurrenceThreshold {
			continue
		}

		pattern := a.buildPattern(key, group, outcomes)
		patterns = append(patterns, pattern)
	}

	// Group by error message similarity
	errGroups := a.groupByErrorSimilarity(outcomes)
	for _, group := range errGroups {
		if len(group) < a.cfg.RecurrenceThreshold {
			continue
		}

		pattern := a.buildErrorPattern(group, outcomes)
		patterns = append(patterns, pattern)
	}

	// Cross-source correlation
	crossPatterns := a.detectCrossSourcePatterns(outcomes)
	patterns = append(patterns, crossPatterns...)

	// Update known patterns
	for i := range patterns {
		if existing, ok := a.knownPatterns[patterns[i].Issue]; ok {
			patterns[i].FirstSeen = existing.FirstSeen
		}
		a.knownPatterns[patterns[i].Issue] = &patterns[i]
	}

	return patterns, nil
}

// HealthScore calculates the overall health score (0.0-1.0) from recent outcomes.
// More recent outcomes are weighted higher.
func (a *Analyzer) HealthScore() float64 {
	outcomes := a.observer.Outcomes(a.cfg.AnalysisWindow)
	if len(outcomes) == 0 {
		return 1.0 // No data = assume healthy
	}

	now := time.Now()
	var weightedSum, totalWeight float64

	for _, out := range outcomes {
		// Recency weight: exponential decay, half-life = 1 hour
		age := now.Sub(out.Timestamp).Hours()
		weight := 1.0 / (1.0 + age)

		score := 0.0
		if out.Success {
			score = out.Quality
			if score == 0 {
				score = 1.0
			}
		}

		weightedSum += score * weight
		totalWeight += weight
	}

	if totalWeight == 0 {
		return 1.0
	}

	return weightedSum / totalWeight
}

type outcomeGroup struct {
	taskType string
	issue    string
	outcomes []Outcome
}

// groupByTaskAndIssue groups outcomes by (task_type, issue) pairs.
func (a *Analyzer) groupByTaskAndIssue(outcomes []Outcome) map[string][]Outcome {
	groups := make(map[string][]Outcome)
	for _, out := range outcomes {
		for _, issue := range out.Issues {
			key := out.TaskType + "|" + issue
			groups[key] = append(groups[key], out)
		}
	}
	return groups
}

// groupByErrorSimilarity groups outcomes by error message similarity using token overlap.
func (a *Analyzer) groupByErrorSimilarity(outcomes []Outcome) [][]Outcome {
	// Filter to only failed outcomes with error messages
	var failed []Outcome
	for _, out := range outcomes {
		if !out.Success && out.ErrorMessage != "" {
			failed = append(failed, out)
		}
	}

	if len(failed) < a.cfg.RecurrenceThreshold {
		return nil
	}

	// Simple token-overlap grouping
	used := make([]bool, len(failed))
	var groups [][]Outcome

	for i := 0; i < len(failed); i++ {
		if used[i] {
			continue
		}
		group := []Outcome{failed[i]}
		used[i] = true
		tokensI := tokenize(failed[i].ErrorMessage)

		for j := i + 1; j < len(failed); j++ {
			if used[j] {
				continue
			}
			tokensJ := tokenize(failed[j].ErrorMessage)
			if tokenOverlap(tokensI, tokensJ) > 0.5 {
				group = append(group, failed[j])
				used[j] = true
			}
		}

		groups = append(groups, group)
	}

	return groups
}

// detectCrossSourcePatterns looks for correlated patterns across sources.
func (a *Analyzer) detectCrossSourcePatterns(outcomes []Outcome) []Pattern {
	var patterns []Pattern

	// Check for session_reset + context_loss correlation
	var sessionResets, contextLosses []Outcome
	for _, out := range outcomes {
		for _, issue := range out.Issues {
			if issue == IssueSessionReset {
				sessionResets = append(sessionResets, out)
			}
			if issue == IssueContextLoss {
				contextLosses = append(contextLosses, out)
			}
		}
	}

	if len(sessionResets) >= 2 && len(contextLosses) >= 1 {
		// Check temporal proximity (within 5 minutes)
		for _, sr := range sessionResets {
			for _, cl := range contextLosses {
				diff := sr.Timestamp.Sub(cl.Timestamp)
				if diff < 0 {
					diff = -diff
				}
				if diff < 5*time.Minute {
					pattern := Pattern{
						ID:          uuid.New().String(),
						Category:    "compound",
						TaskType:    "cross_source",
						Issue:       "session_reset+context_loss",
						Frequency:   len(sessionResets) + len(contextLosses),
						ImpactScore: 0.9,
						FailureRate: 1.0,
						Description: "Session resets correlated with context loss — possible context overflow causing resets",
						SuggestedAction: "Reduce context window or implement context summarization",
						Sources:     uniqueSources(append(sessionResets, contextLosses...)),
						FirstSeen:   earliest(append(sessionResets, contextLosses...)),
						LastSeen:    latest(append(sessionResets, contextLosses...)),
					}
					patterns = append(patterns, pattern)
					return patterns // One compound pattern is enough
				}
			}
		}
	}

	return patterns
}

// buildPattern creates a Pattern from a group of outcomes.
func (a *Analyzer) buildPattern(key string, group []Outcome, allOutcomes []Outcome) Pattern {
	parts := strings.SplitN(key, "|", 2)
	taskType, issue := parts[0], parts[1]

	// Calculate failure rate
	totalForTask := 0
	failedForTask := 0
	for _, out := range allOutcomes {
		if out.TaskType == taskType {
			totalForTask++
			if !out.Success {
				failedForTask++
			}
		}
	}
	failureRate := 0.0
	if totalForTask > 0 {
		failureRate = float64(failedForTask) / float64(totalForTask)
	}

	// Collect sample errors
	var samples []string
	seen := make(map[string]bool)
	for _, out := range group {
		if out.ErrorMessage != "" && !seen[out.ErrorMessage] {
			samples = append(samples, out.ErrorMessage)
			seen[out.ErrorMessage] = true
			if len(samples) >= 3 {
				break
			}
		}
	}

	// Determine category
	category := categorizeIssue(issue)

	return Pattern{
		ID:              uuid.New().String(),
		Category:        category,
		TaskType:        taskType,
		Issue:           issue,
		Frequency:       len(group),
		ImpactScore:     calculateImpact(group),
		FailureRate:     failureRate,
		Description:     fmt.Sprintf("Recurring %s in %s tasks (%d occurrences)", issue, taskType, len(group)),
		SampleErrors:    samples,
		SuggestedAction: suggestAction(issue),
		Sources:         uniqueSources(group),
		FirstSeen:       earliest(group),
		LastSeen:        latest(group),
	}
}

// buildErrorPattern creates a Pattern from error-message-similar outcomes.
func (a *Analyzer) buildErrorPattern(group []Outcome, allOutcomes []Outcome) Pattern {
	// Use first error as representative
	representative := group[0].ErrorMessage
	if len(representative) > 100 {
		representative = representative[:100] + "..."
	}

	var samples []string
	seen := make(map[string]bool)
	for _, out := range group {
		if out.ErrorMessage != "" && !seen[out.ErrorMessage] {
			samples = append(samples, out.ErrorMessage)
			seen[out.ErrorMessage] = true
			if len(samples) >= 3 {
				break
			}
		}
	}

	issues := detectIssues(group[0].ErrorMessage)
	issue := IssueUnknown
	if len(issues) > 0 {
		issue = issues[0]
	}

	return Pattern{
		ID:              uuid.New().String(),
		Category:        categorizeIssue(issue),
		TaskType:        "mixed",
		Issue:           "error_cluster:" + issue,
		Frequency:       len(group),
		ImpactScore:     calculateImpact(group),
		FailureRate:     1.0,
		Description:     fmt.Sprintf("Cluster of %d similar errors: %s", len(group), representative),
		SampleErrors:    samples,
		SuggestedAction: suggestAction(issue),
		Sources:         uniqueSources(group),
		FirstSeen:       earliest(group),
		LastSeen:        latest(group),
	}
}

// Helper functions

func tokenize(s string) map[string]bool {
	tokens := make(map[string]bool)
	for _, word := range strings.Fields(strings.ToLower(s)) {
		tokens[word] = true
	}
	return tokens
}

func tokenOverlap(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	overlap := 0
	for k := range a {
		if b[k] {
			overlap++
		}
	}
	smaller := len(a)
	if len(b) < smaller {
		smaller = len(b)
	}
	return float64(overlap) / float64(smaller)
}

func uniqueSources(outcomes []Outcome) []Source {
	seen := make(map[Source]bool)
	var sources []Source
	for _, out := range outcomes {
		if !seen[out.Source] {
			seen[out.Source] = true
			sources = append(sources, out.Source)
		}
	}
	return sources
}

func earliest(outcomes []Outcome) time.Time {
	if len(outcomes) == 0 {
		return time.Time{}
	}
	sort.Slice(outcomes, func(i, j int) bool {
		return outcomes[i].Timestamp.Before(outcomes[j].Timestamp)
	})
	return outcomes[0].Timestamp
}

func latest(outcomes []Outcome) time.Time {
	if len(outcomes) == 0 {
		return time.Time{}
	}
	sort.Slice(outcomes, func(i, j int) bool {
		return outcomes[i].Timestamp.After(outcomes[j].Timestamp)
	})
	return outcomes[0].Timestamp
}

func calculateImpact(outcomes []Outcome) float64 {
	if len(outcomes) == 0 {
		return 0
	}
	failures := 0
	for _, out := range outcomes {
		if !out.Success {
			failures++
		}
	}
	// Impact = failure proportion * log-scaled frequency
	failRate := float64(failures) / float64(len(outcomes))
	// Cap at 1.0
	impact := failRate * (1.0 + float64(len(outcomes))/10.0)
	if impact > 1.0 {
		impact = 1.0
	}
	return impact
}

func categorizeIssue(issue string) string {
	switch issue {
	case IssueRateLimit, IssueTimeout:
		return "retry_logic"
	case IssueModelError:
		return "model_selection"
	case IssueEmptyResponse:
		return "routing_config"
	default:
		return "operational"
	}
}

func suggestAction(issue string) string {
	switch issue {
	case IssueRateLimit:
		return "Increase retry backoff or add rate limiting"
	case IssueEmptyResponse:
		return "Check model routing, add response validation"
	case IssueTimeout:
		return "Increase timeout or switch to faster model"
	case IssueModelError:
		return "Update model configuration or switch provider"
	case IssueToolFailure:
		return "Check tool availability and error handling"
	case IssueContextLoss:
		return "Implement context summarization or reduce context window"
	case IssueSessionReset:
		return "Check connection stability and add reconnection logic"
	default:
		return "Manual investigation required"
	}
}
