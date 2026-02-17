package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/clawinfra/evoclaw/internal/types"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// MockMQTTToken implements mqtt.Token for testing
type MockMQTTToken struct {
	err     error
	timeout bool
}

func (m *MockMQTTToken) Wait() bool {
	return true
}

func (m *MockMQTTToken) WaitTimeout(duration time.Duration) bool {
	return !m.timeout
}

func (m *MockMQTTToken) Done() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func (m *MockMQTTToken) Error() error {
	return m.err
}

// MockMQTTClient implements MQTTClient for testing
type MockMQTTClient struct {
	ConnectFunc    func() mqtt.Token
	DisconnectFunc func(quiesce uint)
	PublishFunc    func(topic string, qos byte, retained bool, payload interface{}) mqtt.Token
	SubscribeFunc  func(topic string, qos byte, callback mqtt.MessageHandler) mqtt.Token
	IsConnectedVal bool
	
	// Store callbacks for testing
	messageHandler mqtt.MessageHandler
	statusHandler  mqtt.MessageHandler
}

func (m *MockMQTTClient) Connect() mqtt.Token {
	if m.ConnectFunc != nil {
		return m.ConnectFunc()
	}
	m.IsConnectedVal = true
	return &MockMQTTToken{err: nil}
}

func (m *MockMQTTClient) Disconnect(quiesce uint) {
	if m.DisconnectFunc != nil {
		m.DisconnectFunc(quiesce)
	}
	m.IsConnectedVal = false
}

func (m *MockMQTTClient) Publish(topic string, qos byte, retained bool, payload interface{}) mqtt.Token {
	if m.PublishFunc != nil {
		return m.PublishFunc(topic, qos, retained, payload)
	}
	return &MockMQTTToken{err: nil}
}

func (m *MockMQTTClient) Subscribe(topic string, qos byte, callback mqtt.MessageHandler) mqtt.Token {
	if m.SubscribeFunc != nil {
		return m.SubscribeFunc(topic, qos, callback)
	}
	
	// Store the callback for later use
	switch topic {
	case "evoclaw/agents/+/reports":
		m.messageHandler = callback
	case "evoclaw/agents/+/status":
		m.statusHandler = callback
	}
	
	return &MockMQTTToken{err: nil}
}

func (m *MockMQTTClient) IsConnected() bool {
	return m.IsConnectedVal
}

// MockMQTTMessage implements mqtt.Message for testing
type MockMQTTMessage struct {
	topic   string
	payload []byte
}

func (m *MockMQTTMessage) Duplicate() bool      { return false }
func (m *MockMQTTMessage) Qos() byte             { return 0 }
func (m *MockMQTTMessage) Retained() bool        { return false }
func (m *MockMQTTMessage) Topic() string         { return m.topic }
func (m *MockMQTTMessage) MessageID() uint16     { return 0 }
func (m *MockMQTTMessage) Payload() []byte       { return m.payload }
func (m *MockMQTTMessage) Ack()                  {}

