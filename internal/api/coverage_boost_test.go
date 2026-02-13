package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/clawinfra/evoclaw/internal/evolution"
	"github.com/clawinfra/evoclaw/internal/security"
)

// Test handleAuthToken
func TestHandleAuthToken(t *testing.T) {
	s := newTestServerV2(t)
	s.jwtSecret = []byte("test-secret")

	// Valid request
	body := map[string]string{
		"agent_id": "test-agent",
		"role":     "owner",
		"api_key":  "test-key",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/auth/token", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()
	s.handleAuthToken(w, req)

	if w.Code != 200 {
		t.Errorf("status code = %d, want 200", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["token"] == nil {
		t.Error("expected token in response")
	}
}

func TestHandleAuthTokenInvalidMethod(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest("GET", "/api/auth/token", nil)
	w := httptest.NewRecorder()
	s.handleAuthToken(w, req)

	if w.Code != 405 {
		t.Errorf("status code = %d, want 405", w.Code)
	}
}

func TestHandleAuthTokenInvalidJSON(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest("POST", "/api/auth/token", bytes.NewReader([]byte("invalid")))
	w := httptest.NewRecorder()
	s.handleAuthToken(w, req)

	if w.Code != 400 {
		t.Errorf("status code = %d, want 400", w.Code)
	}
}

func TestHandleAuthTokenMissingFields(t *testing.T) {
	s := newTestServerV2(t)
	body := map[string]string{"agent_id": "test"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/auth/token", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()
	s.handleAuthToken(w, req)

	if w.Code != 400 {
		t.Errorf("status code = %d, want 400", w.Code)
	}
}

func TestHandleAuthTokenInvalidRole(t *testing.T) {
	s := newTestServerV2(t)
	body := map[string]string{
		"agent_id": "test",
		"role":     "invalid-role",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/auth/token", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()
	s.handleAuthToken(w, req)

	if w.Code != 400 {
		t.Errorf("status code = %d, want 400", w.Code)
	}
}

func TestHandleAuthTokenDevMode(t *testing.T) {
	s := newTestServerV2(t)
	s.jwtSecret = nil // Dev mode

	body := map[string]string{
		"agent_id": "test-agent",
		"role":     "owner",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/auth/token", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()
	s.handleAuthToken(w, req)

	if w.Code != 200 {
		t.Errorf("status code = %d, want 200", w.Code)
	}
}

// Test handleAgentMetrics
func TestHandleAgentMetricsV3(t *testing.T) {
	s, agent := newTestServerWithAgent(t)
	w := httptest.NewRecorder()
	s.handleAgentMetrics(w, agent)

	if w.Code != 200 {
		t.Errorf("status code = %d, want 200", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["agent_id"] != "test-agent" {
		t.Error("expected agent_id in response")
	}
}

// Test handleAgentEvolve
func TestHandleAgentEvolveV3(t *testing.T) {
	s, agent := newTestServerWithAgent(t)
	w := httptest.NewRecorder()
	s.handleAgentEvolve(w, agent)

	if w.Code != 200 {
		t.Errorf("status code = %d, want 200", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["message"] != "evolution triggered" {
		t.Error("expected evolution message")
	}
}

// Test handleAgentMemory
func TestHandleAgentMemoryV3(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	
	// Add some memory
	mem := s.memory.Get("test-agent")
	mem.Add("user", "Hello")
	mem.Add("assistant", "Hi there")

	w := httptest.NewRecorder()
	s.handleAgentMemory(w, "test-agent")

	if w.Code != 200 {
		t.Errorf("status code = %d, want 200", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["agent_id"] != "test-agent" {
		t.Error("expected agent_id in response")
	}
}

// Test handleClearMemory
func TestHandleClearMemoryV3(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	
	mem := s.memory.Get("test-agent")
	mem.Add("user", "Hello")

	w := httptest.NewRecorder()
	s.handleClearMemory(w, "test-agent")

	if w.Code != 200 {
		t.Errorf("status code = %d, want 200", w.Code)
	}

	// Verify memory was cleared
	if len(mem.GetMessages()) != 0 {
		t.Error("expected memory to be cleared")
	}
}

func TestHandleClearMemoryNotFound(t *testing.T) {
	s := newTestServerV2(t)
	w := httptest.NewRecorder()
	s.handleClearMemory(w, "nonexistent")

	// Memory.Get() creates memory for any agent ID, so this returns 200
	if w.Code != 200 {
		t.Errorf("status code = %d, want 200", w.Code)
	}
}

// Test handleAgentDetail actions
func TestHandleAgentDetailMetrics(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	req := httptest.NewRequest("GET", "/api/agents/test-agent/metrics", nil)
	w := httptest.NewRecorder()
	s.handleAgentDetail(w, req)

	if w.Code != 200 {
		t.Errorf("status code = %d, want 200", w.Code)
	}
}

func TestHandleAgentDetailEvolve(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	req := httptest.NewRequest("POST", "/api/agents/test-agent/evolve", nil)
	w := httptest.NewRecorder()
	s.handleAgentDetail(w, req)

	if w.Code != 200 {
		t.Errorf("status code = %d, want 200", w.Code)
	}
}

func TestHandleAgentDetailMemory(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	mem := s.memory.Get("test-agent")
	mem.Add("user", "test")

	req := httptest.NewRequest("GET", "/api/agents/test-agent/memory", nil)
	w := httptest.NewRecorder()
	s.handleAgentDetail(w, req)

	if w.Code != 200 {
		t.Errorf("status code = %d, want 200", w.Code)
	}
}

func TestHandleAgentDetailClearMemory(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	s.memory.Get("test-agent")

	req := httptest.NewRequest("DELETE", "/api/agents/test-agent/memory", nil)
	w := httptest.NewRecorder()
	s.handleAgentDetail(w, req)

	if w.Code != 200 {
		t.Errorf("status code = %d, want 200", w.Code)
	}
}

func TestHandleAgentDetailInvalidAction(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	req := httptest.NewRequest("POST", "/api/agents/test-agent/invalid", nil)
	w := httptest.NewRecorder()
	s.handleAgentDetail(w, req)

	if w.Code != 400 {
		t.Errorf("status code = %d, want 400", w.Code)
	}
}

func TestHandleAgentDetailNoID(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest("GET", "/api/agents/", nil)
	w := httptest.NewRecorder()
	s.handleAgentDetail(w, req)

	if w.Code != 404 {
		t.Errorf("status code = %d, want 404", w.Code)
	}
}

// Test middleware
func TestLoggingMiddlewareV3(t *testing.T) {
	s := newTestServerV2(t)
	handler := s.loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status code = %d, want 200", w.Code)
	}
}

func TestCORSMiddlewareV3(t *testing.T) {
	s := newTestServerV2(t)
	handler := s.corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	// Test OPTIONS
	req := httptest.NewRequest("OPTIONS", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("OPTIONS status code = %d, want 200", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("expected CORS header")
	}

	// Test normal request
	req = httptest.NewRequest("GET", "/test", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("GET status code = %d, want 200", w.Code)
	}
}

func TestJWTAuthWrapper(t *testing.T) {
	s := newTestServerV2(t)
	s.jwtSecret = []byte("test-secret")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	wrappedHandler := s.jwtAuthWrapper(handler)

	// Test non-API route (should pass through)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("non-API route status = %d, want 200", w.Code)
	}

	// Test auth token endpoint (should pass through)
	req = httptest.NewRequest("POST", "/api/auth/token", nil)
	w = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("auth token endpoint status = %d, want 200", w.Code)
	}

	// Test protected API route without token (should fail)
	req = httptest.NewRequest("GET", "/api/status", nil)
	w = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Errorf("protected route without token status = %d, want 401", w.Code)
	}

	// Test protected API route with valid token
	token, _ := security.GenerateToken("test-agent", "owner", s.jwtSecret, time.Hour)
	req = httptest.NewRequest("GET", "/api/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("protected route with token status = %d, want 200", w.Code)
	}
}

// Test server Start/Stop
func TestServerStartStop(t *testing.T) {
	s := newTestServerV2(t)
	s.port = 0 // Random port

	ctx, cancel := context.WithCancel(context.Background())
	
	// Start in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Start(ctx)
	}()

	// Give it time to start
	time.Sleep(100 * time.Millisecond)

	// Cancel context to trigger shutdown
	cancel()

	// Wait for shutdown
	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Errorf("Start() error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for server shutdown")
	}
}

// Test SetWebFS and SetEvolution
func TestSetWebFS(t *testing.T) {
	s := newTestServerV2(t)
	s.SetWebFS(os.DirFS("."))
	if s.webFS == nil {
		t.Error("expected webFS to be set")
	}
}

func TestSetEvolution(t *testing.T) {
	s := newTestServerV2(t)
	tmpDir := t.TempDir()
	logger := slog.Default()
	
	evo := evolution.NewEngine(tmpDir, logger)
	
	s.SetEvolution(evo)
	if s.evolution == nil {
		t.Error("expected evolution to be set")
	}
}

// Test handleLogStream
func TestHandleLogStream(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	
	req := httptest.NewRequest("GET", "/api/logs/stream", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)
	
	w := httptest.NewRecorder()
	
	// Start handler in goroutine
	done := make(chan bool)
	go func() {
		s.handleLogStream(w, req)
		done <- true
	}()
	
	select {
	case <-done:
		// Should disconnect when context is cancelled
	case <-time.After(1 * time.Second):
		t.Error("timeout waiting for log stream to close")
	}
}

func TestHandleLogStreamMethodNotAllowed(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest("POST", "/api/logs/stream", nil)
	w := httptest.NewRecorder()
	s.handleLogStream(w, req)

	if w.Code != 405 {
		t.Errorf("status code = %d, want 405", w.Code)
	}
}

// Test handleMemoryStats
func TestHandleMemoryStats(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	
	req := httptest.NewRequest("GET", "/api/memory/stats", nil)
	w := httptest.NewRecorder()
	s.handleMemoryStats(w, req)

	// Should return 503 because tiered memory system is not initialized in test
	if w.Code != 503 {
		t.Errorf("status code = %d, want 503", w.Code)
	}
}

func TestHandleMemoryStatsMethodNotAllowed(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest("POST", "/api/memory/stats", nil)
	w := httptest.NewRecorder()
	s.handleMemoryStats(w, req)

	// Method check happens before initialization check
	if w.Code != 405 {
		t.Errorf("status code = %d, want 405", w.Code)
	}
}

// Test handleMemoryTree
func TestHandleMemoryTree(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	
	req := httptest.NewRequest("GET", "/api/memory/tree", nil)
	w := httptest.NewRecorder()
	s.handleMemoryTree(w, req)

	// Should return 503 because tiered memory system is not initialized in test
	if w.Code != 503 {
		t.Errorf("status code = %d, want 503", w.Code)
	}
}

func TestHandleMemoryTreeMethodNotAllowed(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest("POST", "/api/memory/tree", nil)
	w := httptest.NewRecorder()
	s.handleMemoryTree(w, req)

	if w.Code != 405 {
		t.Errorf("status code = %d, want 405", w.Code)
	}
}

// Test handleMemoryRetrieve
func TestHandleMemoryRetrieve(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	
	req := httptest.NewRequest("GET", "/api/memory/retrieve?q=test&limit=5", nil)
	w := httptest.NewRecorder()
	s.handleMemoryRetrieve(w, req)

	// Should return 503 because tiered memory system is not initialized in test
	if w.Code != 503 {
		t.Errorf("status code = %d, want 503", w.Code)
	}
}

func TestHandleMemoryRetrieveMethodNotAllowed(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest("POST", "/api/memory/retrieve", nil)
	w := httptest.NewRecorder()
	s.handleMemoryRetrieve(w, req)

	// Method check happens first
	if w.Code != 405 {
		t.Errorf("status code = %d, want 405", w.Code)
	}
}

// Test respondJSON error path
func TestRespondJSONErrorV3(t *testing.T) {
	s := newTestServerV2(t)
	w := httptest.NewRecorder()
	
	// Create a value that cannot be encoded to JSON
	invalidData := make(chan int)
	s.respondJSON(w, invalidData)
	
	if w.Code != 500 {
		t.Errorf("status code = %d, want 500", w.Code)
	}
}

// Test sendSSE
func TestSendSSE(t *testing.T) {
	s := newTestServerV2(t)
	w := httptest.NewRecorder()
	
	data := map[string]string{"message": "test"}
	s.sendSSE(w, w, data)
	
	body := w.Body.String()
	if body == "" {
		t.Error("expected SSE data to be written")
	}
}
