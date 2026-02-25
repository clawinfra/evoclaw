package channels

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/clawinfra/evoclaw/internal/types"
)

func newTestWSChannel() *WSChannel {
	return NewWSChannel(slog.Default())
}

// TestWSChannel_Name verifies the channel name constant.
func TestWSChannel_Name(t *testing.T) {
	ch := newTestWSChannel()
	if got := ch.Name(); got != "websocket" {
		t.Fatalf("Name() = %q, want %q", got, "websocket")
	}
}

// TestWSChannel_StartStop verifies Start and Stop do not error.
func TestWSChannel_StartStop(t *testing.T) {
	ch := newTestWSChannel()

	if err := ch.Start(context.Background()); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if err := ch.Stop(); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}
}

// TestWSChannel_StopWithoutStart verifies Stop is safe before Start.
func TestWSChannel_StopWithoutStart(t *testing.T) {
	ch := newTestWSChannel()
	if err := ch.Stop(); err != nil {
		t.Fatalf("Stop() before Start should not error, got: %v", err)
	}
}

// TestWSChannel_Receive verifies Receive returns a non-nil channel.
func TestWSChannel_Receive(t *testing.T) {
	ch := newTestWSChannel()
	recv := ch.Receive()
	if recv == nil {
		t.Fatal("Receive() returned nil channel")
	}
}

// TestWSChannel_RegisterUnregister verifies Register returns a valid channel
// and Unregister removes the entry cleanly.
func TestWSChannel_RegisterUnregister(t *testing.T) {
	ch := newTestWSChannel()

	respCh := ch.Register("msg-1", "req-1", nil)
	if respCh == nil {
		t.Fatal("Register() returned nil channel")
	}

	// Confirm the entry is tracked.
	ch.mu.RLock()
	_, ok := ch.conns["msg-1"]
	ch.mu.RUnlock()
	if !ok {
		t.Fatal("Register() did not store connection entry")
	}

	ch.Unregister("msg-1")

	ch.mu.RLock()
	_, ok = ch.conns["msg-1"]
	ch.mu.RUnlock()
	if ok {
		t.Fatal("Unregister() did not remove connection entry")
	}
}

// TestWSChannel_SendRouting verifies Send delivers a response to the correct respCh.
func TestWSChannel_SendRouting(t *testing.T) {
	ch := newTestWSChannel()

	// Register two distinct message IDs.
	respCh1 := ch.Register("msg-A", "req-A", nil)
	respCh2 := ch.Register("msg-B", "req-B", nil)
	defer ch.Unregister("msg-A")
	defer ch.Unregister("msg-B")

	resp := types.Response{
		AgentID:   "agent-1",
		Content:   "hello from agent",
		MessageID: "msg-A",
	}

	ctx := context.Background()
	if err := ch.Send(ctx, resp); err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	// respCh1 should receive the response.
	select {
	case got := <-respCh1:
		if got.Content != "hello from agent" {
			t.Errorf("got Content %q, want %q", got.Content, "hello from agent")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for response on respCh1")
	}

	// respCh2 should be empty.
	select {
	case unexpected := <-respCh2:
		t.Errorf("respCh2 unexpectedly received: %+v", unexpected)
	default:
		// expected: nothing
	}
}

// TestWSChannel_SendNoRegistration verifies Send returns an error for unknown messageID.
func TestWSChannel_SendNoRegistration(t *testing.T) {
	ch := newTestWSChannel()

	resp := types.Response{MessageID: "ghost-msg"}
	err := ch.Send(context.Background(), resp)
	if err == nil {
		t.Fatal("Send() to unregistered messageID should return an error")
	}
}

// TestWSChannel_SendContextCancelled verifies Send respects a cancelled context.
func TestWSChannel_SendContextCancelled(t *testing.T) {
	ch := newTestWSChannel()

	// Register but make the respCh full so the select falls to ctx.Done().
	respCh := ch.Register("msg-full", "req-full", nil)
	// Fill the buffered channel (capacity 1).
	respCh <- types.Response{MessageID: "msg-full"}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancel

	resp := types.Response{MessageID: "msg-full"}
	err := ch.Send(ctx, resp)
	// With a full buffer and cancelled ctx, we'll hit either ctx.Err() or the default branch.
	// Both indicate proper rejection.
	if err == nil {
		t.Fatal("Send() should return an error when channel is full or context cancelled")
	}

	ch.Unregister("msg-full")
}

// TestWSChannel_InboxDelivery verifies messages pushed to inbox are readable from Receive().
func TestWSChannel_InboxDelivery(t *testing.T) {
	ch := newTestWSChannel()

	msg := types.Message{ID: "inbox-1", Content: "test", Channel: "websocket"}
	ch.inbox <- msg

	select {
	case got := <-ch.Receive():
		if got.ID != "inbox-1" {
			t.Errorf("got ID %q, want %q", got.ID, "inbox-1")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out reading from Receive()")
	}
}
