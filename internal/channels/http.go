package channels

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/clawinfra/evoclaw/internal/types"
)

// HTTPChannel handles HTTP API request-response pairs
type HTTPChannel struct {
	mu             sync.RWMutex
	pending        map[string]chan types.Response // messageID -> response channel
	responseTimeout time.Duration
}

// NewHTTPChannel creates a new HTTP channel handler
func NewHTTPChannel() *HTTPChannel {
	return &HTTPChannel{
		pending:        make(map[string]chan types.Response),
		responseTimeout: 30 * time.Second,
	}
}

// Name returns the channel name
func (h *HTTPChannel) Name() string {
	return "http"
}

// Send delivers a response back to a waiting HTTP request
func (h *HTTPChannel) Send(ctx context.Context, resp types.Response) error {
	h.mu.Lock()
	ch, ok := h.pending[resp.MessageID]
	if ok {
		delete(h.pending, resp.MessageID)
	}
	h.mu.Unlock()

	if !ok {
		return fmt.Errorf("no pending request for message %s", resp.MessageID)
	}

	select {
	case ch <- resp:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("response channel closed")
	}
}

// WaitForResponse waits for a response to the given message ID
func (h *HTTPChannel) WaitForResponse(ctx context.Context, messageID string) (types.Response, error) {
	// Create response channel
	respCh := make(chan types.Response, 1)
	
	h.mu.Lock()
	h.pending[messageID] = respCh
	h.mu.Unlock()

	// Wait for response or timeout
	timeout := time.After(h.responseTimeout)
	select {
	case resp := <-respCh:
		return resp, nil
	case <-timeout:
		h.mu.Lock()
		delete(h.pending, messageID)
		h.mu.Unlock()
		return types.Response{}, fmt.Errorf("response timeout after %v", h.responseTimeout)
	case <-ctx.Done():
		h.mu.Lock()
		delete(h.pending, messageID)
		h.mu.Unlock()
		return types.Response{}, ctx.Err()
	}
}

// Start initializes the HTTP channel (no-op for HTTP as it's always ready)
func (h *HTTPChannel) Start(ctx context.Context) error {
	return nil
}

// Stop cleans up the channel
func (h *HTTPChannel) Stop() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	// Close all pending channels
	for _, ch := range h.pending {
		close(ch)
	}
	h.pending = make(map[string]chan types.Response)
	
	return nil
}

// Receive returns a channel for incoming messages (HTTP doesn't receive via channel)
func (h *HTTPChannel) Receive() <-chan types.Message {
	// HTTP requests come in via API handlers, not through a receive channel
	return nil
}
