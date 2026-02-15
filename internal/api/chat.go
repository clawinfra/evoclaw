package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/clawinfra/evoclaw/internal/orchestrator"
)

// ChatRequest represents an incoming chat message
type ChatRequest struct {
	Agent   string `json:"agent"`   // Agent ID to send message to
	Message string `json:"message"` // User message
	From    string `json:"from"`    // Optional sender ID
}

// ChatResponse represents the agent's reply
type ChatResponse struct {
	Agent     string `json:"agent"`
	Message   string `json:"message"`
	Model     string `json:"model"`
	Timestamp string `json:"timestamp"`
}

// handleChat processes chat messages via HTTP API
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate input
	if req.Agent == "" {
		http.Error(w, "agent field is required", http.StatusBadRequest)
		return
	}
	if req.Message == "" {
		http.Error(w, "message field is required", http.StatusBadRequest)
		return
	}

	// Default sender if not provided
	if req.From == "" {
		req.From = "http-api"
	}

	// Create message for orchestrator
	msg := orchestrator.Message{
		ID:        generateMessageID(),
		Channel:   "http",
		From:      req.From,
		To:        req.Agent,
		Content:   req.Message,
		Timestamp: time.Now(),
		Metadata: map[string]string{
			"source": "http-api",
		},
	}

	// Send to orchestrator inbox
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Get the orchestrator's inbox channel
	inbox := s.orch.GetInbox()
	if inbox == nil {
		http.Error(w, "Orchestrator not ready", http.StatusServiceUnavailable)
		return
	}

	select {
	case inbox <- msg:
		// Message queued successfully
	case <-ctx.Done():
		http.Error(w, "Request timeout", http.StatusRequestTimeout)
		return
	}

	// Wait for the agent's response
	agentResp, err := s.httpChannel.WaitForResponse(ctx, msg.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get response: %v", err), http.StatusGatewayTimeout)
		return
	}

	resp := ChatResponse{
		Agent:     agentResp.AgentID,
		Message:   agentResp.Content,
		Model:     agentResp.Model,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleChatStream processes chat with SSE streaming response
func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate input
	if req.Agent == "" || req.Message == "" {
		http.Error(w, "agent and message fields are required", http.StatusBadRequest)
		return
	}

	// For now, send a simple response
	// TODO: Implement actual streaming from orchestrator
	data, _ := json.Marshal(map[string]string{
		"type":    "response",
		"content": "Streaming support coming soon",
		"agent":   req.Agent,
	})

	_, _ = w.Write([]byte("data: " + string(data) + "\n\n"))
}

// generateMessageID creates a unique message ID
func generateMessageID() string {
	return "msg_" + strings.ReplaceAll(time.Now().Format("20060102150405.000000"), ".", "_")
}
