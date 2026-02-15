package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/clawinfra/evoclaw/internal/types"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	// MQTT topics for agent communication
	commandsTopic  = "evoclaw/agents/%s/commands" // orchestrator → agent
	reportsTopic   = "evoclaw/agents/%s/reports"  // agent → orchestrator
	broadcastTopic = "evoclaw/broadcast"          // orchestrator → all agents
	statusTopic    = "evoclaw/agents/%s/status"   // agent heartbeats
)

// EdgeAgentCommand represents the message format expected by Rust edge agents
type EdgeAgentCommand struct {
	Command   string                 `json:"command"`   // "message", "ping", "status", etc.
	Payload   map[string]interface{} `json:"payload"`   // Command-specific data
	RequestID string                 `json:"request_id"` // Unique request identifier
}

// MQTTChannel implements the Channel interface for MQTT communication
type MQTTChannel struct {
	broker   string
	port     int
	clientID string
	username string
	password string
	logger   *slog.Logger
	inbox    chan types.Message
	client   MQTTClient
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	// Factory function for creating MQTT client
	clientFactory func(opts *mqtt.ClientOptions) MQTTClient
	// Result callback for tool execution results
	resultCallback func(requestID string, result map[string]interface{})
	resultMu       sync.RWMutex
}

// NewMQTT creates a new MQTT channel adapter
func NewMQTT(broker string, port int, username, password string, logger *slog.Logger) *MQTTChannel {
	return &MQTTChannel{
		broker:   broker,
		port:     port,
		clientID: fmt.Sprintf("evoclaw-orchestrator-%d", time.Now().Unix()),
		username: username,
		password: password,
		logger:   logger.With("channel", "mqtt"),
		inbox:    make(chan types.Message, 100),
		clientFactory: func(opts *mqtt.ClientOptions) MQTTClient {
			return &DefaultMQTTClient{client: mqtt.NewClient(opts)}
		},
		resultCallback: nil, // Will be set by orchestrator
	}
}

// NewMQTTWithClient creates an MQTT channel with a custom client factory (for testing)
func NewMQTTWithClient(broker string, port int, username, password string, logger *slog.Logger, clientFactory func(*mqtt.ClientOptions) MQTTClient) *MQTTChannel {
	return &MQTTChannel{
		broker:          broker,
		port:            port,
		clientID:        fmt.Sprintf("evoclaw-orchestrator-%d", time.Now().Unix()),
		username:        username,
		password:        password,
		logger:          logger.With("channel", "mqtt"),
		inbox:           make(chan types.Message, 100),
		clientFactory:   clientFactory,
		resultCallback: nil, // Will be set by orchestrator
	}
}

func (m *MQTTChannel) Name() string {
	return "mqtt"
}

func (m *MQTTChannel) Start(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)

	// Configure MQTT client
	opts := mqtt.NewClientOptions()
	brokerURL := fmt.Sprintf("tcp://%s:%d", m.broker, m.port)
	opts.AddBroker(brokerURL)
	opts.SetClientID(m.clientID)

	if m.username != "" {
		opts.SetUsername(m.username)
		opts.SetPassword(m.password)
	}

	opts.SetKeepAlive(30 * time.Second)
	opts.SetPingTimeout(10 * time.Second)
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(30 * time.Second)

	// Connection lost handler
	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		m.logger.Warn("mqtt connection lost", "error", err)
	})

	// On connect handler
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		m.logger.Info("mqtt connected, subscribing to topics")
		if err := m.subscribe(); err != nil {
			m.logger.Error("failed to subscribe", "error", err)
		}
	})

	m.client = m.clientFactory(opts)

	// Connect
	m.logger.Info("connecting to mqtt broker", "broker", brokerURL)
	token := m.client.Connect()
	if !token.WaitTimeout(10 * time.Second) {
		return fmt.Errorf("connection timeout")
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("connect to mqtt: %w", err)
	}

	m.logger.Info("mqtt channel started")
	return nil
}

