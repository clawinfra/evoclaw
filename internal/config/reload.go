package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"sync"
)

// ReloadResult describes what changed during a config reload.
type ReloadResult struct {
	Changed []string // list of changed fields
	Applied []string // successfully applied
	Skipped []string // require restart
	Errors  []error
}

// restartRequiredFields lists top-level config fields that cannot be
// hot-reloaded and require a full process restart.
var restartRequiredFields = map[string]bool{
	"Server.Port":    true,
	"Server.DataDir": true,
	"MQTT.Port":      true,
	"MQTT.Host":      true,
}

// hotReloadableFields lists fields that can be applied at runtime.
var hotReloadableFields = []string{
	"Models",
	"Channels",
	"Evolution",
	"Scheduler",
	"Server.LogLevel",
	"Agents",
	"Memory",
	"CloudSync",
	"Updates",
}

// mu protects the Config during concurrent reload operations.
var mu sync.RWMutex

// RLock acquires a read lock on the config.
func RLock() { mu.RLock() }

// RUnlock releases a read lock on the config.
func RUnlock() { mu.RUnlock() }

// Reload re-reads the config from path, diffs against the current config,
// and applies hot-reloadable changes in place. Fields that require a
// restart are logged as skipped.
func (c *Config) Reload(path string) (*ReloadResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config for reload: %w", err)
	}

	newCfg := DefaultConfig()
	if err := json.Unmarshal(data, newCfg); err != nil {
		return nil, fmt.Errorf("parse config for reload: %w", err)
	}

	result := &ReloadResult{}

	mu.Lock()
	defer mu.Unlock()

	// Compare and apply each section
	diffAndApply(c, newCfg, result)

	return result, nil
}

// diffAndApply compares old and new configs, applying hot-reloadable changes.
func diffAndApply(old, new *Config, result *ReloadResult) {
	// Server.Port
	if old.Server.Port != new.Server.Port {
		result.Changed = append(result.Changed, "Server.Port")
		result.Skipped = append(result.Skipped, "Server.Port (requires restart)")
	}
	// Server.DataDir
	if old.Server.DataDir != new.Server.DataDir {
		result.Changed = append(result.Changed, "Server.DataDir")
		result.Skipped = append(result.Skipped, "Server.DataDir (requires restart)")
	}
	// Server.LogLevel (hot-reloadable)
	if old.Server.LogLevel != new.Server.LogLevel {
		result.Changed = append(result.Changed, "Server.LogLevel")
		old.Server.LogLevel = new.Server.LogLevel
		result.Applied = append(result.Applied, "Server.LogLevel")
	}

	// MQTT.Port
	if old.MQTT.Port != new.MQTT.Port {
		result.Changed = append(result.Changed, "MQTT.Port")
		result.Skipped = append(result.Skipped, "MQTT.Port (requires restart)")
	}
	// MQTT.Host
	if old.MQTT.Host != new.MQTT.Host {
		result.Changed = append(result.Changed, "MQTT.Host")
		result.Skipped = append(result.Skipped, "MQTT.Host (requires restart)")
	}

	// Models (hot-reloadable)
	if !reflect.DeepEqual(old.Models, new.Models) {
		result.Changed = append(result.Changed, "Models")
		old.Models = new.Models
		result.Applied = append(result.Applied, "Models")
	}

	// Channels (hot-reloadable)
	if !reflect.DeepEqual(old.Channels, new.Channels) {
		result.Changed = append(result.Changed, "Channels")
		old.Channels = new.Channels
		result.Applied = append(result.Applied, "Channels")
	}

	// Evolution (hot-reloadable)
	if !reflect.DeepEqual(old.Evolution, new.Evolution) {
		result.Changed = append(result.Changed, "Evolution")
		old.Evolution = new.Evolution
		result.Applied = append(result.Applied, "Evolution")
	}

	// Scheduler (hot-reloadable)
	if !reflect.DeepEqual(old.Scheduler, new.Scheduler) {
		result.Changed = append(result.Changed, "Scheduler")
		old.Scheduler = new.Scheduler
		result.Applied = append(result.Applied, "Scheduler")
	}

	// Agents (hot-reloadable)
	if !reflect.DeepEqual(old.Agents, new.Agents) {
		result.Changed = append(result.Changed, "Agents")
		old.Agents = new.Agents
		result.Applied = append(result.Applied, "Agents")
	}

	// Memory (hot-reloadable)
	if !reflect.DeepEqual(old.Memory, new.Memory) {
		result.Changed = append(result.Changed, "Memory")
		old.Memory = new.Memory
		result.Applied = append(result.Applied, "Memory")
	}

	// CloudSync (hot-reloadable)
	if !reflect.DeepEqual(old.CloudSync, new.CloudSync) {
		result.Changed = append(result.Changed, "CloudSync")
		old.CloudSync = new.CloudSync
		result.Applied = append(result.Applied, "CloudSync")
	}

	// Updates (hot-reloadable)
	if !reflect.DeepEqual(old.Updates, new.Updates) {
		result.Changed = append(result.Changed, "Updates")
		old.Updates = new.Updates
		result.Applied = append(result.Applied, "Updates")
	}
}

// LogResult logs the reload result at the appropriate levels.
func (r *ReloadResult) LogResult(logger *slog.Logger) {
	if len(r.Changed) == 0 {
		logger.Info("config reload: no changes detected")
		return
	}

	logger.Info("config reload complete",
		"changed", len(r.Changed),
		"applied", len(r.Applied),
		"skipped", len(r.Skipped),
		"errors", len(r.Errors),
	)

	for _, field := range r.Applied {
		logger.Info("config field hot-reloaded", "field", field)
	}

	for _, field := range r.Skipped {
		logger.Warn("config field requires restart", "field", field)
	}

	for _, err := range r.Errors {
		logger.Error("config reload error", "error", err)
	}
}

// IsRestartRequired returns true if the field requires a restart.
func IsRestartRequired(field string) bool {
	return restartRequiredFields[field]
}

// HotReloadableFields returns the list of hot-reloadable field names.
func HotReloadableFields() []string {
	return hotReloadableFields
}
