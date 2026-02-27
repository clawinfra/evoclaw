package onchain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"
)

// HealthResult holds the outcome of a single-chain health check.
type HealthResult struct {
	ChainID     string
	ChainName   string // reported by the node
	BlockHeight uint64
	Latency     time.Duration
	Connected   bool
	Error       string
}

// CheckHealth performs a health check for the given ChainConfig.
// Dispatches to the appropriate handler based on Type.
func CheckHealth(ctx context.Context, cfg ChainConfig) HealthResult {
	switch cfg.Type {
	case "evm":
		return checkEVMHealth(ctx, cfg)
	case "substrate":
		return checkSubstrateHealth(ctx, cfg)
	default:
		return HealthResult{
			ChainID: cfg.ID,
			Error:   fmt.Sprintf("unsupported chain type: %s", cfg.Type),
		}
	}
}

// ── JSON-RPC helpers ──────────────────────────────────────────────────────────

type jsonRPCRequest struct {
	Jsonrpc string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

type jsonRPCResponse struct {
	Jsonrpc string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
	ID      int             `json:"id"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func httpRPC(ctx context.Context, url string, method string, params []interface{}) (json.RawMessage, time.Duration, error) {
	req := jsonRPCRequest{
		Jsonrpc: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, 0, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	start := time.Now()
	resp, err := client.Do(httpReq)
	latency := time.Since(start)
	if err != nil {
		return nil, latency, err
	}
	defer resp.Body.Close() //nolint:errcheck

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, latency, err
	}

	var rpcResp jsonRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, latency, fmt.Errorf("invalid RPC response: %w", err)
	}
	if rpcResp.Error != nil {
		return nil, latency, fmt.Errorf("RPC error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}
	return rpcResp.Result, latency, nil
}

// ── EVM health check ──────────────────────────────────────────────────────────

func checkEVMHealth(ctx context.Context, cfg ChainConfig) HealthResult {
	res := HealthResult{ChainID: cfg.ID, ChainName: cfg.Name}

	// eth_blockNumber
	blockRaw, latency, err := httpRPC(ctx, cfg.RPC, "eth_blockNumber", []interface{}{})
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.Latency = latency

	var blockHex string
	if err := json.Unmarshal(blockRaw, &blockHex); err != nil {
		res.Error = fmt.Sprintf("parse blockNumber: %v", err)
		return res
	}
	blockHex = strings.TrimPrefix(blockHex, "0x")
	blockNum := new(big.Int)
	blockNum.SetString(blockHex, 16)
	res.BlockHeight = blockNum.Uint64()

	// eth_chainId — also used to set chain name when not provided
	chainIDRaw, _, err := httpRPC(ctx, cfg.RPC, "eth_chainId", []interface{}{})
	if err == nil {
		var chainIDHex string
		if jerr := json.Unmarshal(chainIDRaw, &chainIDHex); jerr == nil {
			chainIDHex = strings.TrimPrefix(chainIDHex, "0x")
			id := new(big.Int)
			id.SetString(chainIDHex, 16)
			if res.ChainName == "" {
				res.ChainName = fmt.Sprintf("EVM chainId=%d", id.Int64())
			}
		}
	}

	res.Connected = true
	return res
}

// ── Substrate health check ────────────────────────────────────────────────────

// For Substrate, the RPC is WebSocket but also accepts HTTP JSON-RPC on the
// same endpoint via https/http. We derive the HTTP URL from ws/wss.
func substrateHTTPURL(rpc string) string {
	url := rpc
	url = strings.Replace(url, "wss://", "https://", 1)
	url = strings.Replace(url, "ws://", "http://", 1)
	return url
}



func checkSubstrateHealth(ctx context.Context, cfg ChainConfig) HealthResult {
	res := HealthResult{ChainID: cfg.ID, ChainName: cfg.Name}
	url := substrateHTTPURL(cfg.RPC)

	// system_chain — returns the chain name as a string
	chainRaw, latency, err := httpRPC(ctx, url, "system_chain", []interface{}{})
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.Latency = latency

	var chainName string
	if jerr := json.Unmarshal(chainRaw, &chainName); jerr == nil && chainName != "" {
		res.ChainName = chainName
	}

	// system_syncState — returns currentBlock
	syncRaw, _, err := httpRPC(ctx, url, "system_syncState", []interface{}{})
	if err == nil {
		var syncState struct {
			CurrentBlock uint64 `json:"currentBlock"`
		}
		if jerr := json.Unmarshal(syncRaw, &syncState); jerr == nil {
			res.BlockHeight = syncState.CurrentBlock
		}
	}

	res.Connected = true
	return res
}
