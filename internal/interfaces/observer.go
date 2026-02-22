package interfaces

import "context"

// Observer is the interface for telemetry and observability.
// Implementations can log, emit metrics, feed RSI analysis, etc.
type Observer interface {
	// OnRequest is called when a new request begins.
	OnRequest(ctx context.Context, req ObservedRequest)

	// OnResponse is called when a response completes.
	OnResponse(ctx context.Context, resp ObservedResponse)

	// OnError is called when an error occurs.
	OnError(ctx context.Context, err ObservedError)

	// Flush ensures all buffered observations are persisted.
	Flush() error
}
