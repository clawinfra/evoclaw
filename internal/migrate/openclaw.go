// Package migrate provides tools for migrating from OpenClaw to EvoClaw.
package migrate

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Result describes what was migrated.
type Result struct {
	Memory   []string // memory files copied
	Identity []string // identity fields mapped
	Skills   []string // skills mapped
	Config   []string // config fields mapped
	Cron     []string // cron jobs mapped
	Warnings []string // non-fatal warnings
}

// Options controls migration behavior.
type Options struct {
	Source string // OpenClaw home directory (default ~/.openclaw)
	Target string // EvoClaw home directory (default ~/.evoclaw)
	DryRun bool   // if true, report what would happen without writing
}

// OpenClaw migrates from an OpenClaw installation to EvoClaw format.
func OpenClaw(opts Options) (*Result, error) {
	if opts.Source == "" {
		home, _ := os.UserHomeDir()
		opts.Source = filepath.Join(home, ".openclaw")
	}
	if opts.Target == "" {
		home, _ := os.UserHomeDir()
		opts.Target = filepath.Join(home, ".evoclaw")
	}

	if _, err := os.Stat(opts.Source); os.IsNotExist(err) {
		return nil, fmt.Errorf("source directory does not exist: %s", opts.Source)
	}

	result := &Result{}

	// Migrate memory files
	if err := migrateMemory(opts, result); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("memory migration: %v", err))
	}

	// Migrate identity
	if err := migrateIdentity(opts, result); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("identity migration: %v", err))
	}

	// Migrate skills
	if err := migrateSkills(opts, result); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("skills migration: %v", err))
	}

	// Migrate config
	if err := migrateConfig(opts, result); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("config migration: %v", err))
	}

	// Migrate cron jobs
	if err := migrateCron(opts, result); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("cron migration: %v", err))
	}

	return result, nil
}

// migrateMemory copies MEMORY.md and memory/*.md files.
func migrateMemory(opts Options, result *Result) error {
	// Copy MEMORY.md
	memFile := filepath.Join(opts.Source, "MEMORY.md")
	if _, err := os.Stat(memFile); err == nil {
		dst := filepath.Join(opts.Target, "memory", "MEMORY.md")
		if err := copyFile(memFile, dst, opts.DryRun); err != nil {
			return err
		}
		result.Memory = append(result.Memory, "MEMORY.md")
	}

	// Copy memory/*.md (daily notes)
	memDir := filepath.Join(opts.Source, "memory")
	if info, err := os.Stat(memDir); err == nil && info.IsDir() {
		err := filepath.WalkDir(memDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if !strings.HasSuffix(d.Name(), ".md") {
				return nil
			}
			rel, _ := filepath.Rel(memDir, path)
			dst := filepath.Join(opts.Target, "memory", rel)
			if err := copyFile(path, dst, opts.DryRun); err != nil {
				return err
			}
			result.Memory = append(result.Memory, rel)
			return nil
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// OpenClawIdentity represents parsed identity fields from OpenClaw files.
type OpenClawIdentity struct {
	Name        string `json:"name"`
	Persona     string `json:"persona"`
	Voice       string `json:"voice"`
	Description string `json:"description"`
}

// migrateIdentity parses AGENTS.md, SOUL.md, IDENTITY.md into agent.toml fields.
func migrateIdentity(opts Options, result *Result) error {
	identity := &OpenClawIdentity{}

	// Parse SOUL.md
	soulPath := filepath.Join(opts.Source, "SOUL.md")
	if data, err := os.ReadFile(soulPath); err == nil {
		identity.Persona = string(data)
		result.Identity = append(result.Identity, "SOUL.md → persona")
	}

	// Parse IDENTITY.md
	idPath := filepath.Join(opts.Source, "IDENTITY.md")
	if data, err := os.ReadFile(idPath); err == nil {
		// Extract name from first heading
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "# ") {
				identity.Name = strings.TrimPrefix(line, "# ")
				break
			}
		}
		identity.Description = string(data)
		result.Identity = append(result.Identity, "IDENTITY.md → name, description")
	}

	// Parse AGENTS.md for voice/behavior
	agentsPath := filepath.Join(opts.Source, "AGENTS.md")
	if data, err := os.ReadFile(agentsPath); err == nil {
		content := string(data)
		if strings.Contains(content, "voice") || strings.Contains(content, "Voice") {
			identity.Voice = "balanced" // default extraction
			result.Identity = append(result.Identity, "AGENTS.md → voice")
		}
	}

	if len(result.Identity) == 0 {
		return nil // nothing to migrate
	}

	// Write agent.toml identity section
	dst := filepath.Join(opts.Target, "agent.toml")
	if !opts.DryRun {
		if err := os.MkdirAll(filepath.Dir(dst), 0750); err != nil {
			return err
		}
		tomlContent := fmt.Sprintf("[identity]\nname = %q\npersona = %q\nvoice = %q\n",
			identity.Name, truncate(identity.Persona, 500), identity.Voice)
		if err := os.WriteFile(dst, []byte(tomlContent), 0640); err != nil {
			return err
		}
	}

	return nil
}

// migrateSkills lists installed skills and maps to EvoClaw plugin format.
func migrateSkills(opts Options, result *Result) error {
	skillsDir := filepath.Join(opts.Source, "skills")
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		return nil
	}

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return err
	}

	var plugins []map[string]string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillName := e.Name()
		result.Skills = append(result.Skills, skillName)

		plugins = append(plugins, map[string]string{
			"name":   skillName,
			"source": "openclaw-migrated",
			"path":   filepath.Join(skillsDir, skillName),
		})
	}

	if len(plugins) > 0 && !opts.DryRun {
		dst := filepath.Join(opts.Target, "plugins.json")
		if err := os.MkdirAll(filepath.Dir(dst), 0750); err != nil {
			return err
		}
		data, _ := json.MarshalIndent(plugins, "", "  ")
		if err := os.WriteFile(dst, data, 0640); err != nil {
			return err
		}
	}

	return nil
}

