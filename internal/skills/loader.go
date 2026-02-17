package skills

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Loader discovers and loads skills from a directory.
type Loader struct {
	skillsDir string
	logger    *slog.Logger
}

// NewLoader creates a loader that scans the given directory for skills.
func NewLoader(skillsDir string, logger *slog.Logger) *Loader {
	return &Loader{
		skillsDir: skillsDir,
		logger:    logger,
	}
}

// DefaultSkillsDir returns the default ~/.evoclaw/skills/ path.
func DefaultSkillsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".evoclaw", "skills")
	}
	return filepath.Join(home, ".evoclaw", "skills")
}

// LoadAll discovers and loads all skills from the skills directory.
func (l *Loader) LoadAll() ([]*Skill, error) {
	entries, err := os.ReadDir(l.skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			l.logger.Info("skills directory does not exist, skipping", "dir", l.skillsDir)
			return nil, nil
		}
		return nil, fmt.Errorf("read skills dir: %w", err)
	}

	var skills []*Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillDir := filepath.Join(l.skillsDir, entry.Name())
		skill, err := l.loadSkill(skillDir)
		if err != nil {
			l.logger.Warn("failed to load skill", "dir", skillDir, "error", err)
			continue
		}
		skills = append(skills, skill)
		l.logger.Info("loaded skill", "name", skill.Manifest.Name, "version", skill.Manifest.Version, "tools", len(skill.Tools))
	}
	return skills, nil
}

// loadSkill loads a single skill from its directory.
func (l *Loader) loadSkill(dir string) (*Skill, error) {
	// Parse SKILL.md frontmatter
	manifest, err := l.parseManifest(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	// Load tool definitions from agent.toml
	tools, err := l.loadTools(filepath.Join(dir, "agent.toml"))
	if err != nil {
		// agent.toml is optional; skill may have no tools
		l.logger.Debug("no agent.toml for skill", "dir", dir, "error", err)
		tools = make(map[string]*ToolDef)
	}

	// Expand ~ in command paths and set timeouts
	for _, tool := range tools {
		tool.Command = expandHome(tool.Command)
		if tool.TimeoutSecs > 0 {
			tool.Timeout = time.Duration(tool.TimeoutSecs) * time.Second
		} else {
			tool.Timeout = 30 * time.Second // default
		}
	}

	return &Skill{
		Manifest: *manifest,
		Tools:    tools,
		Dir:      dir,
		Healthy:  true,
	}, nil
}

// parseManifest extracts YAML frontmatter from SKILL.md.
func (l *Loader) parseManifest(path string) (*SkillManifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	var inFrontmatter bool
	var yamlLines []string

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			if inFrontmatter {
				break // end of frontmatter
			}
			inFrontmatter = true
			continue
		}
		if inFrontmatter {
			yamlLines = append(yamlLines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if len(yamlLines) == 0 {
		return nil, fmt.Errorf("no YAML frontmatter found in %s", path)
	}

	var manifest SkillManifest
	if err := yaml.Unmarshal([]byte(strings.Join(yamlLines, "\n")), &manifest); err != nil {
		return nil, fmt.Errorf("parse YAML: %w", err)
	}
	return &manifest, nil
}

// loadTools parses tool definitions from an agent.toml file.
func (l *Loader) loadTools(path string) (map[string]*ToolDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseToolsTOML(data)
}

// expandHome replaces leading ~ with the user's home directory.
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}
