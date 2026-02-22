package security

import "fmt"

// Action represents a tool action to be validated by the security policy.
type Action struct {
	Type    string // "read", "write", "execute", "delete"
	Path    string // file path (if applicable)
	Command string // command string (if applicable)
	Tool    string // tool name
}

// SecurityPolicy enforces workspace sandboxing and command restrictions.
type SecurityPolicy struct {
	WorkspaceOnly   bool
	WorkspacePath   string
	ForbiddenPaths  []string // e.g. /etc, /root, ~/.ssh, ~/.gnupg, ~/.aws
	AllowedCommands []string // e.g. git, npm, cargo, ls, cat, grep
	AllowedRoots    []string // additional allowed directories outside workspace
	AutonomyLevel   string   // "readonly", "supervised", "full"
}

// NewSecurityPolicy creates a SecurityPolicy from a SecurityConfig.
func NewSecurityPolicy(cfg SecurityConfig) *SecurityPolicy {
	return &SecurityPolicy{
		WorkspaceOnly:   cfg.Autonomy.WorkspaceOnly,
		WorkspacePath:   cfg.Sandbox.WorkspacePath,
		ForbiddenPaths:  cfg.Autonomy.ForbiddenPaths,
		AllowedCommands: cfg.Autonomy.AllowedCommands,
		AllowedRoots:    cfg.Autonomy.AllowedRoots,
		AutonomyLevel:   cfg.Autonomy.Level,
	}
}

// DefaultSecurityPolicy returns a reasonable default policy.
func DefaultSecurityPolicy() *SecurityPolicy {
	return &SecurityPolicy{
		WorkspaceOnly: true,
		WorkspacePath: ".",
		ForbiddenPaths: []string{
			"/etc", "/root", "/.ssh", "/.gnupg", "/.aws",
		},
		AllowedCommands: []string{
			"git", "npm", "cargo", "ls", "cat", "grep", "find", "head", "tail", "wc",
		},
		AllowedRoots:  nil,
		AutonomyLevel: "supervised",
	}
}

// ValidatePath checks whether a path is allowed under the policy.
func (p *SecurityPolicy) ValidatePath(path string) error {
	return validatePath(path, p.WorkspacePath, p.ForbiddenPaths, p.AllowedRoots, p.WorkspaceOnly)
}

// ValidateCommand checks whether a command is allowed under the policy.
func (p *SecurityPolicy) ValidateCommand(cmd string) error {
	return validateCommand(cmd, p.AllowedCommands)
}

// IsAllowed determines if an action is permitted, returning (allowed, reason).
func (p *SecurityPolicy) IsAllowed(action Action) (bool, string) {
	// Check autonomy level first
	switch p.AutonomyLevel {
	case "readonly":
		if action.Type == "write" || action.Type == "execute" || action.Type == "delete" {
			return false, fmt.Sprintf("autonomy level 'readonly' blocks %s actions", action.Type)
		}
	case "supervised":
		if action.Type == "delete" {
			return false, "autonomy level 'supervised' blocks delete actions without approval"
		}
	case "full":
		// no autonomy-level restriction
	default:
		return false, fmt.Sprintf("unknown autonomy level: %s", p.AutonomyLevel)
	}

	// Validate path if present
	if action.Path != "" {
		if err := p.ValidatePath(action.Path); err != nil {
			return false, err.Error()
		}
	}

	// Validate command if present
	if action.Command != "" {
		if err := p.ValidateCommand(action.Command); err != nil {
			return false, err.Error()
		}
	}

	return true, ""
}
