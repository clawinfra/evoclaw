package clawchain

import "time"

// AgentInfo represents on-chain agent information.
type AgentInfo struct {
	DID          string            `json:"did"`
	Reputation   uint64            `json:"reputation"`
	Balance      uint64            `json:"balance"`
	RegisteredAt uint64            `json:"registered_at"`
	LastActive   uint64            `json:"last_active"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// ReputationScore holds reputation details for an agent.
type ReputationScore struct {
	AgentDID        string `json:"agent_did"`
	Score           uint64 `json:"score"`
	TotalTasks      uint64 `json:"total_tasks"`
	SuccessfulTasks uint64 `json:"successful_tasks"`
	LastUpdated     uint64 `json:"last_updated"`
}

// ProposalInfo represents a governance proposal on ClawChain.
type ProposalInfo struct {
	ID           uint64 `json:"id"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	Proposer     string `json:"proposer"`
	Status       string `json:"status"`
	VotesFor     uint64 `json:"votes_for"`
	VotesAgainst uint64 `json:"votes_against"`
	CreatedAt    uint64 `json:"created_at"`
	EndsAt       uint64 `json:"ends_at"`
}

// VoteResult is returned after a successful vote submission.
type VoteResult struct {
	TxHash     string `json:"tx_hash"`
	ProposalID uint64 `json:"proposal_id"`
	Vote       string `json:"vote"`
}

// RegisterResult is returned after a successful agent registration.
type RegisterResult struct {
	TxHash string `json:"tx_hash"`
	DID    string `json:"did"`
}

// BalanceInfo holds token balance details.
type BalanceInfo struct {
	Balance uint64 `json:"balance"`
	Symbol  string `json:"symbol"`
}

// SubstrateRPCRequest is the standard JSON-RPC 2.0 request envelope.
type SubstrateRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      uint64      `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

// SubstrateRPCResponse is the standard JSON-RPC 2.0 response envelope.
type SubstrateRPCResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      uint64           `json:"id"`
	Result  interface{}      `json:"result,omitempty"`
	Error   *SubstrateRPCErr `json:"error,omitempty"`
}

// SubstrateRPCErr describes an RPC-level error.
type SubstrateRPCErr struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// MQTTRPCRequest is the envelope edge agents send via MQTT.
type MQTTRPCRequest struct {
	RequestID string      `json:"request_id"`
	Method    string      `json:"method"`
	Params    interface{} `json:"params"`
}

// MQTTRPCResponse is the envelope the proxy sends back to edge agents.
type MQTTRPCResponse struct {
	RequestID string      `json:"request_id"`
	Result    interface{} `json:"result,omitempty"`
	Error     string      `json:"error,omitempty"`
}

// ChainEvent represents an event from the ClawChain blockchain.
type ChainEvent struct {
	BlockNumber uint64      `json:"block_number"`
	EventType   string      `json:"event_type"`
	Data        interface{} `json:"data"`
	Timestamp   time.Time   `json:"timestamp"`
}

// ProxyConfig holds configuration for the ClawChain RPC proxy.
type ProxyConfig struct {
	// NodeURL is the HTTP RPC endpoint of the ClawChain Substrate node.
	NodeURL string `json:"node_url"`
	// WebSocketURL is the WebSocket endpoint for event subscriptions.
	WebSocketURL string `json:"websocket_url"`
	// RequestTimeoutSec is the timeout for individual RPC calls.
	RequestTimeoutSec int `json:"request_timeout_sec"`
}

// DefaultProxyConfig returns sensible defaults for local development.
func DefaultProxyConfig() ProxyConfig {
	return ProxyConfig{
		NodeURL:           "http://localhost:9933",
		WebSocketURL:      "ws://localhost:9944",
		RequestTimeoutSec: 15,
	}
}
