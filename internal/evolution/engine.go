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

	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/genome"
	"github.com/clawinfra/evoclaw/internal/security"
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
	// VBR: whether the last mutation was verified
	Verified bool `json:"verified"`
	// VFM: value-for-money score of the last mutation
	VFMScore float64 `json:"vfmScore"`
}

// VFMScore holds the value-for-money breakdown for a mutation
type VFMScore struct {
	FitnessImprovement float64 `json:"fitness_improvement"`
	TokenCostIncrease  float64 `json:"token_cost_increase"`
	LatencyIncrease    float64 `json:"latency_increase"`
	ParamCountIncrease float64 `json:"param_count_increase"`
	Score              float64 `json:"score"` // fitness_improvement / complexity_cost
}

// EvaluateVFM calculates value-for-money for a mutation
func EvaluateVFM(fitnessImprovement, tokenCostIncrease, latencyIncrease, paramCountIncrease float64) VFMScore {
	complexity := tokenCostIncrease + latencyIncrease + paramCountIncrease
	if complexity <= 0 {
		complexity = 0.001 // avoid division by zero; free improvement is great
	}
	score := fitnessImprovement / complexity
	return VFMScore{
		FitnessImprovement: fitnessImprovement,
		TokenCostIncrease:  tokenCostIncrease,
		LatencyIncrease:    latencyIncrease,
		ParamCountIncrease: paramCountIncrease,
		Score:              score,
	}
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
	feedbackMu sync.RWMutex
	feedback   map[string][]genome.BehaviorFeedback // agentID -> feedback list
}

// NewEngine creates a new evolution engine
func NewEngine(dataDir string, logger *slog.Logger) *Engine {
	dir := filepath.Join(dataDir, "evolution")
	_ = os.MkdirAll(dir, 0750)

	e := &Engine{
		strategies: make(map[string]*Strategy),
		history:    make(map[string][]*Strategy),
		dataDir:    dir,
		logger:     logger,
		feedback:   make(map[string][]genome.BehaviorFeedback),
	}

	// Load existing strategies from disk
	e.loadStrategies()

	return e
}

// GetStrategy returns the current strategy for an agent
func (e *Engine) GetStrategy(agentID string) interface{} {
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
func (e *Engine) Mutate(agentID string, mutationRate float64) (interface{}, error) {
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
	_ = os.WriteFile(path, data, 0640)
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

// verifyGenomeConstraints checks that the genome's constraints are validly signed.
// Unsigned genomes (no key, no sig) are allowed with a warning for backward compat.
func (e *Engine) verifyGenomeConstraints(g *config.Genome) error {
	if len(g.OwnerPublicKey) == 0 && len(g.ConstraintSignature) == 0 {
		e.logger.Warn("genome has unsigned constraints — backward-compat mode")
		return nil
	}
	ok, err := security.VerifyConstraints(g.Constraints, g.ConstraintSignature, g.OwnerPublicKey)
	if err != nil {
		return fmt.Errorf("constraint verification failed: %w", err)
	}
	if !ok {
		return security.ErrInvalidSignature
	}
	return nil
}

// GetGenome returns the current genome for an agent
func (e *Engine) GetGenome(agentID string) (*config.Genome, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.getGenomeLocked(agentID)
}

// getGenomeLocked reads a genome from disk without acquiring locks.
// Caller must hold e.mu (read or write).
func (e *Engine) getGenomeLocked(agentID string) (*config.Genome, error) {
	path := filepath.Join(e.dataDir, fmt.Sprintf("%s-genome.json", agentID))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read genome file: %w", err)
	}

	var genome config.Genome
	if err := json.Unmarshal(data, &genome); err != nil {
		return nil, fmt.Errorf("unmarshal genome: %w", err)
	}

	return &genome, nil
}

// UpdateGenome saves a genome to disk
func (e *Engine) UpdateGenome(agentID string, genome *config.Genome) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.updateGenomeLocked(agentID, genome)
}

// updateGenomeLocked saves a genome to disk without acquiring locks.
// Caller must hold e.mu for writing.
func (e *Engine) updateGenomeLocked(agentID string, genome *config.Genome) error {
	path := filepath.Join(e.dataDir, fmt.Sprintf("%s-genome.json", agentID))
	data, err := json.MarshalIndent(genome, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal genome: %w", err)
	}

	if err := os.WriteFile(path, data, 0640); err != nil {
		return fmt.Errorf("write genome file: %w", err)
	}

	e.logger.Info("genome updated", "agent", agentID)
	return nil
}

