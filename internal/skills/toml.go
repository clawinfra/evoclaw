package skills

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseToolsTOML is a minimal TOML parser for agent.toml tool definitions.
// It handles [tools.name] sections with string, string-array, and int fields.
func ParseToolsTOML(data []byte) (map[string]*ToolDef, error) {
	tools := make(map[string]*ToolDef)
	lines := strings.Split(string(data), "\n")

	var current *ToolDef
	var currentName string

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Section header
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			header := line[1 : len(line)-1]
			if strings.HasPrefix(header, "tools.") {
				currentName = strings.TrimPrefix(header, "tools.")
				current = &ToolDef{Name: currentName}
				tools[currentName] = current
			} else {
				current = nil
			}
			continue
		}

		if current == nil {
			continue
		}

		// Key = value
		eqIdx := strings.Index(line, "=")
		if eqIdx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eqIdx])
		val := strings.TrimSpace(line[eqIdx+1:])

		switch key {
		case "command":
			current.Command = unquote(val)
		case "description":
			current.Description = unquote(val)
		case "timeout_secs":
			n, err := strconv.Atoi(val)
			if err == nil {
				current.TimeoutSecs = n
			}
		case "args":
			arr, err := parseStringArray(val)
			if err != nil {
				return nil, fmt.Errorf("parse args for %s: %w", currentName, err)
			}
			current.Args = arr
		case "env":
			arr, err := parseStringArray(val)
			if err != nil {
				return nil, fmt.Errorf("parse env for %s: %w", currentName, err)
			}
			current.Env = arr
		}
	}

	return tools, nil
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

func parseStringArray(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return nil, fmt.Errorf("expected array, got: %s", s)
	}
	s = s[1 : len(s)-1]
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		result = append(result, unquote(p))
	}
	return result, nil
}
