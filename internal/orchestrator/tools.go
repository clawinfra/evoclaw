package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"log/slog"
)

// ToolDefinition represents a tool from skill.toml
type ToolDefinition struct {
	Name        string            `toml:"name"`
	Binary      string            `toml:"binary"`
	Description string            `toml:"description"`
	Parameters  ToolParameters    `toml:"parameters"`
	Sandbox     string            `toml:"sandboxing"`
	Timeout     int               `toml:"timeout_ms"`
	Permissions []string          `toml:"permissions"`
	Metadata    map[string]string `toml:"metadata"`
}

// ToolSchema represents an LLM-compatible tool schema
type ToolSchema struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
	EvoClawMeta ToolMetadata           `json:"evoclaw,omitempty"`
}

// ToolMetadata contains EvoClaw-specific extensions
type ToolMetadata struct {
	Binary      string   `json:"binary"`
	Timeout     int      `json:"timeout_ms"`
	Sandbox     bool     `json:"sandbox"`
	Permissions []string `json:"permissions"`
	Version     string   `json:"version"`
	Skill       string   `json:"skill"`
}

// ToolParameters defines parameter schema
type ToolParameters struct {
	Properties map[string]ParameterDef `toml:"properties"`
	Required   []string                `toml:"required"`
}

// ParameterDef defines a single parameter
type ParameterDef struct {
	Type        string `toml:"type"`
	Description string `toml:"description"`
	Default     any    `toml:"default,omitempty"`
}

// ToolManager handles tool schema generation
type ToolManager struct {
	skillsPath   string
	capabilities []string
	logger       *slog.Logger
	cache        map[string][]ToolSchema
	builtinTools map[string]*BuiltinTool // pi-style built-in tools (name → tool)
	mu           sync.RWMutex
}

// NewToolManager creates a new tool manager
func NewToolManager(skillsPath string, capabilities []string, logger *slog.Logger) *ToolManager {
	if skillsPath == "" {
		// Default to ~/.evoclaw/skills
		home, _ := os.UserHomeDir()
		skillsPath = filepath.Join(home, ".evoclaw", "skills")
	}

	return &ToolManager{
		skillsPath:   skillsPath,
		capabilities: capabilities,
		logger:       logger.With("component", "tool_manager"),
		cache:        make(map[string][]ToolSchema),
	}
}

// GenerateSchemas generates LLM tool schemas for all available tools
func (tm *ToolManager) GenerateSchemas() ([]ToolSchema, error) {
	tm.mu.RLock()
	if cached, ok := tm.cache["all"]; ok {
		tm.mu.RUnlock()
		return cached, nil
	}
	tm.mu.RUnlock()

	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Discover all skills
	skillDirs, err := os.ReadDir(tm.skillsPath)
	if err != nil {
		return nil, fmt.Errorf("read skills directory: %w", err)
	}

	var allTools []ToolDefinition
	for _, skillDir := range skillDirs {
		if !skillDir.IsDir() {
			continue
		}

		skillPath := filepath.Join(tm.skillsPath, skillDir.Name())
		tomlPath := filepath.Join(skillPath, "skill.toml")

		// Load skill.toml
		tools, err := tm.LoadSkillDefinitions(tomlPath)
		if err != nil {
			tm.logger.Warn("failed to load skill", "skill", skillDir.Name(), "error", err)
			continue
		}

		allTools = append(allTools, tools...)
	}

	// Filter by capabilities
	filtered := tm.FilterByCapabilities(allTools)

	// Convert to schemas
	schemas := make([]ToolSchema, 0, len(filtered))
	for _, tool := range filtered {
		schema, err := tm.DefinitionToSchema(tool)
		if err != nil {
			tm.logger.Warn("failed to convert tool to schema", "tool", tool.Name, "error", err)
			continue
		}
		schemas = append(schemas, schema)
	}

	// Cache results
	tm.cache["all"] = schemas

	tm.logger.Info("generated tool schemas", "count", len(schemas))
	return schemas, nil
}

// LoadSkillDefinitions loads tool definitions from skill.toml
func (tm *ToolManager) LoadSkillDefinitions(tomlPath string) ([]ToolDefinition, error) {
	data, err := os.ReadFile(tomlPath)
	if err != nil {
		return nil, fmt.Errorf("read skill.toml: %w", err)
	}

	var skillConfig struct {
		Tools []ToolDefinition `toml:"tools"`
	}

	if err := toml.Unmarshal(data, &skillConfig); err != nil {
		return nil, fmt.Errorf("parse skill.toml: %w", err)
	}

	return skillConfig.Tools, nil
}

// DefinitionToSchema converts a tool definition to LLM schema
func (tm *ToolManager) DefinitionToSchema(def ToolDefinition) (ToolSchema, error) {
	schema := ToolSchema{
		Name:        def.Name,
		Description: def.Description,
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": make(map[string]interface{}),
		},
		EvoClawMeta: ToolMetadata{
			Binary:      def.Binary,
			Timeout:     def.Timeout,
			Permissions: def.Permissions,
			Version:     def.Metadata["version"],
			Skill:       def.Metadata["skill"],
		},
	}

	// Convert parameters to JSON Schema format
	properties := make(map[string]interface{})
	for name, param := range def.Parameters.Properties {
		prop := map[string]interface{}{
			"type":        param.Type,
			"description": param.Description,
		}
		if param.Default != nil {
			prop["default"] = param.Default
		}
		properties[name] = prop
	}

	schema.Parameters["properties"] = properties

	if len(def.Parameters.Required) > 0 {
		schema.Parameters["required"] = def.Parameters.Required
	}

	return schema, nil
}

