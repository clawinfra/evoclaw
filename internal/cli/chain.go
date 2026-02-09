package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/clawinfra/evoclaw/internal/config"
)

// ChainCommand handles the 'evoclaw chain' subcommands
func ChainCommand(args []string, configPath string) int {
	if len(args) == 0 {
		printChainHelp()
		return 1
	}

	subCmd := args[0]
	switch subCmd {
	case "add":
		return chainAdd(args[1:], configPath)
	case "list":
		return chainList(args[1:], configPath)
	case "remove":
		return chainRemove(args[1:], configPath)
	case "help", "--help", "-h":
		printChainHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown chain subcommand: %s\n", subCmd)
		printChainHelp()
		return 1
	}
}

func printChainHelp() {
	fmt.Println(`Usage: evoclaw chain <subcommand> [options]

Manage blockchain configurations for EvoClaw agents.

Subcommands:
  add <chain-id>    Add a new chain configuration
  list              List all configured chains
  remove <chain-id> Remove a chain configuration

Examples:
  # Add BSC testnet (preset with minimal flags)
  evoclaw chain add bsc-testnet --wallet 0x2331...

  # Add custom EVM chain
  evoclaw chain add base --rpc https://mainnet.base.org --wallet 0x...

  # Add Hyperliquid
  evoclaw chain add hyperliquid --wallet 0x...

  # List all chains
  evoclaw chain list

  # Remove a chain
  evoclaw chain remove bsc-testnet

Supported presets:
  bsc, bsc-testnet, opbnb, opbnb-testnet, ethereum, ethereum-sepolia,
  arbitrum, optimism, polygon, base, hyperliquid, solana, solana-devnet`)
}

func chainAdd(args []string, configPath string) int {
	// Find chain-id (first non-flag argument) and extract it
	var chainIDStr string
	var parsedArgs []string
	
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			// This is a flag, keep it
			parsedArgs = append(parsedArgs, arg)
			// If next arg doesn't start with -, it's the flag value
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				parsedArgs = append(parsedArgs, args[i])
			}
		} else {
			// First non-flag arg is chain-id
			if chainIDStr == "" {
				chainIDStr = arg
			}
		}
	}

	if chainIDStr == "" {
		fmt.Fprintln(os.Stderr, "Error: chain-id required")
		fmt.Fprintln(os.Stderr, "Usage: evoclaw chain add <chain-id> [options]")
		return 1
	}

	// Parse flags
	fs := flag.NewFlagSet("chain add", flag.ExitOnError)
	chainType := fs.String("type", "", "Chain type: evm, solana, substrate, hyperliquid")
	rpcURL := fs.String("rpc", "", "RPC URL (optional for known chains)")
	wallet := fs.String("wallet", "", "Wallet address")
	chainID := fs.Int64("chain-id", 0, "EVM chain ID (auto-detected for presets)")
	explorer := fs.String("explorer", "", "Block explorer URL")

	if err := fs.Parse(parsedArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		return 1
	}

	// Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		return 1
	}

	// Check if preset exists
	preset, isPreset := config.GetChainPreset(chainIDStr)

	// Build chain config
	chainCfg := config.ChainConfig{
		Enabled: true,
	}

	// If preset, use preset values as defaults
	if isPreset {
		chainCfg.Type = preset.Type
		chainCfg.Name = preset.Name
		chainCfg.RPCURL = preset.RPCURL
		chainCfg.ChainID = preset.ChainID
		chainCfg.Explorer = preset.Explorer
	}

	// Override with flags if provided (flags take precedence)
	if *chainType != "" {
		chainCfg.Type = *chainType
	}
	if *rpcURL != "" {
		chainCfg.RPCURL = *rpcURL
	}
	if *chainID != 0 {
		chainCfg.ChainID = *chainID
	}
	if *wallet != "" {
		chainCfg.Wallet = *wallet
	}
	if *explorer != "" {
		chainCfg.Explorer = *explorer
	}

	// For non-preset chains or when no RPC override provided, ensure we keep the value
	if !isPreset {
		// Must have explicit values for custom chains
		if chainCfg.Type == "" {
			chainCfg.Type = *chainType
		}
		if chainCfg.RPCURL == "" {
			chainCfg.RPCURL = *rpcURL
		}
	}

	// Validate required fields
	if chainCfg.Type == "" {
		fmt.Fprintln(os.Stderr, "Error: chain type required (use --type or provide a known preset)")
		return 1
	}
	if chainCfg.RPCURL == "" {
		fmt.Fprintln(os.Stderr, "Error: RPC URL required (use --rpc or provide a known preset)")
		return 1
	}

	// For EVM chains, ChainID is required
	if chainCfg.Type == "evm" && chainCfg.ChainID == 0 {
		fmt.Fprintln(os.Stderr, "Error: chain ID required for EVM chains (use --chain-id or provide a known preset)")
		return 1
	}

	// Add to config
	cfg.AddChain(chainIDStr, chainCfg)

	// Save config
	if err := cfg.Save(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		return 1
	}

	// Success message
	fmt.Printf("✅ Chain added: %s\n", chainIDStr)
	if isPreset {
		fmt.Printf("   Type: %s\n", chainCfg.Type)
		fmt.Printf("   Name: %s\n", chainCfg.Name)
	}
	if chainCfg.ChainID != 0 {
		fmt.Printf("   Chain ID: %d\n", chainCfg.ChainID)
	}
	fmt.Printf("   RPC: %s\n", chainCfg.RPCURL)
	if chainCfg.Wallet != "" {
		fmt.Printf("   Wallet: %s\n", chainCfg.Wallet)
	}
	fmt.Printf("\nUse 'evoclaw chain list' to see all configured chains.\n")

	return 0
}

func chainList(args []string, configPath string) int {
	// Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		return 1
	}

	// Migrate old config if needed
	cfg.MigrateOnChainConfig()

	if len(cfg.Chains) == 0 {
		fmt.Println("No chains configured.")
		fmt.Println("Add a chain with: evoclaw chain add <chain-id>")
		return 0
	}

	fmt.Println("Chains:")
	fmt.Println()

	for chainID, chainCfg := range cfg.Chains {
		status := "✅ enabled"
		if !chainCfg.Enabled {
			status = "❌ disabled"
		}

		// Format chain info
		typeInfo := strings.ToUpper(chainCfg.Type)
		if chainCfg.ChainID != 0 {
			typeInfo = fmt.Sprintf("%s (%d)", typeInfo, chainCfg.ChainID)
		}

		name := chainCfg.Name
		if name == "" {
			name = chainID
		}

		fmt.Printf("  %-20s %-15s %-30s %s\n", chainID, typeInfo, name, status)
	}

	fmt.Println()
	fmt.Printf("Total: %d chain(s)\n", len(cfg.Chains))

	return 0
}

func chainRemove(args []string, configPath string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Error: chain-id required")
		fmt.Fprintln(os.Stderr, "Usage: evoclaw chain remove <chain-id>")
		return 1
	}

	chainID := args[0]

	// Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		return 1
	}

	// Remove chain
	if err := cfg.RemoveChain(chainID); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	// Save config
	if err := cfg.Save(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		return 1
	}

	fmt.Printf("✅ Chain removed: %s\n", chainID)
	return 0
}
