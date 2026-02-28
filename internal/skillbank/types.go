// Package skillbank implements SKILLRL-inspired hierarchical skill learning for EvoClaw.
// It distills reusable skills from agent trajectories, retrieves relevant skills
// for new tasks, and recursively evolves the skill bank over time.
//
// Architecture:
//   - Store: persistent JSONL-backed storage for skills and common mistakes
//   - Distiller: LLM-based extraction of reusable skills from trajectories
//   - Retriever: keyword or embedding-based skill lookup
//   - Injector: formats skills for system-prompt injection
//   - Updater: recursive skill evolution, pruning, and confidence boosting
package skillbank

import (
	"context"
	"time"
)

// Skill represents a reusable principle extracted from agent experience.
type Skill struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Principle   string    `json:"principle"`
	WhenToApply string    `json:"when_to_apply"`
	Example     string    `json:"example,omitempty"`
	Category    string    `json:"category"`     // "general" or task-specific
	TaskType    string    `json:"task_type"`    // empty = general
	Source      string    `json:"source"`       // "distilled", "manual", "evolved"
	Confidence  float64   `json:"confidence"`   // 0.0-1.0
	UsageCount  int       `json:"usage_count"`
	SuccessRate float64   `json:"success_rate"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CommonMistake represents a recurring error pattern agents should avoid.
type CommonMistake struct {
	ID           string `json:"id"`
	Description  string `json:"description"`
	WhyItHappens string `json:"why_it_happens"`
	HowToAvoid   string `json:"how_to_avoid"`
	TaskType     string `json:"task_type,omitempty"`
}

// Trajectory records the full execution trace of a task for distillation.
type Trajectory struct {
	TaskDescription string           `json:"task_description"`
	TaskType        string           `json:"task_type"`
	Steps           []TrajectoryStep `json:"steps"`
	Success         bool             `json:"success"`
	Quality         float64          `json:"quality"` // 0.0-1.0
}

// TrajectoryStep is a single action-observation pair in a trajectory.
type TrajectoryStep struct {
	Action      string    `json:"action"`
	Observation string    `json:"observation"`
	Timestamp   time.Time `json:"timestamp"`
}

// Source constants for skill provenance.
const (
	SourceDistilled = "distilled"
	SourceManual    = "manual"
	SourceEvolved   = "evolved"
)

// Store is the persistence layer for skills and common mistakes.
type Store interface {
	// Add persists a new skill, returning an error if the ID already exists.
	Add(skill Skill) error
	// Get returns a skill by ID.
	Get(id string) (Skill, error)
	// List returns all skills, optionally filtered by category (empty = all).
	List(category string) ([]Skill, error)
	// Update overwrites an existing skill.
	Update(skill Skill) error
	// Delete removes a skill by ID.
	Delete(id string) error
	// Count returns the total number of stored skills.
	Count() int

	// AddMistake persists a new common mistake.
	AddMistake(m CommonMistake) error
	// ListMistakes returns all common mistakes, optionally filtered by task type.
	ListMistakes(taskType string) ([]CommonMistake, error)
	// DeleteMistake removes a mistake by ID.
	DeleteMistake(id string) error
}

// Distiller extracts skills and common mistakes from raw trajectories.
type Distiller interface {
	// Distill processes trajectories and returns reusable skills and common mistakes.
	// Implementations may batch trajectories and call an LLM.
	Distill(ctx context.Context, trajectories []Trajectory) ([]Skill, []CommonMistake, error)
}

// Retriever looks up relevant skills for a given task description.
type Retriever interface {
	// Retrieve returns the top-k skills most relevant to the task description.
	Retrieve(ctx context.Context, taskDescription string, k int) ([]Skill, error)
}

// Updater manages recursive skill evolution.
type Updater interface {
	// Update distills new skills from failure trajectories not covered by existing skills,
	// merges them into the store, and returns the newly added skills.
	Update(ctx context.Context, failures []Trajectory, currentSkills []Skill) ([]Skill, error)
	// PruneStaleSkills archives skills below minSuccessRate that have been used
	// at least minUsage times. Returns the number of pruned skills.
	PruneStaleSkills(ctx context.Context, minSuccessRate float64, minUsage int) (int, error)
	// BoostSkillConfidence updates a skill's success_rate via EMA (α=0.1).
	BoostSkillConfidence(skillID string, succeeded bool) error
}

// Injector formats skills and mistakes for LLM prompt injection.
type Injector interface {
	// FormatForPrompt renders skills and mistakes as a markdown block.
	FormatForPrompt(skills []Skill, mistakes []CommonMistake) string
	// InjectIntoPrompt prepends the formatted block to an existing system prompt.
	InjectIntoPrompt(systemPrompt string, skills []Skill, mistakes []CommonMistake) string
}
