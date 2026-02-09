package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/clawinfra/evoclaw/internal/cloudsync"
	"github.com/google/uuid"
)

// Manager is the main memory system coordinator
type Manager struct {
	hot           *HotMemory
	warm          *WarmMemory
	cold          *ColdMemory
	tree          *MemoryTree
	distiller     *Distiller
	llmDistiller  *LLMDistiller     // LLM-powered distiller
	searcher      *TreeSearcher
	llmSearcher   *LLMTreeSearcher  // LLM-powered tree search
	rebuilder     *TreeRebuilder    // LLM-powered tree rebuilding
	consolidator  *Consolidator
	cfg           MemoryConfig
	llmFunc       LLMCallFunc       // LLM call function
	logger        *slog.Logger
}

// MemoryConfig holds all memory system configuration
type MemoryConfig struct {
	Enabled         bool
	AgentID         string
	AgentName       string
	OwnerName       string

	// Turso connection
	DatabaseURL string
	AuthToken   string

	// Tree settings
	TreeMaxNodes      int
	TreeMaxDepth      int
	TreeRebuildDays   int

	// Hot tier
	HotMaxBytes    int
	HotMaxLessons  int

	// Warm tier
	WarmMaxKB           int
	WarmRetentionDays   int
	WarmEvictionThreshold float64

	// Cold tier
	ColdRetentionYears int

	// Distillation
	DistillationAggression float64
	MaxDistilledBytes      int

	// Scoring
	HalfLifeDays       float64
	ReinforcementBoost float64

	// Consolidation
	Consolidation ConsolidationConfig
}

// DefaultMemoryConfig returns default memory configuration
func DefaultMemoryConfig() MemoryConfig {
	return MemoryConfig{
		Enabled:               true,
		TreeMaxNodes:          MaxTreeNodes,
		TreeMaxDepth:          MaxTreeDepth,
		TreeRebuildDays:       30,
		HotMaxBytes:           MaxHotSizeBytes,
		HotMaxLessons:         MaxCriticalLessons,
		WarmMaxKB:             MaxWarmSizeKB,
		WarmRetentionDays:     WarmRetentionDays,
		WarmEvictionThreshold: WarmEvictionThreshold,
		ColdRetentionYears:    ColdRetentionYears,
		DistillationAggression: 0.7,
		MaxDistilledBytes:     MaxDistilledBytes,
		HalfLifeDays:          30.0,
		ReinforcementBoost:    0.1,
		Consolidation:         DefaultConsolidationConfig(),
	}
}

// NewManager creates a new memory manager
func NewManager(cfg MemoryConfig, logger *slog.Logger) (*Manager, error) {
	if logger == nil {
		logger = slog.Default()
	}

	if !cfg.Enabled {
		logger.Info("memory system disabled")
		return nil, fmt.Errorf("memory system disabled")
	}

	// Validate config
	if cfg.AgentID == "" {
		return nil, fmt.Errorf("agent_id required")
	}
	if cfg.AgentName == "" {
		return nil, fmt.Errorf("agent_name required")
	}
	if cfg.OwnerName == "" {
		return nil, fmt.Errorf("owner_name required")
	}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("database_url required")
	}
	if cfg.AuthToken == "" {
		return nil, fmt.Errorf("auth_token required")
	}

	// Initialize components
	hot := NewHotMemory(cfg.AgentName, cfg.OwnerName)
	
	warmConfig := WarmConfig{
		MaxSizeBytes:      cfg.WarmMaxKB * 1024,
		RetentionDays:     cfg.WarmRetentionDays,
		EvictionThreshold: cfg.WarmEvictionThreshold,
		ScoreConfig: ScoreConfig{
			HalfLifeDays:       cfg.HalfLifeDays,
			ReinforcementBoost: cfg.ReinforcementBoost,
		},
	}
	warm := NewWarmMemory(warmConfig)

	tursoClient := cloudsync.NewClient(cfg.DatabaseURL, cfg.AuthToken, logger)
	cold := NewColdMemory(tursoClient, cfg.AgentID, logger)

	tree := NewMemoryTree()

	distiller := NewDistiller(cfg.DistillationAggression)

	scoreConfig := ScoreConfig{
		HalfLifeDays:       cfg.HalfLifeDays,
		ReinforcementBoost: cfg.ReinforcementBoost,
	}
	searcher := NewTreeSearcher(tree, scoreConfig)

	consolidator := NewConsolidator(warm, cold, tree, cfg.Consolidation, scoreConfig, logger)

	m := &Manager{
		hot:          hot,
		warm:         warm,
		cold:         cold,
		tree:         tree,
		distiller:    distiller,
		llmDistiller: nil,  // Set via SetLLMFunc
		searcher:     searcher,
		llmSearcher:  nil,  // Set via SetLLMFunc
		rebuilder:    nil,  // Set via SetLLMFunc
		consolidator: consolidator,
		cfg:          cfg,
		llmFunc:      nil,
		logger:       logger,
	}

	logger.Info("memory manager created",
		"agent_id", cfg.AgentID,
		"agent_name", cfg.AgentName)

	return m, nil
}

