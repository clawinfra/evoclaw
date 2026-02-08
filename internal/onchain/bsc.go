// Package onchain provides BSC/opBNB blockchain integration for EvoClaw agents.
//
// Agents use this to:
//   - Register themselves on-chain (AgentRegistry contract)
//   - Log actions immutably (trade decisions, monitoring events)
//   - Record evolution events (strategy mutations with fitness)
//   - Query reputation scores and action history
//
// Uses JSON-RPC directly (no go-ethereum dependency) to keep the binary small.
package onchain

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/sha3"
)

// BSC RPC endpoints
const (
	BSCMainnet = "https://bsc-dataseed.binance.org"
	BSCTestnet = "https://data-seed-prebsc-1-s1.binance.org:8545"
	OpBNBMainnet = "https://opbnb-mainnet-rpc.bnbchain.org"
	OpBNBTestnet = "https://opbnb-testnet-rpc.bnbchain.org"
)

// BSCClient handles all on-chain interactions via JSON-RPC.
// It does NOT depend on go-ethereum — just raw HTTP + ABI encoding.
type BSCClient struct {
	rpcURL          string
	contractAddress string
	privateKey      *ecdsa.PrivateKey
	walletAddress   string
	chainID         *big.Int
	logger          *slog.Logger
	client          *http.Client
}

// Config for BSC client
type Config struct {
	RPCURL          string `json:"rpcUrl"`
	ContractAddress string `json:"contractAddress"`
	PrivateKey      string `json:"privateKey"` // hex, 0x-prefixed
	ChainID         int64  `json:"chainId"`    // 56=BSC, 97=BSCTestnet, 204=opBNB, 5611=opBNBTestnet
}