// EvaluateSkill evaluates a specific skill within an agent's genome
func (e *Engine) EvaluateSkill(agentID, skillName string, metrics map[string]float64) (float64, error) {
	genome, err := e.GetGenome(agentID)
	if err != nil {
		return 0, fmt.Errorf("get genome: %w", err)
	}

	skill, ok := genome.Skills[skillName]
	if !ok {
		return 0, fmt.Errorf("skill not found: %s", skillName)
	}

	// Compute fitness for this skill
	fitness := computeFitness(metrics)

	// Update skill fitness (exponential moving average)
	alpha := 0.3
	if skill.Fitness == 0 {
		skill.Fitness = fitness
	} else {
		skill.Fitness = alpha*fitness + (1-alpha)*skill.Fitness
	}

	genome.Skills[skillName] = skill

	if err := e.UpdateGenome(agentID, genome); err != nil {
		return 0, fmt.Errorf("save genome: %w", err)
	}

	e.logger.Info("skill evaluated",
		"agent", agentID,
		"skill", skillName,
		"fitness", skill.Fitness,
	)

	return skill.Fitness, nil
}

// MutateSkill mutates parameters for a specific skill
func (e *Engine) MutateSkill(agentID, skillName string, mutationRate float64) error {
	genome, err := e.GetGenome(agentID)
	if err != nil {
		return fmt.Errorf("get genome: %w", err)
	}

	// Verify constraints are untampered before any mutation
	if err := e.verifyGenomeConstraints(genome); err != nil {
		return fmt.Errorf("constraint verification before skill mutation: %w", err)
	}

	skill, ok := genome.Skills[skillName]
	if !ok {
		return fmt.Errorf("skill not found: %s", skillName)
	}

	// Mutate skill parameters
	mutatedParams := make(map[string]interface{})
	for k, v := range skill.Params {
		switch val := v.(type) {
		case float64:
			mutatedParams[k] = mutateFloat(val, mutationRate, -10000, 10000)
		case int:
			mutatedParams[k] = int(mutateFloat(float64(val), mutationRate, -10000, 10000))
		case bool:
			// Boolean mutation: flip with probability = mutationRate
			if time.Now().UnixNano()%100 < int64(mutationRate*100) {
				mutatedParams[k] = !val
			} else {
				mutatedParams[k] = val
			}
		default:
			mutatedParams[k] = v // Keep unchanged
		}
	}

	skill.Params = mutatedParams
	skill.Version++
	skill.Fitness = 0 // Reset fitness for re-evaluation

	genome.Skills[skillName] = skill

	if err := e.UpdateGenome(agentID, genome); err != nil {
		return fmt.Errorf("save genome: %w", err)
	}

	e.logger.Info("skill mutated",
		"agent", agentID,
		"skill", skillName,
		"version", skill.Version,
		"mutationRate", mutationRate,
	)

	return nil
}

// ShouldEvolveSkill checks if a specific skill needs evolution
func (e *Engine) ShouldEvolveSkill(agentID, skillName string, minFitness float64, minSamples int) (bool, error) {
	genome, err := e.GetGenome(agentID)
	if err != nil {
		return false, fmt.Errorf("get genome: %w", err)
	}

	skill, ok := genome.Skills[skillName]
	if !ok {
		return false, fmt.Errorf("skill not found: %s", skillName)
	}

	if !skill.Enabled {
		return false, nil
	}

	// Check if we have enough samples (based on version - older versions have more samples)
	if skill.Version < minSamples {
		return false, nil
	}

	return skill.Fitness < minFitness, nil
}

// ================================
// Layer 2: Skill Selection & Composition
// ================================

// SkillContribution tracks how much a skill contributes to overall fitness
type SkillContribution struct {
	SkillName    string
	Contribution float64 // Delta fitness when skill is enabled vs disabled
	EvalCount    int
}

// EvaluateSkillContribution measures how much a skill contributes to agent performance
func (e *Engine) EvaluateSkillContribution(agentID string, skillName string) float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()

	genome, err := e.getGenomeLocked(agentID)
	if err != nil {
		e.logger.Error("failed to get genome for skill contribution", "error", err)
		return 0.0
	}

	skill, ok := genome.Skills[skillName]
	if !ok {
		return 0.0
	}

	// Skill contribution is based on:
	// 1. Individual skill fitness (40%)
	// 2. Usage frequency / weight (30%)
	// 3. Dependency satisfaction (30%)

	fitnessScore := skill.Fitness * 0.4
	weightScore := skill.Weight * 0.3

	// Check if all dependencies are enabled
	depScore := 0.0
	if len(skill.Dependencies) == 0 {
		depScore = 0.3 // No dependencies = full score
	} else {
		satisfiedDeps := 0
		for _, dep := range skill.Dependencies {
			if depSkill, ok := genome.Skills[dep]; ok && depSkill.Enabled {
				satisfiedDeps++
			}
		}
		depScore = (float64(satisfiedDeps) / float64(len(skill.Dependencies))) * 0.3
	}

	contribution := fitnessScore + weightScore + depScore

	e.logger.Info("evaluated skill contribution",
		"agent", agentID,
		"skill", skillName,
		"contribution", contribution,
		"fitness", skill.Fitness,
		"weight", skill.Weight,
	)

	return contribution
}