// Start initializes the memory system and starts background tasks
func (m *Manager) Start(ctx context.Context) error {
	m.logger.Info("starting memory system")

	// Initialize cold storage schema
	if err := m.cold.InitSchema(ctx); err != nil {
		return fmt.Errorf("init cold schema: %w", err)
	}

	// Start consolidation tasks
	m.consolidator.Start(ctx)

	m.logger.Info("memory system started")
	return nil
}

// Stop shuts down the memory system
func (m *Manager) Stop() {
	m.logger.Info("stopping memory system")
	m.consolidator.Stop()
	m.logger.Info("memory system stopped")
}

// ProcessConversation processes a raw conversation and stores it in memory
func (m *Manager) ProcessConversation(ctx context.Context, conv RawConversation, category string, importance float64) error {
	// Stage 1 → 2: Distill conversation (use LLM if available)
	var distilled *DistilledFact
	var err error
	
	if m.llmDistiller != nil {
		distilled, err = m.llmDistiller.DistillConversation(conv)
	} else {
		distilled, err = m.distiller.DistillConversation(conv)
	}
	
	if err != nil {
		return fmt.Errorf("distill conversation: %w", err)
	}

	// Create warm entry
	entry := &WarmEntry{
		ID:          uuid.New().String(),
		Timestamp:   conv.Timestamp,
		EventType:   "conversation",
		Category:    category,
		Content:     distilled,
		Importance:  importance,
		AccessCount: 0,
		CreatedAt:   time.Now(),
	}

	// Add to warm tier
	if err := m.warm.Add(entry); err != nil {
		return fmt.Errorf("add to warm: %w", err)
	}

	// Update tree index
	node := m.tree.FindNode(category)
	if node == nil {
		// Create node if it doesn't exist
		coreSummary, _ := m.distiller.GenerateCoreSummary(distilled)
		if err := m.tree.AddNode(category, coreSummary.Text); err != nil {
			m.logger.Warn("failed to add tree node", "category", category, "error", err)
		} else {
			m.logger.Debug("created tree node", "category", category)
		}
	}

	// Increment warm count
	if err := m.tree.IncrementCounts(category, 1, 0); err != nil {
		m.logger.Warn("failed to update tree counts", "category", category, "error", err)
	}

	m.logger.Debug("processed conversation",
		"category", category,
		"importance", importance,
		"warm_count", m.warm.Count())

	return nil
}

// Retrieve finds relevant memories for a query
func (m *Manager) Retrieve(ctx context.Context, query string, maxResults int) ([]*WarmEntry, error) {
	// Search tree index (use LLM searcher if available)
	var searchResults []SearchResult
	if m.llmSearcher != nil {
		searchResults = m.llmSearcher.Search(query, maxResults)
	} else {
		searchResults = m.searcher.Search(query, maxResults)
	}

	if len(searchResults) == 0 {
		m.logger.Debug("no relevant memories found", "query", query)
		return nil, nil
	}

	m.logger.Debug("tree search results",
		"query", query,
		"matches", len(searchResults))

	// Fetch warm memories for relevant categories
	memories := make([]*WarmEntry, 0)
	for _, result := range searchResults {
		categoryMemories := m.warm.GetByCategory(result.Path)
		memories = append(memories, categoryMemories...)

		m.logger.Debug("retrieved from category",
			"category", result.Path,
			"count", len(categoryMemories),
			"relevance", result.Relevance)
	}

	// If not enough warm memories, fetch from cold
	if len(memories) < maxResults {
		for _, result := range searchResults {
			coldMemories, err := m.cold.GetByCategory(ctx, result.Path, maxResults-len(memories))
			if err != nil {
				m.logger.Warn("failed to fetch cold memories",
					"category", result.Path,
					"error", err)
				continue
			}

			// Convert cold entries to warm format (for consistent return type)
			for _, coldEntry := range coldMemories {
				// Parse content JSON
				var distilled DistilledFact
				if err := json.Unmarshal([]byte(coldEntry.Content), &distilled); err != nil {
					m.logger.Warn("failed to parse cold content", "error", err)
					continue
				}

				warmEntry := &WarmEntry{
					ID:           coldEntry.ID,
					Timestamp:    time.Unix(coldEntry.Timestamp, 0),
					EventType:    coldEntry.EventType,
					Category:     coldEntry.Category,
					Content:      &distilled,
					Importance:   coldEntry.Importance,
					AccessCount:  coldEntry.AccessCount,
					CreatedAt:    time.Unix(coldEntry.CreatedAt, 0),
				}
				if coldEntry.LastAccessed != nil {
					warmEntry.LastAccessed = time.Unix(*coldEntry.LastAccessed, 0)
				}

				memories = append(memories, warmEntry)
			}
		}
	}

	m.logger.Info("retrieved memories",
		"query", query,
		"count", len(memories))

	return memories, nil
}

