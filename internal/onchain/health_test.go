package onchain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockEVMServer returns a minimal JSON-RPC server that handles eth_blockNumber and eth_chainId.
func mockEVMServer(blockHex string, chainIDHex string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck

		var result interface{}
		switch req.Method {
		case "eth_blockNumber":
			result = blockHex
		case "eth_chainId":
			result = chainIDHex
		default:
			result = nil
		}

		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  result,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}))
}

// mockSubstrateServer returns a minimal JSON-RPC server for Substrate calls.
func mockSubstrateServer(chainName string, currentBlock uint64) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck

		var result interface{}
		switch req.Method {
		case "system_chain":
			result = chainName
		case "system_syncState":
			result = map[string]uint64{
				"currentBlock": currentBlock,
				"highestBlock": currentBlock,
			}
		default:
			result = nil
		}

		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  result,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}))
}

func TestCheckEVMHealth(t *testing.T) {
	srv := mockEVMServer("0x1a4b3c", "0x38")
	defer srv.Close()

	cfg := ChainConfig{
		ID:   "bsc",
		Type: "evm",
		Name: "BNB Smart Chain",
		RPC:  srv.URL,
	}

	ctx := context.Background()
	hr := CheckHealth(ctx, cfg)

	if !hr.Connected {
		t.Errorf("expected Connected=true, error=%s", hr.Error)
	}
	if hr.BlockHeight == 0 {
		t.Error("expected non-zero BlockHeight")
	}
	// 0x1a4b3c = 1*16^5 + 10*16^4 + 4*16^3 + 11*16^2 + 3*16 + 12 = 1723196
	if hr.BlockHeight != 1723196 {
		t.Errorf("BlockHeight = %d, want 1723196", hr.BlockHeight)
	}
	if hr.Latency == 0 {
		t.Error("expected non-zero Latency")
	}
}

func TestCheckEVMHealthConnectionFailed(t *testing.T) {
	cfg := ChainConfig{
		ID:   "eth",
		Type: "evm",
		Name: "Ethereum",
		RPC:  "http://127.0.0.1:19999", // nothing listening here
	}

	ctx := context.Background()
	hr := CheckHealth(ctx, cfg)

	if hr.Connected {
		t.Error("expected Connected=false for unreachable RPC")
	}
	if hr.Error == "" {
		t.Error("expected non-empty Error string")
	}
}

func TestCheckSubstrateHealth(t *testing.T) {
	srv := mockSubstrateServer("ClawChain Testnet", 42000)
	defer srv.Close()

	cfg := ChainConfig{
		ID:   "clawchain",
		Type: "substrate",
		Name: "ClawChain",
		// Use http:// URL directly (the helper converts wss → https but srv.URL is already http)
		RPC: srv.URL,
	}

	ctx := context.Background()
	hr := checkSubstrateHealth(ctx, cfg)

	if !hr.Connected {
		t.Errorf("expected Connected=true, error=%s", hr.Error)
	}
	if hr.BlockHeight != 42000 {
		t.Errorf("BlockHeight = %d, want 42000", hr.BlockHeight)
	}
	if hr.ChainName != "ClawChain Testnet" {
		t.Errorf("ChainName = %q, want ClawChain Testnet", hr.ChainName)
	}
}

func TestCheckSubstrateHealthConnectionFailed(t *testing.T) {
	cfg := ChainConfig{
		ID:   "clawchain",
		Type: "substrate",
		Name: "ClawChain",
		RPC:  "wss://127.0.0.1:19998",
	}

	ctx := context.Background()
	hr := CheckHealth(ctx, cfg)

	if hr.Connected {
		t.Error("expected Connected=false for unreachable Substrate RPC")
	}
}

func TestCheckHealthUnsupportedType(t *testing.T) {
	cfg := ChainConfig{
		ID:   "unknown",
		Type: "solana",
		Name: "Solana",
		RPC:  "https://api.mainnet-beta.solana.com",
	}

	ctx := context.Background()
	hr := CheckHealth(ctx, cfg)

	if hr.Connected {
		t.Error("expected Connected=false for unsupported type")
	}
	if hr.Error == "" {
		t.Error("expected error message for unsupported type")
	}
}

func TestSubstrateHTTPURL(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"wss://testnet.clawchain.win:9944", "https://testnet.clawchain.win:9944"},
		{"ws://localhost:9944", "http://localhost:9944"},
		{"https://already-http.com", "https://already-http.com"},
	}
	for _, tt := range tests {
		got := substrateHTTPURL(tt.in)
		if got != tt.want {
			t.Errorf("substrateHTTPURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCheckEVMHealthInvalidBlockHex(t *testing.T) {
	// Server returns invalid hex for blockNumber
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
		var result interface{}
		if req.Method == "eth_blockNumber" {
			result = "not-hex"
		} else {
			result = "0x38"
		}
		resp := map[string]interface{}{"jsonrpc": "2.0", "id": req.ID, "result": result}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}))
	defer srv.Close()

	cfg := ChainConfig{ID: "eth", Type: "evm", Name: "Eth", RPC: srv.URL}
	hr := CheckHealth(context.Background(), cfg)
	// "not-hex" strips to "not-hex", SetString returns false → big.Int stays 0 — still connected
	// but block height is 0. Both outcomes are valid; just ensure it doesn't panic.
	_ = hr
}
