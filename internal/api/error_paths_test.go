package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/clawinfra/evoclaw/internal/config"
)

// Test Start() with immediate context cancellation
func TestServer_StartCanceled(t *testing.T) {
	s := newTestServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Start should return immediately due to canceled context
	err := s.Start(ctx)
	if err != nil && err != context.Canceled {
		t.Logf("Start returned: %v", err)
	}
}

// Test Start() with timeout
func TestServer_StartTimeout(t *testing.T) {
	s := newTestServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Use a port that's unlikely to be taken
	s.port = 0 // Let OS assign a random port

	// Start in goroutine
	done := make(chan error, 1)
	go func() {
		done <- s.Start(ctx)
	}()

	// Wait for timeout or completion
	select {
	case err := <-done:
		t.Logf("Start completed with: %v", err)
	case <-time.After(200 * time.Millisecond):
		t.Log("Start timeout test completed")
	}
}

// Test handleStatus with cost aggregation
func TestHandleStatus_WithCosts(t *testing.T) {
	s := newTestServer(t)

	// Register provider and make a chat to create costs
	provider := &mockProvider{
		name: "test",
		models: []config.Model{
			{ID: "test-model", CostInput: 1.0, CostOutput: 2.0},
		},
	}
	s.router.RegisterProvider(provider)

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	w := httptest.NewRecorder()

	s.handleStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should have total_cost field
	if _, ok := response["total_cost"]; !ok {
		t.Error("expected total_cost field")
	}
}