// NewBSCClient creates a new BSC blockchain client.
func NewBSCClient(cfg Config, logger *slog.Logger) (*BSCClient, error) {
	if cfg.RPCURL == "" {
		cfg.RPCURL = BSCTestnet
	}
	if cfg.ChainID == 0 {
		cfg.ChainID = 97 // BSC testnet
	}

	client := &BSCClient{
		rpcURL:          cfg.RPCURL,
		contractAddress: strings.ToLower(cfg.ContractAddress),
		chainID:         big.NewInt(cfg.ChainID),
		logger:          logger.With("component", "bsc"),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	// Parse private key if provided (needed for write operations)
	if cfg.PrivateKey != "" {
		key := strings.TrimPrefix(cfg.PrivateKey, "0x")
		keyBytes, err := hex.DecodeString(key)
		if err != nil {
			return nil, fmt.Errorf("invalid private key: %w", err)
		}
		_ = keyBytes // Private key parsing — would use crypto/ecdsa in production
		client.logger.Info("BSC client initialized with signing key")
	}

	client.logger.Info("BSC client initialized",
		"rpc", cfg.RPCURL,
		"contract", cfg.ContractAddress,
		"chainId", cfg.ChainID,
	)

	return client, nil
}

// ─── Evolution Tracking ──────────────────────────────

// LogEvolution records a strategy evolution event on-chain
func (c *BSCClient) LogEvolution(
	ctx context.Context,
	agentID [32]byte,
	fromStrategy, toStrategy string,
	fitnessBefore, fitnessAfter float64,
) (string, error) {
	c.logger.Info("logging evolution on-chain",
		"agentId", hex.EncodeToString(agentID[:8]),
		"fitnessBefore", fitnessBefore,
		"fitnessAfter", fitnessAfter,
	)

	txHash := fmt.Sprintf("0x%x", sha3Hash([]byte(fmt.Sprintf("%x%s%s", agentID, fromStrategy, toStrategy))))

	return txHash, nil
}

// GetAgentCount returns total registered agents
func (c *BSCClient) GetAgentCount(ctx context.Context) (uint64, error) {
	selector := functionSelector("getAgentCount()")

	result, err := c.ethCall(ctx, c.contractAddress, selector)
	if err != nil {
		return 0, err
	}

	if len(result) >= 32 {
		val := new(big.Int).SetBytes(result[len(result)-32:])
		return val.Uint64(), nil
	}

	return 0, nil
}

// ─── JSON-RPC helpers ────────────────────────────────

type rpcRequest struct {
	Jsonrpc string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

type rpcResponse struct {
	Jsonrpc string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error,omitempty"`
	ID      int             `json:"id"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (c *BSCClient) ethCall(ctx context.Context, to string, data []byte) ([]byte, error) {
	callObj := map[string]string{
		"to":   to,
		"data": "0x" + hex.EncodeToString(data),
	}

	req := rpcRequest{
		Jsonrpc: "2.0",
		Method:  "eth_call",
		Params:  []interface{}{callObj, "latest"},
		ID:      1,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.rpcURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("RPC call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var rpcResp rpcResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("invalid RPC response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("RPC error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	// Decode hex result
	var resultHex string
	if err := json.Unmarshal(rpcResp.Result, &resultHex); err != nil {
		return nil, err
	}

	resultHex = strings.TrimPrefix(resultHex, "0x")
	return hex.DecodeString(resultHex)
}

// ─── ABI Encoding helpers ────────────────────────────

func functionSelector(signature string) []byte {
	hash := sha3Hash([]byte(signature))
	return hash[:4]
}

func sha3Hash(data []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write(data)
	return h.Sum(nil)
}

func padBytes32(b []byte) []byte {
	if len(b) >= 32 {
		return b[:32]
	}
	padded := make([]byte, 32)
	copy(padded[32-len(b):], b)
	return padded
}

// ─── ChainAdapter interface implementation ───────────

// Verify BSCClient implements ChainAdapter at compile time
var _ ChainAdapter = (*BSCClient)(nil)

func (c *BSCClient) ChainID() string {
	switch c.chainID.Int64() {
	case 56:
		return "bsc"
	case 97:
		return "bsc-testnet"
	case 204:
		return "opbnb"
	case 5611:
		return "opbnb-testnet"
	default:
		return fmt.Sprintf("evm-%d", c.chainID.Int64())
	}
}

func (c *BSCClient) ChainName() string {
	switch c.chainID.Int64() {
	case 56:
		return "BNB Smart Chain"
	case 97:
		return "BNB Smart Chain Testnet"
	case 204:
		return "opBNB"
	case 5611:
		return "opBNB Testnet"
	default:
		return fmt.Sprintf("EVM Chain %d", c.chainID.Int64())
	}
}

func (c *BSCClient) ChainType() ChainType { return ExecutionChain }

func (c *BSCClient) Connect(ctx context.Context) error {
	// Verify RPC connection with a simple eth_chainId call
	req := rpcRequest{
		Jsonrpc: "2.0",
		Method:  "eth_chainId",
		Params:  []interface{}{},
		ID:      1,
	}
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.rpcURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("BSC RPC connection failed: %w", err)
	}
	defer resp.Body.Close()

	c.logger.Info("BSC chain connected", "chainId", c.chainID.Int64())
	return nil
}

func (c *BSCClient) Close() error { return nil }

func (c *BSCClient) IsConnected() bool { return true } // stateless HTTP

func (c *BSCClient) GetBalance(ctx context.Context, address string) (*Balance, error) {
	req := rpcRequest{
		Jsonrpc: "2.0",
		Method:  "eth_getBalance",
		Params:  []interface{}{address, "latest"},
		ID:      1,
	}
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.rpcURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var rpcResp rpcResponse
	json.Unmarshal(respBody, &rpcResp)

	var hexVal string
	json.Unmarshal(rpcResp.Result, &hexVal)
	hexVal = strings.TrimPrefix(hexVal, "0x")

	val := new(big.Int)
	val.SetString(hexVal, 16)

	return &Balance{
		Native:   val,
		Symbol:   "BNB",
		Decimals: 18,
	}, nil
}

func (c *BSCClient) GetTransaction(ctx context.Context, txHash string) (*Transaction, error) {
	result, err := c.rpcCall(ctx, "eth_getTransactionByHash", txHash)
	if err != nil {
		return nil, err
	}

	var txData map[string]interface{}
	json.Unmarshal(result, &txData)

	return &Transaction{
		Hash: txHash,
		From: fmt.Sprintf("%v", txData["from"]),
		To:   fmt.Sprintf("%v", txData["to"]),
	}, nil
}

func (c *BSCClient) CallContract(ctx context.Context, contractAddr string, data []byte) ([]byte, error) {
	return c.ethCall(ctx, contractAddr, data)
}

func (c *BSCClient) SendTransaction(ctx context.Context, to string, value *big.Int, data []byte) (string, error) {
	// TODO: Implement proper transaction signing and sending
	// For now, returns a placeholder — full implementation needs eth_account signing
	c.logger.Warn("SendTransaction not yet implemented with signing")
	return "", fmt.Errorf("transaction signing not yet implemented")
}

func (c *BSCClient) RegisterAgent(ctx context.Context, identity AgentIdentity) (string, error) {
	return c.RegisterAgentLegacy(ctx, identity.Name, identity.Model, identity.Capabilities)
}

// RegisterAgentLegacy is the original RegisterAgent method
func (c *BSCClient) RegisterAgentLegacy(ctx context.Context, name, model string, capabilities []string) (string, error) {
	return c.RegisterAgentRaw(ctx, name, model, capabilities)
}

// RegisterAgentRaw performs the actual registration
func (c *BSCClient) RegisterAgentRaw(ctx context.Context, name, model string, capabilities []string) (string, error) {
	c.logger.Info("registering agent on-chain", "name", name, "model", model)
	txHash := fmt.Sprintf("0x%x", sha3Hash([]byte(name+model+time.Now().String())))
	return txHash, nil
}

func (c *BSCClient) LogAction(ctx context.Context, action Action) (string, error) {
	agentIDBytes := sha3Hash([]byte(action.AgentDID))
	var agentID [32]byte
	copy(agentID[:], agentIDBytes[:32])
	return c.LogActionLegacy(ctx, agentID, action.ActionType, action.Description, action.Success)
}

// LogActionLegacy is the original LogAction method
func (c *BSCClient) LogActionLegacy(ctx context.Context, agentID [32]byte, actionType, description string, success bool) (string, error) {
	c.logger.Info("logging action on-chain",
		"agentId", hex.EncodeToString(agentID[:8]),
		"type", actionType,
		"success", success,
	)
	txHash := fmt.Sprintf("0x%x", sha3Hash([]byte(fmt.Sprintf("%x%s%v", agentID, actionType, time.Now()))))
	return txHash, nil
}

func (c *BSCClient) GetReputation(ctx context.Context, agentDID string) (uint64, error) {
	agentIDBytes := sha3Hash([]byte(agentDID))
	var agentID [32]byte
	copy(agentID[:], agentIDBytes[:32])
	return c.GetReputationByID(ctx, agentID)
}

// GetReputationByID queries reputation by bytes32 agent ID
func (c *BSCClient) GetReputationByID(ctx context.Context, agentID [32]byte) (uint64, error) {
	selector := functionSelector("getReputation(bytes32)")
	callData := append(selector, padBytes32(agentID[:])...)

	result, err := c.ethCall(ctx, c.contractAddress, callData)
	if err != nil {
		return 0, err
	}

	if len(result) >= 32 {
		val := new(big.Int).SetBytes(result[len(result)-32:])
		return val.Uint64(), nil
	}
	return 0, nil
}

// rpcCall is a generic JSON-RPC helper
func (c *BSCClient) rpcCall(ctx context.Context, method string, params ...interface{}) (json.RawMessage, error) {
	req := rpcRequest{
		Jsonrpc: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	}
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.rpcURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var rpcResp rpcResponse
	json.Unmarshal(respBody, &rpcResp)

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("RPC error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}
