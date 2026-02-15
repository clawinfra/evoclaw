package skills

import (
	"fmt"
	"log/slog"
	"sync"
)

// Registry maintains loaded skills and their tools.
type Registry struct {
	mu     sync.RWMutex
	skills map[string]*Skill          // skill name -> skill
	tools  map[string]*registeredTool // tool name -> tool+skill
	logger *slog.Logger
}

type registeredTool struct {
	Skill *Skill
	Tool  *ToolDef
}

// NewRegistry creates an empty skill registry.
func NewRegistry(logger *slog.Logger) *Registry {
	return &Registry{
		skills: make(map[string]*Skill),
		tools:  make(map[string]*registeredTool),
		logger: logger,
	}
}

// Register adds a skill and all its tools to the registry.
func (r *Registry) Register(skill *Skill) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := skill.Manifest.Name
	if _, exists := r.skills[name]; exists {
		return fmt.Errorf("skill %q already registered", name)
	}

	r.skills[name] = skill
	for toolName, tool := range skill.Tools {
		fqn := name + "." + toolName
		r.tools[fqn] = &registeredTool{Skill: skill, Tool: tool}
		// Also register short name if no conflict
		if _, exists := r.tools[toolName]; !exists {
			r.tools[toolName] = &registeredTool{Skill: skill, Tool: tool}
		}
	}
	return nil
}

// GetTool looks up a tool by name (either "skill.tool" or just "tool").
func (r *Registry) GetTool(name string) (*ToolDef, *Skill, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rt, ok := r.tools[name]
	if !ok {
		return nil, nil, fmt.Errorf("tool %q not found", name)
	}
	return rt.Tool, rt.Skill, nil
}

// ListSkills returns all registered skills.
func (r *Registry) ListSkills() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Skill, 0, len(r.skills))
	for _, s := range r.skills {
		result = append(result, s)
	}
	return result
}

// ListTools returns all registered tool names.
func (r *Registry) ListTools() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return only fully-qualified names
	var names []string
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// SkillCount returns the number of registered skills.
func (r *Registry) SkillCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.skills)
}

// SetHealth updates a skill's health status.
func (r *Registry) SetHealth(skillName string, healthy bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.skills[skillName]; ok {
		s.Healthy = healthy
	}
}
