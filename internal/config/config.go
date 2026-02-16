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

	// On-chain integration (BSC/opBNB) - DEPRECATED: Use Chains instead
	OnChain OnChainConfig `json:"onchain"`

	// Multi-chain configuration (execution chains)
	Chains map[string]ChainConfig `json:"chains,omitempty"`

	// Cloud sync configuration
	CloudSync CloudSyncConfig `json:"cloudSync,omitempty"`

	// Memory system configuration
	Memory MemoryConfigSettings `json:"memory,omitempty"`

	// Scheduler configuration
	Scheduler SchedulerConfig `json:"scheduler,omitempty"`

	// Auto-update configuration
	Updates *UpdatesConfig `json:"updates,omitempty"`

	// Agent definitions
	Agents []AgentDef `json:"agents"`
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
	Telegram *TelegramConfig `json:"telegram,omitempty"`
	TUI      *TUIConfig      `json:"tui,omitempty"`
}

// OnChainConfig holds BSC/opBNB blockchain settings
// DEPRECATED: Use Chains map instead for multi-chain support
type OnChainConfig struct {
	Enabled         bool   `json:"enabled"`
	RPCURL          string `json:"rpcUrl"`
	ContractAddress string `json:"contractAddress"`
	PrivateKey      string `json:"privateKey,omitempty"` // signing key (hex, 0x-prefixed)
	ChainID         int64  `json:"chainId"`              // 56=BSC, 97=BSCTestnet, 204=opBNB
}

// ChainConfig holds configuration for a blockchain (execution chain)
type ChainConfig struct {
	Enabled  bool   `json:"enabled"`
	Type     string `json:"type"`              // "evm", "solana", "substrate", "hyperliquid"
	Name     string `json:"name"`              // Human-readable: "BNB Smart Chain Testnet"
	RPCURL   string `json:"rpcUrl"`
	ChainID  int64  `json:"chainId,omitempty"` // EVM chains only
	Wallet   string `json:"wallet,omitempty"`  // Wallet address
	Explorer string `json:"explorer,omitempty"` // Block explorer URL
}

type CloudSyncConfig struct {
	Enabled                  bool   `json:"enabled"`
	DatabaseURL              string `json:"databaseUrl"`
	AuthToken                string `json:"authToken"`
	DeviceID                 string `json:"deviceId,omitempty"`
	DeviceKey                string `json:"deviceKey,omitempty"`
	HeartbeatIntervalSeconds int    `json:"heartbeatIntervalSeconds"`
	CriticalSyncEnabled      bool   `json:"criticalSyncEnabled"`
	WarmSyncIntervalMinutes  int    `json:"warmSyncIntervalMinutes"`
	FullSyncIntervalHours    int    `json:"fullSyncIntervalHours"`
	FullSyncRequireWiFi      bool   `json:"fullSyncRequireWifi"`
	MaxOfflineQueueSize      int    `json:"maxOfflineQueueSize"`
}

// MemoryConfigSettings holds tiered memory system configuration
type MemoryConfigSettings struct {
	Enabled    bool               `json:"enabled"`
	Tree       TreeConfig         `json:"tree"`
	Hot        HotConfig          `json:"hot"`
	Warm       WarmConfig         `json:"warm"`
	Cold       ColdConfig         `json:"cold"`
	Distillation DistillationConfig `json:"distillation"`
	Scoring    ScoringConfig      `json:"scoring"`
}

type TreeConfig struct {
	MaxNodes           int `json:"maxNodes"`
	MaxDepth           int `json:"maxDepth"`
	MaxSizeBytes       int `json:"maxSizeBytes"`
	RebuildIntervalDays int `json:"rebuildIntervalDays"`
}

type HotConfig struct {
	MaxSizeBytes       int `json:"maxSizeBytes"`
	MaxLessons         int `json:"maxLessons"`
	MaxActiveProjects  int `json:"maxActiveProjects"`
}

type WarmConfig struct {
	MaxSizeKb          int     `json:"maxSizeKb"`
	RetentionDays      int     `json:"retentionDays"`
	EvictionThreshold  float64 `json:"evictionThreshold"`
	Backend            string  `json:"backend"` // "memory" or "sqlite"
}

type ColdConfig struct {
	Backend         string `json:"backend"` // "turso"
	DatabaseUrl     string `json:"databaseUrl"`
	AuthToken       string `json:"authToken"`
	RetentionYears  int    `json:"retentionYears"`
}

type DistillationConfig struct {
	Aggression        float64 `json:"aggression"` // 0-1
	Model             string  `json:"model"` // "local" or model name
	MaxDistilledBytes int     `json:"maxDistilledBytes"`
}

type ScoringConfig struct {
	HalfLifeDays        float64 `json:"halfLifeDays"`
	ReinforcementBoost  float64 `json:"reinforcementBoost"`
}

// SchedulerConfig holds scheduler configuration
type SchedulerConfig struct {
	Enabled bool                `json:"enabled"`
	Jobs    []SchedulerJobConfig `json:"jobs"`
}

// SchedulerJobConfig defines a scheduled job
type SchedulerJobConfig struct {
	ID       string                `json:"id"`
	Name     string                `json:"name"`
	Schedule ScheduleConfig        `json:"schedule"`
	Action   ActionConfig          `json:"action"`
	Enabled  bool                  `json:"enabled"`
}

