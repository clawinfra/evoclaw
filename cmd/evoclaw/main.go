package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"strings"

	"github.com/clawinfra/evoclaw/internal/agents"
	"github.com/clawinfra/evoclaw/internal/api"
	"github.com/clawinfra/evoclaw/internal/channels"
	"github.com/clawinfra/evoclaw/internal/cli"
	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/evolution"
	"github.com/clawinfra/evoclaw/internal/models"
	"github.com/clawinfra/evoclaw/internal/onchain"
	"github.com/clawinfra/evoclaw/internal/orchestrator"
	"github.com/clawinfra/evoclaw/internal/skills"
)

//go:embed web
var webContent embed.FS

var (
	version   = "0.1.0"
	buildTime = "dev"
)

// App holds all the runtime components
type App struct {
	Config        *config.Config
	Logger        *slog.Logger
	Registry      *agents.Registry
	MemoryStore   *agents.MemoryStore
	Router        *models.Router
	EvoEngine     *evolution.Engine
	ChainRegistry *onchain.ChainRegistry
	Orchestrator  *orchestrator.Orchestrator
	SkillRegistry *skills.Registry
	APIServer     *api.Server
	apiContext    context.Context
	apiCancel     context.CancelFunc
}

func main() {
	os.Exit(run())
}

func run() int {
	// Check for subcommands (look through all args, not just first)
	configPath := "evoclaw.json"
	var subCmd string
	var subCmdIdx int
	
	// First pass: find config flag
	skipNext := false
	for i := 1; i < len(os.Args); i++ {
		if skipNext {
			skipNext = false
			continue
		}
		arg := os.Args[i]
		if arg == "--config" || arg == "-config" {
			if i+1 < len(os.Args) {
				configPath = os.Args[i+1]
				skipNext = true
			}
		}
	}
	
	// Second pass: find subcommand (first non-flag, non-flag-value arg)
	skipNext = false
	for i := 1; i < len(os.Args); i++ {
		if skipNext {
			skipNext = false
			continue
		}
		arg := os.Args[i]
		
		// Skip known flag patterns
		if arg == "--config" || arg == "-config" || arg == "--version" || arg == "-version" {
			if arg == "--config" || arg == "-config" {
				skipNext = true
			}
			continue
		}
		
		// This must be a subcommand or positional arg
		if len(arg) > 0 && arg[0] != '-' {
			subCmd = arg
			subCmdIdx = i
			break
		}
	}
	
	// Handle subcommands
	if subCmd != "" {
		switch subCmd {
		case "chain":
		case "memory":
			// Memory system operations
			return cli.MemoryCommand(os.Args[subCmdIdx+1:], configPath)
			// Pass args after the subcommand
			return cli.ChainCommand(os.Args[subCmdIdx+1:], configPath)
		case "init":
			return cli.InitCommand(os.Args[subCmdIdx+1:])
		case "gateway":
			// Gateway daemon management
			if err := runGatewayCommand(os.Args[subCmdIdx+1:]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return 1
			}
			return 0
		case "start":
			// Explicit start subcommand — falls through to normal server start below
		default:
			fmt.Fprintf(os.Stderr, "Unknown command: %s\n", subCmd)
			fmt.Fprintln(os.Stderr, "Available commands: init, start, chain")
			return 1
		}
	}

	// No subcommand - parse as normal server start
	fs := flag.NewFlagSet("evoclaw", flag.ExitOnError)
	configPathFlag := fs.String("config", "evoclaw.json", "Path to config file")
	showVersion := fs.Bool("version", false, "Show version")
	if err := fs.Parse(os.Args[1:]); err != nil {
		fmt.Printf("Error parsing arguments: %v\n", err)
		os.Exit(1)
	}

	if *showVersion {
		fmt.Printf("EvoClaw v%s (built %s)\n", version, buildTime)
		fmt.Println("Self-evolving agent framework for edge devices")
		fmt.Println("https://github.com/clawinfra/evoclaw")
		return 0
	}
	
	// Use the config path from flag if provided
	if *configPathFlag != "evoclaw.json" {
		configPath = *configPathFlag
	}

	// Setup application
	app, err := setup(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Setup failed: %v\n", err)
		return 1
	}

	// Start services
	if err := startServices(app); err != nil {
		app.Logger.Error("failed to start services", "error", err)
		return 1
	}

	// Print banner
	printBanner(app)

	// Wait for shutdown
	if err := waitForShutdown(app); err != nil {
		app.Logger.Error("shutdown error", "error", err)
		return 1
	}

	return 0
}

