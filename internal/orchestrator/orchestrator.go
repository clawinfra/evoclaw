package orchestrator

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/clawinfra/evoclaw/internal/channels"
	"github.com/clawinfra/evoclaw/internal/clawchain"
	"github.com/clawinfra/evoclaw/internal/cloudsync"
	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/governance"
	"github.com/clawinfra/evoclaw/internal/memory"
	"github.com/clawinfra/evoclaw/internal/onchain"
	"github.com/clawinfra/evoclaw/internal/router"
	"github.com/clawinfra/evoclaw/internal/scheduler"
	"github.com/clawinfra/evoclaw/internal/types"
)

// Message is an alias to types.Message for backward compatibility
type Message = types.Message

// Response is an alias to types.Response for backward compatibility
type Response = types.Response

// AgentState tracks a running agent's state
type AgentState struct {
	ID           string
	Def          config.AgentDef
	Status       string // "running", "idle", "error", "evolving"
	StartedAt    time.Time
	LastActive   time.Time
	MessageCount int64
	ErrorCount   int64
	IsEdgeAgent  bool // true if agent connects via MQTT (runs remotely)
	// Performance metrics for evolution
	Metrics AgentMetrics
	mu      sync.RWMutex
}

// AgentMetrics tracks performance for the evolution engine
type AgentMetrics struct {
	TotalActions      int64
	SuccessfulActions int64
	FailedActions     int64
	AvgResponseMs     float64
	TokensUsed        int64
	CostUSD           float64
	// Custom metrics per agent type
	Custom map[string]float64
}

// Channel is the interface for all messaging channels
type Channel interface {
	Name() string
	Start(ctx context.Context) error
	Stop() error
	Send(ctx context.Context, msg Response) error
	Receive() <-chan Message
}

// ModelProvider is the interface for LLM providers
type ModelProvider interface {
	Name() string
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	Models() []config.Model
}

// EvolutionEngine interface for pluggable evolution
type EvolutionEngine interface {
	GetStrategy(agentID string) interface{}
	Evaluate(agentID string, metrics map[string]float64) float64
	ShouldEvolve(agentID string, minFitness float64) bool
	Mutate(agentID string, mutationRate float64) (interface{}, error)
}

type ChatRequest struct {
	Model        string
	SystemPrompt string
	Messages     []ChatMessage
	MaxTokens    int
	Temperature  float64
	Tools        []ToolSchema `json:"tools,omitempty"` // NEW: Tools for function calling
}

type ChatMessage struct {
	Role       string      `json:"role"`
	Content    string      `json:"content"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"` // NEW: Tool calls from assistant
	ToolCallID string      `json:"tool_call_id,omitempty"`  // NEW: Tool result message
}

type ChatResponse struct {
	Content      string
	Model        string
	TokensInput  int
	TokensOutput int
	FinishReason string
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"` // NEW: Tool calls in response
}

// Orchestrator is the core of EvoClaw
type Orchestrator struct {
	cfg       *config.Config
	channels  map[string]Channel
	providers map[string]ModelProvider
	agents    map[string]*AgentState
	inbox     chan Message
	outbox    chan Response
	logger    *slog.Logger
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	// Evolution engine (optional, set via SetEvolutionEngine)
	evolution EvolutionEngine
	// On-chain integration (BSC/opBNB)
	chainRegistry *onchain.ChainRegistry
	// Cloud sync (Turso)
	cloudSync *cloudsync.Manager
	// Tiered memory system
	memory *memory.Manager
	// Scheduler for periodic tasks
	scheduler *scheduler.Scheduler
	// Self-governance protocols
	governance *governance.Manager
	// Health registry for model selection
	healthRegistry *router.HealthRegistry
	// Tool management (NEW)
	toolManager        *ToolManager
	toolLoop           *ToolLoop
	resultRegistry     map[string]chan *ToolResult
	edgeResultRegistry map[string]chan map[string]interface{} // For edge agent prompt results
	resultMu           sync.RWMutex
	// MQTT channel for edge agent dispatch
	mqttChannel *channels.MQTTChannel
}

// New creates a new Orchestrator
func New(cfg *config.Config, logger *slog.Logger) *Orchestrator {
	ctx, cancel := context.WithCancel(context.Background())
	return &Orchestrator{
		cfg:                cfg,
		channels:           make(map[string]Channel),
		providers:          make(map[string]ModelProvider),
		agents:             make(map[string]*AgentState),
		inbox:              make(chan Message, 1000),
		outbox:             make(chan Response, 1000),
		logger:             logger,
		ctx:                ctx,
		cancel:             cancel,
		resultRegistry:     make(map[string]chan *ToolResult),
		edgeResultRegistry: make(map[string]chan map[string]interface{}),
	}
}

// RegisterChannel adds a messaging channel
func (o *Orchestrator) RegisterChannel(ch Channel) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.channels == nil {
		o.channels = make(map[string]Channel)
	}
	o.channels[ch.Name()] = ch
	if o.logger != nil {
		o.logger.Info("channel registered", "name", ch.Name())
	}
}

// RegisterProvider adds an LLM provider
func (o *Orchestrator) RegisterProvider(p ModelProvider) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.providers[p.Name()] = p
	o.logger.Info("model provider registered", "name", p.Name())
}

// SetEvolutionEngine sets the evolution engine
func (o *Orchestrator) SetEvolutionEngine(e EvolutionEngine) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.evolution = e
	o.logger.Info("evolution engine registered")
}

