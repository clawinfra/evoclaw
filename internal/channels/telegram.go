package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/clawinfra/evoclaw/internal/orchestrator"
)

const (
	telegramAPIURL = "https://api.telegram.org/bot"
	pollTimeout    = 60 // long polling timeout in seconds
)

// TelegramChannel implements the Channel interface for Telegram Bot API
type TelegramChannel struct {
	botToken string
	logger   *slog.Logger
	inbox    chan orchestrator.Message
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	client   HTTPClient
	offset   int64 // for long polling
}

// NewTelegram creates a new Telegram channel adapter
func NewTelegram(botToken string, logger *slog.Logger) *TelegramChannel {
	return NewTelegramWithClient(botToken, logger, &DefaultHTTPClient{
		client: &http.Client{
			Timeout: time.Second * 70, // slightly longer than pollTimeout
		},
	})
}

// NewTelegramWithClient creates a Telegram channel with a custom HTTP client (for testing)
func NewTelegramWithClient(botToken string, logger *slog.Logger, client HTTPClient) *TelegramChannel {
	return &TelegramChannel{
		botToken: botToken,
		logger:   logger.With("channel", "telegram"),
		inbox:    make(chan orchestrator.Message, 100),
		client:   client,
	}
}

func (t *TelegramChannel) Name() string {
	return "telegram"
}

func (t *TelegramChannel) Start(ctx context.Context) error {
	t.ctx, t.cancel = context.WithCancel(ctx)

	// Verify bot token by getting bot info
	if err := t.verifyToken(); err != nil {
		return fmt.Errorf("verify token: %w", err)
	}

	// Start polling loop
	t.wg.Add(1)
	go t.pollLoop()

	t.logger.Info("telegram channel started")
	return nil
}

func (t *TelegramChannel) Stop() error {
	t.logger.Info("stopping telegram channel")
	if t.cancel != nil {
		t.cancel()
	}
	t.wg.Wait()
	close(t.inbox)
	return nil
}

func (t *TelegramChannel) Send(ctx context.Context, msg orchestrator.Response) error {
	params := url.Values{}
	params.Set("chat_id", msg.To)
	params.Set("text", msg.Content)

	if msg.ReplyTo != "" {
		params.Set("reply_to_message_id", msg.ReplyTo)
	}

	apiURL := fmt.Sprintf("%s%s/sendMessage", telegramAPIURL, t.botToken)
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.URL.RawQuery = params.Encode()

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram api error: %s (status %d)", body, resp.StatusCode)
	}

	t.logger.Debug("message sent", "to", msg.To, "length", len(msg.Content))
	return nil
}

func (t *TelegramChannel) Receive() <-chan orchestrator.Message {
	return t.inbox
}

// verifyToken checks that the bot token is valid
func (t *TelegramChannel) verifyToken() error {
	apiURL := fmt.Sprintf("%s%s/getMe", telegramAPIURL, t.botToken)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	
	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("get bot info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("invalid token: %s (status %d)", body, resp.StatusCode)
	}

	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			Username  string `json:"username"`
			FirstName string `json:"first_name"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if !result.OK {
		return fmt.Errorf("api returned ok=false")
	}

	t.logger.Info("bot verified", "username", result.Result.Username, "name", result.Result.FirstName)
	return nil
}

// pollLoop continuously polls for new messages using long polling
func (t *TelegramChannel) pollLoop() {
	defer t.wg.Done()

	t.logger.Info("starting poll loop")

	for {
		select {
		case <-t.ctx.Done():
			t.logger.Info("poll loop stopped")
			return
		default:
			if err := t.pollOnce(); err != nil {
				t.logger.Error("poll error", "error", err)
				// Back off on error
				select {
				case <-t.ctx.Done():
					return
				case <-time.After(time.Second * 5):
				}
			}
		}
	}
}

// pollOnce performs a single getUpdates call
func (t *TelegramChannel) pollOnce() error {
	params := url.Values{}
	params.Set("offset", strconv.FormatInt(t.offset, 10))
	params.Set("timeout", strconv.Itoa(pollTimeout))
	params.Set("allowed_updates", `["message"]`)

	apiURL := fmt.Sprintf("%s%s/getUpdates?%s", telegramAPIURL, t.botToken, params.Encode())

	req, err := http.NewRequestWithContext(t.ctx, "GET", apiURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("get updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("api error: %s (status %d)", body, resp.StatusCode)
	}

	var result struct {
		OK     bool             `json:"ok"`
		Result []TelegramUpdate `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if !result.OK {
		return fmt.Errorf("api returned ok=false")
	}

	// Process each update
	for _, update := range result.Result {
		// Update offset to acknowledge this update
		if int64(update.UpdateID) >= t.offset {
			t.offset = int64(update.UpdateID) + 1
		}

		// Only process text messages for now
		if update.Message == nil || update.Message.Text == "" {
			continue
		}

		msg := orchestrator.Message{
			ID:        strconv.FormatInt(int64(update.Message.MessageID), 10),
			Channel:   "telegram",
			From:      strconv.FormatInt(update.Message.From.ID, 10),
			To:        strconv.FormatInt(update.Message.Chat.ID, 10),
			Content:   update.Message.Text,
			Timestamp: time.Unix(int64(update.Message.Date), 0),
			Metadata: map[string]string{
				"username":   update.Message.From.Username,
				"first_name": update.Message.From.FirstName,
				"chat_type":  update.Message.Chat.Type,
			},
		}

		if update.Message.ReplyToMessage != nil {
			msg.ReplyTo = strconv.FormatInt(int64(update.Message.ReplyToMessage.MessageID), 10)
		}

		select {
		case t.inbox <- msg:
			t.logger.Debug("message received",
				"from", msg.Metadata["username"],
				"chat", msg.To,
				"text", msg.Content,
			)
		case <-t.ctx.Done():
			return nil
		}
	}

	return nil
}

// TelegramUpdate represents an update from the Telegram API
type TelegramUpdate struct {
	UpdateID int              `json:"update_id"`
	Message  *TelegramMessage `json:"message,omitempty"`
}

// TelegramMessage represents a message from Telegram
type TelegramMessage struct {
	MessageID      int              `json:"message_id"`
	From           TelegramUser     `json:"from"`
	Chat           TelegramChat     `json:"chat"`
	Date           int              `json:"date"`
	Text           string           `json:"text,omitempty"`
	ReplyToMessage *TelegramMessage `json:"reply_to_message,omitempty"`
}

// TelegramUser represents a Telegram user
type TelegramUser struct {
	ID        int64  `json:"id"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
}

// TelegramChat represents a Telegram chat
type TelegramChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"` // "private", "group", "supergroup", "channel"
}
