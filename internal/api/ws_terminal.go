package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/clawinfra/evoclaw/internal/security"
	"github.com/clawinfra/evoclaw/internal/types"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// WSRequest is the JSON structure sent by the browser terminal.
type WSRequest struct {
	Type      string `json:"type"`       // "chat", "ping"
	AgentID   string `json:"agent_id"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

// WSResponse is the JSON structure sent back to the browser terminal.
type WSResponse struct {
	Type      string `json:"type"`             // "token", "done", "error", "pong", "system"
	RequestID string `json:"request_id"`
	AgentID   string `json:"agent_id"`
	Content   string `json:"content"`
	Done      bool   `json:"done"`
	Model     string `json:"model,omitempty"`
	Error     string `json:"error,omitempty"`
}

// handleTerminalWS upgrades an HTTP connection to a WebSocket and drives the
// interactive terminal protocol.
//
// Flow:
//  1. Extract ?token= query param.
//  2. Validate JWT (skipped when s.jwtSecret == nil — dev mode).
//  3. Accept the WebSocket upgrade.
//  4. Read loop: wsjson.Read → dispatch by type.
//     - "ping"    → pong immediately.
//     - "chat"    → validate agent → create Message → push to wsChannel inbox →
//     wait for response (30 s) → send WSResponse{done:true}.
//     - unknown  → send error frame.
func (s *Server) handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	// ── 1. JWT authentication ────────────────────────────────────────────────
	if s.jwtSecret != nil {
		tokenStr := r.URL.Query().Get("token")
		if tokenStr == "" {
			http.Error(w, `{"error":"missing token"}`, http.StatusUnauthorized)
			return
		}
		if _, err := security.ValidateToken(tokenStr, s.jwtSecret); err != nil {
			http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
			return
		}
	} else {
		s.logger.Warn("JWT auth disabled (dev mode) — accepting unauthenticated WS terminal")
	}

	// ── 2. Upgrade to WebSocket ───────────────────────────────────────────────
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // accept any Origin for dev convenience
	})
	if err != nil {
		s.logger.Error("websocket accept failed", "error", err)
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "session ended")

	s.logger.Info("ws terminal connected", "remote", r.RemoteAddr)

	// ── 3. Read loop ──────────────────────────────────────────────────────────
	for {
		var req WSRequest
		if err := wsjson.Read(r.Context(), conn, &req); err != nil {
			// Client disconnected or context cancelled — normal exit.
			s.logger.Debug("ws read ended", "error", err)
			return
		}

		switch req.Type {
		case "ping":
			s.wsSendResponse(r.Context(), conn, WSResponse{
				Type:      "pong",
				RequestID: req.RequestID,
			})

		case "chat":
			s.handleWSChat(r.Context(), conn, &req)

		default:
			s.wsSendResponse(r.Context(), conn, WSResponse{
				Type:      "error",
				RequestID: req.RequestID,
				Error:     "unknown message type: " + req.Type,
			})
		}
	}
}

// handleWSChat processes a single chat turn:  validates the target agent,
// enqueues the message in the orchestrator, and streams the response back.
func (s *Server) handleWSChat(ctx context.Context, conn *websocket.Conn, req *WSRequest) {
	// Validate agent exists (if registry is available).
	if s.registry != nil && req.AgentID != "" {
		if _, err := s.registry.Get(req.AgentID); err != nil {
			s.wsSendResponse(ctx, conn, WSResponse{
				Type:      "error",
				RequestID: req.RequestID,
				AgentID:   req.AgentID,
				Error:     "agent not found: " + req.AgentID,
			})
			return
		}
	}

	if req.AgentID == "" {
		s.wsSendResponse(ctx, conn, WSResponse{
			Type:      "error",
			RequestID: req.RequestID,
			Error:     "agent_id is required",
		})
		return
	}

	if req.Message == "" {
		s.wsSendResponse(ctx, conn, WSResponse{
			Type:      "error",
			RequestID: req.RequestID,
			AgentID:   req.AgentID,
			Error:     "message is required",
		})
		return
	}

	// Build orchestrator message.
	msgID := generateMessageID()
	msg := types.Message{
		ID:        msgID,
		Channel:   "websocket",
		From:      "ws-terminal",
		To:        req.AgentID,
		Content:   req.Message,
		Timestamp: time.Now(),
		Metadata: map[string]string{
			"request_id": req.RequestID,
			"source":     "ws-terminal",
		},
	}

	// Register connection so the orchestrator can deliver the response.
	if s.wsChannel == nil {
		s.wsSendResponse(ctx, conn, WSResponse{
			Type:      "error",
			RequestID: req.RequestID,
			Error:     "ws channel not initialised",
		})
		return
	}

	respCh := s.wsChannel.Register(msgID, req.RequestID, conn)
	defer s.wsChannel.Unregister(msgID)

	// Push message into the WS channel inbox; receiveFrom delivers it to the
	// orchestrator's processing loop with Channel="websocket" already set.
	timeout := s.wsTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	chatCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case s.wsChannel.Inbox() <- msg:
	case <-chatCtx.Done():
		s.wsSendResponse(ctx, conn, WSResponse{
			Type:      "error",
			RequestID: req.RequestID,
			AgentID:   req.AgentID,
			Error:     "timeout queuing message to orchestrator",
		})
		return
	}

	// Wait for agent response.
	select {
	case resp, ok := <-respCh:
		if !ok {
			s.wsSendResponse(ctx, conn, WSResponse{
				Type:      "error",
				RequestID: req.RequestID,
				AgentID:   req.AgentID,
				Error:     "response channel closed",
			})
			return
		}
		s.wsSendResponse(ctx, conn, WSResponse{
			Type:      "done",
			RequestID: req.RequestID,
			AgentID:   resp.AgentID,
			Content:   resp.Content,
			Done:      true,
			Model:     resp.Model,
		})

	case <-chatCtx.Done():
		s.wsSendResponse(ctx, conn, WSResponse{
			Type:      "error",
			RequestID: req.RequestID,
			AgentID:   req.AgentID,
			Error:     "response timeout after 30 s",
		})
	}
}

// wsSendResponse marshals and sends a WSResponse frame; errors are logged but not fatal.
func (s *Server) wsSendResponse(ctx context.Context, conn *websocket.Conn, r WSResponse) {
	if err := wsjson.Write(ctx, conn, r); err != nil {
		s.logger.Warn("ws write error", slog.String("error", err.Error()))
	}
}
