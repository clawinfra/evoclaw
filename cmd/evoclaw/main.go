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
	"syscall"

	"github.com/clawinfra/evoclaw/internal/agents"
	"github.com/clawinfra/evoclaw/internal/api"
	"github.com/clawinfra/evoclaw/internal/channels"
	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/evolution"
	"github.com/clawinfra/evoclaw/internal/models"
	"github.com/clawinfra/evoclaw/internal/orchestrator"
)

//go:embed web
var webContent embed.FS

var (
	version   = "0.1.0"
	buildTime = "dev"
)

// App holds all the runtime components
type App struct {
	Config       *config.Config
	Logger       *slog.Logger
	Registry     *agents.Registry
	MemoryStore  *agents.MemoryStore
	Router       *models.Router
	EvoEngine    *evolution.Engine
	Orchestrator *orchestrator.Orchestrator
	APIServer    *api.Server
	apiContext   context.Context
	apiCancel    context.CancelFunc
}

func main() {
	os.Exit(run())
}

func run() int {
	configPath := flag.String("config", "evoclaw.json", "Path to config file")
	showVersion := flag.Bool("version", false, "Show version")
	flag.Parse()

	if *showVersion {
		fmt.Printf("EvoClaw v%s (built %s)\n", version, buildTime)
		fmt.Println("Self-evolving agent framework for edge devices")
		fmt.Println("https://github.com/clawinfra/evoclaw")
		return 0
	}

	// Setup application
	app, err := setup(*configPath)
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

	// Create orchestrator
	app.Orchestrator = orchestrator.New(cfg, app.Logger)

	// Wire evolution engine
	if app.EvoEngine != nil {
		app.Orchestrator.SetEvolutionEngine(app.EvoEngine)
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
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigCh
	app.Logger.Info("shutdown signal received", "signal", sig)

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
