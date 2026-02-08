// Package onchain provides a multi-chain adapter layer for EvoClaw agents.
//
// Architecture:
//
//	ClawChain (Home) — identity, reputation, governance
//	   ↕
//	ChainAdapter interface
//	   ↕
//	BSC / ETH / SOL / Hyperliquid / ... (Execution)
//
// Every chain implements ChainAdapter. The orchestrator holds a registry
// of adapters. Agents request the adapter they need for each task.
// All actions on execution chains are reported back to ClawChain (home)
// for unified reputation tracking.
package onchain

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"sync"
	"time"
)

// ChainType distinguishes the agent's home chain from execution chains.
type ChainType int

const (
	// HomeChain is where agent identity and reputation live (ClawChain)
	HomeChain ChainType = iota
	// ExecutionChain is where agents perform actions (BSC, ETH, etc.)
	ExecutionChain
)

func (ct ChainType) String() string {
	switch ct {
	case HomeChain:
		return "home"
	case ExecutionChain:
		return "execution"
	default:
		return "unknown"
	}
}

// ─── Core Types ──────────────────────────────────────

// AgentIdentity represents an agent's on-chain identity.
type AgentIdentity struct {
	DID          string            // Decentralized Identifier (ClawChain-native)
	Name         string            // Human-readable name
	Model        string            // Current LLM model
	Capabilities []string          // Skills/capabilities
	Owner        string            // Owner wallet address
	Metadata     map[string]string // Extensible metadata
}

// Action represents an on-chain action to be logged.
type Action struct {
	AgentDID    string    // Who performed it
	Chain       string    // Which chain (e.g., "bsc", "ethereum")
	TxHash      string    // Transaction hash on the execution chain
	ActionType  string    // "trade", "deploy", "monitor", "evolve", "governance"
	Description string    // Human-readable summary
	Success     bool      // Whether it succeeded
	Timestamp   time.Time // When it happened
}

// Balance represents a token balance on any chain.
type Balance struct {
	Native   *big.Int // Native token balance (BNB, ETH, SOL, etc.)
	Symbol   string   // Token symbol
	Decimals int      // Token decimals
}

// Transaction represents a chain transaction.
type Transaction struct {
	Hash        string
	From        string
	To          string
	Value       *big.Int
	Data        []byte
	BlockNumber uint64
	Status      bool // true = success
	Timestamp   time.Time
}

// ─── Chain Adapter Interface ─────────────────────────

// ChainAdapter is the universal interface for any blockchain.
// Every chain EvoClaw supports must implement this.
type ChainAdapter interface {
	// Chain identity
	ChainID() string      // unique identifier: "clawchain", "bsc", "ethereum", "solana"
	ChainName() string    // human-readable: "BNB Smart Chain", "ClawChain"
	ChainType() ChainType // HomeChain or ExecutionChain

	// Connection management
	Connect(ctx context.Context) error
	Close() error
	IsConnected() bool

	// Read operations (no signing needed)
	GetBalance(ctx context.Context, address string) (*Balance, error)
	GetTransaction(ctx context.Context, txHash string) (*Transaction, error)
	CallContract(ctx context.Context, contractAddr string, data []byte) ([]byte, error)

	// Write operations (require signing)
	SendTransaction(ctx context.Context, to string, value *big.Int, data []byte) (txHash string, err error)

	// Agent-specific operations
	RegisterAgent(ctx context.Context, identity AgentIdentity) (txHash string, err error)
	LogAction(ctx context.Context, action Action) (txHash string, err error)
	GetReputation(ctx context.Context, agentDID string) (score uint64, err error)
}

// ─── Chain Registry ──────────────────────────────────

// ChainRegistry holds all configured chain adapters.
// The orchestrator uses this to route agent actions to the right chain.
type ChainRegistry struct {
	adapters map[string]ChainAdapter
	homeID   string // the home chain ID (always "clawchain" when available)
	logger   *slog.Logger
	mu       sync.RWMutex
}

// NewChainRegistry creates a new multi-chain registry.
func NewChainRegistry(logger *slog.Logger) *ChainRegistry {
	return &ChainRegistry{
		adapters: make(map[string]ChainAdapter),
		logger:   logger.With("component", "chain-registry"),
	}
}

