package security

import (
	"fmt"
	"strings"
)

// shellInjectionPatterns are patterns that indicate potential shell injection.
var shellInjectionPatterns = []string{
	"$(", "`", "&&", "||", ";", "|", ">", "<", "\n", "\r",
}

// validateCommand checks that a command is on the allowlist and free of injection.
func validateCommand(cmd string, allowedCommands []string) error {
	if cmd == "" {
		return fmt.Errorf("empty command")
	}

	// Block shell injection patterns
	for _, pattern := range shellInjectionPatterns {
		if strings.Contains(cmd, pattern) {
			return fmt.Errorf("command contains blocked pattern %q", pattern)
		}
	}

	// Extract binary name (first token)
	binary := extractBinary(cmd)

	// Check allowlist
	if len(allowedCommands) == 0 {
		return fmt.Errorf("no commands are allowed")
	}

	for _, allowed := range allowedCommands {
		if allowed == "*" {
			return nil // wildcard allows everything
		}
		if binary == allowed {
			return nil
		}
	}

	return fmt.Errorf("command %q is not in the allowed list", binary)
}

// extractBinary returns the base binary name from a command string.
func extractBinary(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return ""
	}
	// Handle path-qualified binaries: /usr/bin/git -> git
	binary := parts[0]
	if idx := strings.LastIndex(binary, "/"); idx >= 0 {
		binary = binary[idx+1:]
	}
	return binary
}
