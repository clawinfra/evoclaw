package cli

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/clawinfra/evoclaw/internal/config"
)

// InitCommand handles the 'evoclaw init' subcommand
func InitCommand(args []string) int {
	fs := flag.NewFlagSet("evoclaw init", flag.ExitOnError)
	nonInteractive := fs.Bool("non-interactive", false, "Run without prompts (requires --provider, --key, --name)")
	provider := fs.String("provider", "", "Model provider: anthropic, ollama, openai, openrouter")
	apiKey := fs.String("key", "", "API key for the model provider")
	agentName := fs.String("name", "", "Agent name")
	skipChain := fs.Bool("skip-chain", false, "Skip ClawChain registration")
	outputPath := fs.String("output", "evoclaw.json", "Output config file path")

	fs.Usage = func() {
		fmt.Println(`Usage: evoclaw init [options]

Initialize a new EvoClaw agent configuration.

Options:`)
		fs.PrintDefaults()
		fmt.Println(`
Examples:
  # Interactive setup
  evoclaw init

  # Scripted setup
  evoclaw init --non-interactive --provider anthropic --key sk-ant-... --name my-agent

  # Skip chain registration
  evoclaw init --skip-chain`)
	}

	if err := fs.Parse(args); err != nil {
		return 1
	}

	// Check if config already exists
	if _, err := os.Stat(*outputPath); err == nil {
		fmt.Printf("⚠️  Config file %s already exists. Overwrite? [y/N]: ", *outputPath)
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Aborted.")
			return 0
		}
	}

	var cfg *config.Config

	if *nonInteractive {
		if *provider == "" || *agentName == "" {
			fmt.Fprintln(os.Stderr, "Error: --provider and --name are required in non-interactive mode")
			return 1
		}
		if *provider != "ollama" && *apiKey == "" {
			fmt.Fprintln(os.Stderr, "Error: --key is required for non-ollama providers")
			return 1
		}
		cfg = buildConfig(*provider, *apiKey, *agentName, false, false, *skipChain)
	} else {
		var err error
		cfg, err = interactiveInit(*skipChain)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return 1
		}
	}

	if err := cfg.Save(*outputPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		return 1
	}

	fmt.Println()
	fmt.Printf("✅ Config written to %s\n", *outputPath)

	// Install core skills
	agentRole := "autonomous agent"
	if len(cfg.Agents) > 0 && cfg.Agents[0].Type != "" {
		agentRole = cfg.Agents[0].Type
	}

	if err := SetupCoreSkills(*agentName, agentRole); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Warning: Core skills installation failed: %v\n", err)
		fmt.Fprintln(os.Stderr, "   Agent will work but skills must be installed manually")
	}

	if err := GenerateAgentFiles(*agentName, agentRole); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Warning: Agent file generation failed: %v\n", err)
		fmt.Fprintln(os.Stderr, "   SOUL.md and AGENTS.md must be created manually")
	}

	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  evoclaw start          # Start the agent")
	fmt.Println("  evoclaw start --config evoclaw.json")
	fmt.Println()
	return 0
}

func interactiveInit(skipChain bool) (*config.Config, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println("  🧬 EvoClaw Init — Configure your agent")
	fmt.Println("  ═══════════════════════════════════════")
	fmt.Println()

	// 1. Agent name
	agentName := prompt(reader, "Agent name", "my-agent")

	// 2. Model provider
	fmt.Println()
	fmt.Println("Model providers:")
	fmt.Println("  1) anthropic  — Claude (recommended)")
	fmt.Println("  2) openai     — GPT-4o")
	fmt.Println("  3) openrouter — Multi-provider gateway")
	fmt.Println("  4) ollama     — Local models (free, no API key)")
	fmt.Println()
	providerChoice := prompt(reader, "Choose provider [1-4 or name]", "1")

	provider := normalizeProvider(providerChoice)
	if provider == "" {
		return nil, fmt.Errorf("unknown provider: %s", providerChoice)
	}

	// 3. API key
	var apiKey string
	if provider != "ollama" {
		apiKey = prompt(reader, fmt.Sprintf("API key for %s", provider), "")
		if apiKey == "" {
			return nil, fmt.Errorf("API key is required for %s", provider)
		}
	}

	// 4. Channels
	fmt.Println()
	fmt.Println("Channels (press Enter to skip):")
	enableTelegram := false
	telegramToken := prompt(reader, "Telegram bot token (from @BotFather)", "")
	if telegramToken != "" {
		enableTelegram = true
	}

	enableMQTT := false
	mqttAnswer := prompt(reader, "Enable MQTT channel? [y/N]", "n")
	if strings.ToLower(mqttAnswer) == "y" || strings.ToLower(mqttAnswer) == "yes" {
		enableMQTT = true
	}

	// 5. ClawChain
	if !skipChain {
		fmt.Println()
		chainAnswer := prompt(reader, "Register on ClawChain? [y/N]", "n")
		if strings.ToLower(chainAnswer) == "y" || strings.ToLower(chainAnswer) == "yes" {
			fmt.Println("  ℹ️  ClawChain registration will happen on first start.")
		}
	}

	cfg := buildConfig(provider, apiKey, agentName, enableTelegram, enableMQTT, skipChain)

	if enableTelegram {
		cfg.Channels.Telegram = &config.TelegramConfig{
			Enabled:  true,
			BotToken: telegramToken,
		}
	}

	return cfg, nil
}

