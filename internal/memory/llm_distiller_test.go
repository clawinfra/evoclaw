package memory

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// mockLLMFunc is a mock LLM function for testing
func mockLLMFunc(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	// Return a mock distilled fact as JSON
	response := map[string]interface{}{
		"fact":    "User discussed garden replanting plans",
		"people":  []string{"Alice", "Bob"},
		"topics":  []string{"gardening", "planning"},
		"actions": []string{"check soil pH", "order seeds"},
		"emotion": "excited",
		"outcome": "plan to start next week",
	}

	data, _ := json.Marshal(response)
	return string(data), nil
}

// mockFailingLLMFunc simulates LLM failure
func mockFailingLLMFunc(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	return "", context.DeadlineExceeded
}

func TestLLMDistiller_DistillConversation(t *testing.T) {
	fallback := NewDistiller(0.7)
	distiller := NewLLMDistiller(fallback, mockLLMFunc, "test-model", nil)

	conv := RawConversation{
		Messages: []Message{
			{Role: "user", Content: "Alice and I are planning to replant the garden next week"},
			{Role: "agent", Content: "That sounds great! Have you checked the soil pH?"},
			{Role: "user", Content: "Not yet, we need to order some seeds too"},
		},
		Timestamp: time.Now(),
	}

	distilled, err := distiller.DistillConversation(conv)
	if err != nil {
		t.Fatalf("DistillConversation failed: %v", err)
	}

	if distilled.Fact != "User discussed garden replanting plans" {
		t.Errorf("unexpected fact: %s", distilled.Fact)
	}

	if len(distilled.People) != 2 {
		t.Errorf("expected 2 people, got %d", len(distilled.People))
	}

	if len(distilled.Topics) != 2 {
		t.Errorf("expected 2 topics, got %d", len(distilled.Topics))
	}

	if distilled.Emotion != "excited" {
		t.Errorf("unexpected emotion: %s", distilled.Emotion)
	}
}

func TestLLMDistiller_FallbackOnFailure(t *testing.T) {
	fallback := NewDistiller(0.7)
	distiller := NewLLMDistiller(fallback, mockFailingLLMFunc, "test-model", nil)

	conv := RawConversation{
		Messages: []Message{
			{Role: "user", Content: "This is a test conversation"},
			{Role: "agent", Content: "I understand"},
		},
		Timestamp: time.Now(),
	}

	distilled, err := distiller.DistillConversation(conv)
	if err != nil {
		t.Fatalf("DistillConversation failed: %v", err)
	}

	// Should succeed via fallback
	if distilled == nil {
		t.Fatal("expected distilled result from fallback")
	}

	if distilled.Fact == "" {
		t.Error("expected non-empty fact from fallback")
	}
}

func TestLLMDistiller_NoLLMFunc(t *testing.T) {
	fallback := NewDistiller(0.7)
	distiller := NewLLMDistiller(fallback, nil, "", nil)

	conv := RawConversation{
		Messages: []Message{
			{Role: "user", Content: "Test message"},
		},
		Timestamp: time.Now(),
	}

	distilled, err := distiller.DistillConversation(conv)
	if err != nil {
		t.Fatalf("DistillConversation failed: %v", err)
	}

	// Should use fallback immediately
	if distilled == nil {
		t.Fatal("expected distilled result from fallback")
	}
}

func TestLLMDistiller_SizeConstraints(t *testing.T) {
	// Mock LLM that returns oversized content
	oversizedLLM := func(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
		response := map[string]interface{}{
			"fact":    "This is a very long fact that exceeds the maximum allowed length and needs to be compressed to fit within the size constraints",
			"people":  []string{"Person1", "Person2", "Person3", "Person4", "Person5", "Person6"},
			"topics":  []string{"Topic1", "Topic2", "Topic3", "Topic4"},
			"actions": []string{"Action1", "Action2", "Action3", "Action4"},
			"emotion": "complex emotional state with transitions",
			"outcome": "A very detailed outcome description that is quite long",
		}
		data, _ := json.Marshal(response)
		return string(data), nil
	}

	fallback := NewDistiller(0.7)
	distiller := NewLLMDistiller(fallback, oversizedLLM, "test-model", nil)

	conv := RawConversation{
		Messages: []Message{
			{Role: "user", Content: "Test"},
		},
		Timestamp: time.Now(),
	}

	distilled, err := distiller.DistillConversation(conv)
	if err != nil {
		t.Fatalf("DistillConversation failed: %v", err)
	}

	// Verify it was compressed to fit
	data, _ := json.Marshal(distilled)
	if len(data) > MaxDistilledBytes {
		t.Errorf("distilled fact exceeds max size: %d > %d", len(data), MaxDistilledBytes)
	}
}

func TestParseDistillationResponse_WithMarkdown(t *testing.T) {
	// Test that markdown code blocks are handled
	response := "```json\n{\"fact\":\"test\",\"people\":[],\"topics\":[]}\n```"

	distilled, err := parseDistillationResponse(response, time.Now())
	if err != nil {
		t.Fatalf("parseDistillationResponse failed: %v", err)
	}

	if distilled.Fact != "test" {
		t.Errorf("unexpected fact: %s", distilled.Fact)
	}
}

func TestParseDistillationResponse_PlainJSON(t *testing.T) {
	response := `{"fact":"plain test","people":["Alice"],"topics":["testing"]}`

	distilled, err := parseDistillationResponse(response, time.Now())
	if err != nil {
		t.Fatalf("parseDistillationResponse failed: %v", err)
	}

	if distilled.Fact != "plain test" {
		t.Errorf("unexpected fact: %s", distilled.Fact)
	}

	if len(distilled.People) != 1 || distilled.People[0] != "Alice" {
		t.Errorf("unexpected people: %v", distilled.People)
	}
}
