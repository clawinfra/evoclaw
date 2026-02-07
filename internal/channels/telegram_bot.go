package channels

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/clawinfra/evoclaw/internal/orchestrator"
)

// TelegramBot wraps TelegramChannel with bot command handling and LLM routing.
// It intercepts incoming messages, handles /commands, and routes regular messages
// to agents via ChatSync.
type TelegramBot struct {
	channel      *TelegramChannel
	orch         *orchestrator.Orchestrator
	logger       *slog.Logger
	defaultAgent string
	allowedUsers []int64
	// Per-user agent selection
	userAgents map[string]string
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// NewTelegramBot creates a new Telegram bot with command handling
func NewTelegramBot(
	channel *TelegramChannel,
	orch *orchestrator.Orchestrator,
	defaultAgent string,
	allowedUsers []int64,
	logger *slog.Logger,
) *TelegramBot {
	return &TelegramBot{
		channel:      channel,
		orch:         orch,
		logger:       logger.With("component", "telegram-bot"),
		defaultAgent: defaultAgent,
		allowedUsers: allowedUsers,
		userAgents:   make(map[string]string),
	}
}

// Start begins the bot message processing loop
func (b *TelegramBot) Start(ctx context.Context) error {
	b.ctx, b.cancel = context.WithCancel(ctx)

	b.wg.Add(1)
	go b.processLoop()

	b.logger.Info("telegram bot started",
		"defaultAgent", b.defaultAgent,
		"allowedUsers", len(b.allowedUsers),
	)
	return nil
}

// Stop shuts down the bot processing loop
func (b *TelegramBot) Stop() {
	if b.cancel != nil {
		b.cancel()
	}
	b.wg.Wait()
	b.logger.Info("telegram bot stopped")
}

// processLoop reads messages from the channel and handles them
func (b *TelegramBot) processLoop() {
	defer b.wg.Done()

	for {
		select {
		case <-b.ctx.Done():
			return
		case msg, ok := <-b.channel.Receive():
			if !ok {
				return
			}
			b.handleMessage(msg)
		}
	}
}

// handleMessage dispatches a message to the appropriate handler
func (b *TelegramBot) handleMessage(msg orchestrator.Message) {
	// Check allowed users
	if !b.isUserAllowed(msg.From) {
		b.logger.Warn("message from unauthorized user", "user", msg.From, "username", msg.Metadata["username"])
		return
	}

	content := strings.TrimSpace(msg.Content)
	chatID := msg.To // Telegram chat ID for responses

	// Parse commands
	if strings.HasPrefix(content, "/") {
		b.handleCommand(content, chatID, msg)
		return
	}

	// Regular message → route to agent LLM
	b.handleChatMessage(content, chatID, msg)
}

// isUserAllowed checks if a user is in the allowed list (empty = allow all)
func (b *TelegramBot) isUserAllowed(userID string) bool {
	if len(b.allowedUsers) == 0 {
		return true
	}

	uid, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		return false
	}

	for _, allowed := range b.allowedUsers {
		if allowed == uid {
			return true
		}
	}
	return false
}

// ParseCommand extracts the command name and arguments from a message.
// Handles formats like: /command, /command args, /command@botname args
// Exported for testing.
func ParseCommand(text string) (command string, args string) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return "", text
	}

	// Remove leading /
	text = text[1:]

	// Split on first space
	parts := strings.SplitN(text, " ", 2)
	cmd := parts[0]

	// Remove @botname from command if present
	if atIdx := strings.Index(cmd, "@"); atIdx > 0 {
		cmd = cmd[:atIdx]
	}

	command = strings.ToLower(cmd)
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}

	return command, args
}

// handleCommand processes bot commands
func (b *TelegramBot) handleCommand(content, chatID string, msg orchestrator.Message) {
	command, args := ParseCommand(content)

	b.logger.Info("bot command received",
		"command", command,
		"args", args,
		"user", msg.Metadata["username"],
	)

	switch command {
	case "start":
		b.sendText(chatID, "🧬 *EvoClaw Bot*\n\nI'm your agent orchestrator. Talk to me and I'll route your messages to the right agent.\n\n"+
			"Commands:\n"+
			"/status — System status\n"+
			"/agents — List agents\n"+
			"/agent <id> — Switch to an agent\n"+
			"/skills <agent_id> — List agent skills\n"+
			"/ask <question> — Ask via current agent\n"+
			"/help — Show this help")

	case "help":
		b.sendText(chatID, "🤖 *Commands:*\n\n"+
			"/status — Show all agents and their status\n"+
			"/agents — List available agents\n"+
			"/agent <id> — Switch to talking to a specific agent\n"+
			"/skills <agent_id> — List an agent's skills\n"+
			"/ask <question> — Ask a question through an agent\n\n"+
			"Or just type a message and it will be routed to your current agent.")

	case "status":
		b.handleStatusCommand(chatID)

	case "agents":
		b.handleAgentsCommand(chatID)

	case "agent":
		b.handleAgentSwitchCommand(chatID, args, msg.From)

	case "skills":
		b.handleSkillsCommand(chatID, args)

	case "ask":
		if args == "" {
			b.sendText(chatID, "Usage: /ask <your question>")
			return
		}
		b.handleChatMessage(args, chatID, msg)

	default:
		b.sendText(chatID, fmt.Sprintf("Unknown command: /%s\nUse /help for available commands.", command))
	}
}

