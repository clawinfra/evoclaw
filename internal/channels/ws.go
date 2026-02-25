package channels

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/clawinfra/evoclaw/internal/types"
	"github.com/coder/websocket"
)

// wsConn tracks an active WebSocket connection along with its pending response channel.
type wsConn struct {
	conn      *websocket.Conn
	requestID string
	respCh    chan types.Response
}

// WSChannel implements the orchestrator Channel interface over WebSocket connections.
// Each incoming WS "chat" request registers itself with a messageID; the orchestrator
// delivers the agent response via Send() which routes it back to the waiting goroutine.
type WSChannel struct {
	mu     sync.RWMutex
	conns  map[string]*wsConn // keyed by messageID
	inbox  chan types.Message
	logger *slog.Logger
	name   string

	ctx    context.Context
	cancel context.CancelFunc
}

// NewWSChannel creates a new WSChannel ready to accept connections.
func NewWSChannel(logger *slog.Logger) *WSChannel {
	ctx, cancel := context.WithCancel(context.Background())
	return &WSChannel{
		conns:  make(map[string]*wsConn),
		inbox:  make(chan types.Message, 256),
		logger: logger.With("channel", "websocket"),
		name:   "websocket",
		ctx:    ctx,
		cancel: cancel,
	}
}

// Name returns the channel identifier used by the orchestrator.
func (c *WSChannel) Name() string {
	return c.name
}

// Start initialises the channel (no background goroutines needed; connections are accepted
// directly by the HTTP handler).
func (c *WSChannel) Start(_ context.Context) error {
	c.logger.Info("WSChannel started")
	return nil
}

// Stop drains and closes all pending connection response channels.
func (c *WSChannel) Stop() error {
	c.cancel()

	c.mu.Lock()
	defer c.mu.Unlock()

	for id, wc := range c.conns {
		close(wc.respCh)
		delete(c.conns, id)
	}

	c.logger.Info("WSChannel stopped")
	return nil
}

// Send routes the response to the connection registered under msg.MessageID.
// It satisfies orchestrator.Channel.
func (c *WSChannel) Send(ctx context.Context, msg types.Response) error {
	c.mu.RLock()
	wc, ok := c.conns[msg.MessageID]
	c.mu.RUnlock()

	if !ok {
		return fmt.Errorf("websocket: no pending connection for message %s", msg.MessageID)
	}

	select {
	case wc.respCh <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("websocket: response channel full for message %s", msg.MessageID)
	}
}

// Receive returns the read-only channel that delivers messages from WS clients to the
// orchestrator's inbox.  The orchestrator's receiveFrom goroutine reads from here.
func (c *WSChannel) Receive() <-chan types.Message {
	return c.inbox
}

// Inbox returns the bidirectional inbox channel so that the WS handler can push
// messages directly (bypassing the orchestrator's receive loop during tests) and
// the orchestrator's receiveFrom goroutine can read from it.
func (c *WSChannel) Inbox() chan types.Message {
	return c.inbox
}

// Register associates a messageID with an active WebSocket connection and returns the
// channel on which the response will be delivered.  The caller (ws_terminal handler)
// must call Unregister after it has consumed the response or timed out.
func (c *WSChannel) Register(messageID, requestID string, conn *websocket.Conn) chan types.Response {
	respCh := make(chan types.Response, 1)

	c.mu.Lock()
	c.conns[messageID] = &wsConn{
		conn:      conn,
		requestID: requestID,
		respCh:    respCh,
	}
	c.mu.Unlock()

	c.logger.Debug("ws connection registered", "messageID", messageID, "requestID", requestID)
	return respCh
}

// Unregister removes the connection mapping for a completed (or timed-out) request.
func (c *WSChannel) Unregister(messageID string) {
	c.mu.Lock()
	delete(c.conns, messageID)
	c.mu.Unlock()

	c.logger.Debug("ws connection unregistered", "messageID", messageID)
}
