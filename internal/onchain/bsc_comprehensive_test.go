package onchain

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewBSCClientComprehensive(t *testing.T) {
	logger := slog.Default()

	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"defaults", Config{}, false},
		{"with RPC", Config{RPCURL: "http://localhost:8545"}, false},
		{"with valid key", Config{PrivateKey: "0x" + hex.EncodeToString(make([]byte, 32))}, false},
		{"with invalid key", Config{PrivateKey: "0xZZZZ"}, true},
		{"custom chain", Config{ChainID: 56}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewBSCClient(tt.cfg, logger)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewBSCClient() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && client == nil {
				t.Error("expected non-nil client")
			}
		})
	}
}

func TestBSCClientChainID(t *testing.T) {
	logger := slog.Default()

	tests := []struct {
		chainID  int64
		wantID   string
		wantName string
	}{
		{56, "bsc", "BNB Smart Chain"},
		{97, "bsc-testnet", "BNB Smart Chain Testnet"},
		{204, "opbnb", "opBNB"},
		{5611, "opbnb-testnet", "opBNB Testnet"},
		{999, "evm-999", "EVM Chain 999"},
	}

	for _, tt := range tests {
		t.Run(tt.wantID, func(t *testing.T) {
			client, _ := NewBSCClient(Config{ChainID: tt.chainID}, logger)
			if got := client.ChainID(); got != tt.wantID {
				t.Errorf("ChainID() = %q, want %q", got, tt.wantID)
			}
			if got := client.ChainName(); got != tt.wantName {
				t.Errorf("ChainName() = %q, want %q", got, tt.wantName)
			}
		})
	}
}

func TestBSCClientChainType(t *testing.T) {
	logger := slog.Default()
	client, _ := NewBSCClient(Config{}, logger)
	if got := client.ChainType(); got != ExecutionChain {
		t.Errorf("ChainType() = %v, want ExecutionChain", got)
	}
}

func TestBSCClientIsConnected(t *testing.T) {
	logger := slog.Default()
	client, _ := NewBSCClient(Config{}, logger)
	if !client.IsConnected() {
		t.Error("expected IsConnected() = true")
	}
}

func TestBSCClientClose(t *testing.T) {
	logger := slog.Default()
	client, _ := NewBSCClient(Config{}, logger)
	if err := client.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}
}

func TestLogEvolution(t *testing.T) {
	logger := slog.Default()
	client, _ := NewBSCClient(Config{}, logger)

	var agentID [32]byte
	copy(agentID[:], []byte("test-agent"))

	txHash, err := client.LogEvolution(context.Background(), agentID, "strategy-a", "strategy-b", 0.5, 0.8)
	if err != nil {
		t.Fatalf("LogEvolution() error: %v", err)
	}
	if txHash == "" {
		t.Error("expected non-empty txHash")
	}
}

func TestRegisterAgent(t *testing.T) {
	logger := slog.Default()
	client, _ := NewBSCClient(Config{}, logger)

	identity := AgentIdentity{
		Name:         "test-agent",
		Model:        "claude-3",
		Capabilities: []string{"trading", "monitoring"},
	}

	txHash, err := client.RegisterAgent(context.Background(), identity)
	if err != nil {
		t.Fatalf("RegisterAgent() error: %v", err)
	}
	if txHash == "" {
		t.Error("expected non-empty txHash")
	}
}

func TestLogAction(t *testing.T) {
	logger := slog.Default()
	client, _ := NewBSCClient(Config{}, logger)

	action := Action{
		AgentDID:    "did:claw:test",
		ActionType:  "trade",
		Description: "bought BTC",
		Success:     true,
	}

	txHash, err := client.LogAction(context.Background(), action)
	if err != nil {
		t.Fatalf("LogAction() error: %v", err)
	}
	if txHash == "" {
		t.Error("expected non-empty txHash")
	}
}

func TestSendTransaction(t *testing.T) {
	logger := slog.Default()
	client, _ := NewBSCClient(Config{}, logger)

	_, err := client.SendTransaction(context.Background(), "0x123", nil, nil)
	if err == nil {
		t.Error("expected error for unimplemented SendTransaction")
	}
}

func TestFunctionSelectorLength(t *testing.T) {
	sel := functionSelector("getAgentCount()")
	if len(sel) != 4 {
		t.Errorf("expected 4 bytes, got %d", len(sel))
	}
}

