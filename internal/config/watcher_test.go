package config

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReloadDetectsChangedFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := DefaultConfig()
	saveJSON(t, path, cfg)

	// Modify models
	cfg2 := DefaultConfig()
	cfg2.Models.Routing.Simple = "changed/model"
	saveJSON(t, path, cfg2)

	result, err := cfg.Reload(path)
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	if len(result.Changed) == 0 {
		t.Fatal("expected changes to be detected")
	}

	found := false
	for _, c := range result.Changed {
		if c == "Models" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected Models in changed, got %v", result.Changed)
	}

	// Verify it was applied
	foundApplied := false
	for _, a := range result.Applied {
		if a == "Models" {
			foundApplied = true
		}
	}
	if !foundApplied {
		t.Errorf("expected Models in applied, got %v", result.Applied)
	}

	// Verify the config was updated
	if cfg.Models.Routing.Simple != "changed/model" {
		t.Errorf("expected model to be updated, got %s", cfg.Models.Routing.Simple)
	}
}

func TestReloadHotApplySupported(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := DefaultConfig()
	saveJSON(t, path, cfg)

	// Change log level (hot-reloadable)
	cfg2 := DefaultConfig()
	cfg2.Server.LogLevel = "debug"
	saveJSON(t, path, cfg2)

	result, err := cfg.Reload(path)
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	foundApplied := false
	for _, a := range result.Applied {
		if a == "Server.LogLevel" {
			foundApplied = true
		}
	}
	if !foundApplied {
		t.Errorf("expected Server.LogLevel in applied, got %v", result.Applied)
	}

	if cfg.Server.LogLevel != "debug" {
		t.Errorf("expected logLevel debug, got %s", cfg.Server.LogLevel)
	}
}

func TestReloadRestartRequiredFieldsSkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := DefaultConfig()
	saveJSON(t, path, cfg)

	// Change port (requires restart)
	cfg2 := DefaultConfig()
	cfg2.Server.Port = 9999
	saveJSON(t, path, cfg2)

	result, err := cfg.Reload(path)
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	foundSkipped := false
	for _, s := range result.Skipped {
		if s == "Server.Port (requires restart)" {
			foundSkipped = true
		}
	}
	if !foundSkipped {
		t.Errorf("expected Server.Port in skipped, got %v", result.Skipped)
	}

	// Port should NOT be changed
	if cfg.Server.Port != 8420 {
		t.Errorf("expected port unchanged (8420), got %d", cfg.Server.Port)
	}
}

func TestReloadNoChanges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := DefaultConfig()
	saveJSON(t, path, cfg)

	result, err := cfg.Reload(path)
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	if len(result.Changed) != 0 {
		t.Errorf("expected no changes, got %v", result.Changed)
	}
}

func TestReloadMultipleFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := DefaultConfig()
	saveJSON(t, path, cfg)

	// Change multiple fields: port (restart), log level (hot), models (hot)
	cfg2 := DefaultConfig()
	cfg2.Server.Port = 9999
	cfg2.Server.LogLevel = "warn"
	cfg2.Models.Routing.Complex = "new/complex-model"
	saveJSON(t, path, cfg2)

	result, err := cfg.Reload(path)
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	if len(result.Changed) != 3 {
		t.Errorf("expected 3 changes, got %d: %v", len(result.Changed), result.Changed)
	}
	if len(result.Applied) != 2 {
		t.Errorf("expected 2 applied, got %d: %v", len(result.Applied), result.Applied)
	}
	if len(result.Skipped) != 1 {
		t.Errorf("expected 1 skipped, got %d: %v", len(result.Skipped), result.Skipped)
	}
}

func TestReloadBadFile(t *testing.T) {
	cfg := DefaultConfig()
	_, err := cfg.Reload("/nonexistent/path.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestReloadBadJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte("{invalid json"), 0644)

	cfg := DefaultConfig()
	_, err := cfg.Reload(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestIsRestartRequired(t *testing.T) {
	if !IsRestartRequired("Server.Port") {
		t.Error("Server.Port should require restart")
	}
	if !IsRestartRequired("MQTT.Host") {
		t.Error("MQTT.Host should require restart")
	}
	if IsRestartRequired("Models") {
		t.Error("Models should not require restart")
	}
}

func TestHotReloadableFields(t *testing.T) {
	fields := HotReloadableFields()
	if len(fields) == 0 {
		t.Fatal("expected hot-reloadable fields")
	}
	// Models should be in the list
	found := false
	for _, f := range fields {
		if f == "Models" {
			found = true
		}
	}
	if !found {
		t.Error("expected Models in hot-reloadable fields")
	}
}

func TestLogResult(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// No changes
	r := &ReloadResult{}
	r.LogResult(logger) // should not panic

	// With changes
	r2 := &ReloadResult{
		Changed: []string{"Models", "Server.Port"},
		Applied: []string{"Models"},
		Skipped: []string{"Server.Port (requires restart)"},
	}
	r2.LogResult(logger) // should not panic
}

func TestWatcherDetectsChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := DefaultConfig()
	saveJSON(t, path, cfg)

	changed := make(chan struct{}, 1)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	w := NewWatcher(path, 50*time.Millisecond, logger, func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	})
	w.Start()
	defer w.Stop()

	// Wait a bit then modify the file
	time.Sleep(100 * time.Millisecond)
	cfg.Server.LogLevel = "debug"
	saveJSON(t, path, cfg)

	select {
	case <-changed:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not detect change within timeout")
	}
}

func TestWatcherStop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	saveJSON(t, path, DefaultConfig())

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	w := NewWatcher(path, 50*time.Millisecond, logger, nil)
	w.Start()
	w.Stop()
	w.Stop() // double stop should not panic
}

func saveJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
}