// GetHotMemory returns the current hot memory (core)
func (m *Manager) GetHotMemory() *HotMemory {
	return m.hot
}

// GetTree returns the memory tree index
func (m *Manager) GetTree() *MemoryTree {
	return m.tree
}

// GetStats returns memory system statistics
func (m *Manager) GetStats(ctx context.Context) (MemoryStats, error) {
	hotSize, _ := m.hot.GetSize()
	coldCount, err := m.cold.Count(ctx)
	if err != nil {
		m.logger.Warn("failed to get cold count", "error", err)
		coldCount = 0
	}

	warmStats := m.warm.GetStats()

	treeData, _ := m.tree.Serialize()

	return MemoryStats{
		HotSizeBytes:   hotSize,
		HotCapacity:    m.cfg.HotMaxBytes,
		WarmCount:      warmStats.TotalEntries,
		WarmSizeBytes:  warmStats.TotalSizeBytes,
		WarmCapacity:   warmStats.CapacityBytes,
		ColdCount:      coldCount,
		TreeNodes:      m.tree.NodeCount,
		TreeDepth:      m.tree.GetDepth(),
		TreeSizeBytes:  len(treeData),
		TreeCapacity:   MaxTreeSizeBytes,
	}, nil
}

// MemoryStats holds statistics about the memory system
type MemoryStats struct {
	HotSizeBytes   int
	HotCapacity    int
	WarmCount      int
	WarmSizeBytes  int
	WarmCapacity   int
	ColdCount      int
	TreeNodes      int
	TreeDepth      int
	TreeSizeBytes  int
	TreeCapacity   int
}

// AddLesson adds a critical lesson to hot memory
func (m *Manager) AddLesson(text, category string, importance float64) error {
	lesson := Lesson{
		Text:       text,
		Importance: importance,
		LearnedAt:  time.Now(),
		Category:   category,
	}

	return m.hot.AddLesson(lesson)
}

// UpdateOwnerProfile updates the owner profile in hot memory
func (m *Manager) UpdateOwnerProfile(personality *string, family, topicsLoved, topicsAvoid *[]string) error {
	return m.hot.UpdateProfile(personality, family, topicsLoved, topicsAvoid)
}

// AddProject adds a project to hot memory
func (m *Manager) AddProject(name, description string) error {
	project := Project{
		Name:        name,
		Description: description,
		StartDate:   time.Now(),
		Status:      "active",
	}

	return m.hot.AddProject(project)
}

// SetLLMFunc sets the LLM call function and initializes LLM-powered components
func (m *Manager) SetLLMFunc(llmFunc LLMCallFunc, model string) {
	m.llmFunc = llmFunc

	if llmFunc == nil {
		m.logger.Info("LLM function cleared, using rule-based memory only")
		m.llmDistiller = nil
		m.llmSearcher = nil
		m.rebuilder = nil
		return
	}

	m.logger.Info("setting up LLM-powered memory components", "model", model)

	// Create LLM-powered distiller
	m.llmDistiller = NewLLMDistiller(m.distiller, llmFunc, model, m.logger)

	// Create LLM-powered tree searcher
	m.llmSearcher = NewLLMTreeSearcher(m.tree, m.searcher, llmFunc, m.logger)

	// Create tree rebuilder
	m.rebuilder = NewTreeRebuilder(m.tree, m.warm, llmFunc, m.logger)

	m.logger.Info("LLM-powered memory components initialized")
}

// RebuildTree uses LLM to restructure the memory tree
func (m *Manager) RebuildTree(ctx context.Context) error {
	if m.rebuilder == nil {
		return fmt.Errorf("tree rebuilder not initialized (call SetLLMFunc first)")
	}

	return m.rebuilder.RebuildTree(ctx)
}
