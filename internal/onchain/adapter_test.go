package onchain

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"testing"
)

func TestChainTypeString(t *testing.T) {
	tests := []struct {
		ct   ChainType
		want string
	}{
		{HomeChain, "home"},
		{ExecutionChain, "execution"},
		{ChainType(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.ct.String(); got != tt.want {
			t.Errorf("ChainType(%d).String() = %q, want %q", tt.ct, got, tt.want)
		}
	}
}

func TestChainRegistry(t *testing.T) {
	logger := slog.Default()
	reg := NewChainRegistry(logger)

	// Empty registry
	if chains := reg.ListChains(); len(chains) != 0 {
		t.Errorf("expected empty chain list, got %v", chains)
	}
	if home := reg.GetHome(); home != nil {
		t.Error("expected nil home chain")
	}

	// Register execution chain
	bsc, _ := NewBSCClient(Config{RPCURL: "http://localhost:8545", ChainID: 97}, logger)
	reg.Register(bsc)

	chains := reg.ListChains()
	if len(chains) != 1 {
		t.Fatalf("expected 1 chain, got %d", len(chains))
	}

	// Get chain
	adapter, err := reg.Get("bsc-testnet")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}

	// Get non-existent chain
	_, err = reg.Get("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent chain")
	}

	// Home should still be nil (BSC is execution chain)
	if home := reg.GetHome(); home != nil {
		t.Error("expected nil home chain for execution-only registry")
	}
}

// mockHomeChain implements ChainAdapter for testing
type mockHomeChain struct {
	logActionErr error
	lastAction   *Action
}

func (m *mockHomeChain) ChainID() string            { return "clawchain" }
func (m *mockHomeChain) ChainName() string           { return "ClawChain" }
func (m *mockHomeChain) ChainType() ChainType        { return HomeChain }
func (m *mockHomeChain) Connect(ctx context.Context) error { return nil }
func (m *mockHomeChain) Close() error                { return nil }
func (m *mockHomeChain) IsConnected() bool           { return true }
func (m *mockHomeChain) GetBalance(ctx context.Context, addr string) (*Balance, error) {
	return &Balance{Native: big.NewInt(1000), Symbol: "CLAW", Decimals: 18}, nil
}
func (m *mockHomeChain) GetTransaction(ctx context.Context, txHash string) (*Transaction, error) {
	return &Transaction{Hash: txHash}, nil
}
func (m *mockHomeChain) CallContract(ctx context.Context, addr string, data []byte) ([]byte, error) {
	return nil, nil
}
func (m *mockHomeChain) SendTransaction(ctx context.Context, to string, value *big.Int, data []byte) (string, error) {
	return "0xabc", nil
}
func (m *mockHomeChain) RegisterAgent(ctx context.Context, id AgentIdentity) (string, error) {
	return "0xreg", nil
}
func (m *mockHomeChain) LogAction(ctx context.Context, action Action) (string, error) {
	m.lastAction = &action
	return "0xlog", m.logActionErr
}
func (m *mockHomeChain) GetReputation(ctx context.Context, did string) (uint64, error) {
	return 100, nil
}

func TestChainRegistryWithHomeChain(t *testing.T) {
	logger := slog.Default()
	reg := NewChainRegistry(logger)

	home := &mockHomeChain{}
	reg.Register(home)

	if got := reg.GetHome(); got == nil {
		t.Fatal("expected home chain")
	}
	if got := reg.GetHome().ChainID(); got != "clawchain" {
		t.Errorf("home chain ID = %q, want clawchain", got)
	}
}

func TestConnectAll(t *testing.T) {
	logger := slog.Default()
	reg := NewChainRegistry(logger)

	home := &mockHomeChain{}
	reg.Register(home)

	if err := reg.ConnectAll(context.Background()); err != nil {
		t.Fatalf("ConnectAll() error: %v", err)
	}
}

func TestCloseAll(t *testing.T) {
	logger := slog.Default()
	reg := NewChainRegistry(logger)

	home := &mockHomeChain{}
	reg.Register(home)
	reg.CloseAll() // should not panic
}

func TestActionReporter(t *testing.T) {
	logger := slog.Default()
	reg := NewChainRegistry(logger)

	home := &mockHomeChain{}
	reg.Register(home)

	reporter := NewActionReporter(reg, logger)

	// Test ExecuteAndReport
	action := Action{
		AgentDID:    "did:claw:test",
		Chain:       "bsc",
		TxHash:      "0x123",
		ActionType:  "trade",
		Description: "test trade",
		Success:     true,
	}
	if err := reporter.ExecuteAndReport(context.Background(), action); err != nil {
		t.Fatalf("ExecuteAndReport() error: %v", err)
	}
	if home.lastAction == nil || home.lastAction.AgentDID != "did:claw:test" {
		t.Error("action not reported to home chain")
	}
}

func TestActionReporterNoHomeChain(t *testing.T) {
	logger := slog.Default()
	reg := NewChainRegistry(logger)
	reporter := NewActionReporter(reg, logger)

	action := Action{AgentDID: "did:claw:test"}
	if err := reporter.ExecuteAndReport(context.Background(), action); err != nil {
		t.Fatalf("expected no error when no home chain, got: %v", err)
	}
}

func TestActionReporterExecute(t *testing.T) {
	logger := slog.Default()
	reg := NewChainRegistry(logger)

	home := &mockHomeChain{}
	reg.Register(home)

	bsc, _ := NewBSCClient(Config{RPCURL: "http://localhost:8545", ChainID: 97}, logger)
	reg.Register(bsc)

	reporter := NewActionReporter(reg, logger)

	txHash, err := reporter.Execute(
		context.Background(),
		"bsc-testnet",
		"did:claw:agent1",
		"trade",
		"executed a trade",
		func(adapter ChainAdapter) (string, bool, error) {
			return "0xtx123", true, nil
		},
	)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if txHash != "0xtx123" {
		t.Errorf("txHash = %q, want 0xtx123", txHash)
	}
}

func TestActionReporterExecuteFailure(t *testing.T) {
	logger := slog.Default()
	reg := NewChainRegistry(logger)

	home := &mockHomeChain{}
	reg.Register(home)

	bsc, _ := NewBSCClient(Config{RPCURL: "http://localhost:8545", ChainID: 97}, logger)
	reg.Register(bsc)

	reporter := NewActionReporter(reg, logger)

	_, err := reporter.Execute(
		context.Background(),
		"bsc-testnet",
		"did:claw:agent1",
		"trade",
		"failed trade",
		func(adapter ChainAdapter) (string, bool, error) {
			return "", false, fmt.Errorf("trade failed")
		},
	)
	// Error from executeFn is returned
	if err == nil {
		t.Error("expected error from failed execution")
	}
}

func TestActionReporterExecuteChainNotFound(t *testing.T) {
	logger := slog.Default()
	reg := NewChainRegistry(logger)
	reporter := NewActionReporter(reg, logger)

	_, err := reporter.Execute(
		context.Background(),
		"nonexistent",
		"did:claw:agent1",
		"trade",
		"test",
		func(adapter ChainAdapter) (string, bool, error) {
			return "0x", true, nil
		},
	)
	if err == nil {
		t.Error("expected error for non-existent chain")
	}
}
