package onchain

import (
	"encoding/hex"
	"log/slog"
	"os"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

func TestFunctionSelector(t *testing.T) {
	// Known keccak256 selectors
	tests := []struct {
		sig    string
		expect string
	}{
		// transfer(address,uint256) = 0xa9059cbb
		{"transfer(address,uint256)", "a9059cbb"},
		// balanceOf(address) = 0x70a08231
		{"balanceOf(address)", "70a08231"},
	}

	for _, tt := range tests {
		sel := functionSelector(tt.sig)
		got := hex.EncodeToString(sel)
		if got != tt.expect {
			t.Errorf("functionSelector(%q) = %s, want %s", tt.sig, got, tt.expect)
		}
	}
}

func TestPadBytes32(t *testing.T) {
	input := []byte{0x01, 0x02, 0x03}
	result := padBytes32(input)

	if len(result) != 32 {
		t.Errorf("expected 32 bytes, got %d", len(result))
	}

	// Should be right-padded (big-endian)
	if result[31] != 0x03 || result[30] != 0x02 || result[29] != 0x01 {
		t.Errorf("unexpected padding: %x", result)
	}
}

func TestNewBSCClient(t *testing.T) {
	cfg := Config{
		RPCURL:          BSCTestnet,
		ContractAddress: "0x0000000000000000000000000000000000000001",
		ChainID:         97,
	}

	client, err := NewBSCClient(cfg, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if client.rpcURL != BSCTestnet {
		t.Errorf("expected BSC testnet URL, got %s", client.rpcURL)
	}

	if client.chainID.Int64() != 97 {
		t.Errorf("expected chain ID 97, got %d", client.chainID.Int64())
	}
}

func TestSha3Hash(t *testing.T) {
	// keccak256("") = c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470
	hash := sha3Hash([]byte(""))
	got := hex.EncodeToString(hash)
	expect := "c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470"
	if got != expect {
		t.Errorf("sha3Hash(\"\") = %s, want %s", got, expect)
	}
}