// Register adds a chain adapter to the registry.
// If it's a HomeChain type, it becomes the default home chain.
func (r *ChainRegistry) Register(adapter ChainAdapter) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := adapter.ChainID()
	r.adapters[id] = adapter

	if adapter.ChainType() == HomeChain {
		r.homeID = id
		r.logger.Info("home chain registered", "chain", id, "name", adapter.ChainName())
	} else {
		r.logger.Info("execution chain registered", "chain", id, "name", adapter.ChainName())
	}
}

// Get returns the adapter for a specific chain.
func (r *ChainRegistry) Get(chainID string) (ChainAdapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	adapter, ok := r.adapters[chainID]
	if !ok {
		return nil, fmt.Errorf("chain not found: %s (available: %v)", chainID, r.ListChains())
	}
	return adapter, nil
}

// GetHome returns the home chain adapter (ClawChain).
// Returns nil if no home chain is configured.
func (r *ChainRegistry) GetHome() ChainAdapter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.homeID == "" {
		return nil
	}
	return r.adapters[r.homeID]
}

// ListChains returns all registered chain IDs.
func (r *ChainRegistry) ListChains() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	chains := make([]string, 0, len(r.adapters))
	for id := range r.adapters {
		chains = append(chains, id)
	}
	return chains
}

// ConnectAll connects to all registered chains.
func (r *ChainRegistry) ConnectAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for id, adapter := range r.adapters {
		if err := adapter.Connect(ctx); err != nil {
			r.logger.Error("failed to connect chain", "chain", id, "error", err)
			return fmt.Errorf("connect %s: %w", id, err)
		}
		r.logger.Info("chain connected", "chain", id)
	}
	return nil
}

// CloseAll disconnects all chains.
func (r *ChainRegistry) CloseAll() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for id, adapter := range r.adapters {
		if err := adapter.Close(); err != nil {
			r.logger.Error("error closing chain", "chain", id, "error", err)
		}
	}
}

// ─── Cross-Chain Action Reporter ─────────────────────

// ActionReporter automatically reports execution chain actions
// back to the home chain for unified reputation tracking.
type ActionReporter struct {
	registry *ChainRegistry
	logger   *slog.Logger
}

// NewActionReporter creates a reporter that bridges execution → home chain.
func NewActionReporter(registry *ChainRegistry, logger *slog.Logger) *ActionReporter {
	return &ActionReporter{
		registry: registry,
		logger:   logger.With("component", "action-reporter"),
	}
}

// Execute performs an action on an execution chain and reports it to the home chain.
// This is the primary way agents interact with external chains.
func (ar *ActionReporter) Execute(
	ctx context.Context,
	chainID string,
	agentDID string,
	actionType string,
	description string,
	executeFn func(adapter ChainAdapter) (txHash string, success bool, err error),
) (txHash string, err error) {
	// Get execution chain
	execChain, err := ar.registry.Get(chainID)
	if err != nil {
		return "", fmt.Errorf("get execution chain: %w", err)
	}

	// Execute on target chain
	txHash, success, err := executeFn(execChain)
	if err != nil {
		ar.logger.Error("execution failed",
			"chain", chainID,
			"agent", agentDID,
			"action", actionType,
			"error", err,
		)
		// Still report failure to home chain
		success = false
	}

	ar.logger.Info("action executed",
		"chain", chainID,
		"txHash", txHash,
		"success", success,
	)

	// Report to home chain (if available)
	homeChain := ar.registry.GetHome()
	if homeChain != nil {
		action := Action{
			AgentDID:    agentDID,
			Chain:       chainID,
			TxHash:      txHash,
			ActionType:  actionType,
			Description: description,
			Success:     success,
			Timestamp:   time.Now(),
		}

		reportTx, reportErr := homeChain.LogAction(ctx, action)
		if reportErr != nil {
			ar.logger.Warn("failed to report to home chain (non-fatal)",
				"error", reportErr,
			)
		} else {
			ar.logger.Info("action reported to home chain",
				"homeTx", reportTx,
				"executionTx", txHash,
			)
		}
	}

	return txHash, err
}

// ExecuteAndReport is a convenience wrapper that takes a pre-built action.
func (ar *ActionReporter) ExecuteAndReport(ctx context.Context, action Action) error {
	homeChain := ar.registry.GetHome()
	if homeChain == nil {
		ar.logger.Warn("no home chain configured, skipping cross-chain report")
		return nil
	}

	_, err := homeChain.LogAction(ctx, action)
	return err
}