// Start begins the orchestrator loop
func (o *Orchestrator) Start() error {
	o.logger.Info("starting EvoClaw orchestrator",
		"port", o.cfg.Server.Port,
		"channels", len(o.channels),
		"providers", len(o.providers),
	)

	// Start all channels
	for name, ch := range o.channels {
		o.logger.Info("starting channel", "name", name)

		// Wire up MQTT result callback and store reference
		if mqttCh, ok := ch.(*channels.MQTTChannel); ok {
			mqttCh.SetResultCallback(o.DeliverToolResult)
			o.mqttChannel = mqttCh // Store reference for edge dispatch
			o.logger.Info("mqtt result callback wired", "channel", name)
		}

		if err := ch.Start(o.ctx); err != nil {
			return fmt.Errorf("start channel %s: %w", name, err)
		}
	}

	// Initialize agents from config
	for _, def := range o.cfg.Agents {
		o.agents[def.ID] = &AgentState{
			ID:          def.ID,
			Def:         def,
			Status:      "idle",
			StartedAt:   time.Now(),
			IsEdgeAgent: def.Remote, // Mark as edge agent if configured as remote
			Metrics: AgentMetrics{
				Custom: make(map[string]float64),
			},
		}
		if def.Remote {
			o.logger.Info("agent initialized", "id", def.ID, "type", def.Type, "mode", "edge")
		} else {
			o.logger.Info("agent initialized", "id", def.ID, "type", def.Type, "mode", "local")
		}

		// Initialize tool manager with first agent's capabilities
		if o.toolManager == nil && len(def.Capabilities) > 0 {
			o.toolManager = NewToolManager("", def.Capabilities, o.logger)
			o.toolLoop = NewToolLoop(o, o.toolManager)
			o.logger.Info("tool manager initialized", "capabilities", def.Capabilities)
		}
	}

	// Start message routing
	go o.routeIncoming()
	go o.routeOutgoing()

	// Start channel receivers
	for _, ch := range o.channels {
		go o.receiveFrom(ch)
	}

	// Start evolution engine if enabled
	if o.cfg.Evolution.Enabled {
		go o.evolutionLoop()
	}

	// Initialize on-chain integration if enabled
	if o.cfg.OnChain.Enabled {
		if err := o.initOnChain(); err != nil {
			o.logger.Warn("on-chain integration failed (non-fatal)", "error", err)
		}
	}

	// Initialize ClawChain DID auto-discovery (ADR-003)
	o.initClawChainDiscovery()

	// Initialize cloud sync if enabled
	if o.cfg.CloudSync.Enabled {
		if err := o.initCloudSync(); err != nil {
			o.logger.Warn("cloud sync failed to initialize (non-fatal)", "error", err)
		}
	}

	// Initialize tiered memory system if enabled
	if o.cfg.Memory.Enabled {
		if err := o.initMemory(); err != nil {
			o.logger.Warn("tiered memory failed to initialize (non-fatal)", "error", err)
		}
	}

	// Initialize self-governance protocols
	if err := o.initGovernance(); err != nil {
		o.logger.Warn("governance failed to initialize (non-fatal)", "error", err)
	}

	// Initialize scheduler if enabled
	if o.cfg.Scheduler.Enabled {
		if err := o.initScheduler(); err != nil {
			o.logger.Warn("scheduler failed to initialize (non-fatal)", "error", err)
		}
	}

	// Initialize health registry for model selection
	if err := o.initHealthRegistry(); err != nil {
		o.logger.Warn("health registry failed to initialize (non-fatal)", "error", err)
	}

	o.logger.Info("EvoClaw orchestrator running")
	return nil
}

// initCloudSync sets up Turso cloud sync
func (o *Orchestrator) initCloudSync() error {
	mgr, err := cloudsync.NewManager(o.cfg.CloudSync, o.logger)
	if err != nil {
		return fmt.Errorf("init cloud sync: %w", err)
	}

	// Initialize schema (creates tables if not exist)
	if err := mgr.InitSchema(o.ctx); err != nil {
		return fmt.Errorf("init cloud sync schema: %w", err)
	}

	// Start background sync (heartbeat + periodic warm/full sync)
	if err := mgr.Start(o.ctx); err != nil {
		return fmt.Errorf("start cloud sync: %w", err)
	}

	o.cloudSync = mgr
	o.logger.Info("cloud sync initialized",
		"deviceId", mgr.DeviceID(),
		"enabled", mgr.IsEnabled(),
	)

	return nil
}

// GetCloudSync returns the cloud sync manager for external access
func (o *Orchestrator) GetCloudSync() *cloudsync.Manager {
	return o.cloudSync
}

// GetMemory returns the tiered memory manager for external access
func (o *Orchestrator) GetMemory() *memory.Manager {
	return o.memory
}

// GetHealthRegistry returns the health registry for external access
func (o *Orchestrator) GetHealthRegistry() *router.HealthRegistry {
	return o.healthRegistry
}

// initHealthRegistry sets up the health registry for model selection
func (o *Orchestrator) initHealthRegistry() error {
	// Build health config from orchestrator config
	healthCfg := router.DefaultHealthConfig()

	// Apply config overrides if specified
	if o.cfg.Models.Health.PersistPath != "" {
		healthCfg.PersistPath = o.cfg.Models.Health.PersistPath
	}
	if o.cfg.Models.Health.FailureThreshold > 0 {
		healthCfg.FailureThreshold = o.cfg.Models.Health.FailureThreshold
	}
	if o.cfg.Models.Health.CooldownMinutes > 0 {
		healthCfg.CooldownPeriod = time.Duration(o.cfg.Models.Health.CooldownMinutes) * time.Minute
	}

	hr, err := router.NewHealthRegistry(healthCfg, o.logger)
	if err != nil {
		return fmt.Errorf("create health registry: %w", err)
	}

	o.healthRegistry = hr

	// Start periodic persistence (every 5 minutes)
	go o.persistHealthLoop()

	o.logger.Info("health registry initialized",
		"persist_path", healthCfg.PersistPath,
		"failure_threshold", healthCfg.FailureThreshold,
		"cooldown", healthCfg.CooldownPeriod,
	)

	return nil
}

// persistHealthLoop periodically persists health state
func (o *Orchestrator) persistHealthLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-o.ctx.Done():
			// Final persist on shutdown
			if err := o.healthRegistry.Persist(); err != nil {
				o.logger.Error("failed to persist health state on shutdown", "error", err)
			}
			return
		case <-ticker.C:
			if err := o.healthRegistry.Persist(); err != nil {
				o.logger.Error("failed to persist health state", "error", err)
			}
		}
	}
}

// initMemory sets up the tiered memory system
func (o *Orchestrator) initMemory() error {
	// Build memory config from orchestrator config
	memCfg := memory.DefaultMemoryConfig()
	memCfg.Enabled = true

	// Agent identity — use first agent or orchestrator defaults
	if len(o.cfg.Agents) > 0 {
		memCfg.AgentID = o.cfg.Agents[0].ID
		memCfg.AgentName = o.cfg.Agents[0].Name
	} else {
		memCfg.AgentID = "evoclaw-orchestrator"
		memCfg.AgentName = "EvoClaw"
	}
	memCfg.OwnerName = "owner" // Will be updated from hot memory

	// Turso connection — prefer memory config, fall back to cloud sync config
	if o.cfg.Memory.Cold.DatabaseUrl != "" {
		memCfg.DatabaseURL = o.cfg.Memory.Cold.DatabaseUrl
		memCfg.AuthToken = o.cfg.Memory.Cold.AuthToken
	} else if o.cfg.CloudSync.DatabaseURL != "" {
		memCfg.DatabaseURL = o.cfg.CloudSync.DatabaseURL
		memCfg.AuthToken = o.cfg.CloudSync.AuthToken
	} else {
		return fmt.Errorf("no database URL configured for memory cold tier")
	}

	// Apply config overrides
	if o.cfg.Memory.Tree.MaxNodes > 0 {
		memCfg.TreeMaxNodes = o.cfg.Memory.Tree.MaxNodes
	}
	if o.cfg.Memory.Tree.MaxDepth > 0 {
		memCfg.TreeMaxDepth = o.cfg.Memory.Tree.MaxDepth
	}
	if o.cfg.Memory.Hot.MaxSizeBytes > 0 {
		memCfg.HotMaxBytes = o.cfg.Memory.Hot.MaxSizeBytes
	}
	if o.cfg.Memory.Warm.MaxSizeKb > 0 {
		memCfg.WarmMaxKB = o.cfg.Memory.Warm.MaxSizeKb
	}
	if o.cfg.Memory.Warm.RetentionDays > 0 {
		memCfg.WarmRetentionDays = o.cfg.Memory.Warm.RetentionDays
	}
	if o.cfg.Memory.Scoring.HalfLifeDays > 0 {
		memCfg.HalfLifeDays = o.cfg.Memory.Scoring.HalfLifeDays
	}
	if o.cfg.Memory.Distillation.Aggression > 0 {
		memCfg.DistillationAggression = o.cfg.Memory.Distillation.Aggression
	}

	mgr, err := memory.NewManager(memCfg, o.logger)
	if err != nil {
		return fmt.Errorf("create memory manager: %w", err)
	}

	if err := mgr.Start(o.ctx); err != nil {
		return fmt.Errorf("start memory manager: %w", err)
	}

	o.memory = mgr

	// Inject LLM callback for intelligent distillation + search
	llmModel := "default" // Use whatever model the orchestrator has configured
	if o.cfg.Memory.Distillation.Model != "" {
		llmModel = o.cfg.Memory.Distillation.Model
	}

	mgr.SetLLMFunc(func(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
		provider := o.findProvider(llmModel)
		if provider == nil {
			return "", fmt.Errorf("no LLM provider available for model %s", llmModel)
		}
		// Extract just the model ID (after the /) for the API request
		modelID := llmModel
		if idx := strings.Index(llmModel, "/"); idx > 0 {
			modelID = llmModel[idx+1:]
		}
		resp, err := provider.Chat(ctx, ChatRequest{
			Model:        modelID,
			SystemPrompt: systemPrompt,
			Messages:     []ChatMessage{{Role: "user", Content: userPrompt}},
			MaxTokens:    512,
			Temperature:  0.3,
		})
		if err != nil {
			return "", err
		}
		return resp.Content, nil
	}, llmModel)

	o.logger.Info("tiered memory system initialized",
		"agent", memCfg.AgentID,
		"warm_max_kb", memCfg.WarmMaxKB,
		"half_life_days", memCfg.HalfLifeDays,
		"llm_model", llmModel,
	)

	return nil
}

