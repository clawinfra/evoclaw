// Package onchain provides multi-chain configuration and health management.
package onchain

// ChainConfig holds the configuration for a single blockchain.
// Used for the standalone ~/.evoclaw/chains.json persistence layer,
// independent of the main evoclaw.json config.
type ChainConfig struct {
	ID   string `json:"id"`
	Type string `json:"type"` // "evm" or "substrate"
	Name string `json:"name"`
	RPC  string `json:"rpc"`
}

// Presets is the canonical list of well-known chains supported by EvoClaw.
// Keyed by chain-id (the value passed to `evoclaw chain add <chain-id>`).
var Presets = map[string]ChainConfig{
	"bsc": {
		ID:   "bsc",
		Type: "evm",
		Name: "BNB Smart Chain",
		RPC:  "https://bsc-dataseed.binance.org",
	},
	"bsc-testnet": {
		ID:   "bsc-testnet",
		Type: "evm",
		Name: "BSC Testnet",
		RPC:  "https://data-seed-prebsc-1-s1.binance.org:8545",
	},
	"eth": {
		ID:   "eth",
		Type: "evm",
		Name: "Ethereum",
		RPC:  "https://eth.llamarpc.com",
	},
	"arbitrum": {
		ID:   "arbitrum",
		Type: "evm",
		Name: "Arbitrum One",
		RPC:  "https://arb1.arbitrum.io/rpc",
	},
	"base": {
		ID:   "base",
		Type: "evm",
		Name: "Base",
		RPC:  "https://mainnet.base.org",
	},
	"opbnb": {
		ID:   "opbnb",
		Type: "evm",
		Name: "opBNB",
		RPC:  "https://opbnb-mainnet-rpc.bnbchain.org",
	},
	"polygon": {
		ID:   "polygon",
		Type: "evm",
		Name: "Polygon",
		RPC:  "https://polygon-rpc.com",
	},
	"clawchain": {
		ID:   "clawchain",
		Type: "substrate",
		Name: "ClawChain Testnet",
		RPC:  "wss://testnet.clawchain.win:9944",
	},
}

// GetPreset returns the preset ChainConfig for a known chain ID.
// Returns the preset and true if found; zero value and false otherwise.
func GetPreset(chainID string) (ChainConfig, bool) {
	p, ok := Presets[chainID]
	return p, ok
}
