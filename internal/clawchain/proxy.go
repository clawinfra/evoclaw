package clawchain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// RPCCaller abstracts the HTTP call to the Substrate node,
// allowing tests to inject a mock transport.
type RPCCaller interface {
	Call(ctx context.Context, url string, req SubstrateRPCRequest) (*SubstrateRPCResponse, error)
}

// EventCallback is invoked when a chain event is received via WebSocket.
type EventCallback func(event ChainEvent)

// Proxy forwards edge-agent RPC requests to a ClawChain Substrate node.
type Proxy struct {
	cfg       ProxyConfig
	logger    *slog.Logger
	caller    RPCCaller
	requestID atomic.Uint64

	mu        sync.RWMutex
	callbacks []EventCallback
}

// New creates a new ClawChain RPC proxy.
func New(cfg ProxyConfig, logger *slog.Logger) *Proxy {
	return &Proxy{
		cfg:    cfg,
		logger: logger,
		caller: &httpCaller{timeout: time.Duration(cfg.RequestTimeoutSec) * time.Second},
	}
}

// NewWithCaller creates a proxy with a custom RPC caller (useful for testing).
func NewWithCaller(cfg ProxyConfig, logger *slog.Logger, caller RPCCaller) *Proxy {
	return &Proxy{
		cfg:    cfg,
		logger: logger,
		caller: caller,
	}
}

// ── Public RPC methods ────────────────────────────────────────────────

// RegisterAgent registers an agent DID on-chain.
func (p *Proxy) RegisterAgent(ctx context.Context, did string, metadata map[string]string) (*RegisterResult, error) {
	if did == "" {
		return nil, fmt.Errorf("did must not be empty")
	}

	params := []interface{}{did, metadata}
	result, err := p.call(ctx, "clawchain_registerAgent", params)
	if err != nil {
		return nil, fmt.Errorf("register agent: %w", err)
	}

	raw, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal register result: %w", err)
	}

	var reg RegisterResult
	if err := json.Unmarshal(raw, &reg); err != nil {
		return nil, fmt.Errorf("unmarshal register result: %w", err)
	}
	return &reg, nil
}

// GetReputation queries the reputation score for an agent.
func (p *Proxy) GetReputation(ctx context.Context, did string) (*ReputationScore, error) {
	if did == "" {
		return nil, fmt.Errorf("did must not be empty")
	}

	result, err := p.call(ctx, "clawchain_getReputation", []interface{}{did})
	if err != nil {
		return nil, fmt.Errorf("get reputation: %w", err)
	}

	raw, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal reputation: %w", err)
	}

	var rep ReputationScore
	if err := json.Unmarshal(raw, &rep); err != nil {
		return nil, fmt.Errorf("unmarshal reputation: %w", err)
	}
	return &rep, nil
}

// GetBalance queries the token balance for an agent.
func (p *Proxy) GetBalance(ctx context.Context, did string) (*BalanceInfo, error) {
	if did == "" {
		return nil, fmt.Errorf("did must not be empty")
	}

	result, err := p.call(ctx, "clawchain_getBalance", []interface{}{did})
	if err != nil {
		return nil, fmt.Errorf("get balance: %w", err)
	}

	raw, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal balance: %w", err)
	}

	var bal BalanceInfo
	if err := json.Unmarshal(raw, &bal); err != nil {
		return nil, fmt.Errorf("unmarshal balance: %w", err)
	}
	return &bal, nil
}

// Vote casts a governance vote for the given agent.
func (p *Proxy) Vote(ctx context.Context, did string, proposalID uint64, vote string) (*VoteResult, error) {
	if did == "" {
		return nil, fmt.Errorf("did must not be empty")
	}
	if vote != "for" && vote != "against" {
		return nil, fmt.Errorf("vote must be 'for' or 'against', got %q", vote)
	}

	params := []interface{}{did, proposalID, vote}
	result, err := p.call(ctx, "clawchain_vote", params)
	if err != nil {
		return nil, fmt.Errorf("vote: %w", err)
	}

	raw, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal vote result: %w", err)
	}

	var vr VoteResult
	if err := json.Unmarshal(raw, &vr); err != nil {
		return nil, fmt.Errorf("unmarshal vote result: %w", err)
	}
	return &vr, nil
}

