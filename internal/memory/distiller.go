package memory

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	MaxDistilledBytes = 100 // Stage 2: distilled fact
	MaxCoreSummaryBytes = 30  // Stage 3: core summary
)

// DistilledFact represents a compressed conversation (Stage 2)
type DistilledFact struct {
	Fact     string    `json:"fact"`               // Main takeaway (<80 chars)
	Emotion  string    `json:"emotion,omitempty"`  // Emotional state change
	People   []string  `json:"people,omitempty"`   // People mentioned
	Topics   []string  `json:"topics,omitempty"`   // Topics/categories
	Date     time.Time `json:"date"`               // When it happened
	Actions  []string  `json:"actions,omitempty"`  // Action items
	Outcome  string    `json:"outcome,omitempty"`  // Result/conclusion
}

// CoreSummary is the ultra-compressed version (Stage 3)
type CoreSummary struct {
	Text string    `json:"text"` // One-line summary (<30 bytes)
	Date time.Time `json:"date"`
}

// RawConversation represents the input to distillation
type RawConversation struct {
	Messages  []Message `json:"messages"`
	Timestamp time.Time `json:"timestamp"`
}

// Message is a single message in a conversation
type Message struct {
	Role    string `json:"role"` // "user" or "agent"
	Content string `json:"content"`
}

// Distiller compresses conversations into distilled facts
type Distiller struct {
	aggression float64 // 0-1, higher = more aggressive compression
}

// NewDistiller creates a new distiller with the given aggression level
func NewDistiller(aggression float64) *Distiller {
	if aggression < 0 {
		aggression = 0
	}
	if aggression > 1 {
		aggression = 1
	}

	return &Distiller{
		aggression: aggression,
	}
}

// DistillConversation converts raw conversation to distilled fact (Stage 1 → 2)
// For now, implements rule-based distillation (not LLM-powered yet)
func (d *Distiller) DistillConversation(conv RawConversation) (*DistilledFact, error) {
	if len(conv.Messages) == 0 {
		return nil, fmt.Errorf("empty conversation")
	}

	// Extract entities and information
	people := d.extractPeople(conv)
	topics := d.extractTopics(conv)
	actions := d.extractActions(conv)
	emotion := d.extractEmotion(conv)
	fact := d.extractMainFact(conv)
	outcome := d.extractOutcome(conv)

	distilled := &DistilledFact{
		Fact:    fact,
		Emotion: emotion,
		People:  people,
		Topics:  topics,
		Date:    conv.Timestamp,
		Actions: actions,
		Outcome: outcome,
	}

	// Serialize and check size
	data, err := json.Marshal(distilled)
	if err != nil {
		return nil, fmt.Errorf("marshal distilled fact: %w", err)
	}

	// Compress until it fits
	for len(data) > MaxDistilledBytes {
		distilled = d.compressDistilledFact(distilled)
		data, err = json.Marshal(distilled)
		if err != nil {
			return nil, fmt.Errorf("marshal distilled fact: %w", err)
		}
	}

	return distilled, nil
}

// GenerateCoreSummary creates an ultra-compressed summary (Stage 2 → 3)
func (d *Distiller) GenerateCoreSummary(fact *DistilledFact) (*CoreSummary, error) {
	// Create one-line summary
	summary := fact.Fact

	// Truncate if needed
	if len(summary) > MaxCoreSummaryBytes {
		summary = summary[:MaxCoreSummaryBytes-3] + "..."
	}

	return &CoreSummary{
		Text: summary,
		Date: fact.Date,
	}, nil
}

// extractMainFact extracts the key fact from the conversation
func (d *Distiller) extractMainFact(conv RawConversation) string {
	// Combine all user messages (they contain the facts)
	var facts []string
	for _, msg := range conv.Messages {
		if msg.Role == "user" {
			// Extract sentences that look like statements
			sentences := d.extractStatements(msg.Content)
			facts = append(facts, sentences...)
		}
	}

	if len(facts) == 0 {
		return "conversation"
	}

	// Take the longest/most informative sentence
	mainFact := facts[0]
	for _, f := range facts {
		if len(f) > len(mainFact) && len(f) < 80 {
			mainFact = f
		}
	}

	return d.cleanText(mainFact)
}

// extractPeople finds names/people mentioned
func (d *Distiller) extractPeople(conv RawConversation) []string {
	people := make(map[string]bool)

	// Simple heuristic: capitalized words that appear multiple times
	for _, msg := range conv.Messages {
		words := strings.Fields(msg.Content)
		for _, word := range words {
			// Check if starts with capital and is not at sentence start
			if len(word) > 2 && word[0] >= 'A' && word[0] <= 'Z' {
				clean := strings.Trim(word, ".,!?;:")
				if len(clean) > 2 {
					people[clean] = true
				}
			}
		}
	}

	// Convert to slice
	result := make([]string, 0, len(people))
	for p := range people {
		result = append(result, p)
		if len(result) >= 5 { // Max 5 people
			break
		}
	}

	return result
}