// setup initializes all application components
func setup(configPath string) (*App, error) {
	app := &App{}

	// Setup logger (initially at Info level)
	app.Logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	app.Logger.Info("starting EvoClaw",
		"version", version,
		"config", configPath,
	)

	// Load config
	cfg, err := loadConfig(configPath, app.Logger)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	app.Config = cfg

	// Recreate logger with config's log level
	logLevel := parseLogLevel(cfg.Server.LogLevel)
	app.Logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))

	// Create agent registry
	registry, err := agents.NewRegistry(cfg.Server.DataDir, app.Logger)
	if err != nil {
		return nil, fmt.Errorf("create registry: %w", err)
	}
	app.Registry = registry

	// Load existing agents
	if err := registry.Load(); err != nil {
		return nil, fmt.Errorf("load agents: %w", err)
	}

	// Initialize agents from config
	if err := initializeAgents(registry, cfg, app.Logger); err != nil {
		return nil, fmt.Errorf("initialize agents: %w", err)
	}

	// Create memory store
	memoryStore, err := agents.NewMemoryStore(cfg.Server.DataDir, app.Logger)
	if err != nil {
		return nil, fmt.Errorf("create memory store: %w", err)
	}
	app.MemoryStore = memoryStore

	// Create model router
	app.Router = models.NewRouter(app.Logger)

	// Register model providers
	if err := registerProviders(app.Router, cfg, app.Logger); err != nil {
		return nil, fmt.Errorf("register providers: %w", err)
	}

	// Create evolution engine if enabled
	if cfg.Evolution.Enabled {
		app.EvoEngine = evolution.NewEngine(cfg.Server.DataDir, app.Logger)
		app.Logger.Info("evolution engine enabled",
			"evalInterval", cfg.Evolution.EvalIntervalSec,
			"minSamples", cfg.Evolution.MinSamplesForEval,
		)
	}

	// Create chain registry and setup chains
	app.ChainRegistry = onchain.NewChainRegistry(app.Logger)
	if err := setupChains(app.ChainRegistry, cfg, app.Logger); err != nil {
		return nil, fmt.Errorf("setup chains: %w", err)
	}

	// Create orchestrator
	app.Orchestrator = orchestrator.New(cfg, app.Logger)

	// Wire evolution engine
	if app.EvoEngine != nil {
		app.Orchestrator.SetEvolutionEngine(app.EvoEngine)
	}

	// Load skills
	skillsDir := skills.DefaultSkillsDir()
	skillLoader := skills.NewLoader(skillsDir, app.Logger)
	app.SkillRegistry = skills.NewRegistry(app.Logger)
	loadedSkills, err := skillLoader.LoadAll()
	if err != nil {
		app.Logger.Warn("failed to load skills", "error", err)
	} else {
		for _, s := range loadedSkills {
			if regErr := app.SkillRegistry.Register(s); regErr != nil {
				app.Logger.Warn("failed to register skill", "name", s.Manifest.Name, "error", regErr)
			}
		}
		if count := app.SkillRegistry.SkillCount(); count > 0 {
			app.Logger.Info("skills loaded", "count", count)
		}
	}

	// Register channels
	if err := registerChannels(app.Orchestrator, cfg, app.Logger); err != nil {
		return nil, fmt.Errorf("register channels: %w", err)
	}

	// Register providers to orchestrator
	registerProvidersToOrchestrator(app.Orchestrator, app.Router, cfg)

	// Create API server
	app.APIServer = api.NewServer(
		cfg.Server.Port,
		app.Orchestrator,
		app.Registry,
		app.MemoryStore,
		app.Router,
		app.Logger,
	)

	// Embed web dashboard assets
	webFS, err := fs.Sub(webContent, "web")
	if err != nil {
		app.Logger.Warn("web dashboard assets not available", "error", err)
	} else {
		app.APIServer.SetWebFS(webFS)
		app.Logger.Info("web dashboard embedded")
	}

	return app, nil
}