// handleStatusCommand shows system status
func (b *TelegramBot) handleStatusCommand(chatID string) {
	agents := b.orch.ListAgents()

	text := "📊 *System Status*\n\n"
	text += fmt.Sprintf("Agents: %d\n\n", len(agents))

	for _, a := range agents {
		emoji := "🟢"
		switch a.Status {
		case "error":
			emoji = "🔴"
		case "evolving":
			emoji = "🟡"
		case "running":
			emoji = "🔵"
		}
		text += fmt.Sprintf("%s *%s* (%s)\n", emoji, a.ID, a.Def.Type)
		text += fmt.Sprintf("   Model: `%s`\n", a.Def.Model)
		text += fmt.Sprintf("   Messages: %d | Errors: %d\n", a.MessageCount, a.ErrorCount)
		if a.Metrics.TotalActions > 0 {
			successRate := float64(a.Metrics.SuccessfulActions) / float64(a.Metrics.TotalActions) * 100
			text += fmt.Sprintf("   Success: %.1f%% | Avg: %.0fms\n", successRate, a.Metrics.AvgResponseMs)
		}
		text += "\n"
	}

	b.sendMarkdown(chatID, text)
}

// handleAgentsCommand lists available agents
func (b *TelegramBot) handleAgentsCommand(chatID string) {
	agents := b.orch.ListAgents()

	if len(agents) == 0 {
		b.sendText(chatID, "No agents registered.")
		return
	}

	text := "🤖 *Available Agents:*\n\n"
	for _, a := range agents {
		text += fmt.Sprintf("• `%s` — %s (%s)\n", a.ID, a.Def.Name, a.Def.Type)
	}
	text += fmt.Sprintf("\nUse /agent <id> to switch.\nCurrent: `%s`", b.defaultAgent)

	b.sendMarkdown(chatID, text)
}

// handleAgentSwitchCommand switches the user's active agent
func (b *TelegramBot) handleAgentSwitchCommand(chatID, agentID, userID string) {
	if agentID == "" {
		b.sendText(chatID, "Usage: /agent <agent_id>")
		return
	}

	// Verify agent exists
	info := b.orch.GetAgentInfo(agentID)
	if info == nil {
		b.sendText(chatID, fmt.Sprintf("❌ Agent not found: %s\nUse /agents to see available agents.", agentID))
		return
	}

	b.mu.Lock()
	b.userAgents[userID] = agentID
	b.mu.Unlock()

	b.sendMarkdown(chatID, fmt.Sprintf("✅ Switched to agent `%s` (%s)", agentID, info.Def.Type))
}

// handleSkillsCommand lists an agent's skills
func (b *TelegramBot) handleSkillsCommand(chatID, agentID string) {
	if agentID == "" {
		agentID = b.defaultAgent
	}

	info := b.orch.GetAgentInfo(agentID)
	if info == nil {
		b.sendText(chatID, fmt.Sprintf("❌ Agent not found: %s", agentID))
		return
	}

	text := fmt.Sprintf("🎯 *Skills for %s:*\n\n", agentID)
	if len(info.Def.Skills) == 0 {
		text += "No skills configured."
	} else {
		for _, skill := range info.Def.Skills {
			text += fmt.Sprintf("• `%s`\n", skill)
		}
	}

	b.sendMarkdown(chatID, text)
}

// handleChatMessage sends a regular message to an agent via ChatSync
func (b *TelegramBot) handleChatMessage(content, chatID string, msg orchestrator.Message) {
	// Determine which agent to use
	agentID := b.getAgentForUser(msg.From)

	b.logger.Info("routing chat message",
		"user", msg.Metadata["username"],
		"agent", agentID,
		"length", len(content),
	)

	// Call ChatSync
	req := orchestrator.ChatSyncRequest{
		AgentID: agentID,
		UserID:  msg.From,
		Message: content,
	}

	resp, err := b.orch.ChatSync(b.ctx, req)
	if err != nil {
		b.logger.Error("chat sync failed", "error", err)
		b.sendText(chatID, fmt.Sprintf("❌ Error: %v", err))
		return
	}

	// Send response back via Telegram
	responseText := resp.Response
	if responseText == "" {
		responseText = "(empty response)"
	}

	// Add metadata footer
	footer := fmt.Sprintf("\n\n_— %s via %s (%dms)_", agentID, resp.Model, resp.ElapsedMs)

	b.sendMarkdown(chatID, responseText+footer)
}

// getAgentForUser returns the agent assigned to a user, or the default
func (b *TelegramBot) getAgentForUser(userID string) string {
	b.mu.RLock()
	agent, ok := b.userAgents[userID]
	b.mu.RUnlock()

	if ok {
		return agent
	}
	return b.defaultAgent
}

// sendText sends a plain text message
func (b *TelegramBot) sendText(chatID, text string) {
	b.sendMessage(chatID, text, "")
}

// sendMarkdown sends a Markdown-formatted message
func (b *TelegramBot) sendMarkdown(chatID, text string) {
	b.sendMessage(chatID, text, "Markdown")
}

// sendMessage sends a message via the Telegram API
func (b *TelegramBot) sendMessage(chatID, text, parseMode string) {
	params := url.Values{}
	params.Set("chat_id", chatID)
	params.Set("text", text)
	if parseMode != "" {
		params.Set("parse_mode", parseMode)
	}

	apiURL := fmt.Sprintf("%s%s/sendMessage", telegramAPIURL, b.channel.botToken)
	req, err := http.NewRequestWithContext(b.ctx, "POST", apiURL, nil)
	if err != nil {
		b.logger.Error("failed to create request", "error", err)
		return
	}
	req.URL.RawQuery = params.Encode()

	resp, err := b.channel.client.Do(req)
	if err != nil {
		b.logger.Error("failed to send message", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b.logger.Error("telegram API error", "status", resp.StatusCode)
		// If markdown fails, retry as plain text
		if parseMode == "Markdown" {
			b.sendText(chatID, text)
		}
	}
}
