package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/clawinfra/evoclaw/internal/types"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	// MQTT topics for agent communication
	commandsTopic      = "evoclaw/agents/%s/commands"      // orchestrator → agent
	reportsTopic       = "evoclaw/agents/%s/reports"       // agent → orchestrator
	broadcastTopic     = "evoclaw/broadcast"               // orchestrator → all agents
	statusTopic        = "evoclaw/agents/%s/status"        // agent heartbeats
	capabilitiesTopic  = "evoclaw/agents/%s/capabilities"  // agent capability advertisement (retained)
)

// EdgeAgentCommand represents the message format expected by Rust edge agents
type EdgeAgentCommand struct {
	Command   string                 `json:"command"`   // "message", "ping", "status", etc.
	Payload   map[string]interface{} `json:"payload"`   // Command-specific data
	RequestID string                 `json:"request_id"` // Unique request identifier
}

// EdgeAgentInfo tracks the status of an edge agent connected via MQTT
type EdgeAgentInfo struct {
	AgentID      string
	Status       string    // "online", "idle", "busy", "error"
	LastSeen     time.Time
	Uptime       float64
	CPU          float64
	MemoryMB     float64
	Capabilities string    // one-liner capability summary, published on startup
}

// PendingRequest tracks a request waiting for response
type PendingRequest struct {
	RequestID string
	Response  chan *EdgeAgentResponse
	CreatedAt time.Time
}

