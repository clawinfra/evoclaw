package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/clawinfra/evoclaw/internal/types"
)

const (
	telegramAPIURL = "https://api.telegram.org/bot"
	pollTimeout    = 60 // long polling timeout in seconds
)

// TelegramChannel implements the Channel interface for Telegram Bot API
type TelegramChannel struct {
	botToken string
	logger   *slog.Logger
	inbox    chan types.Message
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
		inbox:    make(chan types.Message, 100),
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

// Send sends a response via Telegram. Supports:
//   - Plain text messages
//   - Photo (URLs ending in .jpg/.png/.gif/.webp)
//   - Document (file:// URLs or other paths)
//   - Voice (URLs ending in .ogg/.mp3/.m4a)
//   - Inline keyboard buttons
//   - Edit existing messages (EditMessageID > 0)
//   - Reply-to (ReplyToID > 0)
func (t *TelegramChannel) Send(ctx context.Context, msg types.Response) error {
	content := msg.Content

	// Detect media type by URL suffix
	lower := strings.ToLower(content)
	isPhoto := strings.HasPrefix(lower, "http") &&
		(strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg") ||
			strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".gif") ||
			strings.HasSuffix(lower, ".webp"))
	isVoice := strings.HasPrefix(lower, "http") &&
		(strings.HasSuffix(lower, ".ogg") || strings.HasSuffix(lower, ".mp3") ||
			strings.HasSuffix(lower, ".m4a"))
	isDocument := strings.HasPrefix(lower, "http") && !isPhoto && !isVoice &&
		strings.Contains(lower, ".")

	// --- Edit existing message ---
	if msg.EditMessageID > 0 {
		return t.editMessage(ctx, msg.To, msg.EditMessageID, content)
	}

	// --- Photo ---
	if isPhoto {
		return t.sendPhoto(ctx, msg.To, content, msg.ReplyToID)
	}

	// --- Voice ---
	if isVoice {
		return t.sendVoice(ctx, msg.To, content, msg.ReplyToID)
	}

	// --- Document (non-photo, non-voice remote URL) ---
	if isDocument {
		return t.sendDocument(ctx, msg.To, content, msg.ReplyToID)
	}

	// --- Plain text (with optional inline keyboard) ---
	return t.sendMessage(ctx, msg.To, content, msg.ReplyTo, msg.ReplyToID, msg.Buttons)
}

// sendMessage sends a plain-text message with optional inline keyboard.
// replyTo is the legacy string reply (from types.Response.ReplyTo);
// replyToID is the int64 version (from types.Response.ReplyToID).
func (t *TelegramChannel) sendMessage(ctx context.Context, chatID, text, replyTo string, replyToID int64, buttons [][]types.Button) error {
	// Build JSON body for richer payloads (inline keyboard / parse_mode)
	body := map[string]interface{}{
		"chat_id": chatID,
		"text":    text,
	}

	// Reply-to: prefer typed ReplyToID, fall back to legacy string ReplyTo
	if replyToID > 0 {
		body["reply_to_message_id"] = replyToID
	} else if replyTo != "" {
		body["reply_to_message_id"] = replyTo
	}

	// Inline keyboard
	if len(buttons) > 0 {
		keyboard := buildInlineKeyboard(buttons)
		body["reply_markup"] = keyboard
	}

	return t.postJSON(ctx, "sendMessage", body)
}

// sendPhoto sends a photo by URL
func (t *TelegramChannel) sendPhoto(ctx context.Context, chatID, photoURL string, replyToID int64) error {
	body := map[string]interface{}{
		"chat_id": chatID,
		"photo":   photoURL,
	}
	if replyToID > 0 {
		body["reply_to_message_id"] = replyToID
	}
	return t.postJSON(ctx, "sendPhoto", body)
}

// sendVoice sends a voice message by URL
func (t *TelegramChannel) sendVoice(ctx context.Context, chatID, voiceURL string, replyToID int64) error {
	body := map[string]interface{}{
		"chat_id": chatID,
		"voice":   voiceURL,
	}
	if replyToID > 0 {
		body["reply_to_message_id"] = replyToID
	}
	return t.postJSON(ctx, "sendVoice", body)
}

// sendDocument sends a document by URL
func (t *TelegramChannel) sendDocument(ctx context.Context, chatID, docURL string, replyToID int64) error {
	body := map[string]interface{}{
		"chat_id":  chatID,
		"document": docURL,
	}
	if replyToID > 0 {
		body["reply_to_message_id"] = replyToID
	}
	return t.postJSON(ctx, "sendDocument", body)
}

