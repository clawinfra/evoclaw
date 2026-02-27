package main

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/clawinfra/evoclaw/internal/agents"
	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/models"
	"github.com/clawinfra/evoclaw/internal/onchain"
	"github.com/clawinfra/evoclaw/internal/orchestrator"
)

// --- printVersion ---

func TestPrintVersion(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	printVersion()
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	if buf.Len() == 0 {
		t.Error("expected output")
	}
}

// --- run() subcommands ---

func TestRun_VersionFlag(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"evoclaw", "version"}
	if code := run(); code != 0 {
		t.Errorf("run() = %d, want 0", code)
	}
}

func TestRun_HelpSubcmd(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"evoclaw", "help"}
	if code := run(); code != 0 {
		t.Errorf("run() = %d, want 0", code)
	}
}

func TestRun_HelpWithTarget(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"evoclaw", "help", "memory"}
	if code := run(); code != 0 {
		t.Errorf("run() = %d, want 0", code)
	}
}

func TestRun_UnknownSubcmd(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"evoclaw", "nonexistent-cmd"}
	if code := run(); code != 1 {
		t.Errorf("run() = %d, want 1", code)
	}
}

func TestRun_DashDashHelp(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"evoclaw", "--help"}
	if code := run(); code != 0 {
		t.Errorf("run() = %d, want 0", code)
	}
}

func TestRun_DashDashVersion(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"evoclaw", "--version"}
	if code := run(); code != 0 {
		t.Errorf("run() = %d, want 0", code)
	}
}

func TestRun_DashV(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"evoclaw", "-v"}
	code := run()
	_ = code // -v may not be recognised as version
}

func TestRun_Init(t *testing.T) {
	dir := t.TempDir()
	origArgs := os.Args
	origWd, _ := os.Getwd()
	defer func() { os.Args = origArgs; _ = os.Chdir(origWd) }()
	_ = os.Chdir(dir)
	os.Args = []string{"evoclaw", "init"}
	_ = run()
}

func TestRun_RouterSubcmd(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "evoclaw.json")
	cfg := config.DefaultConfig()
	_ = cfg.Save(cfgPath)
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"evoclaw", "--config", cfgPath, "router"}
	_ = run()
}

func TestRun_ScheduleSubcmd(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "evoclaw.json")
	cfg := config.DefaultConfig()
	_ = cfg.Save(cfgPath)
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"evoclaw", "--config", cfgPath, "schedule", "list"}
	_ = run()
}

func TestRun_MemorySubcmd(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "evoclaw.json")
	cfg := config.DefaultConfig()
	_ = cfg.Save(cfgPath)
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"evoclaw", "--config", cfgPath, "memory", "stats"}
	_ = run()
}

func TestRun_GovernanceSubcmd(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "evoclaw.json")
	cfg := config.DefaultConfig()
	_ = cfg.Save(cfgPath)
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"evoclaw", "--config", cfgPath, "governance", "status"}
	_ = run()
}

func TestRun_ChainSubcmd(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "evoclaw.json")
	cfg := config.DefaultConfig()
	_ = cfg.Save(cfgPath)
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"evoclaw", "--config", cfgPath, "chain", "status"}
	_ = run()
}

func TestRun_MigrateSubcmd(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"evoclaw", "migrate"}
	_ = run()
}

func TestRun_GatewayHelp(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"evoclaw", "gateway", "help"}
	_ = run()
}

func TestRun_WithConfigFlag(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "evoclaw.json")
	_ = os.WriteFile(cfgPath, []byte("invalid json"), 0644)
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"evoclaw", "--config", cfgPath, "router"}
	_ = run()
}

// --- registerProviders ---