// ScheduleConfig defines when a job runs
type ScheduleConfig struct {
	Kind       string `json:"kind"` // "interval", "cron", "at"
	IntervalMs int64  `json:"intervalMs,omitempty"`
	Expr       string `json:"expr,omitempty"` // cron expression
	Time       string `json:"time,omitempty"` // "HH:MM" for daily
	Timezone   string `json:"timezone,omitempty"`
}

// UpdatesConfig holds auto-update settings
type UpdatesConfig struct {
	Enabled            bool `json:"enabled"`            // Check for updates
	AutoInstall        bool `json:"autoInstall"`        // Automatically install updates
	CheckInterval      int  `json:"checkInterval"`      // Check interval in seconds (default 86400 = 24h)
	IncludePrereleases bool `json:"includePrereleases"` // Include beta/alpha releases
}

// ActionConfig defines what a job does
type ActionConfig struct {
	Kind    string            `json:"kind"` // "shell", "agent", "mqtt", "http"
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	AgentID string            `json:"agentId,omitempty"`
	Message string            `json:"message,omitempty"`
	Topic   string            `json:"topic,omitempty"`
	Payload map[string]any    `json:"payload,omitempty"`
	URL     string            `json:"url,omitempty"`
	Method  string            `json:"method,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type TUIConfig struct {
	Enabled bool `json:"enabled"`
}

type TelegramConfig struct {
	Enabled  bool   `json:"enabled"`
	BotToken string `json:"botToken"`
}

type ModelsConfig struct {
	Providers map[string]ProviderConfig `json:"providers"`
	// Routing rules: task complexity → model selection
	Routing ModelRouting `json:"routing"`
	// Health registry configuration
	Health ModelHealthConfig `json:"health"`
}

type ModelHealthConfig struct {
	PersistPath      string `json:"persistPath"`
	FailureThreshold int    `json:"failureThreshold"`
	CooldownMinutes  int    `json:"cooldownMinutes"`
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
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Type         string          `json:"type"` // "orchestrator", "trader", "monitor", "governance"
	Model        string          `json:"model"`
	SystemPrompt string          `json:"systemPrompt,omitempty"`
	Skills       []string        `json:"skills"`
	Capabilities []string        `json:"capabilities,omitempty"`
	Genome       *Genome         `json:"genome,omitempty"`
	Config       map[string]string `json:"config,omitempty"`
	// Container isolation settings
	Container ContainerConfig `json:"container"`
}

// Genome defines the complete genetic makeup of an agent
// This is re-exported from internal/genome for convenience
type Genome struct {
	Identity            GenomeIdentity              `json:"identity"`
	Skills              map[string]SkillGenome      `json:"skills"`
	Behavior            GenomeBehavior              `json:"behavior"`
	Constraints         GenomeConstraints           `json:"constraints"`
	ConstraintSignature []byte                      `json:"constraint_signature,omitempty"`
	OwnerPublicKey      []byte                      `json:"owner_public_key,omitempty"`
}

// GenomeIdentity defines the agent's identity layer
type GenomeIdentity struct {
	Name    string `json:"name"`
	Persona string `json:"persona"`
	Voice   string `json:"voice"` // concise, verbose, balanced, etc.
}

// SkillGenome defines evolvable parameters for a specific skill
type SkillGenome struct {
	Enabled    bool                   `json:"enabled"`
	Weight     float64                `json:"weight,omitempty"`
	Strategies []string               `json:"strategies,omitempty"`
	Params     map[string]interface{} `json:"params"`
	Fitness      float64                `json:"fitness"`
	Version      int                    `json:"version"`
	Dependencies []string               `json:"dependencies,omitempty"`
	EvalCount    int                    `json:"eval_count,omitempty"`
	Verified     bool                   `json:"verified,omitempty"`
	VFMScore     float64                `json:"vfm_score,omitempty"`
}

// GenomeBehavior defines behavioral traits
type GenomeBehavior struct {
	RiskTolerance    float64            `json:"risk_tolerance"`              // 0.0-1.0
	Verbosity        float64            `json:"verbosity"`                   // 0.0-1.0
	Autonomy         float64            `json:"autonomy"`                    // 0.0-1.0
	PromptStyle      string             `json:"prompt_style,omitempty"`      // Layer 3
	ToolPreferences  map[string]float64 `json:"tool_preferences,omitempty"`  // Layer 3
	ResponsePatterns []string           `json:"response_patterns,omitempty"` // Layer 3
}

// GenomeConstraints defines hard boundaries (non-evolvable)
type GenomeConstraints struct {
	MaxLossUSD     float64  `json:"max_loss_usd,omitempty"`
	AllowedAssets  []string `json:"allowed_assets,omitempty"`
	BlockedActions []string `json:"blocked_actions,omitempty"`
	MaxDivergence  float64  `json:"max_divergence,omitempty"`
	MinVFMScore    float64  `json:"min_vfm_score,omitempty"`
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
		Updates: &UpdatesConfig{
			Enabled:            true,
			AutoInstall:        false, // Notify but don't auto-install
			CheckInterval:      86400, // 24 hours
			IncludePrereleases: false,
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
