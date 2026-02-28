package skillbank

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultDistillerModel  = "anthropic-proxy-6/glm-4.7"
	distillerBatchSize     = 10
	distillerSystemPrompt  = `You are an expert agent trainer. Given a batch of task trajectories, extract reusable skills and common mistakes that future agents should know.

Return ONLY valid JSON in this exact format:
{
  "skills": [
    {
      "title": "short skill name",
      "principle": "the reusable principle in 1-2 sentences",
      "when_to_apply": "condition or trigger",
      "example": "optional concrete example",
      "category": "general or specific category",
      "task_type": "task type or empty for general",
      "confidence": 0.8
    }
  ],
  "mistakes": [
    {
      "description": "what went wrong",
      "why_it_happens": "root cause",
      "how_to_avoid": "actionable fix",
      "task_type": "task type or empty"
    }
  ]
}

Focus on patterns that appear across multiple trajectories. Omit one-off flukes.`
)

// LLMDistiller extracts skills from trajectories via an OpenAI-compatible LLM API.
type LLMDistiller struct {
	apiURL string
	apiKey string
	model  string
	client *http.Client
}

// NewLLMDistiller creates a new LLM-backed distiller.
// apiURL should point to an OpenAI-compatible /v1/chat/completions endpoint.
// If model is empty, defaultDistillerModel is used.
func NewLLMDistiller(apiURL, apiKey, model string) *LLMDistiller {
	if model == "" {
		model = defaultDistillerModel
	}
	return &LLMDistiller{
		apiURL: strings.TrimRight(apiURL, "/"),
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

// Distill processes trajectories in batches of up to 10 and returns extracted skills and mistakes.
func (d *LLMDistiller) Distill(ctx context.Context, trajectories []Trajectory) ([]Skill, []CommonMistake, error) {
	var allSkills []Skill
	var allMistakes []CommonMistake

	for i := 0; i < len(trajectories); i += distillerBatchSize {
		end := i + distillerBatchSize
		if end > len(trajectories) {
			end = len(trajectories)
		}
		batch := trajectories[i:end]

		skills, mistakes, err := d.distillBatch(ctx, batch)
		if err != nil {
			return nil, nil, fmt.Errorf("distill batch %d-%d: %w", i, end, err)
		}
		allSkills = append(allSkills, skills...)
		allMistakes = append(allMistakes, mistakes...)
	}

	return allSkills, allMistakes, nil
}

func (d *LLMDistiller) distillBatch(ctx context.Context, batch []Trajectory) ([]Skill, []CommonMistake, error) {
	userContent := buildDistillPrompt(batch)

	reqBody := map[string]any{
		"model": d.model,
		"messages": []map[string]string{
			{"role": "system", "content": distillerSystemPrompt},
			{"role": "user", "content": userContent},
		},
		"temperature": 0.3,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		d.apiURL+"/v1/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if d.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiKey)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("api error %d: %s", resp.StatusCode, body)
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, nil, fmt.Errorf("decode response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return nil, nil, fmt.Errorf("empty choices in response")
	}

	return parseDistillResponse(chatResp.Choices[0].Message.Content)
}

// buildDistillPrompt serializes trajectories into a human-readable prompt.
func buildDistillPrompt(trajectories []Trajectory) string {
	var sb strings.Builder
	sb.WriteString("Here are the agent trajectories to analyze:\n\n")

	for i, t := range trajectories {
		fmt.Fprintf(&sb, "--- Trajectory %d ---\n", i+1)
		fmt.Fprintf(&sb, "Task: %s\n", t.TaskDescription)
		fmt.Fprintf(&sb, "Type: %s\n", t.TaskType)
		fmt.Fprintf(&sb, "Success: %v | Quality: %.2f\n", t.Success, t.Quality)
		for j, step := range t.Steps {
			fmt.Fprintf(&sb, "  Step %d: %s\n    → %s\n", j+1, step.Action, step.Observation)
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\nExtract reusable skills and common mistakes from the above trajectories.")
	return sb.String()
}

// distillResponse is the expected JSON structure from the LLM.
type distillResponse struct {
	Skills   []rawSkill   `json:"skills"`
	Mistakes []rawMistake `json:"mistakes"`
}

type rawSkill struct {
	Title       string  `json:"title"`
	Principle   string  `json:"principle"`
	WhenToApply string  `json:"when_to_apply"`
	Example     string  `json:"example"`
	Category    string  `json:"category"`
	TaskType    string  `json:"task_type"`
	Confidence  float64 `json:"confidence"`
}

type rawMistake struct {
	Description  string `json:"description"`
	WhyItHappens string `json:"why_it_happens"`
	HowToAvoid   string `json:"how_to_avoid"`
	TaskType     string `json:"task_type"`
}

// parseDistillResponse extracts the JSON payload from an LLM response,
// handling potential markdown code fences.
func parseDistillResponse(content string) ([]Skill, []CommonMistake, error) {
	// Strip markdown fences if present
	content = strings.TrimSpace(content)
	if idx := strings.Index(content, "```json"); idx != -1 {
		content = content[idx+7:]
		if end := strings.Index(content, "```"); end != -1 {
			content = content[:end]
		}
	} else if idx := strings.Index(content, "```"); idx != -1 {
		content = content[idx+3:]
		if end := strings.Index(content, "```"); end != -1 {
			content = content[:end]
		}
	}
	// Find first '{' in case there's preamble text
	if idx := strings.Index(content, "{"); idx > 0 {
		content = content[idx:]
	}

	var dr distillResponse
	if err := json.Unmarshal([]byte(content), &dr); err != nil {
		return nil, nil, fmt.Errorf("parse distill JSON: %w (content: %.200s)", err, content)
	}

	now := time.Now()
	skills := make([]Skill, 0, len(dr.Skills))
	for i, rs := range dr.Skills {
		if rs.Confidence == 0 {
			rs.Confidence = 0.7
		}
		if rs.Category == "" {
			rs.Category = "general"
		}
		skills = append(skills, Skill{
			ID:          fmt.Sprintf("distilled-%d-%d", now.UnixNano(), i),
			Title:       rs.Title,
			Principle:   rs.Principle,
			WhenToApply: rs.WhenToApply,
			Example:     rs.Example,
			Category:    rs.Category,
			TaskType:    rs.TaskType,
			Source:      SourceDistilled,
			Confidence:  rs.Confidence,
			SuccessRate: rs.Confidence,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}

	mistakes := make([]CommonMistake, 0, len(dr.Mistakes))
	for i, rm := range dr.Mistakes {
		mistakes = append(mistakes, CommonMistake{
			ID:           fmt.Sprintf("mistake-%d-%d", now.UnixNano(), i),
			Description:  rm.Description,
			WhyItHappens: rm.WhyItHappens,
			HowToAvoid:   rm.HowToAvoid,
			TaskType:     rm.TaskType,
		})
	}

	return skills, mistakes, nil
}
