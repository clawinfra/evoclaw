package channels

import (
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// MQTTClient is an interface for MQTT client operations
// This allows us to mock MQTT calls in tests
type MQTTClient interface {
	Connect() mqtt.Token
	Disconnect(quiesce uint)
	Publish(topic string, qos byte, retained bool, payload interface{}) mqtt.Token
	Subscribe(topic string, qos byte, callback mqtt.MessageHandler) mqtt.Token
	IsConnected() bool
}

// DefaultMQTTClient wraps the paho MQTT client
type DefaultMQTTClient struct {
	client mqtt.Client
}

func (d *DefaultMQTTClient) Connect() mqtt.Token {
	return d.client.Connect()
}

func (d *DefaultMQTTClient) Disconnect(quiesce uint) {
	d.client.Disconnect(quiesce)
}

func (d *DefaultMQTTClient) Publish(topic string, qos byte, retained bool, payload interface{}) mqtt.Token {
	return d.client.Publish(topic, qos, retained, payload)
}

func (d *DefaultMQTTClient) Subscribe(topic string, qos byte, callback mqtt.MessageHandler) mqtt.Token {
	return d.client.Subscribe(topic, qos, callback)
}

func (d *DefaultMQTTClient) IsConnected() bool {
	return d.client.IsConnected()
}
