package interfaces

import "context"

// Tool is the interface for executable tools.
type Tool interface {
	// Name returns the tool identifier.
	Name() string

	// Description returns a human-readable description.
	Description() string

	// Execute runs the tool with the given parameters.
	Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error)

	// Schema returns the tool's input schema for LLM function calling.
	Schema() ToolSchema
}

// ToolRegistry manages a collection of tools.
type ToolRegistry interface {
	// Register adds a tool to the registry.
	Register(tool Tool) error

	// Get returns the tool with the given name.
	Get(name string) (Tool, bool)

	// List returns all registered tools.
	List() []Tool
}