func TestPadBytes32Comprehensive(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want int
	}{
		{"short", []byte{1, 2, 3}, 32},
		{"exact", make([]byte, 32), 32},
		{"long", make([]byte, 40), 32},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := padBytes32(tt.in)
			if len(got) != tt.want {
				t.Errorf("len(padBytes32) = %d, want %d", len(got), tt.want)
			}
		})
	}
}

func newMockRPCServer(t *testing.T, handler func(req rpcRequest) interface{}) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Logf("decode error: %v", err)
			http.Error(w, "bad request", 400)
			return
		}
		result := handler(req)
		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  result,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

func TestConnect(t *testing.T) {
	srv := newMockRPCServer(t, func(req rpcRequest) interface{} {
		return "0x61"
	})
	defer srv.Close()

	logger := slog.Default()
	client, _ := NewBSCClient(Config{RPCURL: srv.URL, ChainID: 97}, logger)

	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("Connect() error: %v", err)
	}
}

func TestGetBalance(t *testing.T) {
	srv := newMockRPCServer(t, func(req rpcRequest) interface{} {
		return "0x0000000000000000000000000000000000000000000000000de0b6b3a7640000"
	})
	defer srv.Close()

	logger := slog.Default()
	client, _ := NewBSCClient(Config{RPCURL: srv.URL, ChainID: 97}, logger)

	balance, err := client.GetBalance(context.Background(), "0x123")
	if err != nil {
		t.Fatalf("GetBalance() error: %v", err)
	}
	if balance.Symbol != "BNB" {
		t.Errorf("symbol = %q, want BNB", balance.Symbol)
	}
	if balance.Native == nil || balance.Native.Sign() <= 0 {
		t.Error("expected positive balance")
	}
}

func TestGetTransaction(t *testing.T) {
	srv := newMockRPCServer(t, func(req rpcRequest) interface{} {
		return map[string]interface{}{
			"hash": "0xabc",
			"from": "0x111",
			"to":   "0x222",
		}
	})
	defer srv.Close()

	logger := slog.Default()
	client, _ := NewBSCClient(Config{RPCURL: srv.URL, ChainID: 97}, logger)

	tx, err := client.GetTransaction(context.Background(), "0xabc")
	if err != nil {
		t.Fatalf("GetTransaction() error: %v", err)
	}
	if tx.Hash != "0xabc" {
		t.Errorf("hash = %q, want 0xabc", tx.Hash)
	}
}

func TestGetAgentCount(t *testing.T) {
	srv := newMockRPCServer(t, func(req rpcRequest) interface{} {
		return "0x0000000000000000000000000000000000000000000000000000000000000005"
	})
	defer srv.Close()

	logger := slog.Default()
	client, _ := NewBSCClient(Config{RPCURL: srv.URL, ContractAddress: "0xcontract", ChainID: 97}, logger)

	count, err := client.GetAgentCount(context.Background())
	if err != nil {
		t.Fatalf("GetAgentCount() error: %v", err)
	}
	if count != 5 {
		t.Errorf("count = %d, want 5", count)
	}
}

func TestGetReputation(t *testing.T) {
	srv := newMockRPCServer(t, func(req rpcRequest) interface{} {
		return "0x000000000000000000000000000000000000000000000000000000000000000a"
	})
	defer srv.Close()

	logger := slog.Default()
	client, _ := NewBSCClient(Config{RPCURL: srv.URL, ContractAddress: "0xcontract", ChainID: 97}, logger)

	score, err := client.GetReputation(context.Background(), "did:claw:test")
	if err != nil {
		t.Fatalf("GetReputation() error: %v", err)
	}
	if score != 10 {
		t.Errorf("score = %d, want 10", score)
	}
}

func TestEthCallRPCError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"error":   map[string]interface{}{"code": -32000, "message": "execution reverted"},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	logger := slog.Default()
	client, _ := NewBSCClient(Config{RPCURL: srv.URL, ContractAddress: "0xcontract"}, logger)

	_, err := client.GetAgentCount(context.Background())
	if err == nil {
		t.Error("expected error for RPC error response")
	}
}

func TestCallContract(t *testing.T) {
	srv := newMockRPCServer(t, func(req rpcRequest) interface{} {
		return "0x00000000000000000000000000000000000000000000000000000000000000ff"
	})
	defer srv.Close()

	logger := slog.Default()
	client, _ := NewBSCClient(Config{RPCURL: srv.URL}, logger)

	result, err := client.CallContract(context.Background(), "0xcontract", []byte{0x01, 0x02})
	if err != nil {
		t.Fatalf("CallContract() error: %v", err)
	}
	if len(result) == 0 {
		t.Error("expected non-empty result")
	}
}