// loadConfig loads configuration from file or creates default
func loadConfig(path string, logger *slog.Logger) (*config.Config, error) {
	cfg, err := config.Load(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			logger.Info("no config found, creating default")
			cfg = config.DefaultConfig()
			if err := cfg.Save(path); err != nil {
				return nil, fmt.Errorf("save default config: %w", err)
			}
			logger.Info("default config created", "path", path)
			return cfg, nil
		}
		return nil, err
	}
	return cfg, nil
}

// parseLogLevel converts string log level to slog.Level
func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// initializeAgents creates agents from config if they don't exist
func initializeAgents(registry *agents.Registry, cfg *config.Config, logger *slog.Logger) error {
	for _, agentDef := range cfg.Agents {
		if _, err := registry.Get(agentDef.ID); err == nil {
			logger.Info("agent already loaded", "id", agentDef.ID)
			continue
		}

		if _, err := registry.Create(agentDef); err != nil {
			return fmt.Errorf("create agent %s: %w", agentDef.ID, err)
		}
	}
	return nil
}

// setupChains initializes blockchain adapters from config
func setupChains(registry *onchain.ChainRegistry, cfg *config.Config, logger *slog.Logger) error {
	// Migrate old OnChain config if needed
	cfg.MigrateOnChainConfig()

	// No chains configured - that's ok
	if len(cfg.Chains) == 0 {
		logger.Info("no chains configured")
		return nil
	}

	ctx := context.Background()

	// Setup each configured chain
	for chainID, chainCfg := range cfg.Chains {
		if !chainCfg.Enabled {
			logger.Info("chain disabled, skipping", "chain", chainID)
			continue
		}

		logger.Info("setting up chain",
			"chain", chainID,
			"type", chainCfg.Type,
			"name", chainCfg.Name,
		)

		switch chainCfg.Type {
		case "evm":
			// Create EVM adapter (BSCClient works for any EVM chain)
			bscCfg := onchain.Config{
				RPCURL:  chainCfg.RPCURL,
				ChainID: chainCfg.ChainID,
			}
			client, err := onchain.NewBSCClient(bscCfg, logger)
			if err != nil {
				logger.Error("failed to create EVM client",
					"chain", chainID,
					"error", err,
				)
				continue
			}

			// Connect to the chain
			if err := client.Connect(ctx); err != nil {
				logger.Warn("failed to connect to chain (will retry later)",
					"chain", chainID,
					"error", err,
				)
			}

			registry.Register(client)
			logger.Info("chain registered",
				"chain", chainID,
				"chainId", chainCfg.ChainID,
			)

		case "solana", "hyperliquid", "substrate":
			logger.Warn("chain type not yet implemented",
				"chain", chainID,
				"type", chainCfg.Type,
			)
			// TODO: Implement other chain types

		default:
			logger.Error("unknown chain type",
				"chain", chainID,
				"type", chainCfg.Type,
			)
		}
	}

	return nil
}

// registerProviders registers model providers to the router
func registerProviders(router *models.Router, cfg *config.Config, logger *slog.Logger) error {
	for providerName, provCfg := range cfg.Models.Providers {
		logger.Info("initializing provider", "name", providerName, "models", len(provCfg.Models))

		// Detect provider type from name or baseUrl
		providerType := providerName
		if strings.Contains(provCfg.BaseURL, "/anthropic") || strings.HasPrefix(providerName, "anthropic") {
			providerType = "anthropic"
		}

		switch providerType {
		case "anthropic":
			p := models.NewAnthropicProvider(provCfg)
			p.SetName(providerName)
			router.RegisterProvider(p)
		case "ollama":
			router.RegisterProvider(models.NewOllamaProvider(provCfg))
		case "openai":
			router.RegisterProvider(models.NewOpenAIProvider("openai", provCfg))
		case "openrouter":
			router.RegisterProvider(models.NewOpenAIProvider("openrouter", provCfg))
		default:
			// Assume OpenAI-compatible
			router.RegisterProvider(models.NewOpenAIProvider(providerName, provCfg))
		}
	}
	return nil
}

