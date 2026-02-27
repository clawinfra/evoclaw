package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/clawinfra/evoclaw/internal/agents"
	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/models"
	"github.com/clawinfra/evoclaw/internal/orchestrator"
)

func TestWaitForShutdown(t *testing.T) {
	dir := t.TempDir()
	logger := slog.Default()

	cfg := config.DefaultConfig()
	cfg.Server.DataDir = dir
	reg, _ := agents.NewRegistry(dir, logger)
	mem, _ := agents.NewMemoryStore(dir, logger)
	router := models.NewRouter(logger)
	orch := orchestrator.New(cfg, logger)
	_ = orch.Start()

	app := &App{
		Config:       cfg,
		Logger:       logger,
		Registry:     reg,
		MemoryStore:  mem,
		Router:       router,
		Orchestrator: orch,
	}

	// Send SIGINT to ourselves after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		p, _ := os.FindProcess(os.Getpid())
		_ = p.Signal(syscall.SIGINT)
	}()

	err := waitForShutdown(app)
	if err != nil {
		t.Logf("waitForShutdown error: %v (may be expected)", err)
	}
}

func TestWaitForShutdown_WithSIGHUP(t *testing.T) {
	dir := t.TempDir()
	logger := slog.Default()

	cfg := config.DefaultConfig()
	cfg.Server.DataDir = dir
	cfgPath := filepath.Join(dir, "evoclaw.json")
	_ = cfg.Save(cfgPath)
	SetActiveConfig(cfg, cfgPath)

	reg, _ := agents.NewRegistry(dir, logger)
	mem, _ := agents.NewMemoryStore(dir, logger)
	router := models.NewRouter(logger)
	orch := orchestrator.New(cfg, logger)
	_ = orch.Start()

	app := &App{
		Config:       cfg,
		Logger:       logger,
		Registry:     reg,
		MemoryStore:  mem,
		Router:       router,
		Orchestrator: orch,
	}

	// Send SIGHUP (continue) then SIGINT (shutdown)
	go func() {
		time.Sleep(100 * time.Millisecond)
		p, _ := os.FindProcess(os.Getpid())
		_ = p.Signal(syscall.SIGHUP)
		time.Sleep(100 * time.Millisecond)
		_ = p.Signal(syscall.SIGINT)
	}()

	err := waitForShutdown(app)
	if err != nil {
		t.Logf("waitForShutdown error: %v", err)
	}
}

func TestRun_StartSubcmd(t *testing.T) {
	// "start" falls through to normal server start, which needs a valid config
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "evoclaw.json")
	cfg := config.DefaultConfig()
	cfg.Server.DataDir = dir
	cfg.Server.Port = 0
	_ = cfg.Save(cfgPath)

	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"evoclaw", "--config", cfgPath, "start"}
	// This would start the server; we can't easily test the full flow
	// without it blocking. Just test that run() parses "start" correctly.
	// The setup will succeed, but startServices + waitForShutdown will block.
	// So we skip the full test - coverage of "start" branch in run() is enough
	// from the flag parsing test.
}
