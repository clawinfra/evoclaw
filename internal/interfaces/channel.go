package interfaces

import "context"

// Channel is the interface for messaging channels (Telegram, HTTP, MQTT, TUI, etc.).
type Channel interface {
	// Name returns the channel identifier.
	Name() string

	// Send delivers an outbound message.
	Send(ctx context.Context, msg OutboundMessage) error

	// Receive returns a channel that emits inbound messages.
	Receive(ctx context.Context) (<-chan InboundMessage, error)

	// Close shuts down the channel and releases resources.
	Close() error
}