// extractTopics identifies main topics
func (d *Distiller) extractTopics(conv RawConversation) []string {
	// Simple keyword extraction
	keywords := make(map[string]int)

	for _, msg := range conv.Messages {
		words := d.extractKeywords(msg.Content)
		for _, word := range words {
			keywords[word]++
		}
	}

	// Get top keywords by frequency
	type kv struct {
		Key   string
		Value int
	}
	var sorted []kv
	for k, v := range keywords {
		if v >= 2 { // Mentioned at least twice
			sorted = append(sorted, kv{k, v})
		}
	}

	// Sort by frequency
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Value > sorted[i].Value {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Take top 3
	topics := make([]string, 0)
	for i := 0; i < len(sorted) && i < 3; i++ {
		topics = append(topics, sorted[i].Key)
	}

	return topics
}

// extractActions finds action items or tasks
func (d *Distiller) extractActions(conv RawConversation) []string {
	actions := make([]string, 0)

	actionWords := []string{"will", "going to", "need to", "should", "must", "plan to", "remind"}

	for _, msg := range conv.Messages {
		lower := strings.ToLower(msg.Content)
		for _, trigger := range actionWords {
			if strings.Contains(lower, trigger) {
				// Extract sentence containing the action
				sentences := strings.Split(msg.Content, ".")
				for _, sent := range sentences {
					if strings.Contains(strings.ToLower(sent), trigger) {
						actions = append(actions, d.cleanText(sent))
						break
					}
				}
			}
		}
	}

	// Limit to 2 actions
	if len(actions) > 2 {
		actions = actions[:2]
	}

	return actions
}

// extractEmotion detects emotional state changes
func (d *Distiller) extractEmotion(conv RawConversation) string {
	emotionWords := map[string][]string{
		"happy":    {"happy", "great", "wonderful", "excited", "joy", "glad"},
		"sad":      {"sad", "upset", "disappointed", "hurt", "sorry"},
		"worried":  {"worried", "concerned", "anxious", "nervous", "scared"},
		"relieved": {"better", "relieved", "okay", "recovered", "fine"},
		"angry":    {"angry", "mad", "frustrated", "annoyed"},
	}

	detected := make(map[string]int)

	for _, msg := range conv.Messages {
		lower := strings.ToLower(msg.Content)
		for emotion, words := range emotionWords {
			for _, word := range words {
				if strings.Contains(lower, word) {
					detected[emotion]++
				}
			}
		}
	}

	// If we detected transitions (e.g., worried → relieved)
	if detected["worried"] > 0 && detected["relieved"] > 0 {
		return "concern→relief"
	}
	if detected["sad"] > 0 && detected["happy"] > 0 {
		return "sad→happy"
	}

	// Otherwise return the strongest emotion
	maxEmotion := ""
	maxCount := 0
	for emotion, count := range detected {
		if count > maxCount {
			maxEmotion = emotion
			maxCount = count
		}
	}

	return maxEmotion
}

// extractOutcome finds the result or conclusion
func (d *Distiller) extractOutcome(conv RawConversation) string {
	outcomeWords := []string{"so", "therefore", "result", "ended", "turned out", "finally"}

	for _, msg := range conv.Messages {
		lower := strings.ToLower(msg.Content)
		for _, trigger := range outcomeWords {
			if strings.Contains(lower, trigger) {
				// Extract sentence with outcome
				sentences := strings.Split(msg.Content, ".")
				for _, sent := range sentences {
					if strings.Contains(strings.ToLower(sent), trigger) {
						return d.cleanText(sent)
					}
				}
			}
		}
	}

	return ""
}

// extractStatements extracts declarative sentences
func (d *Distiller) extractStatements(text string) []string {
	sentences := strings.Split(text, ".")
	statements := make([]string, 0)

	for _, sent := range sentences {
		sent = strings.TrimSpace(sent)
		if len(sent) > 10 && !strings.HasSuffix(sent, "?") {
			statements = append(statements, sent)
		}
	}

	return statements
}

// extractKeywords extracts meaningful words (similar to tree_search.go)
func (d *Distiller) extractKeywords(text string) []string {
	stopwords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"but": true, "is": true, "are": true, "was": true, "were": true,
		"be": true, "been": true, "being": true, "have": true, "has": true,
		"had": true, "do": true, "does": true, "did": true, "will": true,
		"would": true, "could": true, "should": true,
	}

	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return r == ' ' || r == ',' || r == '.' || r == '!' || r == '?' || r == ';' || r == ':'
	})

	keywords := make([]string, 0)
	for _, word := range words {
		word = strings.TrimSpace(word)
		if len(word) > 3 && !stopwords[word] {
			keywords = append(keywords, word)
		}
	}

	return keywords
}

// cleanText removes extra whitespace and trims
func (d *Distiller) cleanText(text string) string {
	text = strings.TrimSpace(text)
	text = strings.Join(strings.Fields(text), " ")
	return text
}

// compressDistilledFact aggressively compresses a fact that's too large
func (d *Distiller) compressDistilledFact(fact *DistilledFact) *DistilledFact {
	// Truncate fact progressively
	if len(fact.Fact) > 50 {
		fact.Fact = fact.Fact[:50]
	} else if len(fact.Fact) > 40 {
		fact.Fact = fact.Fact[:40]
	}

	// Progressively remove arrays
	if len(fact.Actions) > 0 {
		fact.Actions = nil
	}
	if len(fact.People) > 2 {
		fact.People = fact.People[:2]
	} else if len(fact.People) > 1 {
		fact.People = fact.People[:1]
	} else if len(fact.People) > 0 {
		fact.People = nil
	}
	
	if len(fact.Topics) > 2 {
		fact.Topics = fact.Topics[:2]
	} else if len(fact.Topics) > 1 {
		fact.Topics = fact.Topics[:1]
	} else if len(fact.Topics) > 0 {
		fact.Topics = nil
	}

	// Remove outcome and emotion
	fact.Outcome = ""
	fact.Emotion = ""

	return fact
}
