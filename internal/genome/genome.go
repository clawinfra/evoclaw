package genome

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/security"
)

// Genome defines the complete genetic makeup of an agent
type Genome struct {
	Identity            GenomeIdentity              `json:"identity"`
	Skills              map[string]SkillGenome      `json:"skills"`
	Behavior            GenomeBehavior              `json:"behavior"`
	Constraints         GenomeConstraints           `json:"constraints"`
	ConstraintSignature []byte                      `json:"constraint_signature,omitempty"`
	OwnerPublicKey      []byte                      `json:"owner_public_key,omitempty"`
}

// GenomeIdentity defines the agent's identity layer
type GenomeIdentity struct {
	Name    string `json:"name"`
	Persona string `json:"persona"`
	Voice   string `json:"voice"` // concise, verbose, balanced, etc.
}

// SkillGenome defines evolvable parameters for a specific skill
type SkillGenome struct {
	Enabled      bool                   `json:"enabled"`
	Weight       float64                `json:"weight"`        // Layer 2: How much the agent relies on this skill (0.0-1.0)
	Strategies   []string               `json:"strategies,omitempty"`
	Params       map[string]interface{} `json:"params"`
	Fitness      float64                `json:"fitness"`
	Version      int                    `json:"version"`
	Dependencies []string               `json:"dependencies,omitempty"` // Layer 2: Skills this skill depends on
	EvalCount    int                    `json:"eval_count"`             // Layer 2: Number of evaluations for this skill
	Verified     bool                   `json:"verified,omitempty"`     // VBR: last mutation verified
	VFMScore     float64                `json:"vfm_score,omitempty"`    // VFM: value-for-money of last mutation
}

// GenomeBehavior defines behavioral traits
type GenomeBehavior struct {
	RiskTolerance    float64            `json:"risk_tolerance"`    // 0.0-1.0
	Verbosity        float64            `json:"verbosity"`         // 0.0-1.0
	Autonomy         float64            `json:"autonomy"`          // 0.0-1.0
	PromptStyle      string             `json:"prompt_style"`      // Layer 3: "concise", "detailed", "socratic"
	ToolPreferences  map[string]float64 `json:"tool_preferences"`  // Layer 3: tool usage weights
	ResponsePatterns []string           `json:"response_patterns"` // Layer 3: evolved response templates
}

// GenomeConstraints defines hard boundaries (non-evolvable)
type GenomeConstraints struct {
	MaxLossUSD     float64  `json:"max_loss_usd,omitempty"`
	AllowedAssets  []string `json:"allowed_assets,omitempty"`
	BlockedActions []string `json:"blocked_actions,omitempty"`
	MaxDivergence  float64  `json:"max_divergence,omitempty"`  // ADL: max mutation distance from original
	MinVFMScore    float64  `json:"min_vfm_score,omitempty"`   // VFM: minimum value-for-money threshold
}

