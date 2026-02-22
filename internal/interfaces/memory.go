package interfaces

import "context"

// MemoryBackend is the interface for pluggable memory storage.
// Implementations may use local files, SQLite, vector DBs, etc.
type MemoryBackend interface {
	// Store persists content with the given key and metadata.
	Store(ctx context.Context, key string, content []byte, metadata map[string]string) error

	// Retrieve searches memory and returns up to limit matching entries.
	Retrieve(ctx context.Context, query string, limit int) ([]MemoryEntry, error)

	// Delete removes the entry with the given key.
	Delete(ctx context.Context, key string) error

	// HealthCheck verifies the memory backend is operational.
	HealthCheck(ctx context.Context) error
}