func (m *MQTTChannel) Stop() error {
	m.logger.Info("stopping mqtt channel")

	if m.cancel != nil {
		m.cancel()
	}

	if m.client != nil && m.client.IsConnected() {
		m.client.Disconnect(250)
	}

	m.wg.Wait()
	close(m.inbox)
	return nil
}

func (m *MQTTChannel) Send(ctx context.Context, msg types.Response) error {
	if !m.client.IsConnected() {
		return fmt.Errorf("mqtt not connected")
	}

	// Determine the topic based on the recipient
	topic := fmt.Sprintf(commandsTopic, msg.To)

	// Convert orchestrator Response to edge agent command format
	// Generate request_id from metadata or create a new one
	requestID := fmt.Sprintf("req-%d", time.Now().UnixNano())
	if msg.MessageID != "" {
		requestID = msg.MessageID
	}

	// Build command for edge agent
	cmd := EdgeAgentCommand{
		Command:   "message", // Default command for natural language content
		RequestID: requestID,
		Payload: map[string]interface{}{
			"agent_id": msg.AgentID,
			"content":  msg.Content,
			"reply_to": msg.ReplyTo,
			"metadata": msg.Metadata,
			"sent_at":  time.Now().Unix(),
		},
	}

	// Override command if specified in metadata
	if msg.Metadata != nil {
		if overrideCmd, ok := msg.Metadata["command"]; ok {
			cmd.Command = overrideCmd
		}
	}

	// Serialize message to edge agent format
	payload, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	// Publish with QoS 1 (at least once delivery)
	token := m.client.Publish(topic, 1, false, payload)
	if !token.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("publish timeout")
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("publish: %w", err)
	}

	m.logger.Debug("message sent", "topic", topic, "size", len(payload), "command", cmd.Command)
	return nil
}

func (m *MQTTChannel) Receive() <-chan types.Message {
	return m.inbox
}

// subscribe to relevant MQTT topics
func (m *MQTTChannel) subscribe() error {
	// Subscribe to all agent reports (wildcard)
	reportPattern := "evoclaw/agents/+/reports"
	token := m.client.Subscribe(reportPattern, 1, m.handleMessage)
	if !token.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("subscribe timeout")
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("subscribe to %s: %w", reportPattern, err)
	}
	m.logger.Info("subscribed", "topic", reportPattern)

	// Subscribe to all agent status updates
	statusPattern := "evoclaw/agents/+/status"
	token = m.client.Subscribe(statusPattern, 1, m.handleStatus)
	if !token.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("subscribe timeout")
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("subscribe to %s: %w", statusPattern, err)
	}
	m.logger.Info("subscribed", "topic", statusPattern)

	return nil
}

