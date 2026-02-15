package channels

import (
	"context"
	"fmt"
	"testing"

	"github.com/clawinfra/evoclaw/internal/orchestrator"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Test MQTT Start with connection error
func TestMQTT_StartConnectionError(t *testing.T) {
	mockClient := &MockMQTTClient{
		ConnectFunc: func() mqtt.Token {
			return &MockMQTTToken{err: fmt.Errorf("connection failed")}
		},
	}

	mqttChan := NewMQTTWithClient(
		"localhost",
		1883,
		"user",
		"pass",
		testLogger(),
		func(opts *mqtt.ClientOptions) MQTTClient {
			return mockClient
		},
	)

	err := mqttChan.Start(context.Background())
	if err == nil {
		t.Error("expected error from Start when connection fails")
	}
}

// Test MQTT Start with subscribe error - SKIPPED
// The subscribe error is logged but doesn't fail Start()

// Test MQTT Send with publish error
func TestMQTT_SendPublishError(t *testing.T) {
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

	msg := orchestrator.Response{
		AgentID: "agent-1",
		Content: "test",
		To:      "target-agent",
	}

	err := mqttChan.Send(context.Background(), msg)
	if err == nil {
		t.Error("expected error from Send when publish fails")
	}
}

// Test MQTT Send when not connected
func TestMQTT_SendNotConnected(t *testing.T) {
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

	msg := orchestrator.Response{
		AgentID: "agent-1",
		Content: "test",
		To:      "target-agent",
	}

	err := mqttChan.Send(context.Background(), msg)
	if err == nil {
		t.Error("expected error from Send when not connected")
	}
}

// handleMessage is tested comprehensively in mqtt_comprehensive_test.go

// Test MQTT Broadcast with publish error
func TestMQTT_BroadcastPublishError(t *testing.T) {
	mockClient := &MockMQTTClient{
		IsConnectedVal: true,
		PublishFunc: func(topic string, qos byte, retained bool, payload interface{}) mqtt.Token {
			return &MockMQTTToken{err: fmt.Errorf("broadcast failed")}
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

	err := mqttChan.Broadcast(context.Background(), "broadcast message")
	if err == nil {
		t.Error("expected error from Broadcast when publish fails")
	}
}

// The Telegram tests require proper HTTP mocking which is already done in telegram_comprehensive_test.go