// OptimizeSkillWeights rebalances skill weights based on fitness contributions
func (e *Engine) OptimizeSkillWeights(agentID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	genome, err := e.getGenomeLocked(agentID)
	if err != nil {
		return fmt.Errorf("get genome: %w", err)
	}

	// Verify constraints are untampered before any mutation
	if err := e.verifyGenomeConstraints(genome); err != nil {
		return fmt.Errorf("constraint verification before weight optimization: %w", err)
	}

	// Calculate total fitness across enabled skills
	totalFitness := 0.0
	enabledSkills := []string{}
	for name, skill := range genome.Skills {
		if skill.Enabled && skill.EvalCount > 0 {
			totalFitness += skill.Fitness
			enabledSkills = append(enabledSkills, name)
		}
	}

	if totalFitness == 0 || len(enabledSkills) == 0 {
		e.logger.Warn("no fitness data for weight optimization", "agent", agentID)
		return nil
	}

	// Rebalance weights proportionally to fitness
	for _, name := range enabledSkills {
		skill := genome.Skills[name]
		// Weight = skill_fitness / total_fitness, normalized to 0.0-1.0
		newWeight := skill.Fitness / totalFitness
		if newWeight > 1.0 {
			newWeight = 1.0
		}
		skill.Weight = newWeight
		genome.Skills[name] = skill

		e.logger.Info("optimized skill weight",
			"agent", agentID,
			"skill", name,
			"weight", newWeight,
			"fitness", skill.Fitness,
		)
	}

	return e.updateGenomeLocked(agentID, genome)
}

// ShouldDisableSkill determines if a skill is consistently underperforming
func (e *Engine) ShouldDisableSkill(agentID string, skillName string) bool {
	genome, err := e.GetGenome(agentID)
	if err != nil {
		return false
	}

	skill, ok := genome.Skills[skillName]
	if !ok || !skill.Enabled {
		return false
	}

	// Disable if:
	// 1. Evaluated at least 10 times
	// 2. Fitness below 0.2 (very poor)
	// 3. Weight below 0.1 (very low contribution)
	if skill.EvalCount >= 10 && skill.Fitness < 0.2 && skill.Weight < 0.1 {
		e.logger.Warn("skill marked for disabling",
			"agent", agentID,
			"skill", skillName,
			"fitness", skill.Fitness,
			"weight", skill.Weight,
			"evalCount", skill.EvalCount,
		)
		return true
	}

	return false
}

// ShouldEnableSkill determines if a disabled skill should be re-enabled
func (e *Engine) ShouldEnableSkill(agentID string, skillName string) bool {
	genome, err := e.GetGenome(agentID)
	if err != nil {
		return false
	}

	skill, ok := genome.Skills[skillName]
	if !ok || skill.Enabled {
		return false
	}

	// Re-enable if:
	// 1. All dependencies are now satisfied
	// 2. Other skills in the same category are performing well
	allDepsEnabled := true
	for _, dep := range skill.Dependencies {
		if depSkill, ok := genome.Skills[dep]; !ok || !depSkill.Enabled {
			allDepsEnabled = false
			break
		}
	}

	if allDepsEnabled {
		e.logger.Info("skill marked for re-enabling",
			"agent", agentID,
			"skill", skillName,
			"reason", "dependencies satisfied",
		)
		return true
	}

	return false
}

// CompositionFitness evaluates how well skills work together
func (e *Engine) CompositionFitness(agentID string, metrics map[string]float64) float64 {
	genome, err := e.GetGenome(agentID)
	if err != nil {
		return 0.0
	}

	// Calculate individual skill fitness sum
	individualFitnessSum := 0.0
	enabledCount := 0
	for _, skill := range genome.Skills {
		if skill.Enabled {
			individualFitnessSum += skill.Fitness
			enabledCount++
		}
	}

	if enabledCount == 0 {
		return 0.0
	}

	avgIndividualFitness := individualFitnessSum / float64(enabledCount)

	// Overall agent performance from metrics
	overallFitness := computeFitness(metrics)

	// Composition bonus: if overall > sum of parts, skills synergize well
	// Composition penalty: if overall < sum of parts, skills conflict
	compositionScore := overallFitness - avgIndividualFitness

	e.logger.Info("composition fitness evaluated",
		"agent", agentID,
		"overallFitness", overallFitness,
		"avgIndividualFitness", avgIndividualFitness,
		"compositionScore", compositionScore,
	)

	return compositionScore
}