func TestMQTTStart_Success(t *testing.T) {
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

func TestMQTTStart_ConnectionFailed(t *testing.T) {
	mockClient := &MockMQTTClient{
		ConnectFunc: func() mqtt.Token {
			return &MockMQTTToken{err: fmt.Errorf("connection refused")}
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

	ctx := context.Background()
	err := mqttChan.Start(ctx)
	if err == nil {
		t.Fatal("expected error for failed connection")
	}
}

func TestMQTTStart_ConnectionTimeout(t *testing.T) {
	mockClient := &MockMQTTClient{
		ConnectFunc: func() mqtt.Token {
			return &MockMQTTToken{timeout: true}
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

	ctx := context.Background()
	err := mqttChan.Start(ctx)
	if err == nil {
		t.Fatal("expected error for connection timeout")
	}
}

func TestMQTTSend_Success(t *testing.T) {
	publishCalled := false
	var publishedTopic string
	var publishedPayload []byte

	mockClient := &MockMQTTClient{
		IsConnectedVal: true,
		PublishFunc: func(topic string, qos byte, retained bool, payload interface{}) mqtt.Token {
			publishCalled = true
			publishedTopic = topic
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

	msg := types.Response{
		AgentID: "agent-1",
		Content: "Test message",
		To:      "device-123",
		ReplyTo: "msg-456",
		Channel: "mqtt",
	}

	err := mqttChan.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if !publishCalled {
		t.Fatal("expected Publish to be called")
	}

	if publishedTopic != "evoclaw/agents/device-123/commands" {
		t.Errorf("unexpected topic: %s", publishedTopic)
	}

	// Verify payload — Send wraps in EdgeAgentCommand
	var cmd EdgeAgentCommand
	if err := json.Unmarshal(publishedPayload, &cmd); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}

	if cmd.Payload["agent_id"] != "agent-1" {
		t.Errorf("expected agent_id 'agent-1', got %v", cmd.Payload["agent_id"])
	}
	if cmd.Payload["content"] != "Test message" {
		t.Errorf("expected content 'Test message', got %v", cmd.Payload["content"])
	}
	if cmd.Payload["reply_to"] != "msg-456" {
		t.Errorf("expected reply_to 'msg-456', got %v", cmd.Payload["reply_to"])
	}
}

func TestMQTTSend_NotConnected(t *testing.T) {
	mockClient := &MockMQTTClient{
		IsConnectedVal: false,
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

	msg := types.Response{
		Content: "Test",
		To:      "device-1",
		Channel: "mqtt",
	}

	err := mqttChan.Send(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error when not connected")
	}
}

func TestMQTTSend_PublishTimeout(t *testing.T) {
	mockClient := &MockMQTTClient{
		IsConnectedVal: true,
		PublishFunc: func(topic string, qos byte, retained bool, payload interface{}) mqtt.Token {
			return &MockMQTTToken{timeout: true}
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

	msg := types.Response{
		Content: "Test",
		To:      "device-1",
		Channel: "mqtt",
	}

	err := mqttChan.Send(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error for publish timeout")
	}
}

func TestMQTTHandleMessage(t *testing.T) {
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

	mqttChan.ctx = context.Background()
	mqttChan.inbox = make(chan types.Message, 10)

	// Simulate message payload
	payload := map[string]interface{}{
		"agent_id": "agent-1",
		"content":  "Hello from agent",
		"reply_to": "",
		"metadata": map[string]string{"key": "value"},
		"sent_at":  time.Now().Unix(),
	}
	payloadBytes, _ := json.Marshal(payload)

	mockMsg := &MockMQTTMessage{
		topic:   "evoclaw/agents/agent-1/reports",
		payload: payloadBytes,
	}

	// Call the handler
	mqttChan.handleMessage(nil, mockMsg)

	// Check inbox
	select {
	case msg := <-mqttChan.inbox:
		if msg.From != "agent-1" {
			t.Errorf("expected from 'agent-1', got %s", msg.From)
		}
		if msg.Content != "Hello from agent" {
			t.Errorf("expected content 'Hello from agent', got %s", msg.Content)
		}
		if msg.Metadata["mqtt_topic"] != "evoclaw/agents/agent-1/reports" {
			t.Errorf("expected mqtt_topic metadata, got %s", msg.Metadata["mqtt_topic"])
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected message in inbox")
	}
}

func TestMQTTHandleMessage_InvalidJSON(t *testing.T) {
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

	mqttChan.ctx = context.Background()
	mqttChan.inbox = make(chan types.Message, 10)

	mockMsg := &MockMQTTMessage{
		topic:   "evoclaw/agents/agent-1/reports",
		payload: []byte("invalid json{{{"),
	}

	// Call the handler - should not panic
	mqttChan.handleMessage(nil, mockMsg)

	// Check that nothing was added to inbox
	select {
	case <-mqttChan.inbox:
		t.Fatal("didn't expect message in inbox for invalid JSON")
	case <-time.After(10 * time.Millisecond):
		// Good
	}
}

func TestMQTTHandleStatus(t *testing.T) {
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

	// Simulate status payload
	status := map[string]interface{}{
		"agent_id":        "agent-1",
		"status":          "online",
		"timestamp":       time.Now().Unix(),
		"uptime_seconds":  123.45,
		"cpu_percent":     45.6,
		"memory_mb":       256.0,
	}
	statusBytes, _ := json.Marshal(status)

	mockMsg := &MockMQTTMessage{
		topic:   "evoclaw/agents/agent-1/status",
		payload: statusBytes,
	}

	// Call the handler - should not panic
	mqttChan.handleStatus(nil, mockMsg)
}

func TestMQTTBroadcast_Success(t *testing.T) {
	publishCalled := false
	var publishedTopic string

	mockClient := &MockMQTTClient{
		IsConnectedVal: true,
		PublishFunc: func(topic string, qos byte, retained bool, payload interface{}) mqtt.Token {
			publishCalled = true
			publishedTopic = topic
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

	err := mqttChan.Broadcast(context.Background(), "Broadcast message")
	if err != nil {
		t.Fatalf("Broadcast failed: %v", err)
	}

	if !publishCalled {
		t.Fatal("expected Publish to be called")
	}

	if publishedTopic != "evoclaw/broadcast" {
		t.Errorf("expected topic 'evoclaw/broadcast', got %s", publishedTopic)
	}
}

func TestMQTTBroadcast_NotConnected(t *testing.T) {
	mockClient := &MockMQTTClient{
		IsConnectedVal: false,
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
		t.Fatal("expected error when not connected")
	}
}

func TestMQTTName(t *testing.T) {
	mqttChan := NewMQTT("localhost", 1883, "", "", testLogger())
	if mqttChan.Name() != "mqtt" {
		t.Errorf("expected name 'mqtt', got %s", mqttChan.Name())
	}
}

func TestMQTTReceive(t *testing.T) {
	mqttChan := NewMQTT("localhost", 1883, "", "", testLogger())
	ch := mqttChan.Receive()
	if ch == nil {
		t.Error("expected non-nil receive channel")
	}
}

func TestMQTTSubscribe_Success(t *testing.T) {
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

	mqttChan.client = mockClient

	err := mqttChan.subscribe()
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	// Check that handlers were registered
	if mockClient.messageHandler == nil {
		t.Error("expected message handler to be registered")
	}
	if mockClient.statusHandler == nil {
		t.Error("expected status handler to be registered")
	}
}

func TestMQTTSubscribe_Timeout(t *testing.T) {
	mockClient := &MockMQTTClient{
		SubscribeFunc: func(topic string, qos byte, callback mqtt.MessageHandler) mqtt.Token {
			return &MockMQTTToken{timeout: true}
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
		t.Fatal("expected error for subscribe timeout")
	}
}

func TestMQTTStop_WhenNotConnected(t *testing.T) {
	mockClient := &MockMQTTClient{
		IsConnectedVal: false,
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
	mqttChan.cancel = func() {} // Mock cancel function

	err := mqttChan.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

// These tests exercise the DefaultMQTTClient wrapper methods
// They don't connect to a real MQTT broker but verify the interface works

func TestDefaultMQTTClient_WrapperMethods(t *testing.T) {
	// Create actual paho client (won't connect)
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost:1883")
	opts.SetClientID("test-client")
	opts.SetAutoReconnect(false)
	
	pahoClient := mqtt.NewClient(opts)
	wrapper := &DefaultMQTTClient{client: pahoClient}

	// Test IsConnected (should be false without connecting)
	if wrapper.IsConnected() {
		t.Error("expected IsConnected to be false")
	}

	// Test Disconnect (should not panic even when not connected)
	wrapper.Disconnect(0)

	// These would require actual broker:
	// - Connect()
	// - Publish()
	// - Subscribe()
}
