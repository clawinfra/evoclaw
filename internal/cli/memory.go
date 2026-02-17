package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/clawinfra/evoclaw/internal/memory"
)

// MemoryCommand handles the 'evoclaw memory' subcommands
func MemoryCommand(args []string, configPath string) int {
	if len(args) == 0 {
		printMemoryHelp()
		return 1
	}

	subCmd := args[0]
	switch subCmd {
	case "consolidate":
		return memoryConsolidate(args[1:], configPath)
	case "store":
		return memoryStore(args[1:], configPath)
	case "retrieve":
		return memoryRetrieve(args[1:], configPath)
	case "status":
		return memoryStatus(args[1:], configPath)
	case "help", "--help", "-h":
		printMemoryHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown memory subcommand: %s\n", subCmd)
		printMemoryHelp()
		return 1
	}
}

func printMemoryHelp() {
	fmt.Print(`Usage: evoclaw memory <subcommand> [options]

Manage the built-in tiered memory system (hot/warm/cold tiers).
This provides CLI access to EvoClaw's native memory consolidation,
making it compatible with OpenClaw skill scripts.

Subcommands:
  consolidate --mode <quick|daily|monthly>   Run memory consolidation
  store --text "fact" --category "category"  Store a memory entry
  retrieve --query "search term"              Search memory
  status                                      Show memory system status

Examples:
  # Quick consolidation (warm eviction)
  evoclaw memory consolidate --mode quick

  # Store a new memory
  evoclaw memory store --text "User prefers dark mode" --category "preferences"

  # Search memory
  evoclaw memory retrieve --query "dark mode"

  # Check memory stats
  evoclaw memory status
`)
}

func memoryConsolidate(args []string, configPath string) int {
	fs := flag.NewFlagSet("consolidate", flag.ExitOnError)
	mode := fs.String("mode", "quick", "Consolidation mode: quick, daily, monthly")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		return 1
	}

	mgr, err := getMemoryManager(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	defer mgr.Stop()

	ctx := context.Background()
	start := time.Now()

	switch *mode {
	case "quick":
		fmt.Println("Running quick consolidation (warm eviction)...")
		if consolidator := mgr.GetConsolidator(); consolidator != nil {
			consolidator.TriggerWarmEviction(ctx)
		}

	case "daily":
		fmt.Println("Running daily consolidation (tree pruning)...")
		if consolidator := mgr.GetConsolidator(); consolidator != nil {
			consolidator.TriggerTreePrune()
		}

	case "monthly":
		fmt.Println("Running monthly consolidation (cold cleanup + tree rebuild)...")
		if consolidator := mgr.GetConsolidator(); consolidator != nil {
			consolidator.TriggerColdCleanup(ctx)
			// Tree rebuild not yet fully implemented
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown mode: %s (use quick, daily, or monthly)\n", *mode)
		return 1
	}

	fmt.Printf("✓ Consolidation complete (%s)\n", time.Since(start).Round(time.Millisecond))
	return 0
}

func memoryStore(args []string, configPath string) int {
	fs := flag.NewFlagSet("store", flag.ExitOnError)
	text := fs.String("text", "", "Memory text (required)")
	category := fs.String("category", "", "Memory category (required)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		return 1
	}

	if *text == "" || *category == "" {
		fmt.Fprintln(os.Stderr, "Error: --text and --category are required")
		return 1
	}

	mgr, err := getMemoryManager(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	defer mgr.Stop()

	ctx := context.Background()

	entry := &memory.MemoryEntry{
		ID:        fmt.Sprintf("cli-%d", time.Now().UnixNano()),
		Text:      *text,
		Category:  *category,
		CreatedAt: time.Now(),
		Tier:      "warm",
	}

	if err := mgr.Store(ctx, entry); err != nil {
		fmt.Fprintf(os.Stderr, "Error storing memory: %v\n", err)
		return 1
	}

	fmt.Printf("✓ Stored memory in category: %s\n", *category)
	return 0
}

func memoryRetrieve(args []string, configPath string) int {
	fs := flag.NewFlagSet("retrieve", flag.ExitOnError)
	query := fs.String("query", "", "Search query (required)")
	limit := fs.Int("limit", 5, "Maximum results")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		return 1
	}

	if *query == "" {
		fmt.Fprintln(os.Stderr, "Error: --query is required")
		return 1
	}

	mgr, err := getMemoryManager(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	defer mgr.Stop()

	ctx := context.Background()

	results, err := mgr.Search(ctx, *query, *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error searching memory: %v\n", err)
		return 1
	}

	if len(results) == 0 {
		fmt.Println("No results found")
		return 0
	}

	fmt.Printf("Found %d results:\n\n", len(results))
	for i, entry := range results {
		fmt.Printf("%d. [%s] %s\n", i+1, entry.Category, entry.Text)
		if entry.Score > 0 {
			fmt.Printf("   Score: %.2f | Tier: %s\n", entry.Score, entry.Tier)
		}
		fmt.Println()
	}

	return 0
}

func memoryStatus(args []string, configPath string) int {
	mgr, err := getMemoryManager(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	defer mgr.Stop()

	ctx := context.Background()

	status, err := mgr.GetStatus(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting status: %v\n", err)
		return 1
	}

	// Pretty print JSON
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(status); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding status: %v\n", err)
		return 1
	}

	return 0
}

func getMemoryManager(configPath string) (*memory.Manager, error) {
	cfg, err := loadConfigFromFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	if !cfg.Memory.Enabled {
		return nil, fmt.Errorf("memory system not enabled in config (set memory.enabled=true)")
	}

	// Build memory config
	memCfg := memory.DefaultMemoryConfig()
	memCfg.Enabled = true
	memCfg.DatabaseURL = cfg.Memory.Cold.DatabaseUrl
	memCfg.AuthToken = cfg.Memory.Cold.AuthToken
	memCfg.TreeMaxNodes = cfg.Memory.Tree.MaxNodes
	memCfg.TreeMaxDepth = cfg.Memory.Tree.MaxDepth
	memCfg.WarmMaxKB = cfg.Memory.Warm.MaxSizeKb
	memCfg.HalfLifeDays = cfg.Memory.Scoring.HalfLifeDays

	// Get agent ID from first agent in config
	if len(cfg.Agents) > 0 {
		memCfg.AgentID = cfg.Agents[0].ID
		memCfg.AgentName = cfg.Agents[0].Name
	} else {
		memCfg.AgentID = "default"
		memCfg.AgentName = "Agent"
	}

	memCfg.OwnerName = "Owner" // TODO: Get from config

	logger := getLogger()

	// Create manager (no LLM func for CLI - fallback mode)
	mgr, err := memory.NewManager(memCfg, logger)
	if err != nil {
		return nil, fmt.Errorf("create memory manager: %w", err)
	}

	// Start (initializes cold schema, starts consolidator)
	ctx := context.Background()
	if err := mgr.Start(ctx); err != nil {
		return nil, fmt.Errorf("start memory manager: %w", err)
	}

	return mgr, nil
}
