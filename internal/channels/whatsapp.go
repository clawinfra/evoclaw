// Package channels provides communication channel adapters for EvoClaw.
// This file implements the WhatsApp channel via go.mau.fi/whatsmeow.
package channels

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	watypes "go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	// modernc.org/sqlite is the pure-Go SQLite driver registered as "sqlite".
	_ "modernc.org/sqlite"

	evotypes "github.com/clawinfra/evoclaw/internal/types"
)

// WhatsAppChannel implements the Channel interface for WhatsApp using whatsmeow.
//
// Authentication flow:
//   - First Start(): if no session stored → print QR code to stdout; user scans
//     with the WhatsApp phone app (Settings → Linked Devices → Link a Device).
//   - Subsequent starts: reconnect directly using the stored session.
//
// Session state is persisted to a SQLite database at the configured dbPath.
// whatsmeow handles auto-reconnect internally on unexpected disconnects.
type WhatsAppChannel struct {
	dbPath string
	logger *slog.Logger

	client    *whatsmeow.Client
	container *sqlstore.Container

	inbox  chan evotypes.Message
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	once   sync.Once // ensure Stop's cleanup runs exactly once
}

// NewWhatsApp creates a new WhatsApp channel adapter.
//
//   - dbPath: path to the SQLite file used for session persistence
//     (e.g. "/var/lib/evoclaw/whatsapp.db"). Created on first run.
func NewWhatsApp(dbPath string, logger *slog.Logger) *WhatsAppChannel {
	return &WhatsAppChannel{
		dbPath: dbPath,
		logger: logger.With("channel", "whatsapp"),
		inbox:  make(chan evotypes.Message, 100),
	}
}

// Name returns the channel identifier.
func (w *WhatsAppChannel) Name() string {
	return "whatsapp"
}

// Start connects to WhatsApp. On first run (no stored session), a QR code is
// printed to stdout. Scanning it with the phone completes authentication.
func (w *WhatsAppChannel) Start(ctx context.Context) error {
	w.ctx, w.cancel = context.WithCancel(ctx)

	// Open the SQLite session database
	rawDB, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_foreign_keys=on", w.dbPath))
	if err != nil {
		return fmt.Errorf("open whatsapp session db: %w", err)
	}

	container := sqlstore.NewWithDB(rawDB, "sqlite", waLog.Noop)
	if err := container.Upgrade(w.ctx); err != nil {
		return fmt.Errorf("upgrade whatsapp session db: %w", err)
	}
	w.container = container

	// Get (or create) the device store entry
	deviceStore, err := container.GetFirstDevice(w.ctx)
	if err != nil {
		return fmt.Errorf("get device store: %w", err)
	}

	w.client = whatsmeow.NewClient(deviceStore, waLog.Noop)
	w.client.AddEventHandler(w.handleEvent)

	if w.client.Store.ID == nil {
		// No stored session → QR login required
		qrChan, _ := w.client.GetQRChannel(w.ctx)
		if err := w.client.Connect(); err != nil {
			return fmt.Errorf("whatsapp connect (qr): %w", err)
		}

		w.logger.Info("no stored session found, waiting for QR scan…")
		_, _ = fmt.Fprintln(os.Stdout, "[WhatsApp] Open WhatsApp on your phone → Linked Devices → Link a Device, then scan:")

		for evt := range qrChan {
			switch evt.Event {
			case "code":
				// Print the raw WA multi-device QR string.
				// Users can pipe to `qrencode -t ansiutf8` or use any QR library.
				_, _ = fmt.Fprintf(os.Stdout, "\n%s\n\n", evt.Code)
				w.logger.Info("QR code printed to stdout — scan with WhatsApp")
			case "success":
				w.logger.Info("QR code scanned successfully, logged in")
			case "timeout":
				return fmt.Errorf("whatsapp QR code timed out — restart to try again")
			case "err":
				return fmt.Errorf("whatsapp QR error: %s", evt.Error)
			}
		}
	} else {
		// Already authenticated — just reconnect
		if err := w.client.Connect(); err != nil {
			return fmt.Errorf("whatsapp reconnect: %w", err)
		}
		w.logger.Info("whatsapp reconnected", "jid", w.client.Store.ID.String())
	}

	w.logger.Info("whatsapp channel started")
	return nil
}

// Stop disconnects from WhatsApp and shuts down the channel.
func (w *WhatsAppChannel) Stop() error {
	w.logger.Info("stopping whatsapp channel")
	w.once.Do(func() {
		if w.cancel != nil {
			w.cancel()
		}
		if w.client != nil {
			w.client.Disconnect()
		}
		w.wg.Wait()
		close(w.inbox)
	})
	return nil
}

// Send delivers a message to the given WhatsApp JID (e.g. "15551234567@s.whatsapp.net").
// Content routing:
//   - URL ending with .jpg/.jpeg/.png/.gif/.webp → image
//   - URL ending with document extensions (.pdf, .zip, .docx…) → document
//   - Anything else → plain text conversation message
func (w *WhatsAppChannel) Send(ctx context.Context, msg evotypes.Response) error {
	if w.client == nil || !w.client.IsConnected() {
		return fmt.Errorf("whatsapp: not connected")
	}

	jid, err := watypes.ParseJID(msg.To)
	if err != nil {
		return fmt.Errorf("whatsapp: invalid JID %q: %w", msg.To, err)
	}

	lower := strings.ToLower(msg.Content)
	switch {
	case waIsImageURL(lower):
		return w.sendImage(ctx, jid, msg.Content)
	case waIsDocumentURL(lower):
		return w.sendDocument(ctx, jid, msg.Content)
	default:
		return w.sendText(ctx, jid, msg.Content)
	}
}

