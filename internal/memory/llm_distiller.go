package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// LLMCallFunc is the signature for calling an LLM
// Allows decoupling from specific LLM providers
type LLMCallFunc func(ctx context.Context, systemPrompt, userPrompt string) (string, error)

// LLMDistiller uses an LLM to distill conversations with better accuracy
type LLMDistiller struct {
	fallback *Distiller   // rule-based fallback
	llmFunc  LLMCallFunc  // function to call LLM
	model    string       // model to use (informational)
	logger   *slog.Logger
	timeout  time.Duration
}

// NewLLMDistiller creates a new LLM-powered distiller
func NewLLMDistiller(fallback *Distiller, llmFunc LLMCallFunc, model string, logger *slog.Logger) *LLMDistiller {
	if logger == nil {
		logger = slog.Default()
	}
	if fallback == nil {
		// Create default fallback if none provided
		fallback = NewDistiller(0.7)
	}

	return &LLMDistiller{
		fallback: fallback,
		llmFunc:  llmFunc,
		model:    model,
		logger:   logger,
		timeout:  30 * time.Second, // Default 30s timeout
	}
}

// DistillConversation converts raw conversation to distilled fact using LLM
// Falls back to rule-based distillation if LLM fails
func (d *LLMDistiller) DistillConversation(conv RawConversation) (*DistilledFact, error) {
	// If no LLM function provided, use fallback immediately
	if d.llmFunc == nil {
		d.logger.Debug("no LLM function, using fallback distiller")
		return d.fallback.DistillConversation(conv)
	}

	// Try LLM distillation with timeout
	ctx, cancel := context.WithTimeout(context.Background(), d.timeout)
	defer cancel()

	distilled, err := d.distillWithLLM(ctx, conv)
	if err != nil {
		d.logger.Warn("LLM distillation failed, using fallback",
			"error", err,
			"model", d.model)
		return d.fallback.DistillConversation(conv)
	}

	d.logger.Debug("LLM distillation succeeded",
		"fact", distilled.Fact,
		"people_count", len(distilled.People),
		"topics_count", len(distilled.Topics))

	return distilled, nil
}

// distillWithLLM performs LLM-powered distillation
func (d *LLMDistiller) distillWithLLM(ctx context.Context, conv RawConversation) (*DistilledFact, error) {
	systemPrompt := buildDistillationSystemPrompt()
	userPrompt := buildDistillationUserPrompt(conv)

	// Call LLM
	response, err := d.llmFunc(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// Parse LLM response
	distilled, err := parseDistillationResponse(response, conv.Timestamp)
	if err != nil {
		return nil, fmt.Errorf("parse LLM response: %w", err)
	}

	// Validate and compress if needed
	distilled, err = d.validateAndCompress(distilled)
	if err != nil {
		return nil, fmt.Errorf("validate distilled fact: %w", err)
	}

	return distilled, nil
}

// buildDistillationSystemPrompt creates the system prompt for distillation
func buildDistillationSystemPrompt() string {
	return `You are a memory distillation engine. Given a conversation, extract:
- fact: one-sentence summary of what happened (max 100 chars)
- people: names mentioned (max 5)
- topics: main topics discussed (max 3)
- actions: action items or tasks (max 3)
- emotion: emotional state or transition (e.g., "worried", "concern→relief")
- outcome: result or conclusion if any

Respond with ONLY a JSON object, no markdown code blocks, no explanation.

Example:
{"fact":"User asked about garden replanting timeline","people":["Alice","Bob"],"topics":["gardening","timeline"],"actions":["check soil pH","order seeds"],"emotion":"excited","outcome":"plan to start next week"}`
}

// buildDistillationUserPrompt formats the conversation for the LLM
func buildDistillationUserPrompt(conv RawConversation) string {
	var sb strings.Builder
	sb.WriteString("Conversation:\n\n")

	for i, msg := range conv.Messages {
		role := "Human"
		if msg.Role == "agent" || msg.Role == "assistant" {
			role = "Agent"
		}
		sb.WriteString(fmt.Sprintf("%d. %s: %s\n", i+1, role, msg.Content))
	}

	sb.WriteString("\nExtract the memory distillation as JSON:")
	return sb.String()
}

// parseDistillationResponse parses the LLM's JSON response
func parseDistillationResponse(response string, timestamp time.Time) (*DistilledFact, error) {
	// Clean up response - remove markdown code blocks if present
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	// Parse JSON
	var distilled DistilledFact
	if err := json.Unmarshal([]byte(response), &distilled); err != nil {
		return nil, fmt.Errorf("unmarshal JSON: %w (response: %s)", err, response)
	}

	// Set timestamp
	distilled.Date = timestamp

	return &distilled, nil
}

// validateAndCompress ensures the distilled fact fits size constraints
func (d *LLMDistiller) validateAndCompress(fact *DistilledFact) (*DistilledFact, error) {
	// Serialize to check size
	data, err := json.Marshal(fact)
	if err != nil {
		return nil, fmt.Errorf("marshal distilled fact: %w", err)
	}

	// If it fits, we're done
	if len(data) <= MaxDistilledBytes {
		return fact, nil
	}

	d.logger.Debug("distilled fact too large, compressing",
		"original_size", len(data),
		"max_size", MaxDistilledBytes)

	// Compress using fallback's compression logic
	for len(data) > MaxDistilledBytes {
		fact = d.fallback.compressDistilledFact(fact)
		data, err = json.Marshal(fact)
		if err != nil {
			return nil, fmt.Errorf("marshal compressed fact: %w", err)
		}
	}

	d.logger.Debug("compressed to fit", "final_size", len(data))
	return fact, nil
}

// GenerateCoreSummary creates an ultra-compressed summary (delegates to fallback)
func (d *LLMDistiller) GenerateCoreSummary(fact *DistilledFact) (*CoreSummary, error) {
	return d.fallback.GenerateCoreSummary(fact)
}

// SetTimeout sets the LLM call timeout
func (d *LLMDistiller) SetTimeout(timeout time.Duration) {
	d.timeout = timeout
}
