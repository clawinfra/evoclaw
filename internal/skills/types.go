package skills

import "time"

// SkillManifest represents parsed SKILL.md frontmatter metadata.
type SkillManifest struct {
	Name        string   `yaml:"name"`
	Version     string   `yaml:"version"`
	Description string   `yaml:"description"`
	Author      string   `yaml:"author"`
	License     string   `yaml:"license"`
	Metadata    Metadata `yaml:"metadata"`
}

// Metadata holds evoclaw-specific skill configuration.
type Metadata struct {
	EvoClaw EvoclawMeta `yaml:"evoclaw"`
}

// EvoclawMeta contains permission and env requirements.
type EvoclawMeta struct {
	Permissions []string `yaml:"permissions"`
	Env         []string `yaml:"env"`
}

// ToolDef represents a tool definition loaded from agent.toml.
type ToolDef struct {
	Name        string        `toml:"-"`
	Command     string        `toml:"command"`
	Description string        `toml:"description"`
	Args        []string      `toml:"args"`
	Env         []string      `toml:"env"`
	TimeoutSecs int           `toml:"timeout_secs"`
	Timeout     time.Duration `toml:"-"`
}

// Skill is a fully loaded skill with manifest and tools.
type Skill struct {
	Manifest SkillManifest
	Tools    map[string]*ToolDef
	Dir      string // absolute path to skill directory
	Healthy  bool
}

// ToolResult holds the output of a tool execution.
type ToolResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}