// BehaviorFeedback represents user feedback on agent behavior (Layer 3)
type BehaviorFeedback struct {
	AgentID   string    `json:"agent_id"`
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`  // "approval", "correction", "engagement", "dismissal"
	Score     float64   `json:"score"` // -1.0 to 1.0
	Context   string    `json:"context"`
}

// DefaultGenome returns a sensible default genome
func DefaultGenome() *Genome {
	return &Genome{
		Identity: GenomeIdentity{
			Name:    "unnamed-agent",
			Persona: "helpful, reliable",
			Voice:   "balanced",
		},
		Skills: make(map[string]SkillGenome),
		Behavior: GenomeBehavior{
			RiskTolerance:    0.3,
			Verbosity:        0.5,
			Autonomy:         0.5,
			PromptStyle:      "balanced",
			ToolPreferences:  make(map[string]float64),
			ResponsePatterns: []string{},
		},
		Constraints: GenomeConstraints{
			MaxLossUSD:     1000.0,
			AllowedAssets:  []string{},
			BlockedActions: []string{},
		},
	}
}

// Validate checks if the genome is valid
func (g *Genome) Validate() error {
	// Check behavior bounds
	if g.Behavior.RiskTolerance < 0 || g.Behavior.RiskTolerance > 1 {
		return fmt.Errorf("risk_tolerance must be between 0 and 1")
	}
	if g.Behavior.Verbosity < 0 || g.Behavior.Verbosity > 1 {
		return fmt.Errorf("verbosity must be between 0 and 1")
	}
	if g.Behavior.Autonomy < 0 || g.Behavior.Autonomy > 1 {
		return fmt.Errorf("autonomy must be between 0 and 1")
	}

	// Check constraints
	if g.Constraints.MaxLossUSD < 0 {
		return fmt.Errorf("max_loss_usd cannot be negative")
	}

	return nil
}

// VerifyConstraints checks the cryptographic signature on the genome's constraints.
// If both OwnerPublicKey and ConstraintSignature are empty the genome is considered
// unsigned — this is allowed for backward compatibility but a warning is logged.
func (g *Genome) VerifyConstraints() error {
	if len(g.OwnerPublicKey) == 0 && len(g.ConstraintSignature) == 0 {
		slog.Warn("genome has unsigned constraints — backward-compat mode")
		return nil
	}
	// Convert local GenomeConstraints to config.GenomeConstraints for the security package.
	cc := g.Constraints.toConfig()
	ok, err := security.VerifyConstraints(cc, g.ConstraintSignature, g.OwnerPublicKey)
	if err != nil {
		return fmt.Errorf("constraint verification failed: %w", err)
	}
	if !ok {
		return security.ErrInvalidSignature
	}
	return nil
}

// toConfig converts the genome-local GenomeConstraints to config.GenomeConstraints.
func (c GenomeConstraints) toConfig() config.GenomeConstraints {
	return config.GenomeConstraints{
		MaxLossUSD:     c.MaxLossUSD,
		AllowedAssets:  c.AllowedAssets,
		BlockedActions: c.BlockedActions,
		MaxDivergence:  c.MaxDivergence,
		MinVFMScore:    c.MinVFMScore,
	}
}

// Clone creates a deep copy of the genome
func (g *Genome) Clone() (*Genome, error) {
	data, err := json.Marshal(g)
	if err != nil {
		return nil, err
	}
	
	var clone Genome
	if err := json.Unmarshal(data, &clone); err != nil {
		return nil, err
	}
	
	return &clone, nil
}

// GetSkill returns a skill genome by name, or nil if not found
func (g *Genome) GetSkill(skillName string) *SkillGenome {
	if skill, ok := g.Skills[skillName]; ok {
		return &skill
	}
	return nil
}

// SetSkill updates or creates a skill genome
func (g *Genome) SetSkill(skillName string, skill SkillGenome) {
	if g.Skills == nil {
		g.Skills = make(map[string]SkillGenome)
	}
	g.Skills[skillName] = skill
}

// EnabledSkills returns a list of enabled skill names
func (g *Genome) EnabledSkills() []string {
	enabled := []string{}
	for name, skill := range g.Skills {
		if skill.Enabled {
			enabled = append(enabled, name)
		}
	}
	return enabled
}

// FromLegacy converts a legacy map[string]interface{} genome to the typed Genome struct
func FromLegacy(legacy map[string]interface{}) (*Genome, error) {
	// Convert to JSON and back to get proper types
	data, err := json.Marshal(legacy)
	if err != nil {
		return nil, fmt.Errorf("marshal legacy genome: %w", err)
	}

	var g Genome
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("unmarshal to genome: %w", err)
	}

	return &g, nil
}

// ToLegacy converts a typed Genome to the legacy map format (for backward compatibility)
func (g *Genome) ToLegacy() (map[string]interface{}, error) {
	data, err := json.Marshal(g)
	if err != nil {
		return nil, err
	}

	var legacy map[string]interface{}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, err
	}

	return legacy, nil
}
