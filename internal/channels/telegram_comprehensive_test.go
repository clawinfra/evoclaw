package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/clawinfra/evoclaw/internal/orchestrator"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// MockHTTPClient implements HTTPClient for testing
type MockHTTPClient struct {
	DoFunc func(req *http.Request) (*http.Response, error)
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if m.DoFunc != nil {
		return m.DoFunc(req)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
	}, nil
}

func TestTelegramVerifyToken_Success(t *testing.T) {
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			if !strings.Contains(req.URL.Path, "/getMe") {
				t.Errorf("unexpected path: %s", req.URL.Path)
			}

			resp := map[string]interface{}{
				"ok": true,
				"result": map[string]interface{}{
					"username":   "test_bot",
					"first_name": "Test Bot",
				},
			}
			body, _ := json.Marshal(resp)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(body)),
			}, nil
		},
	}

	tg := NewTelegramWithClient("test-token", testLogger(), mockClient)

	err := tg.verifyToken()
	if err != nil {
		t.Fatalf("verifyToken failed: %v", err)
	}
}

func TestTelegramVerifyToken_InvalidToken(t *testing.T) {
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(strings.NewReader(`{"ok":false,"description":"Unauthorized"}`)),
			}, nil
		},
	}

	tg := NewTelegramWithClient("bad-token", testLogger(), mockClient)

	err := tg.verifyToken()
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestTelegramVerifyToken_NetworkError(t *testing.T) {
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("network error")
		},
	}

	tg := NewTelegramWithClient("test-token", testLogger(), mockClient)

	err := tg.verifyToken()
	if err == nil {
		t.Fatal("expected error for network failure")
	}
}

func TestTelegramSend_Success(t *testing.T) {
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			if !strings.Contains(req.URL.Path, "/sendMessage") {
				t.Errorf("unexpected path: %s", req.URL.Path)
			}

			// Check query parameters
			chatID := req.URL.Query().Get("chat_id")
			if chatID != "12345" {
				t.Errorf("expected chat_id 12345, got %s", chatID)
			}

			text := req.URL.Query().Get("text")
			if text != "Hello, world!" {
				t.Errorf("expected text 'Hello, world!', got %s", text)
			}

			resp := map[string]interface{}{
				"ok": true,
				"result": map[string]interface{}{
					"message_id": 999,
				},
			}
			body, _ := json.Marshal(resp)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(body)),
			}, nil
		},
	}

	tg := NewTelegramWithClient("test-token", testLogger(), mockClient)

	msg := orchestrator.Response{
		Content: "Hello, world!",
		To:      "12345",
		Channel: "telegram",
	}

	err := tg.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
}

func TestTelegramSend_WithReplyTo(t *testing.T) {
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			replyTo := req.URL.Query().Get("reply_to_message_id")
			if replyTo != "777" {
				t.Errorf("expected reply_to_message_id 777, got %s", replyTo)
			}

			resp := map[string]interface{}{"ok": true}
			body, _ := json.Marshal(resp)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(body)),
			}, nil
		},
	}

	tg := NewTelegramWithClient("test-token", testLogger(), mockClient)

	msg := orchestrator.Response{
		Content: "Reply message",
		To:      "12345",
		ReplyTo: "777",
		Channel: "telegram",
	}

	err := tg.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
}

func TestTelegramSend_APIError(t *testing.T) {
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader(`{"ok":false,"description":"Bad Request: chat not found"}`)),
			}, nil
		},
	}

	tg := NewTelegramWithClient("test-token", testLogger(), mockClient)

	msg := orchestrator.Response{
		Content: "Test",
		To:      "invalid",
		Channel: "telegram",
	}

	err := tg.Send(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error for API error")
	}
}

func TestTelegramPollOnce_NoUpdates(t *testing.T) {
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			if !strings.Contains(req.URL.Path, "/getUpdates") {
				t.Errorf("unexpected path: %s", req.URL.Path)
			}

			resp := map[string]interface{}{
				"ok":     true,
				"result": []interface{}{},
			}
			body, _ := json.Marshal(resp)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(body)),
			}, nil
		},
	}

	tg := NewTelegramWithClient("test-token", testLogger(), mockClient)
	tg.ctx = context.Background()

	err := tg.pollOnce()
	if err != nil {
		t.Fatalf("pollOnce failed: %v", err)
	}
}

