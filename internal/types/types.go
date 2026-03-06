// Package types provides shared types used across EvoClaw packages
// to avoid import cycles between channels and orchestrator.
package types

import "time"

// Button represents an inline keyboard button for Telegram
type Button struct {
	Text         string // Button label displayed to user
	CallbackData string // Data sent back on click (for callback buttons)
	URL          string // Optional: open URL when clicked
}

// Message represents a message from any channel
type Message struct {
	ID        string
	Channel   string // "whatsapp", "telegram", "mqtt"
	From      string
	To        string
	Content   string
	Timestamp time.Time
	ReplyTo   string
	Metadata  map[string]string

	// Telegram-specific fields
	Command  string   // e.g. "start" from /start@botname
	Args     []string // command arguments after the command
	ChatType string   // "private", "group", "supergroup", "channel"
	ThreadID int64    // forum topic thread id (message_thread_id)
}

// Response represents an agent's response
type Response struct {
	AgentID   string
	Content   string
	Channel   string
	To        string
	ReplyTo   string
	MessageID string
	Model     string
	Metadata  map[string]string

	// Telegram-specific fields
	Buttons       [][]Button // inline keyboard rows (nil = no keyboard)
	EditMessageID int64      // if >0, edit this existing message instead of sending
	ReplyToID     int64      // if >0, send as a reply to this message ID
}

// ToolResult represents the result of a tool execution from an edge agent
type ToolResult struct {
	Tool      string `json:"tool"`
	Status    string `json:"status"` // "success", "error", "timeout"
	Result    string `json:"result"`
	Error     string `json:"error,omitempty"`
	ErrorType string `json:"error_type,omitempty"`
	ExitCode  int    `json:"exit_code,omitempty"`
	ElapsedMs int64  `json:"elapsed_ms"`
}

// AgentMetrics tracks performance metrics for agents
type AgentMetrics struct {
	TotalActions      int64
	SuccessfulActions int64
	FailedActions     int64
	TokensUsed        int64
	AvgResponseMs     float64
	CostUSD           float64
	Custom            map[string]float64
}

// AgentInfo is a minimal agent info struct for TUI/display purposes
// to avoid import cycles. The orchestrator's full AgentInfo has more fields.
type AgentInfo struct {
	ID           string
	Name         string
	Model        string
	Status       string
	StartedAt    time.Time
	LastActive   time.Time
	MessageCount int64
	ErrorCount   int64
	Metrics      AgentMetrics
}

// BotChatSyncRequest is used by channels.TelegramBot to call orchestrator without import cycles.
type BotChatSyncRequest struct {
	AgentID        string
	UserID         string
	Message        string
	ConversationID string
}

// BotChatSyncResponse is the response from a bot chat sync call.
type BotChatSyncResponse struct {
	AgentID   string
	Response  string
	Model     string
	ElapsedMs int64
}