func prompt(reader *bufio.Reader, label, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("  %s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("  %s: ", label)
	}
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultVal
	}
	return input
}

func normalizeProvider(input string) string {
	input = strings.TrimSpace(strings.ToLower(input))
	switch input {
	case "1", "anthropic":
		return "anthropic"
	case "2", "openai":
		return "openai"
	case "3", "openrouter":
		return "openrouter"
	case "4", "ollama":
		return "ollama"
	default:
		return ""
	}
}

func buildConfig(provider, apiKey, agentName string, enableTelegram, enableMQTT bool, skipChain bool) *config.Config {
	cfg := config.DefaultConfig()

	// Setup provider
	cfg.Models.Providers = map[string]config.ProviderConfig{}

	switch provider {
	case "anthropic":
		cfg.Models.Providers["anthropic"] = config.ProviderConfig{
			BaseURL: "https://api.anthropic.com",
			APIKey:  apiKey,
			Models: []config.Model{
				{
					ID:            "claude-sonnet-4-20250514",
					Name:          "Claude Sonnet 4",
					ContextWindow: 200000,
					CostInput:     3.0,
					CostOutput:    15.0,
					Capabilities:  []string{"reasoning", "code", "vision"},
				},
			},
		}
		cfg.Models.Routing = config.ModelRouting{
			Simple:   "anthropic/claude-sonnet-4-20250514",
			Complex:  "anthropic/claude-sonnet-4-20250514",
			Critical: "anthropic/claude-sonnet-4-20250514",
		}
	case "openai":
		cfg.Models.Providers["openai"] = config.ProviderConfig{
			BaseURL: "https://api.openai.com/v1",
			APIKey:  apiKey,
			Models: []config.Model{
				{
					ID:            "gpt-4o",
					Name:          "GPT-4o",
					ContextWindow: 128000,
					CostInput:     2.5,
					CostOutput:    10.0,
					Capabilities:  []string{"reasoning", "code", "vision"},
				},
			},
		}
		cfg.Models.Routing = config.ModelRouting{
			Simple:   "openai/gpt-4o",
			Complex:  "openai/gpt-4o",
			Critical: "openai/gpt-4o",
		}
	case "openrouter":
		cfg.Models.Providers["openrouter"] = config.ProviderConfig{
			BaseURL: "https://openrouter.ai/api/v1",
			APIKey:  apiKey,
			Models: []config.Model{
				{
					ID:            "anthropic/claude-sonnet-4-20250514",
					Name:          "Claude Sonnet 4 (via OpenRouter)",
					ContextWindow: 200000,
					CostInput:     3.0,
					CostOutput:    15.0,
					Capabilities:  []string{"reasoning", "code", "vision"},
				},
			},
		}
		cfg.Models.Routing = config.ModelRouting{
			Simple:   "openrouter/anthropic/claude-sonnet-4-20250514",
			Complex:  "openrouter/anthropic/claude-sonnet-4-20250514",
			Critical: "openrouter/anthropic/claude-sonnet-4-20250514",
		}
	case "ollama":
		cfg.Models.Providers["ollama"] = config.ProviderConfig{
			BaseURL: "http://localhost:11434",
			Models: []config.Model{
				{
					ID:            "llama3.2:3b",
					Name:          "Llama 3.2 3B",
					ContextWindow: 8192,
					CostInput:     0.0,
					CostOutput:    0.0,
					Capabilities:  []string{"reasoning"},
				},
			},
		}
		cfg.Models.Routing = config.ModelRouting{
			Simple:   "ollama/llama3.2:3b",
			Complex:  "ollama/llama3.2:3b",
			Critical: "ollama/llama3.2:3b",
		}
	}

	// Setup agent
	modelRef := cfg.Models.Routing.Complex
	cfg.Agents = []config.AgentDef{
		{
			ID:           "agent-1",
			Name:         agentName,
			Type:         "orchestrator",
			Model:        modelRef,
			SystemPrompt: "You are a helpful AI assistant. Be concise, accurate, and friendly.",
			Skills:       []string{"chat", "search", "analysis"},
		},
	}

	// Channels
	if !enableTelegram {
		cfg.Channels.Telegram = nil
	}
	if !enableMQTT {
		cfg.MQTT = config.MQTTConfig{}
	}

	return cfg
}
