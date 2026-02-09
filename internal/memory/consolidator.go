package memory

import (
	"context"
	"log/slog"
	"time"
)

// ConsolidationConfig holds consolidation task settings
type ConsolidationConfig struct {
	WarmEvictionInterval  time.Duration // How often to evict from warm (default: 1 hour)
	TreePruneInterval     time.Duration // How often to prune dead nodes (default: 24 hours)
	TreeRebuildInterval   time.Duration // How often to rebuild tree (default: 30 days)
	ColdCleanupInterval   time.Duration // How often to cleanup frozen (default: 30 days)
	Enabled               bool
}

// DefaultConsolidationConfig returns default consolidation settings
func DefaultConsolidationConfig() ConsolidationConfig {
	return ConsolidationConfig{
		WarmEvictionInterval:  1 * time.Hour,
		TreePruneInterval:     24 * time.Hour,
		TreeRebuildInterval:   30 * 24 * time.Hour, // 30 days
		ColdCleanupInterval:   30 * 24 * time.Hour, // 30 days
		Enabled:               true,
	}
}

// Consolidator manages periodic memory consolidation tasks
type Consolidator struct {
	warm      *WarmMemory
	cold      *ColdMemory
	tree      *MemoryTree
	cfg       ConsolidationConfig
	scoreConfig ScoreConfig
	logger    *slog.Logger
	stopCh    chan struct{}
	doneCh    chan struct{}
}

// NewConsolidator creates a new consolidator
func NewConsolidator(
	warm *WarmMemory,
	cold *ColdMemory,
	tree *MemoryTree,
	cfg ConsolidationConfig,
	scoreConfig ScoreConfig,
	logger *slog.Logger,
) *Consolidator {
	if logger == nil {
		logger = slog.Default()
	}

	return &Consolidator{
		warm:      warm,
		cold:      cold,
		tree:      tree,
		cfg:       cfg,
		scoreConfig: scoreConfig,
		logger:    logger,
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
}

// Start begins background consolidation tasks
func (c *Consolidator) Start(ctx context.Context) {
	if !c.cfg.Enabled {
		c.logger.Info("consolidation disabled")
		close(c.doneCh)
		return
	}

	c.logger.Info("starting consolidation tasks")

	go c.runWarmEviction(ctx)
	go c.runTreePrune(ctx)
	go c.runTreeRebuild(ctx)
	go c.runColdCleanup(ctx)
}

// Stop stops all consolidation tasks
func (c *Consolidator) Stop() {
	c.logger.Info("stopping consolidation tasks")
	close(c.stopCh)
	<-c.doneCh
}

// runWarmEviction periodically evicts expired warm memories
func (c *Consolidator) runWarmEviction(ctx context.Context) {
	ticker := time.NewTicker(c.cfg.WarmEvictionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			c.logger.Debug("warm eviction task stopped")
			return
		case <-ticker.C:
			c.doWarmEviction(ctx)
		}
	}
}

// doWarmEviction performs one round of warm memory eviction
func (c *Consolidator) doWarmEviction(ctx context.Context) {
	start := time.Now()
	evicted := c.warm.EvictExpired()

	if len(evicted) == 0 {
		c.logger.Debug("warm eviction: no entries to evict")
		return
	}

	c.logger.Info("evicting warm memories",
		"count", len(evicted),
		"duration", time.Since(start))

	// Archive evicted entries to cold storage
	archived := 0
	for _, entry := range evicted {
		if err := c.cold.Add(ctx, entry); err != nil {
			c.logger.Warn("failed to archive to cold",
				"id", entry.ID,
				"error", err)
			continue
		}
		archived++

		// Update tree counts
		if err := c.tree.IncrementCounts(entry.Category, -1, 1); err != nil {
			c.logger.Warn("failed to update tree counts",
				"category", entry.Category,
				"error", err)
		}
	}

	c.logger.Info("warm eviction complete",
		"evicted", len(evicted),
		"archived", archived,
		"duration", time.Since(start))
}

// runTreePrune periodically prunes dead tree nodes
func (c *Consolidator) runTreePrune(ctx context.Context) {
	ticker := time.NewTicker(c.cfg.TreePruneInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			c.logger.Debug("tree prune task stopped")
			return
		case <-ticker.C:
			c.doTreePrune()
		}
	}
}

// doTreePrune performs one round of tree pruning
func (c *Consolidator) doTreePrune() {
	start := time.Now()

	// Remove nodes with no memories and not updated in 60 days
	removed := c.tree.PruneDeadNodes(60)

	if removed > 0 {
		c.logger.Info("tree prune complete",
			"nodes_removed", removed,
			"duration", time.Since(start))
	} else {
		c.logger.Debug("tree prune: no dead nodes")
	}
}

// runTreeRebuild periodically rebuilds the tree structure
func (c *Consolidator) runTreeRebuild(ctx context.Context) {
	ticker := time.NewTicker(c.cfg.TreeRebuildInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			c.logger.Debug("tree rebuild task stopped")
			return
		case <-ticker.C:
			c.doTreeRebuild(ctx)
		}
	}
}

// doTreeRebuild performs one round of tree restructuring
// For now, just logs — full LLM-powered rebuild needs model integration
func (c *Consolidator) doTreeRebuild(ctx context.Context) {
	start := time.Now()

	c.logger.Info("tree rebuild triggered",
		"nodes", c.tree.NodeCount,
		"depth", c.tree.GetDepth())

	// TODO: Implement LLM-powered tree restructuring
	// This would involve:
	// 1. Analyze recent activity patterns
	// 2. Identify nodes that should be merged/split
	// 3. Update tree structure accordingly
	// 4. Migrate memories to new category paths

	c.logger.Info("tree rebuild complete",
		"duration", time.Since(start))
}

// runColdCleanup periodically removes frozen entries from cold storage
func (c *Consolidator) runColdCleanup(ctx context.Context) {
	ticker := time.NewTicker(c.cfg.ColdCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			c.logger.Debug("cold cleanup task stopped")
			close(c.doneCh)
			return
		case <-ticker.C:
			c.doColdCleanup(ctx)
		}
	}
}

// doColdCleanup performs one round of cold storage cleanup
func (c *Consolidator) doColdCleanup(ctx context.Context) {
	start := time.Now()

	deleted, err := c.cold.DeleteFrozen(ctx, ColdRetentionYears, c.scoreConfig)
	if err != nil {
		c.logger.Warn("cold cleanup failed", "error", err)
		return
	}

	if deleted > 0 {
		c.logger.Info("cold cleanup complete",
			"deleted", deleted,
			"duration", time.Since(start))
	} else {
		c.logger.Debug("cold cleanup: no frozen entries to delete")
	}
}

// TriggerWarmEviction manually triggers warm memory eviction
func (c *Consolidator) TriggerWarmEviction(ctx context.Context) {
	c.logger.Info("manually triggering warm eviction")
	c.doWarmEviction(ctx)
}

// TriggerTreePrune manually triggers tree pruning
func (c *Consolidator) TriggerTreePrune() {
	c.logger.Info("manually triggering tree prune")
	c.doTreePrune()
}

// TriggerColdCleanup manually triggers cold storage cleanup
func (c *Consolidator) TriggerColdCleanup(ctx context.Context) {
	c.logger.Info("manually triggering cold cleanup")
	c.doColdCleanup(ctx)
}
