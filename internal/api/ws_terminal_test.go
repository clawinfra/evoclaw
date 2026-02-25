package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/clawinfra/evoclaw/internal/agents"
	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/models"
	"github.com/clawinfra/evoclaw/internal/orchestrator"
	"github.com/clawinfra/evoclaw/internal/security"
	"github.com/clawinfra/evoclaw/internal/types"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// ─── helpers ────────────────────────────────────────────────────────────────

func wsTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

// newWSTestServer creates a Server wired to a real Orchestrator and a WSChannel
// and starts a goroutine that pretends to be the agent: it reads the first message
// from the orchestrator inbox and delivers a canned response via wsChannel.Send.
// If respond is false the goroutine is not started (for timeout tests).
func newWSTestServer(t *testing.T, respond bool) (*Server, *httptest.Server, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	logger := wsTestLogger()

	registry, _ := agents.NewRegistry(tmpDir, logger)
	memory, _ := agents.NewMemoryStore(tmpDir, logger)
	router := models.NewRouter(logger)

	orch := orchestrator.New(&config.Config{}, logger)

	srv := NewServer(8421, orch, registry, memory, router, logger)
	srv.wsTimeout = 200 * time.Millisecond // short timeout for tests

	// Start a fake agent loop so responses are delivered promptly.
	// Reads from wsChannel.Inbox() — the same channel the WS handler pushes to.
	ctx, cancel := context.WithCancel(context.Background())
	if respond {
		go func() {
			inbox := srv.wsChannel.Inbox()
			for {
				select {
				case msg, ok := <-inbox:
					if !ok {
						return
					}
					resp := types.Response{
						AgentID:   msg.To,
						Content:   "echo: " + msg.Content,
						Channel:   "websocket",
						MessageID: msg.ID,
						Model:     "test-model",
					}
					_ = srv.wsChannel.Send(context.Background(), resp)
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	ts := httptest.NewServer(srv.wsTerminalHandler())
	cleanup := func() {
		cancel()
		ts.Close()
	}
	return srv, ts, cleanup
}

// wsTerminalHandler returns an http.Handler for the terminal WS endpoint,
// bypassing the full mux for targeted testing.
func (s *Server) wsTerminalHandler() *wsHandlerFunc {
	return &wsHandlerFunc{s: s}
}

type wsHandlerFunc struct{ s *Server }

func (h *wsHandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.s.handleTerminalWS(w, r)
}

// dialWS connects to the test server WS endpoint with optional token.
func dialWS(t *testing.T, ts *httptest.Server, token string) (*websocket.Conn, context.CancelFunc, error) {
	t.Helper()
	url := "ws" + strings.TrimPrefix(ts.URL, "http")
	if token != "" {
		url += "?token=" + token
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	return conn, cancel, nil
}

// validToken generates a short-lived JWT using the given secret.
func validToken(secret []byte) string {
	tok, _ := security.GenerateToken("user-1", "owner", secret, time.Hour)
	return tok
}

// expiredToken generates a JWT that is already expired.
func expiredToken(secret []byte) string {
	tok, _ := security.GenerateToken("user-1", "owner", secret, -time.Minute)
	return tok
}

// ─── auth tests ─────────────────────────────────────────────────────────────

// TestWSAuth_NoToken verifies a 401 when no ?token= is provided and auth is enabled.
func TestWSAuth_NoToken(t *testing.T) {
	tmpDir := t.TempDir()
	logger := wsTestLogger()
	registry, _ := agents.NewRegistry(tmpDir, logger)
	memory, _ := agents.NewMemoryStore(tmpDir, logger)
	router := models.NewRouter(logger)
	orch := orchestrator.New(&config.Config{}, logger)

	srv := NewServer(8422, orch, registry, memory, router, logger)
	srv.jwtSecret = []byte("test-secret")

	ts := httptest.NewServer(srv.wsTerminalHandler())
	defer ts.Close()

	url := "ws" + strings.TrimPrefix(ts.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, resp, err := websocket.Dial(ctx, url, nil)
	if err == nil {
		t.Fatal("expected dial to fail with 401, but it succeeded")
	}
	if resp != nil && resp.StatusCode != 401 {
		t.Errorf("expected status 401, got %d", resp.StatusCode)
	}
}

// TestWSAuth_InvalidToken verifies a 401 for a malformed token.
func TestWSAuth_InvalidToken(t *testing.T) {
	tmpDir := t.TempDir()
	logger := wsTestLogger()
	registry, _ := agents.NewRegistry(tmpDir, logger)
	memory, _ := agents.NewMemoryStore(tmpDir, logger)
	router := models.NewRouter(logger)
	orch := orchestrator.New(&config.Config{}, logger)

	srv := NewServer(8423, orch, registry, memory, router, logger)
	srv.jwtSecret = []byte("test-secret")

	ts := httptest.NewServer(srv.wsTerminalHandler())
	defer ts.Close()

	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "?token=not.a.valid.jwt"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, resp, err := websocket.Dial(ctx, url, nil)
	if err == nil {
		t.Fatal("expected dial to fail with 401, but it succeeded")
	}
	if resp != nil && resp.StatusCode != 401 {
		t.Errorf("expected status 401, got %d", resp.StatusCode)
	}
}

// TestWSAuth_ExpiredToken verifies a 401 for a token that has already expired.
func TestWSAuth_ExpiredToken(t *testing.T) {
	secret := []byte("test-secret")
	tmpDir := t.TempDir()
	logger := wsTestLogger()
	registry, _ := agents.NewRegistry(tmpDir, logger)
	memory, _ := agents.NewMemoryStore(tmpDir, logger)
	router := models.NewRouter(logger)
	orch := orchestrator.New(&config.Config{}, logger)

	srv := NewServer(8424, orch, registry, memory, router, logger)
	srv.jwtSecret = secret

	ts := httptest.NewServer(srv.wsTerminalHandler())
	defer ts.Close()

	tok := expiredToken(secret)
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "?token=" + tok
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, resp, err := websocket.Dial(ctx, url, nil)
	if err == nil {
		t.Fatal("expected dial to fail with 401 for expired token")
	}
	if resp != nil && resp.StatusCode != 401 {
		t.Errorf("expected status 401, got %d", resp.StatusCode)
	}
}

// TestWSAuth_ValidToken_DevMode verifies that dev mode (jwtSecret == nil) accepts
// connections without any token.
func TestWSAuth_ValidToken_DevMode(t *testing.T) {
	_, ts, cleanup := newWSTestServer(t, false)
	defer cleanup()

	// No token provided — should succeed in dev mode.
	url := "ws" + strings.TrimPrefix(ts.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("expected successful connection in dev mode, got: %v", err)
	}
	conn.Close(websocket.StatusNormalClosure, "")
}

// ─── chat tests ──────────────────────────────────────────────────────────────

// TestWSChat_ValidMessage verifies a complete chat round-trip over WebSocket.
func TestWSChat_ValidMessage(t *testing.T) {
	srv, ts, cleanup := newWSTestServer(t, true)
	defer cleanup()

	// Register a test agent so the agent-lookup passes.
	agentDef := config.AgentDef{ID: "agent-ws-1", Name: "WS Agent"}
	_, err := srv.registry.Create(agentDef)
	if err != nil {
		t.Fatalf("failed to create test agent: %v", err)
	}

	conn, cancel, err := dialWS(t, ts, "")
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer func() {
		conn.Close(websocket.StatusNormalClosure, "")
		cancel()
	}()

	reqCtx := context.Background()
	req := WSRequest{
		Type:      "chat",
		AgentID:   "agent-ws-1",
		Message:   "hello agent",
		RequestID: "req-1",
	}
	if err := wsjson.Write(reqCtx, conn, req); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	var resp WSResponse
	if err := wsjson.Read(reqCtx, conn, &resp); err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if resp.Type != "done" {
		t.Errorf("expected type 'done', got %q (error: %s)", resp.Type, resp.Error)
	}
	if !resp.Done {
		t.Error("expected Done=true")
	}
	if resp.RequestID != "req-1" {
		t.Errorf("RequestID = %q, want %q", resp.RequestID, "req-1")
	}
	if !strings.Contains(resp.Content, "echo: hello agent") {
		t.Errorf("unexpected content: %q", resp.Content)
	}
}

// TestWSChat_InvalidAgent verifies an error response when the agent does not exist.
func TestWSChat_InvalidAgent(t *testing.T) {
	_, ts, cleanup := newWSTestServer(t, false)
	defer cleanup()

	conn, cancel, err := dialWS(t, ts, "")
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer func() {
		conn.Close(websocket.StatusNormalClosure, "")
		cancel()
	}()

	reqCtx := context.Background()
	req := WSRequest{
		Type:      "chat",
		AgentID:   "nonexistent-agent",
		Message:   "hello?",
		RequestID: "req-invalid",
	}
	if err := wsjson.Write(reqCtx, conn, req); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	var resp WSResponse
	if err := wsjson.Read(reqCtx, conn, &resp); err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if resp.Type != "error" {
		t.Errorf("expected type 'error', got %q", resp.Type)
	}
	if resp.Error == "" {
		t.Error("expected non-empty Error field")
	}
}

// TestWSChat_Timeout verifies a timeout error when the orchestrator never responds.
func TestWSChat_Timeout(t *testing.T) {
	srv, ts, cleanup := newWSTestServer(t, false) // respond=false → orchestrator never answers
	defer cleanup()

	// Register agent so it passes agent validation.
	agentDef := config.AgentDef{ID: "agent-timeout", Name: "Timeout Agent"}
	_, _ = srv.registry.Create(agentDef)

	conn, cancel, err := dialWS(t, ts, "")
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer func() {
		conn.Close(websocket.StatusNormalClosure, "")
		cancel()
	}()

	reqCtx, reqCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer reqCancel()

	req := WSRequest{
		Type:      "chat",
		AgentID:   "agent-timeout",
		Message:   "will this timeout?",
		RequestID: "req-timeout",
	}
	if err := wsjson.Write(reqCtx, conn, req); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	var resp WSResponse
	if err := wsjson.Read(reqCtx, conn, &resp); err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if resp.Type != "error" {
		t.Errorf("expected type 'error' on timeout, got %q", resp.Type)
	}
}

// TestWSChat_MalformedJSON verifies that sending non-JSON closes the connection gracefully.
func TestWSChat_MalformedJSON(t *testing.T) {
	_, ts, cleanup := newWSTestServer(t, false)
	defer cleanup()

	conn, cancel, err := dialWS(t, ts, "")
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer func() {
		cancel()
	}()

	// Send raw malformed bytes.
	ctx, cancelCtx := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelCtx()

	if err := conn.Write(ctx, websocket.MessageText, []byte("not json at all {{{")); err != nil {
		// Write might fail if server already closed — that's acceptable.
		return
	}

	// Server should reject and close the connection.  A subsequent read should error.
	var resp WSResponse
	err = wsjson.Read(ctx, conn, &resp)
	// Either the server closed the connection (err != nil) or it sent an error frame.
	if err == nil && resp.Type != "error" {
		t.Errorf("expected connection close or error frame after malformed JSON, got type=%q", resp.Type)
	}
	conn.Close(websocket.StatusNormalClosure, "")
}

// ─── ping/pong test ──────────────────────────────────────────────────────────

// TestWSPingPong verifies the ping → pong round-trip.
func TestWSPingPong(t *testing.T) {
	_, ts, cleanup := newWSTestServer(t, false)
	defer cleanup()

	conn, cancel, err := dialWS(t, ts, "")
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer func() {
		conn.Close(websocket.StatusNormalClosure, "")
		cancel()
	}()

	reqCtx := context.Background()
	req := WSRequest{
		Type:      "ping",
		RequestID: "ping-1",
	}
	if err := wsjson.Write(reqCtx, conn, req); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	var resp WSResponse
	if err := wsjson.Read(reqCtx, conn, &resp); err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if resp.Type != "pong" {
		t.Errorf("expected type 'pong', got %q", resp.Type)
	}
	if resp.RequestID != "ping-1" {
		t.Errorf("RequestID = %q, want %q", resp.RequestID, "ping-1")
	}
}