// editMessage edits an existing message
func (t *TelegramChannel) editMessage(ctx context.Context, chatID string, messageID int64, newText string) error {
	body := map[string]interface{}{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       newText,
	}
	return t.postJSON(ctx, "editMessageText", body)
}

// postJSON posts a JSON body to a Telegram Bot API method
func (t *TelegramChannel) postJSON(ctx context.Context, method string, body map[string]interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	apiURL := fmt.Sprintf("%s%s/%s", telegramAPIURL, t.botToken, method)
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram api error (%s): %s (status %d)", method, b, resp.StatusCode)
	}

	t.logger.Debug("api call succeeded", "method", method, "to", body["chat_id"])
	return nil
}

// buildInlineKeyboard converts [][]types.Button into Telegram inline_keyboard JSON structure
func buildInlineKeyboard(rows [][]types.Button) map[string]interface{} {
	keyboard := make([][]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		kbRow := make([]map[string]interface{}, 0, len(row))
		for _, btn := range row {
			b := map[string]interface{}{
				"text": btn.Text,
			}
			if btn.URL != "" {
				b["url"] = btn.URL
			} else {
				b["callback_data"] = btn.CallbackData
			}
			kbRow = append(kbRow, b)
		}
		keyboard = append(keyboard, kbRow)
	}
	return map[string]interface{}{
		"inline_keyboard": keyboard,
	}
}

func (t *TelegramChannel) Receive() <-chan types.Message {
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
	defer resp.Body.Close() //nolint:errcheck

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
	// Include callback_query and message updates
	params.Set("allowed_updates", `["message","callback_query"]`)

	apiURL := fmt.Sprintf("%s%s/getUpdates?%s", telegramAPIURL, t.botToken, params.Encode())

	req, err := http.NewRequestWithContext(t.ctx, "GET", apiURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("get updates: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

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

		// Only process messages that have text (skip stickers, photos without caption, etc.)
		if update.Message == nil || update.Message.Text == "" {
			continue
		}

		msg := t.buildMessage(update.Message)

		select {
		case t.inbox <- msg:
			t.logger.Debug("message received",
				"from", msg.Metadata["username"],
				"chat", msg.To,
				"chat_type", msg.ChatType,
				"text", msg.Content,
			)
		case <-t.ctx.Done():
			return nil
		}
	}

	return nil
}

// buildMessage converts a raw TelegramMessage into a types.Message, populating
// all new fields: Command, Args, ChatType, ThreadID.
func (t *TelegramChannel) buildMessage(tm *TelegramMessage) types.Message {
	msg := types.Message{
		ID:        strconv.FormatInt(int64(tm.MessageID), 10),
		Channel:   "telegram",
		From:      strconv.FormatInt(tm.From.ID, 10),
		To:        strconv.FormatInt(tm.Chat.ID, 10),
		Content:   tm.Text,
		Timestamp: time.Unix(int64(tm.Date), 0),
		ChatType:  tm.Chat.Type,
		ThreadID:  tm.MessageThreadID,
		Metadata: map[string]string{
			"username":   tm.From.Username,
			"first_name": tm.From.FirstName,
			"chat_type":  tm.Chat.Type,
		},
	}

	if tm.ReplyToMessage != nil {
		msg.ReplyTo = strconv.FormatInt(int64(tm.ReplyToMessage.MessageID), 10)
	}

	// Parse bot commands: /command@botname arg1 arg2
	if strings.HasPrefix(tm.Text, "/") {
		parts := strings.Fields(tm.Text)
		if len(parts) > 0 {
			cmd := parts[0][1:] // strip the leading /
			// Strip @botname suffix if present
			if idx := strings.Index(cmd, "@"); idx >= 0 {
				cmd = cmd[:idx]
			}
			msg.Command = cmd
			msg.Args = parts[1:]
			// Keep Content as the full original text for flexibility
		}
	}

	return msg
}

// TelegramUpdate represents an update from the Telegram API
type TelegramUpdate struct {
	UpdateID int              `json:"update_id"`
	Message  *TelegramMessage `json:"message,omitempty"`
}

// TelegramMessage represents a message from Telegram
type TelegramMessage struct {
	MessageID       int              `json:"message_id"`
	From            TelegramUser     `json:"from"`
	Chat            TelegramChat     `json:"chat"`
	Date            int              `json:"date"`
	Text            string           `json:"text,omitempty"`
	ReplyToMessage  *TelegramMessage `json:"reply_to_message,omitempty"`
	MessageThreadID int64            `json:"message_thread_id,omitempty"` // forum topic thread id
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