// registerChannels registers communication channels to orchestrator
func registerChannels(orch *orchestrator.Orchestrator, cfg *config.Config, logger *slog.Logger) error {
	// Telegram
	if cfg.Channels.Telegram != nil && cfg.Channels.Telegram.Enabled {
		logger.Info("enabling telegram channel")
		telegram := channels.NewTelegram(cfg.Channels.Telegram.BotToken, logger)
		orch.RegisterChannel(telegram)
	}

	// MQTT
	if cfg.MQTT.Port > 0 {
		logger.Info("enabling mqtt channel",
			"host", cfg.MQTT.Host,
			"port", cfg.MQTT.Port,
		)
		mqtt := channels.NewMQTT(
			cfg.MQTT.Host,
			cfg.MQTT.Port,
			cfg.MQTT.Username,
			cfg.MQTT.Password,
			logger,
		)
		orch.RegisterChannel(mqtt)
	}

	return nil
}

// registerProvidersToOrchestrator registers providers from router to orchestrator
func registerProvidersToOrchestrator(orch *orchestrator.Orchestrator, router *models.Router, cfg *config.Config) {
	for providerName := range cfg.Models.Providers {
		modelInfos := router.ListModels()
		for _, info := range modelInfos {
			if info.Provider == providerName {
				orch.RegisterProvider(info.ProviderImpl)
				break
			}
		}
	}
}

// startServices starts all services
func startServices(app *App) error {
	// Start orchestrator
	if err := app.Orchestrator.Start(); err != nil {
		return fmt.Errorf("start orchestrator: %w", err)
	}

	// Start API server in background
	app.apiContext, app.apiCancel = context.WithCancel(context.Background())
	go func() {
		if err := app.APIServer.Start(app.apiContext); err != nil {
			app.Logger.Error("API server error", "error", err)
		}
	}()

	return nil
}

// printBanner displays the startup banner
func printBanner(app *App) {
	fmt.Println()
	fmt.Println("  ╔═══════════════════════════════════════╗")
	fmt.Println("  ║        🧬 EvoClaw v" + version + "            ║")
	fmt.Println("  ║  Self-Evolving Agent Framework        ║")
	fmt.Println("  ║  Every device is an agent.            ║")
	fmt.Println("  ║  Every agent evolves.                 ║")
	fmt.Println("  ╚═══════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("  🌐 API: http://localhost:%d\n", app.Config.Server.Port)
	fmt.Printf("  📊 Dashboard: http://localhost:%d\n", app.Config.Server.Port)
	fmt.Printf("  🤖 Agents: %d loaded\n", len(app.Registry.List()))
	fmt.Printf("  🧠 Models: %d available\n", len(app.Router.ListModels()))
	fmt.Println()
}

// waitForShutdown waits for termination signal and performs graceful shutdown
func waitForShutdown(app *App) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, getShutdownSignals()...)

	for {
		sig := <-sigCh
		
		// Handle platform-specific signals (SIGHUP, SIGUSR1 on Unix)
		if handlePlatformSignal(sig, app.Logger) {
			continue
		}
		
		// SIGINT or SIGTERM - proceed to shutdown
		app.Logger.Info("shutdown signal received", "signal", sig)
		break
	}

	// Stop API server
	if app.apiCancel != nil {
		app.apiCancel()
	}

	// Graceful shutdown
	app.Logger.Info("saving state...")
	if err := app.Registry.SaveAll(); err != nil {
		app.Logger.Error("failed to save agents", "error", err)
	}
	if err := app.MemoryStore.SaveAll(); err != nil {
		app.Logger.Error("failed to save memory", "error", err)
	}

	// Stop orchestrator
	if err := app.Orchestrator.Stop(); err != nil {
		return fmt.Errorf("stop orchestrator: %w", err)
	}

	app.Logger.Info("EvoClaw stopped")
	return nil
}
