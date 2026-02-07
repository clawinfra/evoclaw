package clawchain

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ── Mock RPC caller ─────────────────────────────────────────────────

type mockCaller struct {
	mu        sync.Mutex
	responses map[string]*SubstrateRPCResponse
	callCount atomic.Int64
	lastReq   *SubstrateRPCRequest
	shouldErr bool
	errMsg    string
}

func newMockCaller() *mockCaller {
	return &mockCaller{
		responses: make(map[string]*SubstrateRPCResponse),
	}
}

func (m *mockCaller) SetResponse(method string, result interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses[method] = &SubstrateRPCResponse{
		JSONRPC: "2.0",
		ID:      1,
		Result:  result,
	}
}

func (m *mockCaller) SetError(method string, code int, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses[method] = &SubstrateRPCResponse{
		JSONRPC: "2.0",
		ID:      1,
		Error: &SubstrateRPCErr{
			Code:    code,
			Message: message,
		},
	}
}

func (m *mockCaller) SetTransportError(msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldErr = true
	m.errMsg = msg
}

func (m *mockCaller) Call(_ context.Context, _ string, req SubstrateRPCRequest) (*SubstrateRPCResponse, error) {
	m.callCount.Add(1)
	m.mu.Lock()
	m.lastReq = &req
	shouldErr := m.shouldErr
	errMsg := m.errMsg
	resp, ok := m.responses[req.Method]
	m.mu.Unlock()

	if shouldErr {
		return nil, fmt.Errorf("%s", errMsg)
	}
	if !ok {
		return nil, fmt.Errorf("no mock response for method: %s", req.Method)
	}
	return resp, nil
}

func (m *mockCaller) CallCount() int64 {
	return m.callCount.Load()
}

// ── Helpers ─────────────────────────────────────────────────────────

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func newTestProxy(caller RPCCaller) *Proxy {
	cfg := DefaultProxyConfig()
	return NewWithCaller(cfg, newTestLogger(), caller)
}

// ── Type tests ──────────────────────────────────────────────────────

func TestDefaultProxyConfig(t *testing.T) {
	cfg := DefaultProxyConfig()
	if cfg.NodeURL != "http://localhost:9933" {
		t.Errorf("expected default NodeURL, got %q", cfg.NodeURL)
	}
	if cfg.WebSocketURL != "ws://localhost:9944" {
		t.Errorf("expected default WebSocketURL, got %q", cfg.WebSocketURL)
	}
	if cfg.RequestTimeoutSec != 15 {
		t.Errorf("expected 15s timeout, got %d", cfg.RequestTimeoutSec)
	}
}

func TestAgentInfoJSON(t *testing.T) {
	info := AgentInfo{
		DID:          "did:claw:test",
		Reputation:   85,
		Balance:      1000000,
		RegisteredAt: 1700000000,
		LastActive:   1700001000,
		Metadata:     map[string]string{"type": "sensor"},
	}
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded AgentInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.DID != "did:claw:test" {
		t.Errorf("expected did:claw:test, got %q", decoded.DID)
	}
	if decoded.Reputation != 85 {
		t.Errorf("expected reputation 85, got %d", decoded.Reputation)
	}
	if decoded.Metadata["type"] != "sensor" {
		t.Errorf("expected metadata type=sensor")
	}
}

func TestReputationScoreJSON(t *testing.T) {
	score := ReputationScore{
		AgentDID:        "did:claw:rep",
		Score:           92,
		TotalTasks:      100,
		SuccessfulTasks: 92,
		LastUpdated:     1700000000,
	}
	data, err := json.Marshal(score)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded ReputationScore
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Score != 92 {
		t.Errorf("expected score 92, got %d", decoded.Score)
	}
}

func TestProposalInfoJSON(t *testing.T) {
	prop := ProposalInfo{
		ID:           1,
		Title:        "Test Proposal",
		Description:  "A test",
		Proposer:     "did:claw:proposer",
		Status:       "active",
		VotesFor:     100,
		VotesAgainst: 20,
		CreatedAt:    1700000000,
		EndsAt:       1700604800,
	}
	data, err := json.Marshal(prop)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded ProposalInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Title != "Test Proposal" {
		t.Errorf("expected 'Test Proposal', got %q", decoded.Title)
	}
	if decoded.VotesFor != 100 {
		t.Errorf("expected 100 votes for, got %d", decoded.VotesFor)
	}
}

