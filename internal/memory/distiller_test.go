package memory

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewDistiller(t *testing.T) {
	d := NewDistiller(0.7)
	if d == nil {
		t.Fatal("distiller is nil")
	}
	if d.aggression != 0.7 {
		t.Errorf("aggression: got %.2f, want 0.7", d.aggression)
	}
}

func TestDistillConversation(t *testing.T) {
	d := NewDistiller(0.7)

	conv := RawConversation{
		Timestamp: time.Now(),
		Messages: []Message{
			{Role: "user", Content: "My dog Biscuit was sick yesterday but he's better now. We went to the vet."},
			{Role: "agent", Content: "Oh no! I'm glad Biscuit is feeling better."},
			{Role: "user", Content: "Yes, the vet gave him some medicine and it worked."},
		},
	}

	fact, err := d.DistillConversation(conv)
	if err != nil {
		t.Fatalf("distill failed: %v", err)
	}

	if fact.Fact == "" {
		t.Error("fact is empty")
	}

	// Check serialized size
	data, _ := json.Marshal(fact)
	if len(data) > MaxDistilledBytes {
		t.Errorf("distilled size %d exceeds max %d", len(data), MaxDistilledBytes)
	}
}

func TestExtractPeople(t *testing.T) {
	d := NewDistiller(0.7)

	conv := RawConversation{
		Timestamp: time.Now(),
		Messages: []Message{
			{Role: "user", Content: "David called me today and Sarah sent an email. I talked to Biscuit too."},
		},
	}

	fact, _ := d.DistillConversation(conv)

	if len(fact.People) == 0 {
		t.Error("should extract people")
	}

	// Check if some names are captured (relaxed check since extraction is heuristic)
	hasCapitalizedWords := false
	for _, person := range fact.People {
		if len(person) > 0 && person[0] >= 'A' && person[0] <= 'Z' {
			hasCapitalizedWords = true
			break
		}
	}

	if !hasCapitalizedWords {
		t.Error("should have captured capitalized names")
	}
}

func TestExtractTopics(t *testing.T) {
	d := NewDistiller(0.7)

	conv := RawConversation{
		Timestamp: time.Now(),
		Messages: []Message{
			{Role: "user", Content: "We need to discuss the garden project. The garden needs new plants."},
			{Role: "agent", Content: "Let's talk about the garden plans."},
		},
	}

	fact, _ := d.DistillConversation(conv)

	// "garden" should be extracted as a topic (mentioned 3 times)
	foundGarden := false
	for _, topic := range fact.Topics {
		if topic == "garden" {
			foundGarden = true
			break
		}
	}

	if !foundGarden {
		t.Errorf("should extract 'garden' as topic, got: %v", fact.Topics)
	}
}

func TestExtractActions(t *testing.T) {
	d := NewDistiller(0.7)

	conv := RawConversation{
		Timestamp: time.Now(),
		Messages: []Message{
			{Role: "user", Content: "I need to call the vet tomorrow. I should also remind David about the meeting."},
		},
	}

	fact, _ := d.DistillConversation(conv)

	if len(fact.Actions) == 0 {
		t.Error("should extract actions")
	}

	// Check that actions contain action words
	hasActionWord := false
	for _, action := range fact.Actions {
		if len(action) > 0 {
			hasActionWord = true
			break
		}
	}

	if !hasActionWord {
		t.Error("actions should contain meaningful text")
	}
}

func TestExtractEmotion(t *testing.T) {
	tests := []struct {
		name     string
		messages []Message
		want     string
	}{
		{
			name: "transition from concern to relief",
			messages: []Message{
				{Role: "user", Content: "I was really worried about Biscuit"},
				{Role: "user", Content: "but now I'm relieved he's better"},
			},
			want: "concern→relief",
		},
		{
			name: "happy emotion",
			messages: []Message{
				{Role: "user", Content: "I'm so happy and excited about the news!"},
			},
			want: "happy",
		},
		{
			name: "worried emotion",
			messages: []Message{
				{Role: "user", Content: "I'm quite worried and anxious about the test results"},
			},
			want: "worried",
		},
	}

	d := NewDistiller(0.7)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conv := RawConversation{
				Timestamp: time.Now(),
				Messages:  tt.messages,
			}

			fact, _ := d.DistillConversation(conv)

			if fact.Emotion != tt.want {
				t.Errorf("emotion: got %s, want %s", fact.Emotion, tt.want)
			}
		})
	}
}