// handleMessage processes incoming messages from agents
func (m *MQTTChannel) handleMessage(client mqtt.Client, mqttMsg mqtt.Message) {
	m.wg.Add(1)
	defer m.wg.Done()

	m.logger.Debug("mqtt message received", "topic", mqttMsg.Topic())

	// Try to parse as generic map first to check for tool results
	var genericPayload map[string]interface{}
	if err := json.Unmarshal(mqttMsg.Payload(), &genericPayload); err == nil {
		// Check if this is a tool result (has "tool" and "status" fields)
		if toolName, ok := genericPayload["tool"].(string); ok {
			if status, ok := genericPayload["status"].(string); ok {
				// This is a tool result, route to orchestrator
				m.logger.Debug("tool result detected",
					"tool", toolName,
					"status", status,
					"request_id", genericPayload["request_id"],
				)
				m.handleToolResult(genericPayload)
				return // Don't forward to inbox as regular message
			}
		}
	}

	// Regular message handling
	var payload struct {
		AgentID  string            `json:"agent_id"`
		Content  string            `json:"content"`
		ReplyTo  string            `json:"reply_to,omitempty"`
		Metadata map[string]string `json:"metadata,omitempty"`
		SentAt   int64             `json:"sent_at"`
	}

	if err := json.Unmarshal(mqttMsg.Payload(), &payload); err != nil {
		m.logger.Error("failed to parse mqtt message", "error", err)
		return
	}

	msg := types.Message{
		ID:        fmt.Sprintf("mqtt-%d", time.Now().UnixNano()),
		Channel:   "mqtt",
		From:      payload.AgentID,
		To:        "orchestrator",
		Content:   payload.Content,
		Timestamp: time.Unix(payload.SentAt, 0),
		ReplyTo:   payload.ReplyTo,
		Metadata:  payload.Metadata,
	}

	if msg.Metadata == nil {
		msg.Metadata = make(map[string]string)
	}
	msg.Metadata["mqtt_topic"] = mqttMsg.Topic()

	select {
	case m.inbox <- msg:
		m.logger.Debug("message queued", "from", msg.From, "length", len(msg.Content))
	case <-m.ctx.Done():
		return
	default:
		m.logger.Warn("inbox full, dropping message", "from", msg.From)
	}
}

// handleStatus processes agent heartbeat/status updates
func (m *MQTTChannel) handleStatus(client mqtt.Client, mqttMsg mqtt.Message) {
	m.wg.Add(1)
	defer m.wg.Done()

	m.logger.Debug("agent status update", "topic", mqttMsg.Topic())

	var status struct {
		AgentID   string  `json:"agent_id"`
		Status    string  `json:"status"` // "online", "idle", "busy", "error"
		Timestamp int64   `json:"timestamp"`
		Uptime    float64 `json:"uptime_seconds,omitempty"`
		CPU       float64 `json:"cpu_percent,omitempty"`
		Memory    float64 `json:"memory_mb,omitempty"`
	}

	if err := json.Unmarshal(mqttMsg.Payload(), &status); err != nil {
		m.logger.Error("failed to parse status", "error", err)
		return
	}

	m.logger.Debug("agent status",
		"agent", status.AgentID,
		"status", status.Status,
		"uptime", status.Uptime,
	)

	// TODO: Update agent registry with heartbeat info
	// This would integrate with internal/agents/registry.go
}

// Broadcast sends a message to all agents
func (m *MQTTChannel) Broadcast(ctx context.Context, content string) error {
	if !m.client.IsConnected() {
		return fmt.Errorf("mqtt not connected")
	}

	payload, err := json.Marshal(map[string]interface{}{
		"content": content,
		"sent_at": time.Now().Unix(),
	})
	if err != nil {
		return fmt.Errorf("marshal broadcast: %w", err)
	}

	token := m.client.Publish(broadcastTopic, 1, false, payload)
	if !token.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("broadcast timeout")
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("broadcast: %w", err)
	}

	m.logger.Info("broadcast sent", "size", len(payload))
	return nil
}

// SetResultCallback sets the callback for tool execution results
func (m *MQTTChannel) SetResultCallback(cb func(requestID string, result map[string]interface{})) {
	m.resultMu.Lock()
	defer m.resultMu.Unlock()
	m.resultCallback = cb
	m.logger.Debug("result callback registered")
}

// handleToolResult processes a tool execution result from edge agents
func (m *MQTTChannel) handleToolResult(payload map[string]interface{}) {
	requestID, _ := payload["request_id"].(string)

	m.resultMu.RLock()
	callback := m.resultCallback
	m.resultMu.RUnlock()

	if callback == nil {
		m.logger.Warn("no result callback registered, dropping tool result",
			"request_id", requestID,
			"tool", payload["tool"],
		)
		return
	}

	// Deliver result to orchestrator via callback
	callback(requestID, payload)

	m.logger.Debug("tool result delivered to orchestrator",
		"request_id", requestID,
		"tool", payload["tool"],
		"status", payload["status"],
	)
}
