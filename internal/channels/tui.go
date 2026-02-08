package channels

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/clawinfra/evoclaw/internal/orchestrator"
)

// ─────────────────────────────────────────────────────
// TUI Channel — implements orchestrator.Channel
// ─────────────────────────────────────────────────────

// TUIChannel bridges the Bubble Tea TUI with the EvoClaw orchestrator.
// It implements the Channel interface so the orchestrator treats it
// like any other messaging channel (Telegram, MQTT, etc).
type TUIChannel struct {
	logger   *slog.Logger
	inbox    chan orchestrator.Message
	outbox   chan string // responses rendered into chat
	ctx      context.Context
	cancel   context.CancelFunc
	program  *tea.Program
	mu       sync.Mutex
	agentsFn func() []orchestrator.AgentInfo // callback to get live agent state
}

// NewTUI creates a new terminal UI channel.
// agentsFn is called periodically to refresh the agent sidebar.
func NewTUI(logger *slog.Logger, agentsFn func() []orchestrator.AgentInfo) *TUIChannel {
	return &TUIChannel{
		logger:   logger.With("channel", "tui"),
		inbox:    make(chan orchestrator.Message, 100),
		outbox:   make(chan string, 100),
		agentsFn: agentsFn,
	}
}

func (t *TUIChannel) Name() string { return "tui" }

func (t *TUIChannel) Start(ctx context.Context) error {
	t.ctx, t.cancel = context.WithCancel(ctx)

	model := newTUIModel(t)
	t.program = tea.NewProgram(model, tea.WithAltScreen())

	// Run the TUI in a goroutine — it blocks on stdin
	go func() {
		if _, err := t.program.Run(); err != nil {
			t.logger.Error("TUI crashed", "error", err)
		}
		// TUI exited (user pressed ctrl+c) — cancel the channel context
		t.cancel()
	}()

	t.logger.Info("TUI channel started")
	return nil
}

func (t *TUIChannel) Stop() error {
	if t.program != nil {
		t.program.Quit()
	}
	if t.cancel != nil {
		t.cancel()
	}
	return nil
}

func (t *TUIChannel) Send(_ context.Context, msg orchestrator.Response) error {
	// Push response text into the TUI's chat viewport
	t.outbox <- msg.Content
	if t.program != nil {
		t.program.Send(agentResponseMsg{content: msg.Content, agentID: msg.AgentID})
	}
	return nil
}

func (t *TUIChannel) Receive() <-chan orchestrator.Message { return t.inbox }

// sendUserMessage is called from the TUI model when the user presses Enter
func (t *TUIChannel) sendUserMessage(text string) {
	t.inbox <- orchestrator.Message{
		ID:        fmt.Sprintf("tui-%d", time.Now().UnixNano()),
		Channel:   "tui",
		From:      "user",
		Content:   text,
		Timestamp: time.Now(),
	}
}

// ─────────────────────────────────────────────────────
// Bubble Tea messages
// ─────────────────────────────────────────────────────

type agentResponseMsg struct {
	content string
	agentID string
}

type tickMsg struct{}

// ─────────────────────────────────────────────────────
// Styles
// ─────────────────────────────────────────────────────

var (
	// Colors
	primaryColor   = lipgloss.Color("#7C3AED") // violet
	secondaryColor = lipgloss.Color("#06B6D4") // cyan
	mutedColor     = lipgloss.Color("#6B7280") // gray
	successColor   = lipgloss.Color("#10B981") // green
	errorColor     = lipgloss.Color("#EF4444") // red
	warnColor      = lipgloss.Color("#F59E0B") // amber
	bgColor        = lipgloss.Color("#1F2937") // dark

	// Sidebar styles
	sidebarStyle = lipgloss.NewStyle().
			Width(28).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor).
			Padding(1, 1)

	sidebarTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			MarginBottom(1)

	agentOnline = lipgloss.NewStyle().
			Foreground(successColor)

	agentOffline = lipgloss.NewStyle().
			Foreground(mutedColor)

	agentLabel = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E5E7EB"))

	metricStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			PaddingLeft(2)

	// Chat styles
	chatBorder = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(secondaryColor)

	userMsg = lipgloss.NewStyle().
		Foreground(secondaryColor).
		Bold(true)

	agentMsg = lipgloss.NewStyle().
			Foreground(successColor).
			Bold(true)

	chatText = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E5E7EB"))

	// Header/footer
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(primaryColor).
			Padding(0, 1)

	footerStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	statusOnline = lipgloss.NewStyle().
			Foreground(successColor).
			Bold(true)
)