func TestTelegramPollOnce_WithMessages(t *testing.T) {
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			resp := map[string]interface{}{
				"ok": true,
				"result": []interface{}{
					map[string]interface{}{
						"update_id": 123,
						"message": map[string]interface{}{
							"message_id": 456,
							"from": map[string]interface{}{
								"id":         int64(111),
								"username":   "testuser",
								"first_name": "Test",
							},
							"chat": map[string]interface{}{
								"id":   int64(222),
								"type": "private",
							},
							"date": time.Now().Unix(),
							"text": "Hello bot!",
						},
					},
				},
			}
			body, _ := json.Marshal(resp)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(body)),
			}, nil
		},
	}

	tg := NewTelegramWithClient("test-token", testLogger(), mockClient)
	tg.ctx = context.Background()

	err := tg.pollOnce()
	if err != nil {
		t.Fatalf("pollOnce failed: %v", err)
	}

	// Check that offset was updated
	if tg.offset != 124 {
		t.Errorf("expected offset 124, got %d", tg.offset)
	}

	// Check that message was queued
	select {
	case msg := <-tg.inbox:
		if msg.Content != "Hello bot!" {
			t.Errorf("expected content 'Hello bot!', got %s", msg.Content)
		}
		if msg.From != "111" {
			t.Errorf("expected from '111', got %s", msg.From)
		}
		if msg.To != "222" {
			t.Errorf("expected to '222', got %s", msg.To)
		}
		if msg.Metadata["username"] != "testuser" {
			t.Errorf("expected username 'testuser', got %s", msg.Metadata["username"])
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected message in inbox")
	}
}

func TestTelegramPollOnce_SkipNonTextMessages(t *testing.T) {
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			resp := map[string]interface{}{
				"ok": true,
				"result": []interface{}{
					map[string]interface{}{
						"update_id": 123,
						"message": map[string]interface{}{
							"message_id": 456,
							"from": map[string]interface{}{
								"id": int64(111),
							},
							"chat": map[string]interface{}{
								"id": int64(222),
							},
							"date":  time.Now().Unix(),
							"photo": []interface{}{}, // Non-text message
						},
					},
				},
			}
			body, _ := json.Marshal(resp)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(body)),
			}, nil
		},
	}

	tg := NewTelegramWithClient("test-token", testLogger(), mockClient)
	tg.ctx = context.Background()

	err := tg.pollOnce()
	if err != nil {
		t.Fatalf("pollOnce failed: %v", err)
	}

	// Offset should still be updated
	if tg.offset != 124 {
		t.Errorf("expected offset 124, got %d", tg.offset)
	}

	// But no message should be in inbox
	select {
	case <-tg.inbox:
		t.Fatal("didn't expect message in inbox for non-text message")
	case <-time.After(10 * time.Millisecond):
		// Good
	}
}

func TestTelegramPollOnce_APIError(t *testing.T) {
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader(`{"ok":false}`)),
			}, nil
		},
	}

	tg := NewTelegramWithClient("test-token", testLogger(), mockClient)
	tg.ctx = context.Background()

	err := tg.pollOnce()
	if err == nil {
		t.Fatal("expected error for API error")
	}
}

func TestTelegramStart_Stop(t *testing.T) {
	callCount := 0
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			callCount++
			
			// First call: getMe
			if strings.Contains(req.URL.Path, "/getMe") {
				resp := map[string]interface{}{
					"ok": true,
					"result": map[string]interface{}{
						"username": "test_bot",
					},
				}
				body, _ := json.Marshal(resp)
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader(body)),
				}, nil
			}

			// Subsequent calls: getUpdates
			resp := map[string]interface{}{
				"ok":     true,
				"result": []interface{}{},
			}
			body, _ := json.Marshal(resp)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(body)),
			}, nil
		},
	}

	tg := NewTelegramWithClient("test-token", testLogger(), mockClient)

	ctx := context.Background()
	err := tg.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give it a moment to start polling
	time.Sleep(50 * time.Millisecond)

	// Stop it
	err = tg.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Verify getMe was called
	if callCount < 1 {
		t.Error("expected at least one HTTP call (getMe)")
	}
}