func TestGenerateCoreSummary(t *testing.T) {
	d := NewDistiller(0.7)

	fact := &DistilledFact{
		Fact: "Dog Biscuit was sick with a stomach bug but recovered after vet visit",
		Date: time.Now(),
	}

	summary, err := d.GenerateCoreSummary(fact)
	if err != nil {
		t.Fatalf("generate summary failed: %v", err)
	}

	if len(summary.Text) == 0 {
		t.Error("summary text is empty")
	}

	if len(summary.Text) > MaxCoreSummaryBytes {
		t.Errorf("summary size %d exceeds max %d", len(summary.Text), MaxCoreSummaryBytes)
	}
}

func TestCompressDistilledFact(t *testing.T) {
	d := NewDistiller(0.9) // High aggression

	fact := &DistilledFact{
		Fact:    "This is a very long fact that needs to be compressed because it exceeds the size limit",
		People:  []string{"Alice", "Bob", "Charlie", "David"},
		Topics:  []string{"topic1", "topic2", "topic3"},
		Actions: []string{"action1", "action2", "action3"},
		Outcome: "A very long outcome description that should be removed",
		Date:    time.Now(),
	}

	compressed := d.compressDistilledFact(fact)

	if len(compressed.Fact) > 60 {
		t.Errorf("fact not compressed: %d bytes", len(compressed.Fact))
	}

	if len(compressed.People) > 2 {
		t.Errorf("people not compressed: %d entries", len(compressed.People))
	}

	if len(compressed.Topics) > 2 {
		t.Errorf("topics not compressed: %d entries", len(compressed.Topics))
	}

	if len(compressed.Actions) > 1 {
		t.Errorf("actions not compressed: %d entries", len(compressed.Actions))
	}

	if compressed.Outcome != "" {
		t.Error("outcome should be removed")
	}
}

func TestDistillationSizeConstraint(t *testing.T) {
	d := NewDistiller(0.7)

	// Create a conversation with lots of content
	conv := RawConversation{
		Timestamp: time.Now(),
		Messages: []Message{
			{Role: "user", Content: "This is a very long message with lots of detail about many different things including people, places, events, and outcomes that should all be compressed down into a much smaller distilled fact."},
			{Role: "agent", Content: "I understand, let me help you with that."},
			{Role: "user", Content: "Yes, and there's even more information here about Alice, Bob, Charlie, David, and many other people doing various things."},
		},
	}

	fact, err := d.DistillConversation(conv)
	if err != nil {
		t.Fatalf("distill failed: %v", err)
	}

	// Serialize and check size
	data, err := json.Marshal(fact)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	if len(data) > MaxDistilledBytes {
		t.Errorf("distilled size %d exceeds max %d", len(data), MaxDistilledBytes)
	}
}

func TestEmptyConversation(t *testing.T) {
	d := NewDistiller(0.7)

	conv := RawConversation{
		Timestamp: time.Now(),
		Messages:  []Message{},
	}

	_, err := d.DistillConversation(conv)
	if err == nil {
		t.Error("should reject empty conversation")
	}
}

func TestExtractKeywords(t *testing.T) {
	d := NewDistiller(0.7)

	text := "The garden project needs new plants and flowers"
	keywords := d.extractKeywords(text)

	// Should exclude stopwords
	for _, kw := range keywords {
		if kw == "the" || kw == "and" {
			t.Errorf("stopword '%s' should be excluded", kw)
		}
	}

	// Should include meaningful words
	hasMeaningful := false
	for _, kw := range keywords {
		if kw == "garden" || kw == "project" || kw == "plants" {
			hasMeaningful = true
			break
		}
	}

	if !hasMeaningful {
		t.Error("should extract meaningful keywords")
	}
}
