package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// handleAgentMemory: returns memory (or creates it)
func TestHandleAgentMemory_Response(t *testing.T) {
	s := newTestServerV2(t)
	w := httptest.NewRecorder()
	s.handleAgentMemory(w, "some-agent-id")
	// Memory store auto-creates on Get, so either 200 or 404 is acceptable
	if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
		t.Errorf("got %d, unexpected", w.Code)
	}
}

// handleClearMemory: returns memory cleared or 404
func TestHandleClearMemory_Response(t *testing.T) {
	s := newTestServerV2(t)
	w := httptest.NewRecorder()
	s.handleClearMemory(w, "some-agent-id")
	if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
		t.Errorf("got %d, unexpected", w.Code)
	}
}

// WriteError helper
func TestWriteError_Helper(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, http.StatusBadRequest, "test error message")
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", w.Code)
	}
}