// ─────────────────────────────────────────────────────
// TUI Model
// ─────────────────────────────────────────────────────

type tuiModel struct {
	channel  *TUIChannel
	input    textarea.Model
	chat     viewport.Model
	messages []chatEntry
	width    int
	height   int
	ready    bool
}

type chatEntry struct {
	sender  string // "user" or agent ID
	content string
	time    time.Time
	isUser  bool
}

func newTUIModel(ch *TUIChannel) tuiModel {
	ti := textarea.New()
	ti.Placeholder = "Type a message..."
	ti.Focus()
	ti.CharLimit = 4096
	ti.SetHeight(3)
	ti.ShowLineNumbers = false
	ti.KeyMap.InsertNewline.SetEnabled(false) // Enter sends, Shift+Enter for newline

	return tuiModel{
		channel:  ch,
		input:    ti,
		messages: []chatEntry{},
	}
}

func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		tickCmd(),
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "enter":
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, nil
			}

			// Add to chat
			m.messages = append(m.messages, chatEntry{
				sender:  "You",
				content: text,
				time:    time.Now(),
				isUser:  true,
			})

			// Send to orchestrator via channel
			m.channel.sendUserMessage(text)

			// Clear input
			m.input.Reset()

			// Scroll chat to bottom
			m.chat.SetContent(m.renderChat())
			m.chat.GotoBottom()

			return m, nil
		}

	case agentResponseMsg:
		m.messages = append(m.messages, chatEntry{
			sender:  msg.agentID,
			content: msg.content,
			time:    time.Now(),
			isUser:  false,
		})
		m.chat.SetContent(m.renderChat())
		m.chat.GotoBottom()
		return m, nil

	case tickMsg:
		// Refresh sidebar (agent status updates)
		cmds = append(cmds, tickCmd())

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		sidebarW := 30
		chatW := m.width - sidebarW - 3 // 3 for borders/gap
		chatH := m.height - 8            // header + input + footer

		if !m.ready {
			m.chat = viewport.New(chatW, chatH)
			m.chat.SetContent(m.renderChat())
			m.ready = true
		} else {
			m.chat.Width = chatW
			m.chat.Height = chatH
			m.chat.SetContent(m.renderChat())
		}

		m.input.SetWidth(chatW - 2)
	}

	// Update sub-components
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)

	m.chat, cmd = m.chat.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m tuiModel) View() string {
	if !m.ready {
		return "Initializing EvoClaw TUI..."
	}

	// Header
	header := headerStyle.Width(m.width).Render(
		"  🧬 EvoClaw Terminal  " + statusOnline.Render("● ONLINE"),
	)

	// Sidebar
	sidebar := m.renderSidebar()

	// Chat area (viewport + input)
	chatArea := chatBorder.Width(m.width - 33).Render(m.chat.View())
	inputArea := m.input.View()

	// Compose right pane
	rightPane := lipgloss.JoinVertical(lipgloss.Left, chatArea, inputArea)

	// Compose main body
	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, " ", rightPane)

	// Footer
	footer := footerStyle.Render(
		"  Enter: send │ Ctrl+C: quit │ ↑↓: scroll chat",
	)

	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

// ─────────────────────────────────────────────────────
// Rendering helpers
// ─────────────────────────────────────────────────────

