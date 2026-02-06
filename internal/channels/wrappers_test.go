package channels

import (
	"testing"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Test DefaultMQTTClient wrapper methods
func TestDefaultMQTTClient_Connect(t *testing.T) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost:1883")
	
	client := &DefaultMQTTClient{
		client: mqtt.NewClient(opts),
	}
	
	// This will fail to connect but we're just testing the wrapper
	token := client.Connect()
	if token == nil {
		t.Error("expected non-nil token")
	}
}

func TestDefaultMQTTClient_Publish(t *testing.T) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost:1883")
	
	client := &DefaultMQTTClient{
		client: mqtt.NewClient(opts),
	}
	
	token := client.Publish("test/topic", 0, false, "test message")
	if token == nil {
		t.Error("expected non-nil token")
	}
}

func TestDefaultMQTTClient_Subscribe(t *testing.T) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost:1883")
	
	client := &DefaultMQTTClient{
		client: mqtt.NewClient(opts),
	}
	
	handler := func(client mqtt.Client, msg mqtt.Message) {}
	token := client.Subscribe("test/topic", 0, handler)
	if token == nil {
		t.Error("expected non-nil token")
	}
}

func TestDefaultMQTTClient_Disconnect(t *testing.T) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost:1883")
	
	client := &DefaultMQTTClient{
		client: mqtt.NewClient(opts),
	}
	
	// Should not panic
	client.Disconnect(250)
}

func TestDefaultMQTTClient_IsConnected(t *testing.T) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost:1883")
	
	client := &DefaultMQTTClient{
		client: mqtt.NewClient(opts),
	}
	
	connected := client.IsConnected()
	if connected {
		t.Error("expected not connected")
	}
}
