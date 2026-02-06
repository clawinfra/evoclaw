package evolution

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Strategy represents an agent's current strategy that can be mutated
type Strategy struct {
	ID        string    `json:"id"`
	AgentID   string    `json:"agentId"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
	// Prompt engineering
	SystemPrompt string `json:"systemPrompt"`
	// Model selection preferences
	PreferredModel string `json:"preferredModel"`
	FallbackModel  string `json:"fallbackModel"`
	// Behavioral parameters
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"maxTokens"`
	// Custom strategy parameters (agent-type specific)
	Params map[string]float64 `json:"params"`
	// Fitness score from evaluation
	Fitness float64 `json:"fitness"`
	// Number of evaluations
	EvalCount int `json:"evalCount"`
}

// TradeMetrics for trading-specific evolution
type TradeMetrics struct {
	TotalTrades int     `json:"totalTrades"`
	WinRate     float64 `json:"winRate"`
	ProfitLoss  float64 `json:"profitLoss"`
	SharpeRatio float64 `json:"sharpeRatio"`
	MaxDrawdown float64 `json:"maxDrawdown"`
	AvgHoldTime float64 `json:"avgHoldTimeSec"`
}

// Engine manages the evolutionary process for all agents
type Engine struct {
	strategies map[string]*Strategy   // agentID -> current strategy
	history    map[string][]*Strategy // agentID -> past strategies
	dataDir    string
	logger     *slog.Logger
	mu         sync.RWMutex
}

// NewEngine creates a new evolution engine
func NewEngine(dataDir string, logger *slog.Logger) *Engine {
	dir := filepath.Join(dataDir, "evolution")
	os.MkdirAll(dir, 0750)

	e := &Engine{
		strategies: make(map[string]*Strategy),
		history:    make(map[string][]*Strategy),
		dataDir:    dir,
		logger:     logger,
	}

	// Load existing strategies from disk
	e.loadStrategies()

	return e
}

// GetStrategy returns the current strategy for an agent
func (e *Engine) GetStrategy(agentID string) *Strategy {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.strategies[agentID]
}

// SetStrategy sets the initial strategy for an agent
func (e *Engine) SetStrategy(agentID string, s *Strategy) {
	e.mu.Lock()
	defer e.mu.Unlock()
	s.AgentID = agentID
	s.CreatedAt = time.Now()
	e.strategies[agentID] = s
	e.saveStrategy(s)
}

// Evaluate scores a strategy based on performance metrics
func (e *Engine) Evaluate(agentID string, metrics map[string]float64) float64 {
	e.mu.Lock()
	defer e.mu.Unlock()

	s, ok := e.strategies[agentID]
	if !ok {
		return 0
	}

	// Compute fitness score (higher is better)
	fitness := computeFitness(metrics)

	// Exponential moving average of fitness
	alpha := 0.3 // Weight of new observation
	if s.EvalCount == 0 {
		s.Fitness = fitness
	} else {
		s.Fitness = alpha*fitness + (1-alpha)*s.Fitness
	}
	s.EvalCount++

	e.saveStrategy(s)
	e.logger.Info("strategy evaluated",
		"agent", agentID,
		"fitness", s.Fitness,
		"evalCount", s.EvalCount,
		"rawFitness", fitness,
	)

	return s.Fitness
}

// Mutate creates a new strategy variant based on the current one
func (e *Engine) Mutate(agentID string, mutationRate float64) (*Strategy, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	current, ok := e.strategies[agentID]
	if !ok {
		return nil, fmt.Errorf("no strategy found for agent %s", agentID)
	}

	// Archive current strategy
	e.history[agentID] = append(e.history[agentID], current)

	// Create mutated version
	mutated := &Strategy{
		ID:             fmt.Sprintf("%s-v%d", agentID, current.Version+1),
		AgentID:        agentID,
		Version:        current.Version + 1,
		CreatedAt:      time.Now(),
		SystemPrompt:   current.SystemPrompt, // Prompt mutation handled separately
		PreferredModel: current.PreferredModel,
		FallbackModel:  current.FallbackModel,
		Temperature:    mutateFloat(current.Temperature, mutationRate, 0.0, 2.0),
		MaxTokens:      current.MaxTokens,
		Params:         make(map[string]float64),
		Fitness:        0,
		EvalCount:      0,
	}

	// Mutate custom parameters
	for k, v := range current.Params {
		mutated.Params[k] = mutateFloat(v, mutationRate, -1000, 1000)
	}

	e.strategies[agentID] = mutated
	e.saveStrategy(mutated)

	e.logger.Info("strategy mutated",
		"agent", agentID,
		"version", mutated.Version,
		"mutationRate", mutationRate,
	)

	return mutated, nil
}

// Revert rolls back to the previous strategy if the current one is worse
func (e *Engine) Revert(agentID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	history, ok := e.history[agentID]
	if !ok || len(history) == 0 {
		return fmt.Errorf("no history for agent %s", agentID)
	}

	// Pop the last strategy from history
	prev := history[len(history)-1]
	e.history[agentID] = history[:len(history)-1]
	e.strategies[agentID] = prev
	e.saveStrategy(prev)

	e.logger.Info("strategy reverted",
		"agent", agentID,
		"version", prev.Version,
	)

	return nil
}

// ShouldEvolve determines if an agent needs evolution based on its fitness
func (e *Engine) ShouldEvolve(agentID string, minFitness float64) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	s, ok := e.strategies[agentID]
	if !ok {
		return false
	}

	// Need enough evaluations before evolving
	if s.EvalCount < 5 {
		return false
	}

	return s.Fitness < minFitness
}

// computeFitness calculates a fitness score from metrics
func computeFitness(metrics map[string]float64) float64 {
	// Weighted combination of metrics
	// Higher success rate = better
	successRate := metrics["successRate"]
	// Lower cost = better (invert)
	costEfficiency := 1.0 / (1.0 + metrics["costUSD"])
	// Lower latency = better (invert)
	speedScore := 1.0 / (1.0 + metrics["avgResponseMs"]/1000.0)
	// Custom: profit for traders
	profitScore := math.Max(0, metrics["profitLoss"]+1.0) // Normalize around 1.0

	// Weighted fitness
	fitness := 0.4*successRate + 0.2*costEfficiency + 0.1*speedScore + 0.3*profitScore
	return fitness
}

// mutateFloat applies gaussian-like mutation to a float parameter
func mutateFloat(value, rate, min, max float64) float64 {
	// Simple mutation: add/subtract a percentage
	delta := value * rate * 0.1 // 10% of value * mutation rate
	// Alternate direction randomly based on current nanosecond
	if time.Now().UnixNano()%2 == 0 {
		delta = -delta
	}
	result := value + delta
	// Clamp
	if result < min {
		result = min
	}
	if result > max {
		result = max
	}
	return result
}

func (e *Engine) saveStrategy(s *Strategy) {
	path := filepath.Join(e.dataDir, fmt.Sprintf("%s.json", s.AgentID))
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		e.logger.Error("failed to marshal strategy", "error", err)
		return
	}
	os.WriteFile(path, data, 0640)
}

func (e *Engine) loadStrategies() {
	entries, err := os.ReadDir(e.dataDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(e.dataDir, entry.Name()))
		if err != nil {
			continue
		}
		var s Strategy
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		e.strategies[s.AgentID] = &s
		e.logger.Info("loaded strategy", "agent", s.AgentID, "version", s.Version)
	}
}
