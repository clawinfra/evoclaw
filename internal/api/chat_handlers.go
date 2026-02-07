package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/clawinfra/evoclaw/internal/orchestrator"
)

// ChatRequest is the JSON body for POST /api/chat
type ChatRequest struct {
	AgentID        string `json:"agent_id"`
	Message        string `json:"message"`
	ConversationID string `json:"conversation_id,omitempty"`
}

// ChatResponseJSON is the JSON response for POST /api/chat
type ChatResponseJSON struct {
	AgentID      string `json:"agent_id"`
	Response     string `json:"response"`
	Model        string `json:"model"`
	ElapsedMs    int64  `json:"elapsed_ms"`
	TokensInput  int    `json:"tokens_input"`
	TokensOutput int    `json:"tokens_output"`
	Timestamp    string `json:"timestamp"`
}

// ChatHistoryEntry represents a single chat message in history
type ChatHistoryEntry struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp,omitempty"`
}

// handleChat handles POST /api/chat — synchronous chat with an agent
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	if req.AgentID == "" {
		// Use first available agent
		agents := s.registry.List()
		if len(agents) == 0 {
			http.Error(w, "no agents available", http.StatusServiceUnavailable)
			return
		}
		req.AgentID = agents[0].ID
	}

	// Verify agent exists
	if _, err := s.registry.Get(req.AgentID); err != nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	// Build conversation key for memory
	convKey := req.AgentID
	if req.ConversationID != "" {
		convKey = req.AgentID + ":" + req.ConversationID
	}

	// Get conversation history from memory
	mem := s.memory.Get(convKey)
	history := mem.GetRecentMessages(20) // Last 20 messages for context

	// Call orchestrator ChatSync
	chatReq := orchestrator.ChatSyncRequest{
		AgentID:        req.AgentID,
		UserID:         "dashboard",
		Message:        req.Message,
		ConversationID: req.ConversationID,
		History:        history,
	}

	resp, err := s.orch.ChatSync(r.Context(), chatReq)
	if err != nil {
		s.logger.Error("chat sync error", "agent", req.AgentID, "error", err)
		http.Error(w, fmt.Sprintf("chat error: %v", err), http.StatusInternalServerError)
		return
	}

	// Store in memory
	mem.Add("user", req.Message)
	mem.Add("assistant", resp.Response)
	if err := s.memory.Save(convKey); err != nil {
		s.logger.Error("failed to save chat memory", "key", convKey, "error", err)
	}

	// Send response
	s.respondJSON(w, ChatResponseJSON{
		AgentID:      resp.AgentID,
		Response:     resp.Response,
		Model:        resp.Model,
		ElapsedMs:    resp.ElapsedMs,
		TokensInput:  resp.TokensInput,
		TokensOutput: resp.TokensOutput,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	})
}

// handleChatHistory handles GET /api/chat/history?agent_id=...&limit=50
func (s *Server) handleChatHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		http.Error(w, "agent_id is required", http.StatusBadRequest)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = n
		}
	}

	conversationID := r.URL.Query().Get("conversation_id")
	convKey := agentID
	if conversationID != "" {
		convKey = agentID + ":" + conversationID
	}

	mem := s.memory.Get(convKey)
	if mem == nil {
		s.respondJSON(w, map[string]interface{}{
			"agent_id": agentID,
			"messages": []ChatHistoryEntry{},
		})
		return
	}

	messages := mem.GetRecentMessages(limit)
	history := make([]ChatHistoryEntry, len(messages))
	for i, msg := range messages {
		history[i] = ChatHistoryEntry{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	s.respondJSON(w, map[string]interface{}{
		"agent_id":      agentID,
		"message_count": len(history),
		"messages":      history,
	})
}

// handleChatStream handles GET /api/chat/stream — SSE streaming chat
func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agentID := r.URL.Query().Get("agent_id")
	message := r.URL.Query().Get("message")

	if agentID == "" || message == "" {
		http.Error(w, "agent_id and message are required", http.StatusBadRequest)
		return
	}

	// Verify agent exists
	if _, err := s.registry.Get(agentID); err != nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Get conversation history
	mem := s.memory.Get(agentID)
	history := mem.GetRecentMessages(20)

	// Send thinking indicator
	s.sendSSE(w, flusher, map[string]interface{}{
		"type":     "thinking",
		"agent_id": agentID,
	})

	// Call ChatSync (non-streaming for now — Ollama streaming would require different API)
	chatReq := orchestrator.ChatSyncRequest{
		AgentID: agentID,
		UserID:  "dashboard-stream",
		Message: message,
		History: history,
	}

	resp, err := s.orch.ChatSync(r.Context(), chatReq)
	if err != nil {
		s.sendSSE(w, flusher, map[string]interface{}{
			"type":  "error",
			"error": err.Error(),
		})
		return
	}

	// Store in memory
	mem.Add("user", message)
	mem.Add("assistant", resp.Response)
	_ = s.memory.Save(agentID)

	// Send complete response
	s.sendSSE(w, flusher, map[string]interface{}{
		"type":          "response",
		"agent_id":      resp.AgentID,
		"response":      resp.Response,
		"model":         resp.Model,
		"elapsed_ms":    resp.ElapsedMs,
		"tokens_input":  resp.TokensInput,
		"tokens_output": resp.TokensOutput,
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
	})

	// Send done signal
	s.sendSSE(w, flusher, map[string]interface{}{
		"type": "done",
	})
}