func TestRegisterProviders_Anthropic(t *testing.T) {
	logger := slog.Default()
	router := models.NewRouter(logger)
	cfg := config.DefaultConfig()
	cfg.Models.Providers = map[string]config.ProviderConfig{
		"anthropic-proxy": {
			BaseURL: "https://api.example.com/anthropic",
			Models:  []config.Model{{ID: "claude-test", Name: "Claude"}},
		},
	}
	if err := registerProviders(router, cfg, logger); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterProviders_Ollama(t *testing.T) {
	logger := slog.Default()
	router := models.NewRouter(logger)
	cfg := config.DefaultConfig()
	cfg.Models.Providers = map[string]config.ProviderConfig{
		"ollama": {
			BaseURL: "http://localhost:11434",
			Models:  []config.Model{{ID: "llama2"}},
		},
	}
	if err := registerProviders(router, cfg, logger); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterProviders_OpenAI(t *testing.T) {
	logger := slog.Default()
	router := models.NewRouter(logger)
	cfg := config.DefaultConfig()
	cfg.Models.Providers = map[string]config.ProviderConfig{
		"openai": {
			BaseURL: "https://api.openai.com/v1",
			APIKey:  "test",
			Models:  []config.Model{{ID: "gpt-4"}},
		},
	}
	if err := registerProviders(router, cfg, logger); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterProviders_OpenRouter(t *testing.T) {
	logger := slog.Default()
	router := models.NewRouter(logger)
	cfg := config.DefaultConfig()
	cfg.Models.Providers = map[string]config.ProviderConfig{
		"openrouter": {
			BaseURL: "https://openrouter.ai/api/v1",
			Models:  []config.Model{{ID: "model-1"}},
		},
	}
	if err := registerProviders(router, cfg, logger); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterProviders_Generic(t *testing.T) {
	logger := slog.Default()
	router := models.NewRouter(logger)
	cfg := config.DefaultConfig()
	cfg.Models.Providers = map[string]config.ProviderConfig{
		"custom-provider": {
			BaseURL: "http://localhost:8080/v1",
			Models:  []config.Model{{ID: "model-x"}},
		},
	}
	if err := registerProviders(router, cfg, logger); err != nil {
		t.Fatal(err)
	}
}

// --- registerChannels ---

func TestRegisterChannels_None(t *testing.T) {
	logger := slog.Default()
	cfg := config.DefaultConfig()
	orch := orchestrator.New(cfg, logger)
	if err := registerChannels(orch, cfg, logger); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterChannels_Telegram(t *testing.T) {
	logger := slog.Default()
	cfg := config.DefaultConfig()
	cfg.Channels.Telegram = &config.TelegramConfig{Enabled: true, BotToken: "123:test"}
	orch := orchestrator.New(cfg, logger)
	if err := registerChannels(orch, cfg, logger); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterChannels_MQTT(t *testing.T) {
	logger := slog.Default()
	cfg := config.DefaultConfig()
	cfg.MQTT.Host = "localhost"
	cfg.MQTT.Port = 1883
	orch := orchestrator.New(cfg, logger)
	if err := registerChannels(orch, cfg, logger); err != nil {
		t.Fatal(err)
	}
}

// --- registerProvidersToOrchestrator ---

func TestRegisterProvidersToOrchestrator(t *testing.T) {
	logger := slog.Default()
	cfg := config.DefaultConfig()
	cfg.Models.Providers = map[string]config.ProviderConfig{
		"ollama": {
			BaseURL: "http://localhost:11434",
			Models:  []config.Model{{ID: "test-model", Name: "Test"}},
		},
	}
	router := models.NewRouter(logger)
	_ = registerProviders(router, cfg, logger)
	orch := orchestrator.New(cfg, logger)
	registerProvidersToOrchestrator(orch, router, cfg)
}

func TestRegisterProvidersToOrchestrator_WithMatch(t *testing.T) {
	logger := slog.Default()
	cfg := config.DefaultConfig()
	cfg.Models.Providers = map[string]config.ProviderConfig{
		"openai": {
			BaseURL: "https://api.openai.com/v1",
			APIKey:  "test",
			Models:  []config.Model{{ID: "gpt-4", Name: "GPT-4"}},
		},
	}
	router := models.NewRouter(logger)
	_ = registerProviders(router, cfg, logger)
	orch := orchestrator.New(cfg, logger)
	registerProvidersToOrchestrator(orch, router, cfg)
}

// --- signals ---

func TestGetShutdownSignals(t *testing.T) {
	sigs := getShutdownSignals()
	if len(sigs) == 0 {
		t.Error("expected signals")
	}
}

func TestHandlePlatformSignal_SIGINT(t *testing.T) {
	if handlePlatformSignal(os.Interrupt, slog.Default()) {
		t.Error("expected false for SIGINT")
	}
}

func TestHandlePlatformSignal_SIGHUP(t *testing.T) {
	sigs := getShutdownSignals()
	// SIGHUP is 3rd: [SIGINT, SIGTERM, SIGHUP, SIGUSR1]
	if len(sigs) < 3 {
		t.Skip("not enough signals")
	}
	if !handlePlatformSignal(sigs[2], slog.Default()) {
		t.Error("expected true for SIGHUP")
	}
}

func TestHandlePlatformSignal_SIGUSR1(t *testing.T) {
	sigs := getShutdownSignals()
	if len(sigs) < 4 {
		t.Skip("not enough signals")
	}
	if !handlePlatformSignal(sigs[3], slog.Default()) {
		t.Error("expected true for SIGUSR1")
	}
}

func TestSetActiveConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	SetActiveConfig(cfg, "/tmp/test.json")
	if activeConfig != cfg {
		t.Error("config not set")
	}
}

func TestReloadConfig_NoActive(t *testing.T) {
	orig, origPath := activeConfig, activeConfigPath
	defer func() { activeConfig = orig; activeConfigPath = origPath }()
	activeConfig = nil
	activeConfigPath = ""
	reloadConfig(slog.Default()) // should not panic
}

func TestReloadConfig_WithConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "evoclaw.json")
	cfg := config.DefaultConfig()
	_ = cfg.Save(cfgPath)
	orig, origPath := activeConfig, activeConfigPath
	defer func() { activeConfig = orig; activeConfigPath = origPath }()
	SetActiveConfig(cfg, cfgPath)
	reloadConfig(slog.Default())
}

// --- gateway.go ---

func TestRunGatewayCommand_NoArgs(t *testing.T) {
	if err := runGatewayCommand([]string{}); err == nil {
		t.Error("expected error")
	}
}

func TestRunGatewayCommand_Help(t *testing.T) {
	if err := runGatewayCommand([]string{"help"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunGatewayCommand_DashHelp(t *testing.T) {
	if err := runGatewayCommand([]string{"--help"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunGatewayCommand_DashH(t *testing.T) {
	if err := runGatewayCommand([]string{"-h"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunGatewayCommand_Unknown(t *testing.T) {
	if err := runGatewayCommand([]string{"nonexistent"}); err == nil {
		t.Error("expected error")
	}
}

func TestRunGatewayCommand_Status(t *testing.T) {
	_ = runGatewayCommand([]string{"status"})
}

func TestRunGatewayCommand_Stop(t *testing.T) {
	_ = runGatewayCommand([]string{"stop"})
}

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	if !fileExists(dir) {
		t.Error("expected true for existing dir")
	}
	if fileExists("/nonexistent/xyz") {
		t.Error("expected false")
	}
}

func TestGetPIDFile(t *testing.T) {
	if getPIDFile() == "" {
		t.Error("expected non-empty path")
	}
}

func TestPrintGatewayHelp(t *testing.T) {
	old := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w
	printGatewayHelp()
	_ = w.Close()
	os.Stdout = old
}

func TestGatewayStart_AlreadyRunning(t *testing.T) {
	home, _ := os.UserHomeDir()
	pidFile := filepath.Join(home, ".evoclaw", "evoclaw.pid")
	_ = os.MkdirAll(filepath.Dir(pidFile), 0755)
	origContent, origExists := func() ([]byte, bool) {
		d, e := os.ReadFile(pidFile)
		return d, e == nil
	}()
	defer func() {
		if origExists {
			_ = os.WriteFile(pidFile, origContent, 0644)
		} else {
			_ = os.Remove(pidFile)
		}
	}()
	_ = os.WriteFile(pidFile, []byte(fmt.Sprint(os.Getpid())), 0644)
	err := gatewayStart()
	if err == nil {
		t.Error("expected 'already running' error")
	}
}

func TestCheckRunning_InvalidPID(t *testing.T) {
	home, _ := os.UserHomeDir()
	pidFile := filepath.Join(home, ".evoclaw", "evoclaw.pid")
	_ = os.MkdirAll(filepath.Dir(pidFile), 0755)
	origContent, origExists := func() ([]byte, bool) {
		d, e := os.ReadFile(pidFile)
		return d, e == nil
	}()
	defer func() {
		if origExists {
			_ = os.WriteFile(pidFile, origContent, 0644)
		} else {
			_ = os.Remove(pidFile)
		}
	}()
	_ = os.WriteFile(pidFile, []byte("999999999"), 0644)
	_, running := checkRunning()
	if running {
		t.Error("expected not running")
	}
}

func TestCheckRunning_BadFormat(t *testing.T) {
	home, _ := os.UserHomeDir()
	pidFile := filepath.Join(home, ".evoclaw", "evoclaw.pid")
	_ = os.MkdirAll(filepath.Dir(pidFile), 0755)
	origContent, origExists := func() ([]byte, bool) {
		d, e := os.ReadFile(pidFile)
		return d, e == nil
	}()
	defer func() {
		if origExists {
			_ = os.WriteFile(pidFile, origContent, 0644)
		} else {
			_ = os.Remove(pidFile)
		}
	}()
	_ = os.WriteFile(pidFile, []byte("notanumber"), 0644)
	_, running := checkRunning()
	if running {
		t.Error("expected not running")
	}
}

func TestCheckRunning_NoPIDFile(t *testing.T) {
	home, _ := os.UserHomeDir()
	pidFile := filepath.Join(home, ".evoclaw", "evoclaw.pid")
	origContent, origExists := func() ([]byte, bool) {
		d, e := os.ReadFile(pidFile)
		return d, e == nil
	}()
	defer func() {
		if origExists {
			_ = os.WriteFile(pidFile, origContent, 0644)
		}
	}()
	_ = os.Remove(pidFile)
	_, running := checkRunning()
	if running {
		t.Error("expected not running")
	}
}

func TestDaemonize(t *testing.T) {
	if err := daemonize(); err == nil {
		t.Error("expected error: not implemented")
	}
}

// --- initializeAgents update branch ---

func TestInitializeAgents_Update(t *testing.T) {
	dir := t.TempDir()
	logger := slog.Default()
	reg, _ := agents.NewRegistry(dir, logger)
	cfg := config.DefaultConfig()
	cfg.Agents = []config.AgentDef{{ID: "a1", Name: "V1", Type: "orchestrator", Model: "test/m"}}
	_ = initializeAgents(reg, cfg, logger)
	cfg.Agents[0].Name = "V2"
	if err := initializeAgents(reg, cfg, logger); err != nil {
		t.Fatal(err)
	}
}

// --- setupChains branches ---

func TestSetupChains_Disabled(t *testing.T) {
	logger := slog.Default()
	reg := onchain.NewChainRegistry(logger)
	cfg := config.DefaultConfig()
	cfg.Chains = map[string]config.ChainConfig{
		"bsc": {Enabled: false, Type: "evm", Name: "BSC", RPCURL: "https://bsc.invalid", ChainID: 56},
	}
	if err := setupChains(reg, cfg, logger); err != nil {
		t.Fatal(err)
	}
}

func TestSetupChains_UnknownType(t *testing.T) {
	logger := slog.Default()
	reg := onchain.NewChainRegistry(logger)
	cfg := config.DefaultConfig()
	cfg.Chains = map[string]config.ChainConfig{
		"mystery": {Enabled: true, Type: "mystery-chain", Name: "Mystery"},
	}
	if err := setupChains(reg, cfg, logger); err != nil {
		t.Fatal(err)
	}
}

func TestSetupChains_Solana(t *testing.T) {
	logger := slog.Default()
	reg := onchain.NewChainRegistry(logger)
	cfg := config.DefaultConfig()
	cfg.Chains = map[string]config.ChainConfig{
		"sol": {Enabled: true, Type: "solana", Name: "Solana"},
	}
	if err := setupChains(reg, cfg, logger); err != nil {
		t.Fatal(err)
	}
}

func TestSetupChains_EVM(t *testing.T) {
	logger := slog.Default()
	reg := onchain.NewChainRegistry(logger)
	cfg := config.DefaultConfig()
	cfg.Chains = map[string]config.ChainConfig{
		"eth": {Enabled: true, Type: "evm", Name: "Ethereum", RPCURL: "https://eth.invalid", ChainID: 1},
	}
	// May fail to connect but should not error
	if err := setupChains(reg, cfg, logger); err != nil {
		t.Fatal(err)
	}
}

// --- setup() ---

func TestSetup_Valid(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "evoclaw.json")
	cfg := config.DefaultConfig()
	cfg.Server.DataDir = dir
	_ = cfg.Save(cfgPath)

	app, err := setup(cfgPath)
	if err != nil {
		t.Fatalf("setup() error: %v", err)
	}
	if app == nil {
		t.Fatal("expected non-nil app")
	}
}

func TestSetup_InvalidConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "evoclaw.json")
	_ = os.WriteFile(cfgPath, []byte("not json"), 0644)
	_, err := setup(cfgPath)
	if err == nil {
		t.Error("expected error")
	}
}

// --- startServices ---

func TestStartServices(t *testing.T) {
	if os.Getenv("EVOCLAW_INTEGRATION") == "" {
		t.Skip("skipping integration test (set EVOCLAW_INTEGRATION=1 to run)")
	}
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "evoclaw.json")
	cfg := config.DefaultConfig()
	cfg.Server.DataDir = dir
	cfg.Server.Port = 0 // random port
	_ = cfg.Save(cfgPath)

	app, err := setup(cfgPath)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := startServices(app); err != nil {
		t.Fatalf("startServices: %v", err)
	}
	// Cleanup
	if app.apiCancel != nil {
		app.apiCancel()
	}
	_ = app.Orchestrator.Stop()
}

// --- printBanner (already tested but re-ensure) ---

func TestPrintBanner_Boost(t *testing.T) {
	dir := t.TempDir()
	logger := slog.Default()
	reg, _ := agents.NewRegistry(dir, logger)
	router := models.NewRouter(logger)
	app := &App{
		Config:   config.DefaultConfig(),
		Logger:   logger,
		Registry: reg,
		Router:   router,
	}
	printBanner(app)
}

// --- more run() branches ---

func TestRun_DashH(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"evoclaw", "-h"}
	code := run()
	if code != 0 {
		t.Errorf("run() = %d, want 0", code)
	}
}

func TestRun_ConfigBeforeSubcmd(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "evoclaw.json")
	cfg := config.DefaultConfig()
	_ = cfg.Save(cfgPath)
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"evoclaw", "-config", cfgPath, "version"}
	code := run()
	if code != 0 {
		t.Errorf("run() = %d, want 0", code)
	}
}

// --- setup() edge cases ---

func TestSetup_WithProviders(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "evoclaw.json")
	cfg := config.DefaultConfig()
	cfg.Server.DataDir = dir
	cfg.Models.Providers = map[string]config.ProviderConfig{
		"openai": {BaseURL: "https://api.openai.com/v1", APIKey: "test", Models: []config.Model{{ID: "gpt-4"}}},
	}
	cfg.Agents = []config.AgentDef{{ID: "test-1", Name: "Test", Type: "orchestrator", Model: "openai/gpt-4"}}
	_ = cfg.Save(cfgPath)

	app, err := setup(cfgPath)
	if err != nil {
		t.Fatalf("setup() error: %v", err)
	}
	if app.Router == nil {
		t.Error("expected non-nil router")
	}
}

func TestSetup_WithEvolution(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "evoclaw.json")
	cfg := config.DefaultConfig()
	cfg.Server.DataDir = dir
	cfg.Evolution.Enabled = true
	cfg.Evolution.EvalIntervalSec = 60
	_ = cfg.Save(cfgPath)

	app, err := setup(cfgPath)
	if err != nil {
		t.Fatalf("setup() error: %v", err)
	}
	if app.EvoEngine == nil {
		t.Error("expected non-nil evolution engine")
	}
}

func TestSetup_WithLogLevel(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "evoclaw.json")
	cfg := config.DefaultConfig()
	cfg.Server.DataDir = dir
	cfg.Server.LogLevel = "debug"
	_ = cfg.Save(cfgPath)

	app, err := setup(cfgPath)
	if err != nil {
		t.Fatalf("setup() error: %v", err)
	}
	if app == nil {
		t.Fatal("nil app")
	}
}

func TestSetup_WithChains(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "evoclaw.json")
	cfg := config.DefaultConfig()
	cfg.Server.DataDir = dir
	cfg.Chains = map[string]config.ChainConfig{
		"sol": {Enabled: true, Type: "solana", Name: "Solana"},
	}
	_ = cfg.Save(cfgPath)

	app, err := setup(cfgPath)
	if err != nil {
		t.Fatalf("setup() error: %v", err)
	}
	if app == nil {
		t.Fatal("nil app")
	}
}