// initGovernance sets up self-governance protocols (WAL, VBR, ADL, VFM)
func (o *Orchestrator) initGovernance() error {
	govCfg := governance.DefaultConfig()
	govCfg.Logger = o.logger

	// Use data directory from server config
	govCfg.BaseDir = filepath.Join(o.cfg.Server.DataDir, "governance")

	mgr, err := governance.NewManager(govCfg)
	if err != nil {
		return fmt.Errorf("create governance manager: %w", err)
	}

	o.governance = mgr
	o.logger.Info("governance protocols initialized", "base_dir", govCfg.BaseDir)

	return nil
}

// initScheduler sets up the job scheduler
func (o *Orchestrator) initScheduler() error {
	// Create scheduler with orchestrator as executor
	o.scheduler = scheduler.NewScheduler(o, o.logger)

	// Load jobs from config
	jobs := make([]*scheduler.Job, len(o.cfg.Scheduler.Jobs))
	for i, jobCfg := range o.cfg.Scheduler.Jobs {
		jobs[i] = &scheduler.Job{
			ID:   jobCfg.ID,
			Name: jobCfg.Name,
			Schedule: scheduler.ScheduleConfig{
				Kind:       jobCfg.Schedule.Kind,
				IntervalMs: jobCfg.Schedule.IntervalMs,
				Expr:       jobCfg.Schedule.Expr,
				Time:       jobCfg.Schedule.Time,
				Timezone:   jobCfg.Schedule.Timezone,
			},
			Action: scheduler.ActionConfig{
				Kind:    jobCfg.Action.Kind,
				Command: jobCfg.Action.Command,
				Args:    jobCfg.Action.Args,
				AgentID: jobCfg.Action.AgentID,
				Message: jobCfg.Action.Message,
				Topic:   jobCfg.Action.Topic,
				Payload: jobCfg.Action.Payload,
				URL:     jobCfg.Action.URL,
				Method:  jobCfg.Action.Method,
				Headers: jobCfg.Action.Headers,
			},
			Enabled: jobCfg.Enabled,
		}
	}

	if err := o.scheduler.LoadJobs(jobs); err != nil {
		return fmt.Errorf("load scheduler jobs: %w", err)
	}

	// Start scheduler
	if err := o.scheduler.Start(o.ctx); err != nil {
		return fmt.Errorf("start scheduler: %w", err)
	}

	o.logger.Info("scheduler initialized", "jobs", len(jobs))
	return nil
}

// ExecuteAgent implements scheduler.Executor interface
func (o *Orchestrator) ExecuteAgent(ctx context.Context, agentID, message string) error {
	// Find agent
	_, exists := o.agents[agentID]
	if !exists {
		return fmt.Errorf("agent not found: %s", agentID)
	}

	// Create message
	msg := Message{
		ID:        fmt.Sprintf("scheduler-%d", time.Now().UnixNano()),
		From:      "scheduler",
		To:        agentID,
		Content:   message,
		Timestamp: time.Now(),
	}

	// TODO: Implement message delivery to agent
	// Agent inbox channel not yet implemented in AgentState
	o.logger.Debug("scheduled message queued for agent", "agent", agentID, "message", msg.Content)
	return nil
}

// PublishMQTT implements scheduler.Executor interface
func (o *Orchestrator) PublishMQTT(ctx context.Context, topic string, payload map[string]any) error {
	// Find MQTT channel
	var mqttChannel Channel
	for _, ch := range o.channels {
		if ch.Name() == "mqtt" {
			mqttChannel = ch
			break
		}
	}

	if mqttChannel == nil {
		return fmt.Errorf("MQTT channel not available")
	}

	// Send via MQTT channel
	resp := Response{
		To:      topic,
		Content: fmt.Sprintf("%v", payload), // TODO: Better serialization
	}

	return mqttChannel.Send(ctx, resp)
}

// GetScheduler returns the scheduler for external access
func (o *Orchestrator) GetScheduler() *scheduler.Scheduler {
	return o.scheduler
}

// GetGovernance returns the governance manager for external access
func (o *Orchestrator) GetGovernance() *governance.Manager {
	return o.governance
}

// initOnChain sets up BSC/opBNB chain adapter
func (o *Orchestrator) initOnChain() error {
	o.chainRegistry = onchain.NewChainRegistry(o.logger)

	bscCfg := onchain.Config{
		RPCURL:          o.cfg.OnChain.RPCURL,
		ContractAddress: o.cfg.OnChain.ContractAddress,
		PrivateKey:      o.cfg.OnChain.PrivateKey,
		ChainID:         o.cfg.OnChain.ChainID,
	}

	bscClient, err := onchain.NewBSCClient(bscCfg, o.logger)
	if err != nil {
		return fmt.Errorf("init BSC client: %w", err)
	}

	o.chainRegistry.Register(bscClient)

	if err := o.chainRegistry.ConnectAll(o.ctx); err != nil {
		return fmt.Errorf("connect chains: %w", err)
	}

	o.logger.Info("on-chain integration ready",
		"chain", bscClient.ChainName(),
		"chainId", o.cfg.OnChain.ChainID,
		"contract", o.cfg.OnChain.ContractAddress,
	)

	return nil
}

