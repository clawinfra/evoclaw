package security

// SecurityConfig represents the security section of the TOML configuration.
type SecurityConfig struct {
	Autonomy AutonomyConfig `toml:"autonomy"`
	Sandbox  SandboxConfig  `toml:"sandbox"`
}

// AutonomyConfig controls autonomy level and access restrictions.
type AutonomyConfig struct {
	Level           string   `toml:"level"`            // "readonly", "supervised", "full"
	WorkspaceOnly   bool     `toml:"workspace_only"`
	AllowedCommands []string `toml:"allowed_commands"`
	ForbiddenPaths  []string `toml:"forbidden_paths"`
	AllowedRoots    []string `toml:"allowed_roots"`
}

// SandboxConfig controls workspace sandboxing.
type SandboxConfig struct {
	WorkspacePath string `toml:"workspace_path"`
}

// DefaultSecurityConfig returns a reasonable default configuration.
func DefaultSecurityConfig() SecurityConfig {
	return SecurityConfig{
		Autonomy: AutonomyConfig{
			Level:         "supervised",
			WorkspaceOnly: true,
			AllowedCommands: []string{
				"git", "npm", "cargo", "ls", "cat", "grep", "find", "head", "tail", "wc",
			},
			ForbiddenPaths: []string{
				"/etc", "/root", "~/.ssh", "~/.gnupg", "~/.aws",
			},
		},
		Sandbox: SandboxConfig{
			WorkspacePath: ".",
		},
	}
}
