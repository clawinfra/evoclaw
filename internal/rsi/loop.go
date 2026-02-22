package rsi

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Loop is the main RSI closed loop: observe → analyze → fix → verify → observe.
type Loop struct {
	cfg      Config
	logger   *slog.Logger
	observer *Observer
	analyzer *Analyzer
	fixer    *Fixer

	mu           sync.RWMutex
	healthScore  float64
	patterns     []Pattern
	appliedFixes []Fix
}

// NewLoop creates a new RSI Loop.
func NewLoop(cfg Config, logger *slog.Logger) *Loop {
	observer := NewObserver(cfg, logger)
	analyzer := NewAnalyzer(observer, cfg, logger)
	fixer := NewFixer(cfg, logger)

	return &Loop{
		cfg:         cfg,
		logger:      logger,
		observer:    observer,
		analyzer:    analyzer,
		fixer:       fixer,
		healthScore: 1.0,
	}
}

// Start begins the background RSI loop.
func (l *Loop) Start(ctx context.Context) {
	interval := l.cfg.AnalysisInterval
	if interval == 0 {
		interval = 1 * time.Hour
	}

	l.logger.Info("RSI loop started", "interval", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			l.logger.Info("RSI loop stopped")
			return
		case <-ticker.C:
			l.runCycle()
		}
	}
}

// Observer returns the observer for recording outcomes.
func (l *Loop) Observer() *Observer {
	return l.observer
}

// HealthScore returns the current health score (0.0-1.0).
func (l *Loop) HealthScore() float64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.healthScore
}

// Patterns returns the currently detected patterns.
func (l *Loop) Patterns() []Pattern {
	l.mu.RLock()
	defer l.mu.RUnlock()
	result := make([]Pattern, len(l.patterns))
	copy(result, l.patterns)
	return result
}

// runCycle executes one analysis + fix cycle.
func (l *Loop) runCycle() {
	start := time.Now()

	// Analyze
	patterns, err := l.analyzer.Analyze()
	if err != nil {
		l.logger.Error("RSI analysis failed", "error", err)
		return
	}

	// Update health score
	health := l.analyzer.HealthScore()

	l.mu.Lock()
	l.patterns = patterns
	l.healthScore = health
	l.mu.Unlock()

	l.logger.Info("RSI analysis complete",
		"patterns", len(patterns),
		"health", health,
		"elapsed", time.Since(start),
	)

	// Fix cycle
	if len(patterns) > 0 {
		l.fixCycle(patterns)
	}
}

// fixCycle proposes and applies fixes for detected patterns.
func (l *Loop) fixCycle(patterns []Pattern) {
	for _, pattern := range patterns {
		fix, err := l.fixer.ProposeFix(pattern)
		if err != nil {
			l.logger.Error("failed to propose fix",
				"pattern", pattern.ID,
				"error", err,
			)
			continue
		}

		applied, err := l.fixer.ApplyIfSafe(fix)
		if err != nil {
			l.logger.Error("failed to apply fix",
				"fix", fix.ID,
				"error", err,
			)
			continue
		}

		if applied {
			l.mu.Lock()
			l.appliedFixes = append(l.appliedFixes, *fix)
			l.mu.Unlock()
		}
	}
}