// OpenClawConfig represents the OpenClaw gateway config (JSON format).
type OpenClawConfig struct {
	Models struct {
		Providers map[string]json.RawMessage `json:"providers"`
	} `json:"models"`
	Channels struct {
		Telegram *struct {
			Enabled  bool   `json:"enabled"`
			BotToken string `json:"botToken"`
		} `json:"telegram,omitempty"`
		Discord *struct {
			Enabled bool   `json:"enabled"`
			Token   string `json:"token"`
		} `json:"discord,omitempty"`
	} `json:"channels"`
	Heartbeat *struct {
		IntervalMs int    `json:"intervalMs"`
		Prompt     string `json:"prompt"`
	} `json:"heartbeat,omitempty"`
}

// migrateConfig parses OpenClaw gateway config (JSON) to EvoClaw config.toml.
func migrateConfig(opts Options, result *Result) error {
	// Try common config locations
	configPaths := []string{
		filepath.Join(opts.Source, "config.json"),
		filepath.Join(opts.Source, "gateway.json"),
		filepath.Join(opts.Source, "evoclaw.json"),
	}

	var configData []byte
	var configPath string
	for _, p := range configPaths {
		data, err := os.ReadFile(p)
		if err == nil {
			configData = data
			configPath = p
			break
		}
	}

	if configData == nil {
		return nil // no config found
	}

	var ocCfg OpenClawConfig
	if err := json.Unmarshal(configData, &ocCfg); err != nil {
		return fmt.Errorf("parse %s: %w", configPath, err)
	}

	var tomlLines []string

	// Map providers
	if len(ocCfg.Models.Providers) > 0 {
		for name := range ocCfg.Models.Providers {
			result.Config = append(result.Config, fmt.Sprintf("provider: %s", name))
		}
		tomlLines = append(tomlLines, "# Model providers migrated from OpenClaw")
		tomlLines = append(tomlLines, "[models]")
		tomlLines = append(tomlLines, "# Review and update provider API keys")
	}

	// Map channels
	if ocCfg.Channels.Telegram != nil && ocCfg.Channels.Telegram.Enabled {
		result.Config = append(result.Config, "channel: telegram")
		tomlLines = append(tomlLines, "\n[channels.telegram]")
		tomlLines = append(tomlLines, "enabled = true")
		tomlLines = append(tomlLines, fmt.Sprintf("bot_token = %q", ocCfg.Channels.Telegram.BotToken))
	}
	if ocCfg.Channels.Discord != nil && ocCfg.Channels.Discord.Enabled {
		result.Config = append(result.Config, "channel: discord")
		tomlLines = append(tomlLines, "\n[channels.discord]")
		tomlLines = append(tomlLines, "enabled = true")
		tomlLines = append(tomlLines, fmt.Sprintf("token = %q", ocCfg.Channels.Discord.Token))
	}

	// Map heartbeat
	if ocCfg.Heartbeat != nil {
		result.Config = append(result.Config, "heartbeat settings")
		tomlLines = append(tomlLines, "\n[scheduler]")
		tomlLines = append(tomlLines, "enabled = true")
		tomlLines = append(tomlLines, fmt.Sprintf("# Heartbeat interval: %dms", ocCfg.Heartbeat.IntervalMs))
	}

	if len(tomlLines) > 0 && !opts.DryRun {
		dst := filepath.Join(opts.Target, "config.toml")
		if err := os.MkdirAll(filepath.Dir(dst), 0750); err != nil {
			return err
		}
		content := strings.Join(tomlLines, "\n") + "\n"
		if err := os.WriteFile(dst, []byte(content), 0640); err != nil {
			return err
		}
	}

	return nil
}