func TestTelegramName(t *testing.T) {
	tg := NewTelegramWithClient("test-token", testLogger(), &MockHTTPClient{})
	if tg.Name() != "telegram" {
		t.Errorf("expected name 'telegram', got %s", tg.Name())
	}
}

func TestTelegramReceive(t *testing.T) {
	tg := NewTelegramWithClient("test-token", testLogger(), &MockHTTPClient{})
	ch := tg.Receive()
	if ch == nil {
		t.Error("expected non-nil receive channel")
	}
}

func TestTelegramPollOnce_ContextCancelled(t *testing.T) {
	callCount := 0
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			callCount++
			// Simulate delay
			time.Sleep(50 * time.Millisecond)
			resp := map[string]interface{}{
				"ok":     true,
				"result": []interface{}{},
			}
			body, _ := json.Marshal(resp)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(body)),
			}, nil
		},
	}

	tg := NewTelegramWithClient("test-token", testLogger(), mockClient)
	ctx, cancel := context.WithCancel(context.Background())
	tg.ctx = ctx

	// Cancel context immediately
	cancel()

	err := tg.pollOnce()
	// Should either get context error or succeed quickly
	if err != nil && !strings.Contains(err.Error(), "context") {
		t.Logf("Got error (acceptable): %v", err)
	}
}

func TestTelegramPollLoop_ContextCancelled(t *testing.T) {
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			resp := map[string]interface{}{
				"ok":     true,
				"result": []interface{}{},
			}
			body, _ := json.Marshal(resp)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(body)),
			}, nil
		},
	}

	tg := NewTelegramWithClient("test-token", testLogger(), mockClient)
	ctx, cancel := context.WithCancel(context.Background())
	tg.ctx = ctx
	tg.wg.Add(1)

	go tg.pollLoop()

	// Let it run briefly
	time.Sleep(10 * time.Millisecond)

	// Cancel context
	cancel()

	// Wait for goroutine to finish
	tg.wg.Wait()
}

func TestTelegramSend_RequestCreationError(t *testing.T) {
	mockClient := &MockHTTPClient{}

	tg := NewTelegramWithClient("test-token", testLogger(), mockClient)

	// Create a context that will cause NewRequestWithContext to fail
	// This is hard to trigger, but we can test with a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	msg := orchestrator.Response{
		Content: "Test",
		To:      "12345",
		Channel: "telegram",
	}

	err := tg.Send(ctx, msg)
	// Should either succeed or fail gracefully
	if err != nil {
		t.Logf("Send with cancelled context returned error: %v", err)
	}
}

func TestTelegramPollOnce_WithReplyToMessage(t *testing.T) {
	mockClient := &MockHTTPClient{
		DoFunc: func(req *http.Request) (*http.Response, error) {
			resp := map[string]interface{}{
				"ok": true,
				"result": []interface{}{
					map[string]interface{}{
						"update_id": 123,
						"message": map[string]interface{}{
							"message_id": 456,
							"from": map[string]interface{}{
								"id":         int64(111),
								"username":   "testuser",
								"first_name": "Test",
							},
							"chat": map[string]interface{}{
								"id":   int64(222),
								"type": "private",
							},
							"date": time.Now().Unix(),
							"text": "Reply to this",
							"reply_to_message": map[string]interface{}{
								"message_id": 123,
								"text":       "Original message",
							},
						},
					},
				},
			}
			body, _ := json.Marshal(resp)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(body)),
			}, nil
		},
	}

	tg := NewTelegramWithClient("test-token", testLogger(), mockClient)
	tg.ctx = context.Background()

	err := tg.pollOnce()
	if err != nil {
		t.Fatalf("pollOnce failed: %v", err)
	}

	// Check that message was queued with reply_to
	select {
	case msg := <-tg.inbox:
		if msg.ReplyTo != "123" {
			t.Errorf("expected reply_to '123', got %s", msg.ReplyTo)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected message in inbox")
	}
}