func (m tuiModel) renderSidebar() string {
	var sb strings.Builder

	sb.WriteString(sidebarTitle.Render("  Agents"))
	sb.WriteString("\n")

	// Get live agent data
	var agents []orchestrator.AgentInfo
	if m.channel.agentsFn != nil {
		agents = m.channel.agentsFn()
	}

	if len(agents) == 0 {
		sb.WriteString(agentOffline.Render("  No agents registered"))
	}

	for _, a := range agents {
		// Status indicator
		var indicator string
		switch a.Status {
		case "running":
			indicator = agentOnline.Render("●")
		case "evolving":
			indicator = lipgloss.NewStyle().Foreground(warnColor).Render("◉")
		default:
			indicator = agentOffline.Render("○")
		}

		name := agentLabel.Render(a.ID)
		sb.WriteString(fmt.Sprintf("  %s %s\n", indicator, name))

		// Metrics
		sb.WriteString(metricStyle.Render(fmt.Sprintf("status: %s", a.Status)))
		sb.WriteString("\n")
		sb.WriteString(metricStyle.Render(fmt.Sprintf("msgs: %d", a.MessageCount)))
		sb.WriteString("\n")

		if a.Metrics.AvgResponseMs > 0 {
			sb.WriteString(metricStyle.Render(fmt.Sprintf("avg: %.0fms", a.Metrics.AvgResponseMs)))
			sb.WriteString("\n")
		}

		if a.Metrics.TokensUsed > 0 {
			tokens := a.Metrics.TokensUsed
			if tokens > 1000 {
				sb.WriteString(metricStyle.Render(fmt.Sprintf("tokens: %dk", tokens/1000)))
			} else {
				sb.WriteString(metricStyle.Render(fmt.Sprintf("tokens: %d", tokens)))
			}
			sb.WriteString("\n")
		}

		if a.Metrics.CostUSD > 0 {
			sb.WriteString(metricStyle.Render(fmt.Sprintf("cost: $%.3f", a.Metrics.CostUSD)))
			sb.WriteString("\n")
		}

		// Uptime
		uptime := time.Since(a.StartedAt)
		sb.WriteString(metricStyle.Render(fmt.Sprintf("up: %s", formatDuration(uptime))))
		sb.WriteString("\n\n")
	}

	// Summary stats at bottom
	sb.WriteString(sidebarTitle.Render("  System"))
	sb.WriteString("\n")

	totalMsgs := int64(0)
	totalCost := 0.0
	online := 0
	for _, a := range agents {
		totalMsgs += a.MessageCount
		totalCost += a.Metrics.CostUSD
		if a.Status != "idle" || a.MessageCount > 0 {
			online++
		}
	}

	sb.WriteString(metricStyle.Render(fmt.Sprintf("agents: %d/%d", online, len(agents))))
	sb.WriteString("\n")
	sb.WriteString(metricStyle.Render(fmt.Sprintf("total msgs: %d", totalMsgs)))
	sb.WriteString("\n")
	if totalCost > 0 {
		sb.WriteString(metricStyle.Render(fmt.Sprintf("total cost: $%.3f", totalCost)))
		sb.WriteString("\n")
	}

	return sidebarStyle.Height(m.height - 4).Render(sb.String())
}

func (m tuiModel) renderChat() string {
	if len(m.messages) == 0 {
		return lipgloss.NewStyle().
			Foreground(mutedColor).
			Padding(1).
			Render("No messages yet. Start typing to chat with your agent.")
	}

	var sb strings.Builder
	for _, entry := range m.messages {
		ts := entry.time.Format("15:04")
		timeStr := lipgloss.NewStyle().Foreground(mutedColor).Render(ts)

		if entry.isUser {
			sender := userMsg.Render("[You]")
			sb.WriteString(fmt.Sprintf("%s %s %s\n", timeStr, sender, chatText.Render(entry.content)))
		} else {
			sender := agentMsg.Render(fmt.Sprintf("[%s]", entry.sender))
			// Word-wrap long responses
			content := entry.content
			sb.WriteString(fmt.Sprintf("%s %s\n%s\n", timeStr, sender, chatText.Render(content)))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	return fmt.Sprintf("%dd %dh", days, hours)
}
