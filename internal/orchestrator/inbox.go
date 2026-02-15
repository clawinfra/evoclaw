package orchestrator

// GetInbox returns the inbox channel for external message injection
// This allows other components (e.g., HTTP API) to send messages to agents
func (o *Orchestrator) GetInbox() chan<- Message {
	return o.inbox
}