// GetChainRegistry returns the chain registry for external access (API, dashboard)
func (o *Orchestrator) GetChainRegistry() *onchain.ChainRegistry {
	return o.chainRegistry
}

// initClawChainDiscovery starts the ClawChain DID auto-discovery loop (ADR-003).
// It is non-fatal: if auto-discovery is disabled or misconfigured, a warning is
// logged and startup continues normally.
func (o *Orchestrator) initClawChainDiscovery() {
	if !o.cfg.ClawChain.AutoDiscover {
		o.logger.Info("clawchain auto-discovery disabled")
		return
	}

	interval := time.Duration(o.cfg.ClawChain.CheckIntervalHours) * time.Hour
	if interval == 0 {
		interval = 6 * time.Hour
	}

	nodeURL := o.cfg.ClawChain.NodeURL
	if nodeURL == "" {
		nodeURL = "http://testnet.clawchain.win:9944"
	}

	cfg := clawchain.DiscoveryConfig{
		Enabled:       true,
		NodeURL:       nodeURL,
		CheckInterval: interval,
		AgentSeed:     o.cfg.ClawChain.AgentSeed,
		AgentContext:  "https://www.w3.org/ns/did/v1",
		AccountIDHex:  o.cfg.ClawChain.AccountIDHex,
	}

	discoverer := clawchain.NewDiscoverer(cfg, o.logger)
	go discoverer.Start(o.ctx)

	o.logger.Info("clawchain auto-discovery started",
		"node_url", nodeURL,
		"interval", interval,
	)
}

// Stop gracefully shuts down the orchestrator
func (o *Orchestrator) Stop() error {
	o.logger.Info("stopping EvoClaw orchestrator")
	o.cancel()

	// Persist health state before shutdown
	if o.healthRegistry != nil {
		if err := o.healthRegistry.Persist(); err != nil {
			o.logger.Error("error persisting health state", "error", err)
		}
	}

	// Stop tiered memory (flushes consolidation)
	if o.memory != nil {
		o.memory.Stop()
	}

	// Stop scheduler
	if o.scheduler != nil {
		o.scheduler.Stop()
	}

	// Stop cloud sync (flushes offline queue)
	if o.cloudSync != nil {
		if err := o.cloudSync.Stop(); err != nil {
			o.logger.Error("error stopping cloud sync", "error", err)
		}
	}

	for name, ch := range o.channels {
		if err := ch.Stop(); err != nil {
			o.logger.Error("error stopping channel", "name", name, "error", err)
		}
	}

	return nil
}

// receiveFrom pipes messages from a channel into the inbox
func (o *Orchestrator) receiveFrom(ch Channel) {
	for {
		select {
		case <-o.ctx.Done():
			return
		case msg := <-ch.Receive():
			msg.Channel = ch.Name()
			o.inbox <- msg
		}
	}
}

// routeIncoming processes incoming messages and routes to agents
func (o *Orchestrator) routeIncoming() {
	for {
		select {
		case <-o.ctx.Done():
			return
		case msg := <-o.inbox:
			o.handleMessage(msg)
		}
	}
}

// routeOutgoing sends responses back through channels
func (o *Orchestrator) routeOutgoing() {
	for {
		select {
		case <-o.ctx.Done():
			return
		case resp := <-o.outbox:
			o.mu.RLock()
			ch, ok := o.channels[resp.Channel]
			o.mu.RUnlock()

			if !ok {
				o.logger.Error("unknown channel for response", "channel", resp.Channel)
				continue
			}

			if err := ch.Send(o.ctx, resp); err != nil {
				o.logger.Error("error sending response",
					"channel", resp.Channel,
					"error", err,
				)
			}
		}
	}
}

// handleMessage routes a message to the appropriate agent
func (o *Orchestrator) handleMessage(msg Message) {
	o.logger.Info("incoming message",
		"channel", msg.Channel,
		"from", msg.From,
		"length", len(msg.Content),
	)

	// Skip empty messages (e.g., heartbeats, status updates)
	if strings.TrimSpace(msg.Content) == "" {
		o.logger.Debug("skipping empty message", "from", msg.From, "channel", msg.Channel)
		return
	}

	// Determine which agent should handle this
	agentID := o.selectAgent(msg)
	if agentID == "" {
		o.logger.Warn("no agent selected for message", "from", msg.From)
		return
	}

	o.mu.RLock()
	agent, ok := o.agents[agentID]
	o.mu.RUnlock()

	if !ok {
		o.logger.Error("agent not found", "id", agentID)
		return
	}

	// Select the right model based on task complexity and health
	model := o.selectModel(msg, agent)

	// Process with LLM
	go o.processWithAgent(agent, msg, model)
}