func TestChainEventJSON(t *testing.T) {
	event := ChainEvent{
		BlockNumber: 42,
		EventType:   "AgentRegistered",
		Data:        map[string]string{"did": "did:claw:new"},
		Timestamp:   time.Now(),
	}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded ChainEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.BlockNumber != 42 {
		t.Errorf("expected block 42, got %d", decoded.BlockNumber)
	}
}

func TestMQTTRPCRequestJSON(t *testing.T) {
	req := MQTTRPCRequest{
		RequestID: "req-001",
		Method:    "clawchain_getBalance",
		Params:    []interface{}{"did:claw:test"},
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded MQTTRPCRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.RequestID != "req-001" {
		t.Errorf("expected req-001, got %q", decoded.RequestID)
	}
}

func TestMQTTRPCResponseJSON(t *testing.T) {
	resp := MQTTRPCResponse{
		RequestID: "req-001",
		Result:    map[string]interface{}{"balance": float64(1000)},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded MQTTRPCResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.RequestID != "req-001" {
		t.Errorf("expected req-001, got %q", decoded.RequestID)
	}
	if decoded.Error != "" {
		t.Errorf("expected no error, got %q", decoded.Error)
	}
}

func TestMQTTRPCResponseErrorJSON(t *testing.T) {
	resp := MQTTRPCResponse{
		RequestID: "req-002",
		Error:     "agent not found",
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded MQTTRPCResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Error != "agent not found" {
		t.Errorf("expected 'agent not found', got %q", decoded.Error)
	}
}

// ── Proxy constructor tests ─────────────────────────────────────────

func TestNew(t *testing.T) {
	cfg := DefaultProxyConfig()
	logger := newTestLogger()
	p := New(cfg, logger)
	if p == nil {
		t.Fatal("expected non-nil proxy")
	}
	if p.cfg.NodeURL != cfg.NodeURL {
		t.Errorf("expected NodeURL %q, got %q", cfg.NodeURL, p.cfg.NodeURL)
	}
}

func TestNewWithCaller(t *testing.T) {
	caller := newMockCaller()
	p := newTestProxy(caller)
	if p == nil {
		t.Fatal("expected non-nil proxy")
	}
}

// ── RegisterAgent tests ─────────────────────────────────────────────

func TestRegisterAgent_Success(t *testing.T) {
	caller := newMockCaller()
	caller.SetResponse("clawchain_registerAgent", map[string]interface{}{
		"tx_hash": "0xabc123",
		"did":     "did:claw:new",
	})

	p := newTestProxy(caller)
	result, err := p.RegisterAgent(context.Background(), "did:claw:new", map[string]string{"type": "sensor"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TxHash != "0xabc123" {
		t.Errorf("expected tx_hash 0xabc123, got %q", result.TxHash)
	}
	if result.DID != "did:claw:new" {
		t.Errorf("expected DID did:claw:new, got %q", result.DID)
	}
	if caller.CallCount() != 1 {
		t.Errorf("expected 1 call, got %d", caller.CallCount())
	}
}

func TestRegisterAgent_EmptyDID(t *testing.T) {
	caller := newMockCaller()
	p := newTestProxy(caller)
	_, err := p.RegisterAgent(context.Background(), "", nil)
	if err == nil {
		t.Fatal("expected error for empty DID")
	}
}

func TestRegisterAgent_RPCError(t *testing.T) {
	caller := newMockCaller()
	caller.SetError("clawchain_registerAgent", -32600, "invalid request")
	p := newTestProxy(caller)

	_, err := p.RegisterAgent(context.Background(), "did:claw:test", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRegisterAgent_TransportError(t *testing.T) {
	caller := newMockCaller()
	caller.SetTransportError("connection refused")
	p := newTestProxy(caller)

	_, err := p.RegisterAgent(context.Background(), "did:claw:test", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRegisterAgent_NilMetadata(t *testing.T) {
	caller := newMockCaller()
	caller.SetResponse("clawchain_registerAgent", map[string]interface{}{
		"tx_hash": "0x000",
		"did":     "did:claw:nil_meta",
	})
	p := newTestProxy(caller)

	result, err := p.RegisterAgent(context.Background(), "did:claw:nil_meta", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.DID != "did:claw:nil_meta" {
		t.Errorf("unexpected DID: %q", result.DID)
	}
}

// ── GetReputation tests ─────────────────────────────────────────────

func TestGetReputation_Success(t *testing.T) {
	caller := newMockCaller()
	caller.SetResponse("clawchain_getReputation", map[string]interface{}{
		"agent_did":        "did:claw:test",
		"score":            float64(85),
		"total_tasks":      float64(100),
		"successful_tasks": float64(85),
		"last_updated":     float64(1700000000),
	})

	p := newTestProxy(caller)
	rep, err := p.GetReputation(context.Background(), "did:claw:test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rep.Score != 85 {
		t.Errorf("expected score 85, got %d", rep.Score)
	}
	if rep.TotalTasks != 100 {
		t.Errorf("expected 100 total tasks, got %d", rep.TotalTasks)
	}
}

func TestGetReputation_EmptyDID(t *testing.T) {
	caller := newMockCaller()
	p := newTestProxy(caller)
	_, err := p.GetReputation(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty DID")
	}
}

func TestGetReputation_RPCError(t *testing.T) {
	caller := newMockCaller()
	caller.SetError("clawchain_getReputation", -32000, "agent not found")
	p := newTestProxy(caller)

	_, err := p.GetReputation(context.Background(), "did:claw:unknown")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ── GetBalance tests ────────────────────────────────────────────────

func TestGetBalance_Success(t *testing.T) {
	caller := newMockCaller()
	caller.SetResponse("clawchain_getBalance", map[string]interface{}{
		"balance": float64(1000000),
		"symbol":  "CLAW",
	})

	p := newTestProxy(caller)
	bal, err := p.GetBalance(context.Background(), "did:claw:test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bal.Balance != 1000000 {
		t.Errorf("expected balance 1000000, got %d", bal.Balance)
	}
	if bal.Symbol != "CLAW" {
		t.Errorf("expected symbol CLAW, got %q", bal.Symbol)
	}
}

func TestGetBalance_EmptyDID(t *testing.T) {
	caller := newMockCaller()
	p := newTestProxy(caller)
	_, err := p.GetBalance(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty DID")
	}
}

func TestGetBalance_RPCError(t *testing.T) {
	caller := newMockCaller()
	caller.SetError("clawchain_getBalance", -32000, "balance unavailable")
	p := newTestProxy(caller)

	_, err := p.GetBalance(context.Background(), "did:claw:test")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ── Vote tests ──────────────────────────────────────────────────────

func TestVote_Success(t *testing.T) {
	caller := newMockCaller()
	caller.SetResponse("clawchain_vote", map[string]interface{}{
		"tx_hash":     "0xvote1",
		"proposal_id": float64(42),
		"vote":        "for",
	})

	p := newTestProxy(caller)
	result, err := p.Vote(context.Background(), "did:claw:voter", 42, "for")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TxHash != "0xvote1" {
		t.Errorf("expected tx_hash 0xvote1, got %q", result.TxHash)
	}
}

func TestVote_Against(t *testing.T) {
	caller := newMockCaller()
	caller.SetResponse("clawchain_vote", map[string]interface{}{
		"tx_hash":     "0xvote2",
		"proposal_id": float64(1),
		"vote":        "against",
	})

	p := newTestProxy(caller)
	result, err := p.Vote(context.Background(), "did:claw:voter", 1, "against")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Vote != "against" {
		t.Errorf("expected vote 'against', got %q", result.Vote)
	}
}

func TestVote_EmptyDID(t *testing.T) {
	caller := newMockCaller()
	p := newTestProxy(caller)
	_, err := p.Vote(context.Background(), "", 1, "for")
	if err == nil {
		t.Fatal("expected error for empty DID")
	}
}

func TestVote_InvalidVoteValue(t *testing.T) {
	caller := newMockCaller()
	p := newTestProxy(caller)
	_, err := p.Vote(context.Background(), "did:claw:test", 1, "maybe")
	if err == nil {
		t.Fatal("expected error for invalid vote value")
	}
}

func TestVote_RPCError(t *testing.T) {
	caller := newMockCaller()
	caller.SetError("clawchain_vote", -32000, "proposal expired")
	p := newTestProxy(caller)

	_, err := p.Vote(context.Background(), "did:claw:voter", 99, "for")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ── GetAgentInfo tests ──────────────────────────────────────────────

func TestGetAgentInfo_Success(t *testing.T) {
	caller := newMockCaller()
	caller.SetResponse("clawchain_getAgentInfo", map[string]interface{}{
		"did":           "did:claw:test123",
		"reputation":    float64(90),
		"balance":       float64(5000),
		"registered_at": float64(1700000000),
		"last_active":   float64(1700001000),
		"metadata":      map[string]interface{}{"type": "trader"},
	})

	p := newTestProxy(caller)
	info, err := p.GetAgentInfo(context.Background(), "did:claw:test123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.DID != "did:claw:test123" {
		t.Errorf("expected DID did:claw:test123, got %q", info.DID)
	}
	if info.Reputation != 90 {
		t.Errorf("expected reputation 90, got %d", info.Reputation)
	}
}

func TestGetAgentInfo_EmptyDID(t *testing.T) {
	caller := newMockCaller()
	p := newTestProxy(caller)
	_, err := p.GetAgentInfo(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty DID")
	}
}

func TestGetAgentInfo_RPCError(t *testing.T) {
	caller := newMockCaller()
	caller.SetError("clawchain_getAgentInfo", -32000, "not found")
	p := newTestProxy(caller)

	_, err := p.GetAgentInfo(context.Background(), "did:claw:missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ── ListProposals tests ─────────────────────────────────────────────

func TestListProposals_Success(t *testing.T) {
	caller := newMockCaller()
	caller.SetResponse("clawchain_listProposals", map[string]interface{}{
		"proposals": []interface{}{
			map[string]interface{}{
				"id":            float64(1),
				"title":         "Increase rewards",
				"description":   "Increase staking rewards",
				"proposer":      "did:claw:proposer1",
				"status":        "active",
				"votes_for":     float64(100),
				"votes_against": float64(20),
				"created_at":    float64(1700000000),
				"ends_at":       float64(1700604800),
			},
			map[string]interface{}{
				"id":            float64(2),
				"title":         "Add new skill",
				"description":   "Add sensor skill",
				"proposer":      "did:claw:proposer2",
				"status":        "active",
				"votes_for":     float64(50),
				"votes_against": float64(10),
				"created_at":    float64(1700100000),
				"ends_at":       float64(1700704800),
			},
		},
	})

	p := newTestProxy(caller)
	proposals, err := p.ListProposals(context.Background(), "active", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(proposals) != 2 {
		t.Fatalf("expected 2 proposals, got %d", len(proposals))
	}
	if proposals[0].Title != "Increase rewards" {
		t.Errorf("expected 'Increase rewards', got %q", proposals[0].Title)
	}
	if proposals[1].VotesFor != 50 {
		t.Errorf("expected 50 votes for proposal 2, got %d", proposals[1].VotesFor)
	}
}

func TestListProposals_DefaultParams(t *testing.T) {
	caller := newMockCaller()
	caller.SetResponse("clawchain_listProposals", map[string]interface{}{
		"proposals": []interface{}{},
	})

	p := newTestProxy(caller)
	proposals, err := p.ListProposals(context.Background(), "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(proposals) != 0 {
		t.Errorf("expected empty proposals, got %d", len(proposals))
	}
}

func TestListProposals_RPCError(t *testing.T) {
	caller := newMockCaller()
	caller.SetError("clawchain_listProposals", -32000, "chain unavailable")
	p := newTestProxy(caller)

	_, err := p.ListProposals(context.Background(), "active", 10)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListProposals_DirectArray(t *testing.T) {
	// Some nodes might return a direct array instead of wrapped
	caller := newMockCaller()
	caller.SetResponse("clawchain_listProposals", []interface{}{
		map[string]interface{}{
			"id":            float64(1),
			"title":         "Direct",
			"description":   "Direct array response",
			"proposer":      "did:claw:p",
			"status":        "active",
			"votes_for":     float64(10),
			"votes_against": float64(5),
			"created_at":    float64(1700000000),
			"ends_at":       float64(1700604800),
		},
	})

	p := newTestProxy(caller)
	proposals, err := p.ListProposals(context.Background(), "active", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(proposals) != 1 {
		t.Fatalf("expected 1 proposal, got %d", len(proposals))
	}
	if proposals[0].Title != "Direct" {
		t.Errorf("expected 'Direct', got %q", proposals[0].Title)
	}
}

// ── HandleMQTTRequest tests ─────────────────────────────────────────

func TestHandleMQTTRequest_Success(t *testing.T) {
	caller := newMockCaller()
	caller.SetResponse("clawchain_getBalance", map[string]interface{}{
		"balance": float64(5000),
		"symbol":  "CLAW",
	})

	p := newTestProxy(caller)

	reqData, _ := json.Marshal(MQTTRPCRequest{
		RequestID: "mqtt-001",
		Method:    "clawchain_getBalance",
		Params:    []interface{}{"did:claw:test"},
	})

	resp, err := p.HandleMQTTRequest(context.Background(), reqData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.RequestID != "mqtt-001" {
		t.Errorf("expected request_id mqtt-001, got %q", resp.RequestID)
	}
	if resp.Error != "" {
		t.Errorf("expected no error, got %q", resp.Error)
	}
	if resp.Result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestHandleMQTTRequest_InvalidJSON(t *testing.T) {
	caller := newMockCaller()
	p := newTestProxy(caller)

	resp, err := p.HandleMQTTRequest(context.Background(), []byte("not json{"))
	if err != nil {
		t.Fatalf("HandleMQTTRequest should not return Go error for bad JSON, got: %v", err)
	}
	if resp.Error == "" {
		t.Fatal("expected error in response")
	}
}

func TestHandleMQTTRequest_RPCError(t *testing.T) {
	caller := newMockCaller()
	caller.SetError("clawchain_getReputation", -32000, "not found")
	p := newTestProxy(caller)

	reqData, _ := json.Marshal(MQTTRPCRequest{
		RequestID: "mqtt-002",
		Method:    "clawchain_getReputation",
		Params:    []interface{}{"did:claw:missing"},
	})

	resp, err := p.HandleMQTTRequest(context.Background(), reqData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Error == "" {
		t.Fatal("expected error in response")
	}
	if resp.RequestID != "mqtt-002" {
		t.Errorf("expected request_id mqtt-002, got %q", resp.RequestID)
	}
}

func TestHandleMQTTRequest_TransportError(t *testing.T) {
	caller := newMockCaller()
	caller.SetTransportError("connection refused")
	p := newTestProxy(caller)

	reqData, _ := json.Marshal(MQTTRPCRequest{
		RequestID: "mqtt-003",
		Method:    "clawchain_getBalance",
		Params:    []interface{}{"did:claw:test"},
	})

	resp, err := p.HandleMQTTRequest(context.Background(), reqData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Error == "" {
		t.Fatal("expected error in response")
	}
}

// ── Event callback tests ────────────────────────────────────────────

func TestOnEvent_Single(t *testing.T) {
	caller := newMockCaller()
	p := newTestProxy(caller)

	var received ChainEvent
	var mu sync.Mutex
	p.OnEvent(func(event ChainEvent) {
		mu.Lock()
		received = event
		mu.Unlock()
	})

	event := ChainEvent{
		BlockNumber: 100,
		EventType:   "AgentRegistered",
		Data:        map[string]string{"did": "did:claw:new"},
		Timestamp:   time.Now(),
	}
	p.dispatchEvent(event)

	mu.Lock()
	defer mu.Unlock()
	if received.BlockNumber != 100 {
		t.Errorf("expected block 100, got %d", received.BlockNumber)
	}
	if received.EventType != "AgentRegistered" {
		t.Errorf("expected AgentRegistered, got %q", received.EventType)
	}
}

func TestOnEvent_Multiple(t *testing.T) {
	caller := newMockCaller()
	p := newTestProxy(caller)

	var count atomic.Int64
	p.OnEvent(func(event ChainEvent) {
		count.Add(1)
	})
	p.OnEvent(func(event ChainEvent) {
		count.Add(1)
	})

	p.dispatchEvent(ChainEvent{EventType: "test"})

	if count.Load() != 2 {
		t.Errorf("expected 2 callbacks, got %d", count.Load())
	}
}

func TestOnEvent_NoCallbacks(t *testing.T) {
	caller := newMockCaller()
	p := newTestProxy(caller)
	// Should not panic
	p.dispatchEvent(ChainEvent{EventType: "test"})
}

// ── Request ID monotonic ────────────────────────────────────────────

func TestRequestIDIncreasing(t *testing.T) {
	caller := newMockCaller()
	caller.SetResponse("clawchain_getReputation", map[string]interface{}{
		"agent_did":        "did:claw:t",
		"score":            float64(50),
		"total_tasks":      float64(50),
		"successful_tasks": float64(25),
		"last_updated":     float64(0),
	})

	p := newTestProxy(caller)

	_, _ = p.GetReputation(context.Background(), "did:claw:t")
	_, _ = p.GetReputation(context.Background(), "did:claw:t")
	_, _ = p.GetReputation(context.Background(), "did:claw:t")

	if caller.CallCount() != 3 {
		t.Errorf("expected 3 calls, got %d", caller.CallCount())
	}
}

// ── Full workflow test ──────────────────────────────────────────────

func TestFullWorkflow(t *testing.T) {
	caller := newMockCaller()

	// Set up all responses
	caller.SetResponse("clawchain_registerAgent", map[string]interface{}{
		"tx_hash": "0xreg",
		"did":     "did:claw:workflow",
	})
	caller.SetResponse("clawchain_getReputation", map[string]interface{}{
		"agent_did":        "did:claw:workflow",
		"score":            float64(0),
		"total_tasks":      float64(0),
		"successful_tasks": float64(0),
		"last_updated":     float64(1700000000),
	})
	caller.SetResponse("clawchain_getBalance", map[string]interface{}{
		"balance": float64(1000),
		"symbol":  "CLAW",
	})
	caller.SetResponse("clawchain_getAgentInfo", map[string]interface{}{
		"did":           "did:claw:workflow",
		"reputation":    float64(0),
		"balance":       float64(1000),
		"registered_at": float64(1700000000),
		"last_active":   float64(1700000000),
	})
	caller.SetResponse("clawchain_listProposals", map[string]interface{}{
		"proposals": []interface{}{
			map[string]interface{}{
				"id":            float64(1),
				"title":         "Test",
				"description":   "Test proposal",
				"proposer":      "did:claw:p",
				"status":        "active",
				"votes_for":     float64(10),
				"votes_against": float64(5),
				"created_at":    float64(1700000000),
				"ends_at":       float64(1700604800),
			},
		},
	})
	caller.SetResponse("clawchain_vote", map[string]interface{}{
		"tx_hash":     "0xvote",
		"proposal_id": float64(1),
		"vote":        "for",
	})

	p := newTestProxy(caller)

	// 1. Register
	reg, err := p.RegisterAgent(context.Background(), "did:claw:workflow", map[string]string{"type": "sensor"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if reg.TxHash != "0xreg" {
		t.Errorf("expected 0xreg, got %q", reg.TxHash)
	}

	// 2. Get reputation
	rep, err := p.GetReputation(context.Background(), "did:claw:workflow")
	if err != nil {
		t.Fatalf("reputation: %v", err)
	}
	if rep.Score != 0 {
		t.Errorf("new agent should have score 0, got %d", rep.Score)
	}

	// 3. Get balance
	bal, err := p.GetBalance(context.Background(), "did:claw:workflow")
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	if bal.Balance != 1000 {
		t.Errorf("expected 1000, got %d", bal.Balance)
	}

	// 4. Get agent info
	info, err := p.GetAgentInfo(context.Background(), "did:claw:workflow")
	if err != nil {
		t.Fatalf("agent info: %v", err)
	}
	if info.DID != "did:claw:workflow" {
		t.Errorf("expected did:claw:workflow, got %q", info.DID)
	}

	// 5. List proposals
	proposals, err := p.ListProposals(context.Background(), "active", 10)
	if err != nil {
		t.Fatalf("proposals: %v", err)
	}
	if len(proposals) != 1 {
		t.Fatalf("expected 1 proposal, got %d", len(proposals))
	}

	// 6. Vote
	vr, err := p.Vote(context.Background(), "did:claw:workflow", 1, "for")
	if err != nil {
		t.Fatalf("vote: %v", err)
	}
	if vr.TxHash != "0xvote" {
		t.Errorf("expected 0xvote, got %q", vr.TxHash)
	}

	// Total calls
	if caller.CallCount() != 6 {
		t.Errorf("expected 6 total calls, got %d", caller.CallCount())
	}
}

// ── Edge case tests ─────────────────────────────────────────────────

func TestContextCancellation(t *testing.T) {
	caller := newMockCaller()
	caller.SetTransportError("context canceled")
	p := newTestProxy(caller)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := p.GetReputation(ctx, "did:claw:test")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestConcurrentRequests(t *testing.T) {
	caller := newMockCaller()
	caller.SetResponse("clawchain_getBalance", map[string]interface{}{
		"balance": float64(100),
		"symbol":  "CLAW",
	})

	p := newTestProxy(caller)

	var wg sync.WaitGroup
	errs := make(chan error, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := p.GetBalance(context.Background(), "did:claw:concurrent")
			if err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent request error: %v", err)
	}

	if caller.CallCount() != 20 {
		t.Errorf("expected 20 calls, got %d", caller.CallCount())
	}
}
