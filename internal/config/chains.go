package config

import "fmt"

// ChainPreset defines a known chain with defaults
type ChainPreset struct {
	ID       string
	Type     string
	Name     string
	RPCURL   string
	ChainID  int64
	Explorer string
}

// ChainPresets maps chain IDs to their preset configurations
var ChainPresets = map[string]ChainPreset{
	// BNB Smart Chain
	"bsc": {
		ID:       "bsc",
		Type:     "evm",
		Name:     "BNB Smart Chain",
		RPCURL:   "https://bsc-dataseed.binance.org",
		ChainID:  56,
		Explorer: "https://bscscan.com",
	},
	"bsc-testnet": {
		ID:       "bsc-testnet",
		Type:     "evm",
		Name:     "BNB Smart Chain Testnet",
		RPCURL:   "https://data-seed-prebsc-1-s1.binance.org:8545",
		ChainID:  97,
		Explorer: "https://testnet.bscscan.com",
	},
	// opBNB
	"opbnb": {
		ID:       "opbnb",
		Type:     "evm",
		Name:     "opBNB Mainnet",
		RPCURL:   "https://opbnb-mainnet-rpc.bnbchain.org",
		ChainID:  204,
		Explorer: "https://opbnbscan.com",
	},
	"opbnb-testnet": {
		ID:       "opbnb-testnet",
		Type:     "evm",
		Name:     "opBNB Testnet",
		RPCURL:   "https://opbnb-testnet-rpc.bnbchain.org",
		ChainID:  5611,
		Explorer: "https://testnet.opbnbscan.com",
	},
	// Ethereum
	"ethereum": {
		ID:       "ethereum",
		Type:     "evm",
		Name:     "Ethereum Mainnet",
		RPCURL:   "https://eth.llamarpc.com",
		ChainID:  1,
		Explorer: "https://etherscan.io",
	},
	"ethereum-sepolia": {
		ID:       "ethereum-sepolia",
		Type:     "evm",
		Name:     "Ethereum Sepolia Testnet",
		RPCURL:   "https://rpc.sepolia.org",
		ChainID:  11155111,
		Explorer: "https://sepolia.etherscan.io",
	},
	// Layer 2s
	"arbitrum": {
		ID:       "arbitrum",
		Type:     "evm",
		Name:     "Arbitrum One",
		RPCURL:   "https://arb1.arbitrum.io/rpc",
		ChainID:  42161,
		Explorer: "https://arbiscan.io",
	},
	"optimism": {
		ID:       "optimism",
		Type:     "evm",
		Name:     "Optimism",
		RPCURL:   "https://mainnet.optimism.io",
		ChainID:  10,
		Explorer: "https://optimistic.etherscan.io",
	},
	"polygon": {
		ID:       "polygon",
		Type:     "evm",
		Name:     "Polygon",
		RPCURL:   "https://polygon-rpc.com",
		ChainID:  137,
		Explorer: "https://polygonscan.com",
	},
	"base": {
		ID:       "base",
		Type:     "evm",
		Name:     "Base",
		RPCURL:   "https://mainnet.base.org",
		ChainID:  8453,
		Explorer: "https://basescan.org",
	},
	// Non-EVM chains
	"hyperliquid": {
		ID:       "hyperliquid",
		Type:     "hyperliquid",
		Name:     "Hyperliquid DEX",
		RPCURL:   "https://api.hyperliquid.xyz",
		ChainID:  0,
		Explorer: "https://app.hyperliquid.xyz",
	},
	"solana": {
		ID:       "solana",
		Type:     "solana",
		Name:     "Solana Mainnet",
		RPCURL:   "https://api.mainnet-beta.solana.com",
		ChainID:  0,
		Explorer: "https://explorer.solana.com",
	},
	"solana-devnet": {
		ID:       "solana-devnet",
		Type:     "solana",
		Name:     "Solana Devnet",
		RPCURL:   "https://api.devnet.solana.com",
		ChainID:  0,
		Explorer: "https://explorer.solana.com/?cluster=devnet",
	},
}

// GetChainPreset returns a preset configuration for a known chain
func GetChainPreset(chainID string) (ChainPreset, bool) {
	preset, ok := ChainPresets[chainID]
	return preset, ok
}

// AddChain adds or updates a chain configuration
func (c *Config) AddChain(chainID string, cfg ChainConfig) {
	if c.Chains == nil {
		c.Chains = make(map[string]ChainConfig)
	}
	c.Chains[chainID] = cfg
}

// RemoveChain removes a chain configuration
func (c *Config) RemoveChain(chainID string) error {
	if c.Chains == nil {
		return fmt.Errorf("no chains configured")
	}
	if _, ok := c.Chains[chainID]; !ok {
		return fmt.Errorf("chain %s not found", chainID)
	}
	delete(c.Chains, chainID)
	return nil
}

// GetChain returns a specific chain configuration
func (c *Config) GetChain(chainID string) (ChainConfig, bool) {
	if c.Chains == nil {
		return ChainConfig{}, false
	}
	cfg, ok := c.Chains[chainID]
	return cfg, ok
}

// MigrateOnChainConfig migrates the old OnChain config to the new Chains map
// This maintains backward compatibility
func (c *Config) MigrateOnChainConfig() {
	// If Chains is empty but OnChain is enabled, auto-migrate
	if len(c.Chains) == 0 && c.OnChain.Enabled {
		chainID := "bsc-testnet" // default
		if c.OnChain.ChainID == 56 {
			chainID = "bsc"
		} else if c.OnChain.ChainID == 204 {
			chainID = "opbnb"
		} else if c.OnChain.ChainID == 5611 {
			chainID = "opbnb-testnet"
		}

		// Create chain config from OnChain settings
		chainCfg := ChainConfig{
			Enabled: true,
			Type:    "evm",
			RPCURL:  c.OnChain.RPCURL,
			ChainID: c.OnChain.ChainID,
		}

		// Get preset name if available
		if preset, ok := GetChainPreset(chainID); ok {
			chainCfg.Name = preset.Name
			chainCfg.Explorer = preset.Explorer
			// Use preset RPC if not specified
			if chainCfg.RPCURL == "" {
				chainCfg.RPCURL = preset.RPCURL
			}
		}

		c.AddChain(chainID, chainCfg)
	}
}
