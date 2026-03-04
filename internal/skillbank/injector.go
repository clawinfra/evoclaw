package skillbank

import (
	"fmt"
	"strings"
)

// PromptInjector formats skills and mistakes for LLM system-prompt injection.
type PromptInjector struct{}

// NewInjector returns a new PromptInjector.
func NewInjector() *PromptInjector {
	return &PromptInjector{}
}

// FormatForPrompt renders skills and mistakes as a clean markdown block suitable
// for prepending to a system prompt.
//
// Example output:
//
//	## Relevant Skills from Past Experience
//	1. [Systematic Exploration] When: goal object count not met → Search each surface once before revisiting
//	2. [Error Handling] When: making API calls → Always check returned errors, never ignore them
//
//	## Common Mistakes to Avoid
//	- Unused imports cause build failures — remove them before committing
func (inj *PromptInjector) FormatForPrompt(skills []Skill, mistakes []CommonMistake) string {
	if len(skills) == 0 && len(mistakes) == 0 {
		return ""
	}

	var sb strings.Builder

	if len(skills) > 0 {
		sb.WriteString("## Relevant Skills from Past Experience\n")
		for i, s := range skills {
			fmt.Fprintf(&sb, "%d. [%s] When: %s → %s\n", i+1, s.Title, s.WhenToApply, s.Principle)
		}
	}

	if len(mistakes) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("## Common Mistakes to Avoid\n")
		for _, m := range mistakes {
			fmt.Fprintf(&sb, "- %s — %s\n", m.Description, m.HowToAvoid)
		}
	}

	return sb.String()
}

// InjectIntoPrompt prepends the formatted skill block to an existing system prompt.
// If there are no skills or mistakes, the original prompt is returned unchanged.
func (inj *PromptInjector) InjectIntoPrompt(systemPrompt string, skills []Skill, mistakes []CommonMistake) string {
	block := inj.FormatForPrompt(skills, mistakes)
	if block == "" {
		return systemPrompt
	}
	if systemPrompt == "" {
		return block
	}
	return block + "\n" + systemPrompt
}