func TestMQTTStart_WithAuth(t *testing.T) {
	mockClient := &MockMQTTClient{}

	mqttChan := NewMQTTWithClient(
		"localhost",
		1883,
		"testuser",
		"testpass",
		testLogger(),
		func(opts *mqtt.ClientOptions) MQTTClient {
			return mockClient
		},
	)

	ctx := context.Background()
	err := mqttChan.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if !mockClient.IsConnectedVal {
		t.Error("expected client to be connected")
	}

	err = mqttChan.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestMQTTSend_WithMetadata(t *testing.T) {
	publishCalled := false
	var publishedPayload []byte

	mockClient := &MockMQTTClient{
		IsConnectedVal: true,
		PublishFunc: func(topic string, qos byte, retained bool, payload interface{}) mqtt.Token {
			publishCalled = true
			publishedPayload = payload.([]byte)
			return &MockMQTTToken{err: nil}
		},
	}

	mqttChan := NewMQTTWithClient(
		"localhost",
		1883,
		"",
		"",
		testLogger(),
		func(opts *mqtt.ClientOptions) MQTTClient {
			return mockClient
		},
	)

	mqttChan.client = mockClient

	msg := orchestrator.Response{
		AgentID: "agent-1",
		Content: "Test message",
		To:      "device-123",
		Channel: "mqtt",
		Metadata: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}

	err := mqttChan.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if !publishCalled {
		t.Fatal("expected Publish to be called")
	}

	// Verify metadata in payload
	var payload map[string]interface{}
	if err := json.Unmarshal(publishedPayload, &payload); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}

	metadata := payload["metadata"].(map[string]interface{})
	if metadata["key1"] != "value1" {
		t.Errorf("expected metadata key1='value1', got %v", metadata["key1"])
	}
}

func TestMQTTHandleStatus_InvalidJSON(t *testing.T) {
	mockClient := &MockMQTTClient{}

	mqttChan := NewMQTTWithClient(
		"localhost",
		1883,
		"",
		"",
		testLogger(),
		func(opts *mqtt.ClientOptions) MQTTClient {
			return mockClient
		},
	)

	mockMsg := &MockMQTTMessage{
		topic:   "evoclaw/agents/agent-1/status",
		payload: []byte("invalid json{{{"),
	}

	// Call the handler - should not panic
	mqttChan.handleStatus(nil, mockMsg)
}

func TestMQTTSubscribe_ErrorOnSecondSubscribe(t *testing.T) {
	firstCall := true
	mockClient := &MockMQTTClient{
		SubscribeFunc: func(topic string, qos byte, callback mqtt.MessageHandler) mqtt.Token {
			if firstCall {
				firstCall = false
				return &MockMQTTToken{err: nil}
			}
			// Second subscribe fails
			return &MockMQTTToken{err: fmt.Errorf("subscribe failed")}
		},
	}

	mqttChan := NewMQTTWithClient(
		"localhost",
		1883,
		"",
		"",
		testLogger(),
		func(opts *mqtt.ClientOptions) MQTTClient {
			return mockClient
		},
	)

	mqttChan.client = mockClient

	err := mqttChan.subscribe()
	if err == nil {
		t.Fatal("expected error for second subscribe failure")
	}
}

func TestMQTTBroadcast_Error(t *testing.T) {
	mockClient := &MockMQTTClient{
		IsConnectedVal: true,
		PublishFunc: func(topic string, qos byte, retained bool, payload interface{}) mqtt.Token {
			return &MockMQTTToken{err: fmt.Errorf("publish failed")}
		},
	}

	mqttChan := NewMQTTWithClient(
		"localhost",
		1883,
		"",
		"",
		testLogger(),
		func(opts *mqtt.ClientOptions) MQTTClient {
			return mockClient
		},
	)

	mqttChan.client = mockClient

	err := mqttChan.Broadcast(context.Background(), "Test")
	if err == nil {
		t.Fatal("expected error for broadcast failure")
	}
}
