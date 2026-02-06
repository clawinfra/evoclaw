package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds all EvoClaw configuration
type Config struct {
	// Server settings
	Server ServerConfig `json:"server"`

	// MQTT broker for agent mesh
	MQTT MQTTConfig `json:"mqtt"`

	// Channel configurations
	Channels ChannelConfig `json:"channels"`

	// LLM provider settings
	Models ModelsConfig `json:"models"`

	// Evolution engine settings
	Evolution EvolutionConfig `json:"evolution"`

	// Agent definitions
	Agents []AgentDef `json:"agents"`

	// E2B cloud sandbox settings
	Cloud CloudConfig `json:"cloud,omitempty"`
}

type CloudConfig struct {
	Enabled                bool    `json:"enabled"`
	E2BAPIKey              string  `json:"e2bApiKey,omitempty"`
	DefaultTemplate        string  `json:"defaultTemplate"`
	DefaultTimeoutSec      int     `json:"defaultTimeoutSec"`
	MaxAgents              int     `json:"maxAgents"`
	HealthCheckIntervalSec int     `json:"healthCheckIntervalSec"`
	KeepAliveIntervalSec   int     `json:"keepAliveIntervalSec"`
	CreditBudgetUSD        float64 `json:"creditBudgetUsd"`
}

type ServerConfig struct {
	Port     int    `json:"port"`
	DataDir  string `json:"dataDir"`
	LogLevel string `json:"logLevel"`
}

type MQTTConfig struct {
	Port     int    `json:"port"`
	Host     string `json:"host"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type ChannelConfig struct {
	WhatsApp *WhatsAppConfig `json:"whatsapp,omitempty"`
	Telegram *TelegramConfig `json:"telegram,omitempty"`
}

type WhatsAppConfig struct {
	Enabled   bool     `json:"enabled"`
	AllowFrom []string `json:"allowFrom"`
}

type TelegramConfig struct {
	Enabled  bool   `json:"enabled"`
	BotToken string `json:"botToken"`
}

type ModelsConfig struct {
	Providers map[string]ProviderConfig `json:"providers"`
	// Routing rules: task complexity → model selection
	Routing ModelRouting `json:"routing"`
}

type ProviderConfig struct {
	BaseURL string  `json:"baseUrl"`
	APIKey  string  `json:"apiKey"`
	Models  []Model `json:"models"`
}

type Model struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	ContextWindow int      `json:"contextWindow"`
	CostInput     float64  `json:"costInput"`    // per million tokens
	CostOutput    float64  `json:"costOutput"`   // per million tokens
	Capabilities  []string `json:"capabilities"` // "reasoning", "code", "vision"
}

type ModelRouting struct {
	// Simple tasks use cheap models
	Simple string `json:"simple"`
	// Complex tasks use expensive models
	Complex string `json:"complex"`
	// Critical tasks (trading, money) use best available
	Critical string `json:"critical"`
}

type EvolutionConfig struct {
	Enabled bool `json:"enabled"`
	// How often to evaluate agent performance (seconds)
	EvalIntervalSec int `json:"evalIntervalSec"`
	// Minimum trades/actions before evaluation
	MinSamplesForEval int `json:"minSamplesForEval"`
	// Maximum strategy mutation rate (0.0 - 1.0)
	MaxMutationRate float64 `json:"maxMutationRate"`
}

type AgentDef struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Type         string            `json:"type"` // "orchestrator", "trader", "monitor", "governance"
	Model        string            `json:"model"`
	SystemPrompt string            `json:"systemPrompt,omitempty"`
	Skills       []string          `json:"skills"`
	Config       map[string]string `json:"config,omitempty"`
	// Container isolation settings
	Container ContainerConfig `json:"container"`
}

type ContainerConfig struct {
	Enabled    bool     `json:"enabled"`
	Image      string   `json:"image,omitempty"`
	MemoryMB   int      `json:"memoryMb"`
	CPUShares  int      `json:"cpuShares"`
	Mounts     []Mount  `json:"mounts,omitempty"`
	AllowNet   bool     `json:"allowNet"`
	AllowTools []string `json:"allowTools,omitempty"`
}

type Mount struct {
	HostPath      string `json:"hostPath"`
	ContainerPath string `json:"containerPath"`
	ReadOnly      bool   `json:"readOnly"`
}

// DefaultConfig returns a sensible default configuration
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:     8420,
			DataDir:  "./data",
			LogLevel: "info",
		},
		MQTT: MQTTConfig{
			Port: 1883,
			Host: "0.0.0.0",
		},
		Evolution: EvolutionConfig{
			Enabled:           true,
			EvalIntervalSec:   3600, // every hour
			MinSamplesForEval: 10,
			MaxMutationRate:   0.2,
		},
		Models: ModelsConfig{
			Routing: ModelRouting{
				Simple:   "local/small",
				Complex:  "anthropic/claude-sonnet",
				Critical: "anthropic/claude-opus",
			},
		},
		Cloud: CloudConfig{
			Enabled:                false,
			DefaultTemplate:        "evoclaw-agent",
			DefaultTimeoutSec:      300,
			MaxAgents:              10,
			HealthCheckIntervalSec: 60,
			KeepAliveIntervalSec:   120,
			CreditBudgetUSD:        50.0,
		},
	}
}

// Load reads config from a JSON file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Ensure data directory exists
	if err := os.MkdirAll(cfg.Server.DataDir, 0750); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	return cfg, nil
}

// Save writes config to a JSON file
func (c *Config) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	return os.WriteFile(path, data, 0640)
}