// EdgeAgentResponse represents a response from an edge agent
type EdgeAgentResponse struct {
	AgentID   string                 `json:"agent_id"`
	RequestID string                 `json:"request_id"`
	Content   string                 `json:"content"`
	Model     string                 `json:"model,omitempty"`
	Status    string                 `json:"status"` // "success", "error"
	Error     string                 `json:"error,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
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
	// Edge agent tracking
	edgeAgents   map[string]*EdgeAgentInfo
	edgeAgentsMu sync.RWMutex
	// Pending requests waiting for responses
	pendingRequests   map[string]*PendingRequest
	pendingRequestsMu sync.RWMutex
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
		resultCallback:  nil, // Will be set by orchestrator
		edgeAgents:      make(map[string]*EdgeAgentInfo),
		pendingRequests: make(map[string]*PendingRequest),
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
		resultCallback:  nil, // Will be set by orchestrator
		edgeAgents:      make(map[string]*EdgeAgentInfo),
		pendingRequests: make(map[string]*PendingRequest),
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
			// For prompt command, move content to prompt field and add enable_tools
			if overrideCmd == "prompt" {
				cmd.Payload = map[string]interface{}{
					"prompt":       msg.Content,
					"system_prompt": "",
					"enable_tools":  true,
				}
			}
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

	// Subscribe to agent capability advertisements (retained messages — delivered immediately on connect)
	capPattern := "evoclaw/agents/+/capabilities"
	token = m.client.Subscribe(capPattern, 1, m.handleCapabilities)
	if !token.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("subscribe timeout")
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("subscribe to %s: %w", capPattern, err)
	}
	m.logger.Info("subscribed", "topic", capPattern)

	return nil
}

// handleCapabilities processes capability advertisement messages from edge agents.
// Edge agents publish a retained message on startup describing what they can do.
func (m *MQTTChannel) handleCapabilities(client mqtt.Client, mqttMsg mqtt.Message) {
	var payload struct {
		AgentID      string `json:"agent_id"`
		Capabilities string `json:"capabilities"` // one-liner summary
	}
	if err := json.Unmarshal(mqttMsg.Payload(), &payload); err != nil {
		m.logger.Warn("failed to parse capabilities message", "error", err)
		return
	}
	if payload.AgentID == "" || payload.Capabilities == "" {
		return
	}

	m.edgeAgentsMu.Lock()
	if existing, ok := m.edgeAgents[payload.AgentID]; ok {
		existing.Capabilities = payload.Capabilities
	} else {
		m.edgeAgents[payload.AgentID] = &EdgeAgentInfo{
			AgentID:      payload.AgentID,
			Status:       "online",
			LastSeen:     time.Now(),
			Capabilities: payload.Capabilities,
		}
	}
	m.edgeAgentsMu.Unlock()

	m.logger.Info("edge agent capabilities registered",
		"agent", payload.AgentID,
		"capabilities", payload.Capabilities,
	)
}

// GetEdgeAgentCapabilities returns the capability summary for an edge agent.
func (m *MQTTChannel) GetEdgeAgentCapabilities(agentID string) string {
	m.edgeAgentsMu.RLock()
	defer m.edgeAgentsMu.RUnlock()
	if info, ok := m.edgeAgents[agentID]; ok {
		return info.Capabilities
	}
	return ""
}

// GetOnlineAgentsWithCapabilities returns a map of online agentID → capability summary.
func (m *MQTTChannel) GetOnlineAgentsWithCapabilities() map[string]string {
	m.edgeAgentsMu.RLock()
	defer m.edgeAgentsMu.RUnlock()

	result := make(map[string]string)
	for id, info := range m.edgeAgents {
		if time.Since(info.LastSeen) < 2*time.Minute {
			result[id] = info.Capabilities
		}
	}
	return result
}

// AgentReport represents messages from edge agents (matching Rust struct)
type AgentReport struct {
	AgentID    string                 `json:"agent_id"`
	AgentType  string                 `json:"agent_type"`
	ReportType string                 `json:"report_type"` // "result", "error", "heartbeat", "metric"
	Payload    map[string]interface{} `json:"payload"`
	Timestamp  int64                  `json:"timestamp"`
}

// handleMessage processes incoming messages from agents
func (m *MQTTChannel) handleMessage(client mqtt.Client, mqttMsg mqtt.Message) {
	m.wg.Add(1)
	defer m.wg.Done()

	m.logger.Info("incoming message", "channel", "mqtt", "from", extractAgentID(mqttMsg.Topic()), "length", len(mqttMsg.Payload()))

	// Extract agent ID from topic for heartbeat tracking
	topic := mqttMsg.Topic()
	var agentIDFromTopic string
	if parts := strings.Split(topic, "/"); len(parts) >= 3 {
		agentIDFromTopic = parts[2]
	}

	// Try to parse as AgentReport first (new edge agent format)
	var report AgentReport
	if err := json.Unmarshal(mqttMsg.Payload(), &report); err == nil {
		// Handle different report types
		switch report.ReportType {
		case "result":
			// This is a prompt completion result from edge agent
			m.logger.Debug("result report detected",
				"agent", report.AgentID,
				"request_id", report.Payload["request_id"],
			)
			m.handleEdgeAgentResult(report)
			return
		case "error":
			// Error report from edge agent
			m.logger.Warn("edge agent error report",
				"agent", report.AgentID,
				"error", report.Payload["error"],
			)
			m.handleEdgeAgentResult(report)
			return
		case "heartbeat":
			// Heartbeat - update tracking state and return
			m.logger.Debug("heartbeat received", "agent", report.AgentID)
			agentID := report.AgentID
			if agentID == "" {
				agentID = agentIDFromTopic
			}
			if agentID != "" {
				m.edgeAgentsMu.Lock()
				if existing, ok := m.edgeAgents[agentID]; ok {
					existing.LastSeen = time.Now()
				} else {
					m.edgeAgents[agentID] = &EdgeAgentInfo{
						AgentID:  agentID,
						Status:   "online",
						LastSeen: time.Now(),
					}
				}
				m.edgeAgentsMu.Unlock()
			}
			return
		}
	}

	// Fallback: Try to parse as generic map for old tool result format
	var genericPayload map[string]interface{}
	if err := json.Unmarshal(mqttMsg.Payload(), &genericPayload); err == nil {
		// Check if this is a response to a pending prompt request
		if requestID, ok := genericPayload["request_id"].(string); ok {
			if m.handleEdgeAgentResponse(genericPayload) {
				m.logger.Debug("message routed to pending request", "request_id", requestID)
				return // Message handled, don't forward to inbox
			}
		}

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

	// Regular message handling (fallback for legacy format)
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

// handleEdgeAgentResult processes result/error reports from edge agents
func (m *MQTTChannel) handleEdgeAgentResult(report AgentReport) {
	requestID, _ := report.Payload["request_id"].(string)

	m.resultMu.RLock()
	callback := m.resultCallback
	m.resultMu.RUnlock()

	if callback == nil {
		m.logger.Warn("no result callback registered, dropping edge agent result",
			"request_id", requestID,
			"agent", report.AgentID,
		)
		return
	}

	// Deliver full payload to orchestrator via callback
	callback(requestID, report.Payload)

	m.logger.Info("edge agent result delivered to orchestrator",
		"request_id", requestID,
		"agent", report.AgentID,
		"status", report.Payload["status"],
	)
}

// extractAgentID extracts agent ID from topic like "evoclaw/agents/alex-eye/reports"
func extractAgentID(topic string) string {
	parts := []byte(topic)
	// Find the third slash
	slashCount := 0
	start := 0
	end := len(parts)
	for i, b := range parts {
		if b == '/' {
			slashCount++
			if slashCount == 2 {
				start = i + 1
			} else if slashCount == 3 {
				end = i
				break
			}
		}
	}
	if start > 0 && end > start {
		return string(parts[start:end])
	}
	return "unknown"
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

	// Update edge agent registry
	m.edgeAgentsMu.Lock()
	m.edgeAgents[status.AgentID] = &EdgeAgentInfo{
		AgentID:  status.AgentID,
		Status:   status.Status,
		LastSeen: time.Now(),
		Uptime:   status.Uptime,
		CPU:      status.CPU,
		MemoryMB: status.Memory,
	}
	m.edgeAgentsMu.Unlock()

	m.logger.Debug("agent status updated",
		"agent", status.AgentID,
		"status", status.Status,
		"uptime", status.Uptime,
	)
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

// IsEdgeAgentOnline checks if an agent is connected via MQTT and recently active
func (m *MQTTChannel) IsEdgeAgentOnline(agentID string) bool {
	m.edgeAgentsMu.RLock()
	defer m.edgeAgentsMu.RUnlock()

	info, exists := m.edgeAgents[agentID]
	if !exists {
		return false
	}

	// Consider online if seen within last 2 minutes
	return time.Since(info.LastSeen) < 2*time.Minute
}

// GetEdgeAgentInfo returns info about an edge agent
func (m *MQTTChannel) GetEdgeAgentInfo(agentID string) *EdgeAgentInfo {
	m.edgeAgentsMu.RLock()
	defer m.edgeAgentsMu.RUnlock()

	if info, exists := m.edgeAgents[agentID]; exists {
		// Return a copy
		infoCopy := *info
		return &infoCopy
	}
	return nil
}

// GetOnlineEdgeAgents returns list of currently online edge agents
func (m *MQTTChannel) GetOnlineEdgeAgents() []string {
	m.edgeAgentsMu.RLock()
	defer m.edgeAgentsMu.RUnlock()

	var online []string
	for id, info := range m.edgeAgents {
		if time.Since(info.LastSeen) < 2*time.Minute {
			online = append(online, id)
		}
	}
	return online
}

// SendPromptAndWait sends a prompt to an edge agent and waits for the response
// This is used to forward LLM prompts to edge agents that run their own tool loops
func (m *MQTTChannel) SendPromptAndWait(ctx context.Context, agentID, prompt, systemPrompt string, timeout time.Duration) (*EdgeAgentResponse, error) {
	if !m.client.IsConnected() {
		return nil, fmt.Errorf("mqtt not connected")
	}

	// Check if agent is online
	if !m.IsEdgeAgentOnline(agentID) {
		return nil, fmt.Errorf("edge agent %s is not online", agentID)
	}

	// Generate unique request ID
	requestID := fmt.Sprintf("prompt-%d", time.Now().UnixNano())

	// Create response channel
	respChan := make(chan *EdgeAgentResponse, 1)

	// Register pending request
	m.pendingRequestsMu.Lock()
	m.pendingRequests[requestID] = &PendingRequest{
		RequestID: requestID,
		Response:  respChan,
		CreatedAt: time.Now(),
	}
	m.pendingRequestsMu.Unlock()

	// Cleanup on exit
	defer func() {
		m.pendingRequestsMu.Lock()
		delete(m.pendingRequests, requestID)
		m.pendingRequestsMu.Unlock()
	}()

	// Build command for edge agent
	cmd := EdgeAgentCommand{
		Command:   "prompt",
		RequestID: requestID,
		Payload: map[string]interface{}{
			"prompt":        prompt,
			"system_prompt": systemPrompt,
			"sent_at":       time.Now().Unix(),
		},
	}

	// Serialize and publish
	topic := fmt.Sprintf(commandsTopic, agentID)
	payload, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("marshal command: %w", err)
	}

	token := m.client.Publish(topic, 1, false, payload)
	if !token.WaitTimeout(5 * time.Second) {
		return nil, fmt.Errorf("publish timeout")
	}
	if err := token.Error(); err != nil {
		return nil, fmt.Errorf("publish: %w", err)
	}

	m.logger.Info("prompt sent to edge agent",
		"agent", agentID,
		"request_id", requestID,
		"prompt_length", len(prompt),
	)

	// Wait for response with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case resp := <-respChan:
		return resp, nil
	case <-timeoutCtx.Done():
		return nil, fmt.Errorf("timeout waiting for response from %s", agentID)
	}
}

// handleEdgeAgentResponse routes LLM responses to pending requests
func (m *MQTTChannel) handleEdgeAgentResponse(payload map[string]interface{}) bool {
	requestID, ok := payload["request_id"].(string)
	if !ok || requestID == "" {
		return false
	}

	m.pendingRequestsMu.RLock()
	pending, exists := m.pendingRequests[requestID]
	m.pendingRequestsMu.RUnlock()

	if !exists {
		return false
	}

	// Build response
	resp := &EdgeAgentResponse{
		RequestID: requestID,
	}

	if agentID, ok := payload["agent_id"].(string); ok {
		resp.AgentID = agentID
	}
	if content, ok := payload["content"].(string); ok {
		resp.Content = content
	}
	if model, ok := payload["model"].(string); ok {
		resp.Model = model
	}
	if status, ok := payload["status"].(string); ok {
		resp.Status = status
	} else {
		resp.Status = "success" // Default to success if not specified
	}
	if errMsg, ok := payload["error"].(string); ok {
		resp.Error = errMsg
		resp.Status = "error"
	}
	if metadata, ok := payload["metadata"].(map[string]interface{}); ok {
		resp.Metadata = metadata
	}

	// Send to waiting goroutine
	select {
	case pending.Response <- resp:
		m.logger.Debug("response delivered to pending request",
			"request_id", requestID,
			"agent", resp.AgentID,
			"status", resp.Status,
		)
		return true
	default:
		m.logger.Warn("pending request channel full or closed",
			"request_id", requestID,
		)
		return false
	}
}