// selectAgent picks the best agent for a message using hash-based routing
// for session affinity (same sender → same agent) and natural load balancing.
// If msg.To is set and matches a known agent, that agent is used directly.
func (o *Orchestrator) selectAgent(msg Message) string {
	if len(o.agents) == 0 {
		return ""
	}

	// Explicit target: honour msg.To if it names a registered agent
	if msg.To != "" {
		if _, ok := o.agents[msg.To]; ok {
			return msg.To
		}
	}

	// Get sorted agent IDs for deterministic selection
	ids := make([]string, 0, len(o.agents))
	for id := range o.agents {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	// Single agent: no hashing needed
	if len(ids) == 1 {
		return ids[0]
	}

	// Hash-based routing for session affinity
	// Same sender will always route to the same agent
	h := fnv.New32a()
	h.Write([]byte(msg.From))
	hash := h.Sum32()

	return ids[hash%uint32(len(ids))]
}

// selectModel picks the right model based on task complexity and health
func (o *Orchestrator) selectModel(msg Message, agent *AgentState) string {
	// Start with agent's preferred model
	preferred := agent.Def.Model
	if preferred == "" {
		preferred = o.cfg.Models.Routing.Complex
	}

	// Build fallback list from config
	fallbacks := []string{
		o.cfg.Models.Routing.Simple,
		o.cfg.Models.Routing.Complex,
	}

	// Use health registry to select best model if available
	if o.healthRegistry != nil {
		selected := o.healthRegistry.GetHealthyModel(preferred, fallbacks)
		if selected != preferred {
			o.logger.Debug("model selection adjusted by health registry",
				"preferred", preferred,
				"selected", selected,
			)
		}
		return selected
	}

	// Fallback to preferred model
	return preferred
}

// processWithAgent runs a message through an agent's LLM
func (o *Orchestrator) processWithAgent(agent *AgentState, msg Message, model string) {
	start := time.Now()

	agent.mu.Lock()
	agent.Status = "running"
	agent.LastActive = time.Now()
	agent.MessageCount++
	isEdge := agent.IsEdgeAgent
	agent.mu.Unlock()

	defer func() {
		agent.mu.Lock()
		agent.Status = "idle"
		agent.mu.Unlock()
	}()

	// If this is an edge agent, forward to MQTT instead of processing locally
	if isEdge {
		o.processWithEdgeAgent(agent, msg, model, start)
		return
	}

	var resp *Response
	var err error
	var llmResp *ChatResponse

	// Check if this is an edge agent (connected via MQTT)
	if o.mqttChannel != nil && o.mqttChannel.IsEdgeAgentOnline(agent.ID) {
		// Forward prompt to edge agent instead of processing locally
		o.logger.Info("forwarding to edge agent", "agent", agent.ID)
		edgeResp, edgeErr := o.mqttChannel.SendPromptAndWait(
			o.ctx,
			agent.ID,
			msg.Content,
			agent.Def.SystemPrompt,
			60*time.Second, // 60s timeout for edge agent response
		)

		if edgeErr != nil {
			o.logger.Error("edge agent error", "agent", agent.ID, "error", edgeErr)
			agent.mu.Lock()
			agent.ErrorCount++
			agent.Metrics.FailedActions++
			agent.mu.Unlock()
			return
		}

		// Build response from edge agent
		resp = &Response{
			AgentID:   agent.ID,
			Content:   edgeResp.Content,
			Channel:   msg.Channel,
			To:        msg.From,
			ReplyTo:   msg.ID,
			MessageID: msg.ID,
			Model:     edgeResp.Model,
		}

		elapsed := time.Since(start)
		o.logger.Info("edge agent responded",
			"agent", agent.ID,
			"model", edgeResp.Model,
			"elapsed", elapsed,
			"content_length", len(edgeResp.Content),
		)

		// Send response back through channel
		o.outbox <- *resp

		// Update metrics
		agent.mu.Lock()
		agent.Metrics.TotalActions++
		agent.Metrics.SuccessfulActions++
		agent.Metrics.AvgResponseMs = (agent.Metrics.AvgResponseMs + float64(elapsed.Milliseconds())) / 2
		agent.mu.Unlock()
		return
	}

	// Use tool loop if enabled and agent has capabilities
	if o.toolLoop != nil && len(agent.Def.Capabilities) > 0 {
		tlResp, tlMetrics, tlErr := o.toolLoop.Execute(agent, msg, model)
		if tlErr != nil {
			o.logger.Error("tool loop error", "error", tlErr)
			agent.mu.Lock()
			agent.ErrorCount++
			agent.Metrics.FailedActions++
			agent.mu.Unlock()

			if o.healthRegistry != nil {
				errType := router.ClassifyError(tlErr)
				o.healthRegistry.RecordFailure(model, errType)
			}
			return
		}

		// Log metrics
		o.logger.Debug("tool loop completed",
			"iterations", tlMetrics.TotalIterations,
			"tool_calls", tlMetrics.ToolCalls,
			"errors", tlMetrics.ErrorCount,
		)

		llmResp = &ChatResponse{Content: tlResp.Content}
		resp = &Response{
			AgentID:   agent.ID,
			Content:   tlResp.Content,
			Channel:   msg.Channel,
			To:        msg.From,
			ReplyTo:   msg.ID,
			MessageID: msg.ID,
			Model:     model,
		}
	} else {
		// Legacy: direct LLM call without tools
		llmResp, err = o.processDirect(agent, msg, model)
		if err != nil {
			o.logger.Error("LLM error", "model", model, "error", err)
			agent.mu.Lock()
			agent.ErrorCount++
			agent.Metrics.FailedActions++
			agent.mu.Unlock()

			// Record failure in health registry
			if o.healthRegistry != nil {
				errType := router.ClassifyError(err)
				o.healthRegistry.RecordFailure(model, errType)
				o.logger.Debug("model failure recorded",
					"model", model,
					"error_type", errType,
				)
			}

			return
		}

		resp = &Response{
			AgentID:   agent.ID,
			Content:   llmResp.Content,
			Channel:   msg.Channel,
			To:        msg.From,
			ReplyTo:   msg.ID,
			MessageID: msg.ID,
			Model:     model,
		}
	}

	// Record success in health registry
	if o.healthRegistry != nil {
		o.healthRegistry.RecordSuccess(model)
	}

	elapsed := time.Since(start)

	// Update metrics
	agent.mu.Lock()
	agent.Metrics.TotalActions++
	agent.Metrics.SuccessfulActions++
	agent.Metrics.TokensUsed += int64(llmResp.TokensInput + llmResp.TokensOutput)
	// Running average response time
	n := float64(agent.Metrics.TotalActions)
	agent.Metrics.AvgResponseMs = agent.Metrics.AvgResponseMs*(n-1)/n + float64(elapsed.Milliseconds())/n

	// Prepare metrics for evolution evaluation
	metrics := agent.Metrics
	agent.mu.Unlock()

	// Report to evolution engine if available
	if o.evolution != nil {
		successRate := float64(metrics.SuccessfulActions) / float64(metrics.TotalActions)
		evalMetrics := map[string]float64{
			"successRate":   successRate,
			"avgResponseMs": metrics.AvgResponseMs,
			"costUSD":       metrics.CostUSD,
			"totalActions":  float64(metrics.TotalActions),
		}
		// Add custom metrics
		for k, v := range metrics.Custom {
			evalMetrics[k] = v
		}

		o.evolution.Evaluate(agent.ID, evalMetrics)
	}

	// Send response back
	o.outbox <- *resp

	o.logger.Info("agent responded",
		"agent", agent.ID,
		"model", model,
		"elapsed", elapsed,
		"tokens", llmResp.TokensInput+llmResp.TokensOutput,
	)

	// Log action on-chain if enabled
	if o.chainRegistry != nil {
		go func() {
			reporter := onchain.NewActionReporter(o.chainRegistry, o.logger)
			action := onchain.Action{
				AgentDID:    agent.ID,
				Chain:       "bsc",
				ActionType:  "chat",
				Description: fmt.Sprintf("Processed message via %s (%dms)", model, elapsed.Milliseconds()),
				Success:     true,
				Timestamp:   time.Now(),
			}
			if err := reporter.ExecuteAndReport(o.ctx, action); err != nil {
				o.logger.Debug("on-chain action log failed (non-fatal)", "error", err)
			}
		}()
	}

	// Cloud sync — critical sync after every conversation
	if o.cloudSync != nil && o.cloudSync.IsEnabled() {
		go func() {
			agent.mu.RLock()
			caps := make([]string, len(agent.Def.Capabilities))
			copy(caps, agent.Def.Capabilities)
			genome := make(map[string]interface{})
			if agent.Def.Genome != nil {
				genome["identity"] = agent.Def.Genome.Identity
				genome["skills"] = agent.Def.Genome.Skills
				genome["behavior"] = agent.Def.Genome.Behavior
				genome["constraints"] = agent.Def.Genome.Constraints
			}
			agentMetrics := agent.Metrics
			agent.mu.RUnlock()

			// Build core memory from agent state
			coreMemory := map[string]interface{}{
				"last_conversation": map[string]interface{}{
					"timestamp": time.Now().Unix(),
					"channel":   msg.Channel,
					"from":      msg.From,
					"model":     model,
					"elapsed":   elapsed.Milliseconds(),
				},
				"metrics": map[string]interface{}{
					"total_actions":      agentMetrics.TotalActions,
					"successful_actions": agentMetrics.SuccessfulActions,
					"tokens_used":        agentMetrics.TokensUsed,
					"avg_response_ms":    agentMetrics.AvgResponseMs,
					"cost_usd":           agentMetrics.CostUSD,
				},
			}

			agentMemory := &cloudsync.AgentMemory{
				AgentID:      agent.ID,
				Name:         agent.Def.Name,
				Model:        model,
				Capabilities: caps,
				Genome:       genome,
				CoreMemory:   coreMemory,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := o.cloudSync.SyncCritical(ctx, agentMemory); err != nil {
				o.logger.Debug("cloud sync critical failed (non-fatal)", "error", err)
			} else {
				o.logger.Debug("cloud synced after conversation", "agent", agent.ID)
			}
		}()
	}

	// Tiered memory — distill and store conversation
	if o.memory != nil {
		go func() {
			conv := memory.RawConversation{
				Messages: []memory.Message{
					{Role: "user", Content: msg.Content},
					{Role: "agent", Content: llmResp.Content},
				},
				Timestamp: time.Now(),
			}

			// Categorize by channel for now (tree search will improve this)
			category := fmt.Sprintf("conversations/%s", msg.Channel)
			importance := 0.5 // Default; could be tuned by message content analysis

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := o.memory.ProcessConversation(ctx, conv, category, importance); err != nil {
				o.logger.Debug("memory processing failed (non-fatal)", "error", err)
			} else {
				o.logger.Debug("conversation stored in tiered memory",
					"agent", agent.ID,
					"category", category,
				)
			}
		}()
	}
}

// findProvider locates the right provider for a model string like "anthropic/claude-opus"
func (o *Orchestrator) findProvider(model string) ModelProvider {
	// Model format: "provider/model-name" or "provider-alias/model-name"
	// Extract provider name from model string
	if idx := strings.Index(model, "/"); idx > 0 {
		providerName := model[:idx]
		
		// Match against provider names (ollama, nvidia, zhipu-1, zhipu-2, etc.)
		for _, p := range o.providers {
			pName := p.Name()
			// Exact match or starts with provider name (for aliases like zhipu-1, zhipu-2)
			if pName == providerName || strings.HasPrefix(providerName, pName) {
				return p
			}
		}
	}
	
	// Fallback: return first provider
	for _, p := range o.providers {
		return p
	}
	return nil
}

// evolutionLoop periodically evaluates and improves agents
func (o *Orchestrator) evolutionLoop() {
	ticker := time.NewTicker(time.Duration(o.cfg.Evolution.EvalIntervalSec) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-o.ctx.Done():
			return
		case <-ticker.C:
			o.evaluateAgents()
		}
	}
}

// evaluateAgents runs the evolution engine on all agents (per-skill evaluation)
func (o *Orchestrator) evaluateAgents() {
	if o.evolution == nil {
		return
	}

	o.mu.RLock()
	defer o.mu.RUnlock()

	for _, agent := range o.agents {
		agent.mu.RLock()
		metrics := agent.Metrics
		agentID := agent.ID
		genome := agent.Def.Genome
		agent.mu.RUnlock()

		if metrics.TotalActions < int64(o.cfg.Evolution.MinSamplesForEval) {
			continue
		}

		if genome == nil {
			continue
		}

		// Evaluate each enabled skill separately
		for skillName, skill := range genome.Skills {
			if !skill.Enabled {
				continue
			}

			// Prepare skill-specific metrics
			evalMetrics := o.getSkillMetrics(agent, skillName)

			// Try to cast evolution engine to the extended interface
			type SkillEvolver interface {
				EvaluateSkill(agentID, skillName string, metrics map[string]float64) (float64, error)
				ShouldEvolveSkill(agentID, skillName string, minFitness float64, minSamples int) (bool, error)
				MutateSkill(agentID, skillName string, mutationRate float64) error
			}

			if skillEvo, ok := o.evolution.(SkillEvolver); ok {
				fitness, err := skillEvo.EvaluateSkill(agentID, skillName, evalMetrics)
				if err != nil {
					o.logger.Error("skill evaluation failed",
						"agent", agentID,
						"skill", skillName,
						"error", err,
					)
					continue
				}

				o.logger.Info("skill evaluation",
					"agent", agentID,
					"skill", skillName,
					"fitness", fitness,
					"version", skill.Version,
				)

				// Check if this skill needs evolution
				minFitness := 0.6
				shouldEvolve, err := skillEvo.ShouldEvolveSkill(agentID, skillName, minFitness, o.cfg.Evolution.MinSamplesForEval)
				if err != nil {
					o.logger.Error("evolution check failed",
						"agent", agentID,
						"skill", skillName,
						"error", err,
					)
					continue
				}

				if shouldEvolve {
					o.logger.Warn("skill fitness below threshold, triggering evolution",
						"agent", agentID,
						"skill", skillName,
						"fitness", fitness,
						"threshold", minFitness,
					)

					agent.mu.Lock()
					agent.Status = "evolving"
					agent.mu.Unlock()

					go o.evolveSkill(agent, skillName, fitness)
				}
			} else {
				// Fallback to legacy agent-level evolution
				successRate := float64(metrics.SuccessfulActions) / float64(metrics.TotalActions)
				evalMetrics := map[string]float64{
					"successRate":   successRate,
					"avgResponseMs": metrics.AvgResponseMs,
					"costUSD":       metrics.CostUSD,
					"totalActions":  float64(metrics.TotalActions),
				}
				for k, v := range metrics.Custom {
					evalMetrics[k] = v
				}

				fitness := o.evolution.Evaluate(agentID, evalMetrics)
				minFitness := 0.6
				if o.evolution.ShouldEvolve(agentID, minFitness) {
					agent.mu.Lock()
					agent.Status = "evolving"
					agent.mu.Unlock()
					go o.evolveAgent(agent, fitness)
				}
			}
		}
	}
}

// getSkillMetrics extracts metrics relevant to a specific skill
func (o *Orchestrator) getSkillMetrics(agent *AgentState, skillName string) map[string]float64 {
	agent.mu.RLock()
	defer agent.mu.RUnlock()

	metrics := agent.Metrics
	successRate := 0.0
	if metrics.TotalActions > 0 {
		successRate = float64(metrics.SuccessfulActions) / float64(metrics.TotalActions)
	}

	// Base metrics
	evalMetrics := map[string]float64{
		"successRate":   successRate,
		"avgResponseMs": metrics.AvgResponseMs,
		"costUSD":       metrics.CostUSD,
		"totalActions":  float64(metrics.TotalActions),
	}

	// Add skill-specific custom metrics (prefixed with skill name)
	skillPrefix := skillName + "_"
	for k, v := range metrics.Custom {
		if len(k) >= len(skillPrefix) && k[:len(skillPrefix)] == skillPrefix {
			// Strip prefix and add to eval metrics
			evalMetrics[k[len(skillPrefix):]] = v
		}
	}

	return evalMetrics
}

// evolveSkill performs evolution on a specific skill
func (o *Orchestrator) evolveSkill(agent *AgentState, skillName string, currentFitness float64) {
	defer func() {
		agent.mu.Lock()
		agent.Status = "idle"
		agent.mu.Unlock()
	}()

	o.logger.Info("starting skill evolution",
		"agent", agent.ID,
		"skill", skillName,
		"fitness", currentFitness,
	)

	type SkillEvolver interface {
		MutateSkill(agentID, skillName string, mutationRate float64) error
	}

	if skillEvo, ok := o.evolution.(SkillEvolver); ok {
		if err := skillEvo.MutateSkill(agent.ID, skillName, o.cfg.Evolution.MaxMutationRate); err != nil {
			o.logger.Error("skill mutation failed",
				"agent", agent.ID,
				"skill", skillName,
				"error", err,
			)
			return
		}

		o.logger.Info("skill evolved successfully",
			"agent", agent.ID,
			"skill", skillName,
		)

		// Sync evolution event to cloud
		if o.cloudSync != nil && o.cloudSync.IsEnabled() {
			go func() {
				snapshot := &cloudsync.MemorySnapshot{
					AgentID:   agent.ID,
					Timestamp: time.Now().Unix(),
					Evolution: []cloudsync.EvolutionEntry{
						{
							EventType:    "skill_mutation",
							FitnessScore: currentFitness,
							Metrics: map[string]float64{
								"mutation_rate": o.cfg.Evolution.MaxMutationRate,
							},
							Timestamp: time.Now().Unix(),
						},
					},
				}
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := o.cloudSync.SyncWarm(ctx, snapshot); err != nil {
					o.logger.Debug("cloud sync evolution event failed (non-fatal)", "error", err)
				}
			}()
		}
	}
}

// evolveAgent performs evolution on a single agent
func (o *Orchestrator) evolveAgent(agent *AgentState, currentFitness float64) {
	defer func() {
		agent.mu.Lock()
		agent.Status = "idle"
		agent.mu.Unlock()
	}()

	o.logger.Info("starting agent evolution", "agent", agent.ID, "fitness", currentFitness)

	// Mutate strategy
	_, err := o.evolution.Mutate(agent.ID, o.cfg.Evolution.MaxMutationRate)
	if err != nil {
		o.logger.Error("evolution failed", "agent", agent.ID, "error", err)
		return
	}

	// Reset metrics for new strategy evaluation
	agent.mu.Lock()
	agent.Metrics = AgentMetrics{
		Custom: make(map[string]float64),
	}
	agent.mu.Unlock()

	o.logger.Info("agent evolved successfully", "agent", agent.ID)

	// Sync evolution event to cloud
	if o.cloudSync != nil && o.cloudSync.IsEnabled() {
		go func() {
			snapshot := &cloudsync.MemorySnapshot{
				AgentID:   agent.ID,
				Timestamp: time.Now().Unix(),
				Evolution: []cloudsync.EvolutionEntry{
					{
						EventType:    "mutation",
						FitnessScore: currentFitness,
						Metrics: map[string]float64{
							"mutation_rate": o.cfg.Evolution.MaxMutationRate,
						},
						Timestamp: time.Now().Unix(),
					},
				},
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := o.cloudSync.SyncWarm(ctx, snapshot); err != nil {
				o.logger.Debug("cloud sync evolution event failed (non-fatal)", "error", err)
			}
		}()
	}
}

// GetAgentMetrics returns current metrics for an agent
func (o *Orchestrator) GetAgentMetrics(agentID string) (*AgentMetrics, error) {
	o.mu.RLock()
	agent, ok := o.agents[agentID]
	o.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}

	agent.mu.RLock()
	defer agent.mu.RUnlock()

	m := agent.Metrics
	return &m, nil
}

// AgentInfo is a snapshot of agent state without the mutex
type AgentInfo struct {
	ID           string
	Def          config.AgentDef
	Status       string
	StartedAt    time.Time
	LastActive   time.Time
	MessageCount int64
	ErrorCount   int64
	Metrics      AgentMetrics
}

// ListAgents returns all registered agents as types.AgentInfo (safe copies without mutex)
func (o *Orchestrator) ListAgents() []types.AgentInfo {
	o.mu.RLock()
	defer o.mu.RUnlock()

	agents := make([]types.AgentInfo, 0, len(o.agents))
	for _, a := range o.agents {
		a.mu.RLock()
		agents = append(agents, types.AgentInfo{
			ID:           a.ID,
			Name:         a.Def.Name,
			Model:        a.Def.Model,
			Status:       a.Status,
			StartedAt:    a.StartedAt,
			LastActive:   a.LastActive,
			MessageCount: a.MessageCount,
			ErrorCount:   a.ErrorCount,
		})
		a.mu.RUnlock()
	}
	return agents
}

// RegisterResultHandler registers a handler for a tool result
func (o *Orchestrator) RegisterResultHandler(requestID string, handler func(*ToolResult)) {
	o.resultMu.Lock()
	defer o.resultMu.Unlock()

	o.resultRegistry[requestID] = make(chan *ToolResult, 1)

	go func() {
		result := <-o.resultRegistry[requestID]
		handler(result)
		delete(o.resultRegistry, requestID)
	}()

	o.logger.Debug("result handler registered", "request_id", requestID)
}

// DeliverToolResult delivers a tool result to the waiting handler
func (o *Orchestrator) DeliverToolResult(requestID string, result map[string]interface{}) {
	// Check if this is an edge agent prompt result first
	o.resultMu.RLock()
	edgeCh, isEdge := o.edgeResultRegistry[requestID]
	if !isEdge {
		// Fall back to tool result registry
		toolCh, isTool := o.resultRegistry[requestID]
		o.resultMu.RUnlock()
		
		if !isTool {
			o.logger.Warn("no handler registered for result",
				"request_id", requestID,
			)
			return
		}
		
		// Deliver tool result
		toolResult := &ToolResult{
			Tool:   getString(result, "tool"),
			Status: getString(result, "status"),
			Result: getString(result, "result"),
			Error:  getString(result, "error"),
		}

		if elapsedMs, ok := result["elapsed_ms"].(float64); ok {
			toolResult.ElapsedMs = int64(elapsedMs)
		}

		if exitCode, ok := result["exit_code"].(float64); ok {
			toolResult.ExitCode = int(exitCode)
		}

		select {
		case toolCh <- toolResult:
			o.logger.Debug("tool result delivered",
				"request_id", requestID,
				"tool", toolResult.Tool,
				"status", toolResult.Status,
			)
		case <-time.After(5 * time.Second):
			o.logger.Warn("timeout delivering tool result",
				"request_id", requestID,
			)
			o.resultMu.Lock()
			delete(o.resultRegistry, requestID)
			o.resultMu.Unlock()
		}
		return
	}
	o.resultMu.RUnlock()
	
	// Deliver edge agent prompt result
	select {
	case edgeCh <- result:
		o.logger.Info("edge agent result delivered to HTTP handler",
			"request_id", requestID,
		)
	case <-time.After(5 * time.Second):
		o.logger.Warn("timeout delivering edge agent result",
			"request_id", requestID,
		)
		o.resultMu.Lock()
		delete(o.edgeResultRegistry, requestID)
		o.resultMu.Unlock()
	}
}

// getString safely extracts a string from a map[string]interface{}
func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// processWithEdgeAgent forwards a message to an MQTT edge agent and waits for response
func (o *Orchestrator) processWithEdgeAgent(agent *AgentState, msg Message, model string, start time.Time) {
	requestID := fmt.Sprintf("prompt-%d", time.Now().UnixNano())
	
	o.logger.Info("forwarding to edge agent", "agent", agent.ID)
	
	// Create response channel for this request
	respChan := make(chan map[string]interface{}, 1)
	
	// Register result handler
	o.resultMu.Lock()
	o.edgeResultRegistry[requestID] = respChan
	o.resultMu.Unlock()
	
	// Clean up handler on exit
	defer func() {
		o.resultMu.Lock()
		delete(o.edgeResultRegistry, requestID)
		o.resultMu.Unlock()
	}()
	
	// Send prompt to edge agent via MQTT
	// The MQTT channel will detect command=prompt in metadata and use it
	// But we need to ensure Content goes into the payload as "prompt" field
	// Actually, we need to bypass the normal Send() and create the proper format directly
	// For now, use the metadata approach but the MQTT Send needs updating
	mqttMsg := Response{
		AgentID:   agent.ID,
		Channel:   "mqtt",
		To:        agent.ID,
		MessageID: requestID,
		Content:   msg.Content, // This will become "content" in payload, need to change to "prompt"
		Metadata: map[string]string{
			"command": "prompt",
		},
	}
	
	o.mu.RLock()
	mqttChan, ok := o.channels["mqtt"]
	o.mu.RUnlock()
	
	if !ok {
		o.logger.Error("mqtt channel not found")
		agent.mu.Lock()
		agent.ErrorCount++
		agent.Metrics.FailedActions++
		agent.mu.Unlock()
		return
	}
	
	if err := mqttChan.Send(o.ctx, mqttMsg); err != nil {
		o.logger.Error("failed to send to edge agent", "error", err)
		agent.mu.Lock()
		agent.ErrorCount++
		agent.Metrics.FailedActions++
		agent.mu.Unlock()
		return
	}
	
	o.logger.Info("prompt sent to edge agent", "channel", "mqtt", "agent", agent.ID, "request_id", requestID, "prompt_length", len(msg.Content))
	
	// Wait for response with timeout
	timeout := time.After(60 * time.Second)
	select {
	case result := <-respChan:
		// Extract response content from edge agent result
		content, _ := result["content"].(string)
		status, _ := result["status"].(string)
		
		if status == "error" {
			errorMsg, _ := result["error"].(string)
			o.logger.Error("edge agent error", "agent", agent.ID, "error", errorMsg)
			agent.mu.Lock()
			agent.ErrorCount++
			agent.Metrics.FailedActions++
			agent.mu.Unlock()
			return
		}
		
		elapsed := time.Since(start)
		
		// Extract metadata if available
		var inputTokens, outputTokens int64
		if metadata, ok := result["metadata"].(map[string]interface{}); ok {
			if it, ok := metadata["input_tokens"].(float64); ok {
				inputTokens = int64(it)
			}
			if ot, ok := metadata["output_tokens"].(float64); ok {
				outputTokens = int64(ot)
			}
		}
		
		// Update metrics
		agent.mu.Lock()
		agent.Metrics.TotalActions++
		agent.Metrics.SuccessfulActions++
		agent.Metrics.TokensUsed += inputTokens + outputTokens
		n := float64(agent.Metrics.TotalActions)
		agent.Metrics.AvgResponseMs = agent.Metrics.AvgResponseMs*(n-1)/n + float64(elapsed.Milliseconds())/n
		agent.mu.Unlock()
		
		// Send response back to user
		resp := &Response{
			AgentID:   agent.ID,
			Content:   content,
			Channel:   msg.Channel,
			To:        msg.From,
			ReplyTo:   msg.ID,
			MessageID: msg.ID,
			Model:     model,
		}
		
		select {
		case o.outbox <- *resp:
			o.logger.Info("edge agent response delivered", "agent", agent.ID, "elapsed", elapsed)
		case <-time.After(5 * time.Second):
			o.logger.Warn("timeout sending edge agent response to outbox")
		}
		
	case <-timeout:
		o.logger.Error("edge agent error", "agent", agent.ID, "error", "timeout waiting for response from "+agent.ID)
		agent.mu.Lock()
		agent.ErrorCount++
		agent.Metrics.FailedActions++
		agent.mu.Unlock()
		
	case <-o.ctx.Done():
		return
	}
}

// processDirect processes a message without tools (legacy mode)
func (o *Orchestrator) processDirect(agent *AgentState, msg Message, model string) (*ChatResponse, error) {
	// Extract just the model ID (after the /) for the API request
	modelID := model
	if idx := strings.Index(model, "/"); idx > 0 {
		modelID = model[idx+1:]
	}

	req := ChatRequest{
		Model:        modelID,
		SystemPrompt: agent.Def.SystemPrompt,
		Messages: []ChatMessage{
			{Role: "user", Content: msg.Content},
		},
		MaxTokens:   4096,
		Temperature: 0.7,
	}

	provider := o.findProvider(model)
	if provider == nil {
		return nil, fmt.Errorf("no provider for model: %s", model)
	}

	resp, err := provider.Chat(o.ctx, req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// buildEdgeCallSchema dynamically constructs the edge_call tool schema based on
// currently online edge agents and their advertised capabilities.
// Returns (schema, true) if at least one edge agent is online, (_, false) otherwise.
func (o *Orchestrator) buildEdgeCallSchema() (ToolSchema, bool) {
	if o.mqttChannel == nil {
		return ToolSchema{}, false
	}

	agentCaps := o.mqttChannel.GetOnlineAgentsWithCapabilities()
	if len(agentCaps) == 0 {
		return ToolSchema{}, false
	}

	// Build agent list for description
	var agentLines []string
	for id, caps := range agentCaps {
		if caps != "" {
			agentLines = append(agentLines, fmt.Sprintf("  - %s: %s", id, caps))
		} else {
			agentLines = append(agentLines, fmt.Sprintf("  - %s", id))
		}
	}
	agentDesc := strings.Join(agentLines, "\n")

	schema := ToolSchema{
		Name: "edge_call",
		Description: fmt.Sprintf(
			"Call an edge agent to handle a query using its own tools and sensors. "+
				"The edge agent runs its own LLM+tool loop and returns a natural language answer. "+
				"Use 'query' for natural language requests; use 'action'+'params' for structured calls.\n\n"+
				"Online edge agents:\n%s", agentDesc),
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"agent_id": map[string]interface{}{
					"type":        "string",
					"description": "ID of the target edge agent (e.g. 'alex-eye')",
				},
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Natural language query for the edge agent to handle",
				},
				"action": map[string]interface{}{
					"type":        "string",
					"description": "Optional: specific action name for structured calls",
				},
				"params": map[string]interface{}{
					"type":        "object",
					"description": "Optional: parameters for the structured action",
				},
			},
			"required": []string{"agent_id"},
		},
	}

	return schema, true
}