// FilterByCapabilities filters tools by agent capabilities
func (tm *ToolManager) FilterByCapabilities(tools []ToolDefinition) []ToolDefinition {
	var filtered []ToolDefinition

	for _, tool := range tools {
		if tm.toolAllowed(tool) {
			filtered = append(filtered, tool)
		}
	}

	return filtered
}

// toolAllowed checks if a tool is allowed based on agent capabilities
func (tm *ToolManager) toolAllowed(tool ToolDefinition) bool {
	// If no capabilities specified, allow all
	if len(tm.capabilities) == 0 {
		return true
	}

	// Check if tool requires any capability that agent has
	for _, perm := range tool.Permissions {
		for _, cap := range tm.capabilities {
			if perm == cap {
				return true
			}
		}
	}

	// If tool has no permissions, allow it
	if len(tool.Permissions) == 0 {
		return true
	}

	return false
}

// GetToolTimeout returns the default timeout for a tool
func (tm *ToolManager) GetToolTimeout(toolName string) time.Duration {
	// Default timeouts by tool category
	defaults := map[string]time.Duration{
		"read":       5 * time.Second,
		"write":      5 * time.Second,
		"edit":       5 * time.Second,
		"glob":       5 * time.Second,
		"grep":       5 * time.Second,
		"bash":       30 * time.Second,
		"websearch":  30 * time.Second,
		"webfetch":   30 * time.Second,
		"codesearch": 30 * time.Second,
		"question":   10 * time.Second,
		"git_status": 60 * time.Second,
		"git_diff":   60 * time.Second,
		"git_commit": 60 * time.Second,
		"git_log":    60 * time.Second,
		"git_branch": 60 * time.Second,
	}

	if timeout, ok := defaults[toolName]; ok {
		return timeout
	}

	return 10 * time.Second // Default fallback
}

// GenerateToolSchemaJSON returns the tool schemas as JSON string for LLM APIs
func (tm *ToolManager) GenerateToolSchemaJSON() (string, error) {
	schemas, err := tm.GenerateSchemas()
	if err != nil {
		return "", err
	}

	data, err := json.MarshalIndent(schemas, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal tool schemas: %w", err)
	}

	return string(data), nil
}

// InvalidateCache clears the cached tool schemas
func (tm *ToolManager) InvalidateCache() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.cache = make(map[string][]ToolSchema)
}

// ---------------------------------------------------------------------------
// Built-in tool integration (pi-inspired architecture)
// ---------------------------------------------------------------------------

// RegisterBuiltinTools adds pi-style built-in tools to the manager.
// These take precedence over TOML definitions with the same name.
// When both a TOML tool and a BuiltinTool share the same name, the
// BuiltinTool wins — allowing the factory-created tools to override
// skill.toml definitions while keeping backward compatibility.
func (tm *ToolManager) RegisterBuiltinTools(tools []*BuiltinTool) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Invalidate cache since tool set changed
	tm.cache = make(map[string][]ToolSchema)

	// Store built-in tools
	if tm.builtinTools == nil {
		tm.builtinTools = make(map[string]*BuiltinTool)
	}
	for _, tool := range tools {
		tm.builtinTools[tool.Name()] = tool
		tm.logger.Info("registered built-in tool", "name", tool.Name(), "backend", tool.backend.Name)
	}
}

// GetBuiltinTool returns a registered built-in tool by name, or nil.
func (tm *ToolManager) GetBuiltinTool(name string) *BuiltinTool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	if tm.builtinTools == nil {
		return nil
	}
	return tm.builtinTools[name]
}

// GenerateSchemasWithBuiltins generates LLM tool schemas including registered
// built-in tools. Built-in tools override TOML tools with the same name.
func (tm *ToolManager) GenerateSchemasWithBuiltins() ([]ToolSchema, error) {
	// Get TOML-based schemas
	tomlSchemas, err := tm.GenerateSchemas()
	if err != nil {
		return nil, err
	}

	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if len(tm.builtinTools) == 0 {
		return tomlSchemas, nil
	}

	// Build name set of built-in tools for dedup
	builtinNames := make(map[string]bool)
	for name := range tm.builtinTools {
		builtinNames[name] = true
	}

	// Filter out TOML tools that are overridden by built-ins
	var merged []ToolSchema
	for _, ts := range tomlSchemas {
		if !builtinNames[ts.Name] {
			merged = append(merged, ts)
		}
	}

	// Add built-in tool schemas
	for _, tool := range tm.builtinTools {
		merged = append(merged, builtinToSchema(tool))
	}

	return merged, nil
}

// builtinToSchema converts a BuiltinTool to ToolSchema for LLM API calls.
func builtinToSchema(t *BuiltinTool) ToolSchema {
	return ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters:  t.Schema(),
		EvoClawMeta: ToolMetadata{
			Binary:  "builtin",
			Timeout: int(t.opts.BashTimeout.Milliseconds()),
		},
	}
}