// ================================
// Layer 3: Behavioral Evolution
// ================================

// BehaviorMetrics tracks behavioral performance
type BehaviorMetrics struct {
	ApprovalRate      float64 // 0.0-1.0
	TaskCompletionRate float64 // 0.0-1.0
	CostEfficiency    float64 // USD per successful action
	EngagementScore   float64 // 0.0-1.0
}

// SubmitFeedback records user feedback on agent behavior
func (e *Engine) SubmitFeedback(agentID string, feedbackType string, score float64, context string) error {
	feedback := genome.BehaviorFeedback{
		AgentID:   agentID,
		Timestamp: time.Now(),
		Type:      feedbackType,
		Score:     score,
		Context:   context,
	}

	e.feedbackMu.Lock()
	defer e.feedbackMu.Unlock()

	if e.feedback[agentID] == nil {
		e.feedback[agentID] = []genome.BehaviorFeedback{}
	}

	e.feedback[agentID] = append(e.feedback[agentID], feedback)

	// Keep only last 100 feedback entries per agent
	if len(e.feedback[agentID]) > 100 {
		e.feedback[agentID] = e.feedback[agentID][1:]
	}

	e.logger.Info("feedback submitted",
		"agent", agentID,
		"type", feedbackType,
		"score", score,
	)

	return nil
}

// GetBehaviorMetrics calculates behavioral fitness from feedback
func (e *Engine) GetBehaviorMetrics(agentID string) BehaviorMetrics {
	e.feedbackMu.RLock()
	defer e.feedbackMu.RUnlock()

	feedbackList := e.feedback[agentID]
	if len(feedbackList) == 0 {
		return BehaviorMetrics{
			ApprovalRate:       0.5,
			TaskCompletionRate: 0.5,
			CostEfficiency:     1.0,
			EngagementScore:    0.5,
		}
	}

	// Calculate metrics from feedback
	approvals := 0.0
	completions := 0.0
	engagements := 0.0
	total := float64(len(feedbackList))

	for _, fb := range feedbackList {
		if fb.Type == "approval" && fb.Score > 0 {
			approvals++
		}
		if fb.Type == "completion" {
			completions++
		}
		if fb.Type == "engagement" && fb.Score > 0 {
			engagements++
		}
	}

	return BehaviorMetrics{
		ApprovalRate:       approvals / total,
		TaskCompletionRate: completions / total,
		CostEfficiency:     1.0, // Placeholder: should come from actual cost tracking
		EngagementScore:    engagements / total,
	}
}

// BehavioralFitness calculates behavioral fitness score
func (e *Engine) BehavioralFitness(agentID string) float64 {
	metrics := e.GetBehaviorMetrics(agentID)

	// Weighted behavioral fitness:
	// - User approval rate (40%)
	// - Task completion rate (30%)
	// - Efficiency (cost per successful action) (20%)
	// - Engagement (user continues conversation) (10%)

	fitness := (metrics.ApprovalRate * 40.0) +
		(metrics.TaskCompletionRate * 30.0) +
		(metrics.CostEfficiency * 20.0) +
		(metrics.EngagementScore * 10.0)

	e.logger.Info("behavioral fitness calculated",
		"agent", agentID,
		"fitness", fitness,
		"approvalRate", metrics.ApprovalRate,
		"completionRate", metrics.TaskCompletionRate,
		"engagement", metrics.EngagementScore,
	)

	return fitness
}