// GetAgentInfo retrieves detailed on-chain information about an agent.
func (p *Proxy) GetAgentInfo(ctx context.Context, did string) (*AgentInfo, error) {
	if did == "" {
		return nil, fmt.Errorf("did must not be empty")
	}

	result, err := p.call(ctx, "clawchain_getAgentInfo", []interface{}{did})
	if err != nil {
		return nil, fmt.Errorf("get agent info: %w", err)
	}

	raw, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal agent info: %w", err)
	}

	var info AgentInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return nil, fmt.Errorf("unmarshal agent info: %w", err)
	}
	return &info, nil
}

// ListProposals retrieves governance proposals, filtered by status.
func (p *Proxy) ListProposals(ctx context.Context, status string, limit uint64) ([]ProposalInfo, error) {
	if status == "" {
		status = "active"
	}
	if limit == 0 {
		limit = 10
	}

	params := []interface{}{status, limit}
	result, err := p.call(ctx, "clawchain_listProposals", params)
	if err != nil {
		return nil, fmt.Errorf("list proposals: %w", err)
	}

	raw, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal proposals: %w", err)
	}

	// The node returns {"proposals": [...]}
	var wrapper struct {
		Proposals []ProposalInfo `json:"proposals"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		// Try direct array
		var proposals []ProposalInfo
		if err2 := json.Unmarshal(raw, &proposals); err2 != nil {
			return nil, fmt.Errorf("unmarshal proposals: %w (also tried array: %v)", err, err2)
		}
		return proposals, nil
	}
	return wrapper.Proposals, nil
}

// HandleMQTTRequest processes an MQTT RPC request from an edge agent
// and returns the response to be sent back via MQTT.
func (p *Proxy) HandleMQTTRequest(ctx context.Context, reqData []byte) (*MQTTRPCResponse, error) {
	var req MQTTRPCRequest
	if err := json.Unmarshal(reqData, &req); err != nil {
		return &MQTTRPCResponse{
			Error: fmt.Sprintf("invalid request: %v", err),
		}, nil
	}

	resp := &MQTTRPCResponse{RequestID: req.RequestID}

	result, err := p.call(ctx, req.Method, req.Params)
	if err != nil {
		resp.Error = err.Error()
		return resp, nil
	}

	resp.Result = result
	return resp, nil
}

// OnEvent registers a callback for chain events.
func (p *Proxy) OnEvent(cb EventCallback) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.callbacks = append(p.callbacks, cb)
}

// dispatchEvent sends a chain event to all registered callbacks.
func (p *Proxy) dispatchEvent(event ChainEvent) {
	p.mu.RLock()
	cbs := make([]EventCallback, len(p.callbacks))
	copy(cbs, p.callbacks)
	p.mu.RUnlock()

	for _, cb := range cbs {
		cb(event)
	}
}

// ── Internal RPC plumbing ─────────────────────────────────────────────

func (p *Proxy) call(ctx context.Context, method string, params interface{}) (interface{}, error) {
	id := p.requestID.Add(1)

	req := SubstrateRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	p.logger.Debug("RPC call",
		"method", method,
		"id", id,
	)

	resp, err := p.caller.Call(ctx, p.cfg.NodeURL, req)
	if err != nil {
		p.logger.Error("RPC call failed",
			"method", method,
			"error", err,
		)
		return nil, err
	}

	if resp.Error != nil {
		p.logger.Warn("RPC returned error",
			"method", method,
			"code", resp.Error.Code,
			"message", resp.Error.Message,
		)
		return nil, fmt.Errorf("RPC error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	return resp.Result, nil
}

// ── Default HTTP RPC caller ───────────────────────────────────────────

type httpCaller struct {
	timeout time.Duration
}

func (h *httpCaller) Call(ctx context.Context, url string, req SubstrateRPCRequest) (*SubstrateRPCResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpCtx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(httpCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http call: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", httpResp.StatusCode, string(respBody))
	}

	var rpcResp SubstrateRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &rpcResp, nil
}