// Receive returns the read channel for incoming normalised messages.
func (w *WhatsAppChannel) Receive() <-chan evotypes.Message {
	return w.inbox
}

// --- internal send helpers ---

func (w *WhatsAppChannel) sendText(ctx context.Context, to watypes.JID, text string) error {
	waMsg := &waE2E.Message{Conversation: proto.String(text)}
	if _, err := w.client.SendMessage(ctx, to, waMsg); err != nil {
		return fmt.Errorf("whatsapp sendText: %w", err)
	}
	w.logger.Debug("text message sent", "to", to.String(), "length", len(text))
	return nil
}

func (w *WhatsAppChannel) sendImage(ctx context.Context, to watypes.JID, imageURL string) error {
	data, mimeType, err := waFetchURL(imageURL)
	if err != nil {
		return fmt.Errorf("whatsapp sendImage fetch: %w", err)
	}

	uploaded, err := w.client.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return fmt.Errorf("whatsapp sendImage upload: %w", err)
	}

	waMsg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mimeType),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
		},
	}
	if _, err := w.client.SendMessage(ctx, to, waMsg); err != nil {
		return fmt.Errorf("whatsapp sendImage send: %w", err)
	}
	w.logger.Debug("image sent", "to", to.String(), "url", imageURL)
	return nil
}

func (w *WhatsAppChannel) sendDocument(ctx context.Context, to watypes.JID, docURL string) error {
	data, mimeType, err := waFetchURL(docURL)
	if err != nil {
		return fmt.Errorf("whatsapp sendDocument fetch: %w", err)
	}

	// Extract filename from last path segment
	parts := strings.Split(docURL, "/")
	filename := parts[len(parts)-1]
	if idx := strings.Index(filename, "?"); idx >= 0 {
		filename = filename[:idx] // strip query string
	}
	if filename == "" {
		filename = "file"
	}

	uploaded, err := w.client.Upload(ctx, data, whatsmeow.MediaDocument)
	if err != nil {
		return fmt.Errorf("whatsapp sendDocument upload: %w", err)
	}

	waMsg := &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mimeType),
			FileName:      proto.String(filename),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
		},
	}
	if _, err := w.client.SendMessage(ctx, to, waMsg); err != nil {
		return fmt.Errorf("whatsapp sendDocument send: %w", err)
	}
	w.logger.Debug("document sent", "to", to.String(), "filename", filename)
	return nil
}

// --- event handler ---

func (w *WhatsAppChannel) handleEvent(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		w.handleIncomingMessage(v)

	case *events.Connected:
		w.logger.Info("whatsapp connected")

	case *events.Disconnected:
		w.logger.Warn("whatsapp disconnected; whatsmeow will auto-reconnect")

	case *events.LoggedOut:
		w.logger.Error("whatsapp logged out — session invalidated",
			"reason", v.Reason.String())
	}
}

func (w *WhatsAppChannel) handleIncomingMessage(evt *events.Message) {
	if evt.Info.IsFromMe {
		return // ignore outgoing echo
	}

	text := waExtractText(evt.Message)
	if text == "" {
		return // no extractable text (sticker, unsupported type, etc.)
	}

	msg := evotypes.Message{
		ID:        evt.Info.ID,
		Channel:   "whatsapp",
		From:      evt.Info.Sender.String(),
		To:        evt.Info.Chat.String(),
		Content:   text,
		Timestamp: evt.Info.Timestamp,
		Metadata: map[string]string{
			"push_name": evt.Info.PushName,
			"chat":      evt.Info.Chat.String(),
		},
	}
	if evt.Info.IsGroup {
		msg.Metadata["is_group"] = "true"
	}

	select {
	case w.inbox <- msg:
		w.logger.Debug("message queued",
			"from", msg.From,
			"chat", msg.To,
			"length", len(text),
		)
	case <-w.ctx.Done():
		// shutting down
	default:
		w.logger.Warn("whatsapp inbox full, dropping message", "from", msg.From)
	}
}

// --- helpers ---

// waExtractText returns the best-effort plain text from a WhatsApp message proto.
func waExtractText(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	if s := msg.GetConversation(); s != "" {
		return s
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil && ext.Text != nil {
		return *ext.Text
	}
	if img := msg.GetImageMessage(); img != nil && img.GetCaption() != "" {
		return img.GetCaption()
	}
	if doc := msg.GetDocumentMessage(); doc != nil && doc.GetCaption() != "" {
		return doc.GetCaption()
	}
	return ""
}

// waIsImageURL returns true for HTTP URLs that point to common image formats.
func waIsImageURL(lower string) bool {
	if !strings.HasPrefix(lower, "http") {
		return false
	}
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".gif", ".webp"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// waIsDocumentURL returns true for HTTP URLs whose extension suggests a document.
func waIsDocumentURL(lower string) bool {
	if !strings.HasPrefix(lower, "http") {
		return false
	}
	for _, ext := range []string{
		".pdf", ".zip", ".tar", ".gz", ".7z",
		".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx",
		".txt", ".csv",
		".mp4", ".avi", ".mov",
		".mp3", ".ogg", ".m4a", ".aac",
	} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// waHTTPClient is the shared HTTP client used for media fetching (30 s timeout).
var waHTTPClient = &http.Client{Timeout: 30 * time.Second}

// waFetchURL downloads a URL and returns its bytes and MIME type.
func waFetchURL(rawURL string) ([]byte, string, error) {
	resp, err := waHTTPClient.Get(rawURL)
	if err != nil {
		return nil, "", fmt.Errorf("http get %s: %w", rawURL, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("http %d for %s", resp.StatusCode, rawURL)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read body: %w", err)
	}

	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	// Strip parameters like "; charset=..."
	if idx := strings.Index(mimeType, ";"); idx >= 0 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}

	return data, mimeType, nil
}