// MutateBehavior evolves behavioral parameters based on feedback
func (e *Engine) MutateBehavior(agentID string, feedbackScores map[string]float64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	genome, err := e.getGenomeLocked(agentID)
	if err != nil {
		return fmt.Errorf("get genome: %w", err)
	}

	// Verify constraints are untampered before any mutation
	if err := e.verifyGenomeConstraints(genome); err != nil {
		return fmt.Errorf("constraint verification before behavior mutation: %w", err)
	}

	// Behavioral mutation rate is lower than skill mutation (0.1 vs 0.3)
	behaviorMutationRate := 0.1

	// Mutate risk tolerance based on performance feedback
	if score, ok := feedbackScores["risk"]; ok {
		if score > 0 {
			// Positive feedback: slightly increase risk tolerance
			genome.Behavior.RiskTolerance = mutateFloat(genome.Behavior.RiskTolerance, behaviorMutationRate, 0.0, 1.0)
		} else {
			// Negative feedback: reduce risk tolerance
			genome.Behavior.RiskTolerance *= (1 - behaviorMutationRate)
			if genome.Behavior.RiskTolerance < 0.0 {
				genome.Behavior.RiskTolerance = 0.0
			}
		}
	}

	// Mutate verbosity based on engagement feedback
	if score, ok := feedbackScores["verbosity"]; ok {
		if score > 0 {
			genome.Behavior.Verbosity = mutateFloat(genome.Behavior.Verbosity, behaviorMutationRate, 0.0, 1.0)
		} else {
			genome.Behavior.Verbosity *= (1 - behaviorMutationRate)
			if genome.Behavior.Verbosity < 0.0 {
				genome.Behavior.Verbosity = 0.0
			}
		}
	}

	// Mutate autonomy based on user corrections
	if score, ok := feedbackScores["autonomy"]; ok {
		if score > 0 {
			genome.Behavior.Autonomy = mutateFloat(genome.Behavior.Autonomy, behaviorMutationRate, 0.0, 1.0)
		} else {
			genome.Behavior.Autonomy *= (1 - behaviorMutationRate)
			if genome.Behavior.Autonomy < 0.0 {
				genome.Behavior.Autonomy = 0.0
			}
		}
	}

	// Evolve prompt style based on feedback
	currentStyle := genome.Behavior.PromptStyle
	styles := []string{"concise", "balanced", "detailed", "socratic"}
	behaviorFitness := e.BehavioralFitness(agentID)

	if behaviorFitness < 50.0 {
		// Low fitness: try a different prompt style
		for _, style := range styles {
			if style != currentStyle {
				genome.Behavior.PromptStyle = style
				e.logger.Info("mutated prompt style",
					"agent", agentID,
					"from", currentStyle,
					"to", style,
					"reason", "low behavioral fitness",
				)
				break
			}
		}
	}

	if err := e.updateGenomeLocked(agentID, genome); err != nil {
		return fmt.Errorf("save genome: %w", err)
	}

	e.logger.Info("behavior mutated",
		"agent", agentID,
		"riskTolerance", genome.Behavior.RiskTolerance,
		"verbosity", genome.Behavior.Verbosity,
		"autonomy", genome.Behavior.Autonomy,
		"promptStyle", genome.Behavior.PromptStyle,
	)

	return nil
}

// ================================
// VBR (Verify Before Reporting)
// ================================

// VerifyMutation re-evaluates fitness after a mutation and confirms improvement.
// Returns true if the mutation is verified as an improvement.
func (e *Engine) VerifyMutation(agentID, skillName string, metrics map[string]float64) (bool, error) {
	g, err := e.GetGenome(agentID)
	if err != nil {
		return false, fmt.Errorf("get genome: %w", err)
	}

	skill, ok := g.Skills[skillName]
	if !ok {
		return false, fmt.Errorf("skill not found: %s", skillName)
	}

	preFitness := skill.Fitness
	postFitness := computeFitness(metrics)

	verified := postFitness >= preFitness
	skill.Verified = verified
	g.Skills[skillName] = skill

	// Only persist if verified
	if verified {
		if err := e.UpdateGenome(agentID, g); err != nil {
			return false, err
		}
	}

	e.logger.Info("mutation verification",
		"agent", agentID,
		"skill", skillName,
		"preFitness", preFitness,
		"postFitness", postFitness,
		"verified", verified,
	)

	return verified, nil
}

// ================================
// ADL (Anti-Divergence Limit)
// ================================

// DivergenceScore returns cumulative mutation distance for an agent.
// Each mutation increments by 1; this is the version count from original.
func (e *Engine) DivergenceScore(agentID string) float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()

	s, ok := e.strategies[agentID]
	if !ok {
		return 0
	}
	return float64(s.Version)
}

// CheckADL returns true if the agent has exceeded its divergence limit
// and needs a simplification pass.
func (e *Engine) CheckADL(agentID string, maxDivergence float64) bool {
	if maxDivergence <= 0 {
		return false // no limit set
	}
	return e.DivergenceScore(agentID) > maxDivergence
}

// GetBehaviorHistory returns behavioral evolution history for an agent
func (e *Engine) GetBehaviorHistory(agentID string) ([]genome.BehaviorFeedback, error) {
	e.feedbackMu.RLock()
	defer e.feedbackMu.RUnlock()

	feedback := e.feedback[agentID]
	if feedback == nil {
		return []genome.BehaviorFeedback{}, nil
	}

	return feedback, nil
}