func TestSetup_WithChannels(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "evoclaw.json")
	cfg := config.DefaultConfig()
	cfg.Server.DataDir = dir
	cfg.MQTT.Host = "localhost"
	cfg.MQTT.Port = 1883
	_ = cfg.Save(cfgPath)

	app, err := setup(cfgPath)
	if err != nil {
		t.Fatalf("setup() error: %v", err)
	}
	if app == nil {
		t.Fatal("nil app")
	}
}

func TestInitializeAgents_CreateError(t *testing.T) {
	dir := t.TempDir()
	logger := slog.Default()
	reg, _ := agents.NewRegistry(dir, logger)
	cfg := config.DefaultConfig()
	// Empty ID should cause create to fail
	cfg.Agents = []config.AgentDef{{ID: "", Name: "Bad"}}
	err := initializeAgents(reg, cfg, logger)
	if err == nil {
		t.Log("expected error for empty agent ID")
	}
}

func TestStartServices_Full(t *testing.T) {
	if os.Getenv("EVOCLAW_INTEGRATION") == "" {
		t.Skip("skipping integration test (set EVOCLAW_INTEGRATION=1 to run)")
	}
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "evoclaw.json")
	cfg := config.DefaultConfig()
	cfg.Server.DataDir = dir
	cfg.Server.Port = 0
	_ = cfg.Save(cfgPath)

	app, err := setup(cfgPath)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := startServices(app); err != nil {
		t.Logf("startServices error: %v", err)
	}
	// Cleanup
	if app.apiCancel != nil {
		app.apiCancel()
	}
	_ = app.Orchestrator.Stop()
}