// OpenClawCronJob represents an OpenClaw cron job entry.
type OpenClawCronJob struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Schedule string `json:"schedule"` // cron expression or interval
	Command  string `json:"command"`
	Model    string `json:"model,omitempty"`
	Enabled  bool   `json:"enabled"`
}

// migrateCron converts OpenClaw cron jobs to EvoClaw scheduler format.
func migrateCron(opts Options, result *Result) error {
	cronPaths := []string{
		filepath.Join(opts.Source, "crons.json"),
		filepath.Join(opts.Source, "scheduler.json"),
	}

	var cronData []byte
	for _, p := range cronPaths {
		data, err := os.ReadFile(p)
		if err == nil {
			cronData = data
			break
		}
	}

	if cronData == nil {
		return nil
	}

	var jobs []OpenClawCronJob
	if err := json.Unmarshal(cronData, &jobs); err != nil {
		return fmt.Errorf("parse cron jobs: %w", err)
	}

	type evoJob struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Schedule string `json:"schedule"`
		Action   string `json:"action"`
		Model    string `json:"model,omitempty"`
		Enabled  bool   `json:"enabled"`
	}

	var evoJobs []evoJob
	for _, j := range jobs {
		evoJobs = append(evoJobs, evoJob{
			ID:       j.ID,
			Name:     j.Name,
			Schedule: j.Schedule,
			Action:   j.Command,
			Model:    j.Model,
			Enabled:  j.Enabled,
		})
		result.Cron = append(result.Cron, j.Name)
	}

	if len(evoJobs) > 0 && !opts.DryRun {
		dst := filepath.Join(opts.Target, "scheduler.json")
		if err := os.MkdirAll(filepath.Dir(dst), 0750); err != nil {
			return err
		}
		data, _ := json.MarshalIndent(evoJobs, "", "  ")
		if err := os.WriteFile(dst, data, 0640); err != nil {
			return err
		}
	}

	return nil
}

// copyFile copies src to dst, creating parent directories.
func copyFile(src, dst string, dryRun bool) error {
	if dryRun {
		return nil
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0750); err != nil {
		return err
	}

	return os.WriteFile(dst, data, 0640)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
