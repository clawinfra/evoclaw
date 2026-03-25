// Package loop provides middleware hooks for the EvoClaw agent tool loop.
//
// Hooks intercept key lifecycle events — task completion and per-tool-call
// execution — to inject additional behaviour without modifying core loop logic.
// All hooks are opt-in and disabled by default to preserve existing behaviour.
//
// Configuration (agent.toml):
//
//	[hooks]
//	pre_completion         = true
//	pre_completion_prompt  = "Before completing: 1. Run tests. 2. Compare output to spec."
//	loop_detection         = true
//	loop_detection_edit_threshold = 5
package loop

import (
	"context"
	"fmt"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
// Shared types
// ─────────────────────────────────────────────────────────────────────────────

// ToolCall captures a single tool invocation as seen by the hook layer.
// It mirrors the fields used by the orchestrator's ToolCall type so that
// the loop package stays import-free from the orchestrator package.
type ToolCall struct {
	// ID is the unique call identifier issued by the LLM.
	ID string
	// Name is the registered tool name (e.g. "write_file", "shell").
	Name string
	// Arguments holds the key-value parameters for the call.
	Arguments map[string]interface{}
	// Error is non-empty when the tool returned an error result.
	Error string
}

// AgentState is the minimal agent context passed to hooks.
// Hooks may read and modify this value; a non-nil return replaces the
// caller's copy for the remainder of the current iteration.
type AgentState struct {
	// ID is the agent's unique identifier.
	ID string
	// HasRunVerification is set to true once the agent has completed a
	// verification pass. PreCompletionHook uses this flag to avoid
	// triggering an infinite verification loop.
	HasRunVerification bool
	// InjectedMessages holds extra messages to prepend to the next LLM
	// call. Hooks append to this slice; the loop drains it on each turn.
	InjectedMessages []string
}

// ─────────────────────────────────────────────────────────────────────────────
// Issue #28 — PreCompletionHook
// ─────────────────────────────────────────────────────────────────────────────

// PreCompletionHook is called when the agent signals task completion.
//
// Returns (modified_state, continue_iterating).
// If continue is true the loop injects the messages accumulated in
// modified_state.InjectedMessages and runs one more LLM iteration before
// allowing the agent to exit.
type PreCompletionHook func(ctx context.Context, state AgentState) (AgentState, bool)

// VerificationChecklistHook returns a PreCompletionHook that injects
// checklistPrompt into the agent conversation the first time the agent
// signals completion. On the second completion signal (HasRunVerification
// already true) the hook is a no-op and the agent exits normally.
//
// Usage:
//
//	hook := VerificationChecklistHook(
//	    "Before completing: 1. Run tests. 2. Compare output to spec.",
//	)
func VerificationChecklistHook(checklistPrompt string) PreCompletionHook {
	return func(ctx context.Context, state AgentState) (AgentState, bool) {
		// Agent has already run a verification pass — let it exit.
		if state.HasRunVerification {
			return state, false
		}

		// First completion signal: inject checklist and force another iteration.
		state.HasRunVerification = true
		state.InjectedMessages = append(state.InjectedMessages, checklistPrompt)
		return state, true // continue = true → loop again
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Issue #29 — LoopDetectionHook
// ─────────────────────────────────────────────────────────────────────────────

const (
	defaultEditThreshold      = 5
	defaultToolCallWindow     = 20
	consecutiveCallThreshold  = 3 // same tool+args repeated this many times
	repeatedErrorThreshold    = 3 // same error seen this many times
)

// LoopDetectionHook tracks tool call patterns and injects reconsidering
// context into the agent's next prompt when repetitive behaviour is detected.
//
// Three detection signals are supported:
//  1. Same file (resource) edited N times → edit-threshold intervention.
//  2. Same tool called with identical args 3 consecutive times → identical-call intervention.
//  3. Same error message seen 3 times → repeated-error intervention.
//
// Configuration (agent.toml):
//
//	[hooks]
//	loop_detection               = true
//	loop_detection_edit_threshold = 5
type LoopDetectionHook struct {
	// EditThreshold is the number of edits to the same resource before an
	// intervention message is injected. Default: 5.
	EditThreshold int
	// ToolCallWindow is the rolling window size used when scanning for
	// consecutive identical calls. Default: 20.
	ToolCallWindow int

	// editCounts tracks per-resource edit call counts.
	editCounts map[string]int
	// recentCalls is the rolling window of the most recent ToolCalls.
	recentCalls []ToolCall
	// errorCounts tracks per-error-message occurrence counts.
	errorCounts map[string]int
}

// NewLoopDetectionHook creates a LoopDetectionHook with sensible defaults.
func NewLoopDetectionHook() *LoopDetectionHook {
	return &LoopDetectionHook{
		EditThreshold:  defaultEditThreshold,
		ToolCallWindow: defaultToolCallWindow,
		editCounts:     make(map[string]int),
		recentCalls:    nil,
		errorCounts:    make(map[string]int),
	}
}

// OnToolCall is called after every tool invocation. It updates internal
// tracking state and returns a non-empty intervention message when a
// repetitive pattern is detected. Callers should inject the returned
// message into the next LLM prompt when it is non-empty.
//
// The call argument must be populated with the tool's error field when the
// tool returned an error result so that the repeated-error signal fires.
//
// Signal priority (highest to lowest):
//  1. Same resource edited N times (most actionable — names the file).
//  2. Same error seen N times (surfaces root-cause guidance).
//  3. Same tool+args called consecutively N times (general loop signal).
func (h *LoopDetectionHook) OnToolCall(call ToolCall) string {
	h.ensureInit()

	// ── Maintain rolling window ───────────────────────────────────────────
	h.recentCalls = append(h.recentCalls, call)
	if len(h.recentCalls) > h.ToolCallWindow {
		h.recentCalls = h.recentCalls[len(h.recentCalls)-h.ToolCallWindow:]
	}

	// ── Signal 1: same resource edited N times ────────────────────────────
	if isEditTool(call.Name) {
		resource := extractResource(call)
		if resource != "" {
			h.editCounts[resource]++
			count := h.editCounts[resource]
			if count >= h.EditThreshold {
				return fmt.Sprintf(
					"You have edited %q %d times. Consider reconsidering your approach — "+
						"the current strategy may not be working.",
					resource, count,
				)
			}
		}
	}

	// ── Signal 2: same error message seen N times ─────────────────────────
	// Checked before the consecutive-call signal so that error context is
	// preserved: when a tool fails with the same message repeatedly the
	// error-specific guidance is more useful than a generic "same args" warning.
	if call.Error != "" {
		normalized := normalizeError(call.Error)
		h.errorCounts[normalized]++
		count := h.errorCounts[normalized]
		if count >= repeatedErrorThreshold {
			return fmt.Sprintf(
				"This error has appeared %d times: %q. "+
					"The current fix is not working — reconsider the root cause.",
				count, call.Error,
			)
		}
	}

	// ── Signal 3: same tool+args called consecutively N times ────────────
	if msg := h.detectConsecutiveCalls(call); msg != "" {
		return msg
	}

	return ""
}

// Reset clears all accumulated state. Call this when the agent genuinely
// changes its approach so that counters start fresh.
func (h *LoopDetectionHook) Reset() {
	h.editCounts = make(map[string]int)
	h.recentCalls = nil
	h.errorCounts = make(map[string]int)
}

// ResetResource removes the edit counter for a specific resource, allowing
// the agent to revisit a file after explicitly acknowledging the intervention.
func (h *LoopDetectionHook) ResetResource(resource string) {
	delete(h.editCounts, resource)
}

// ensureInit lazily initialises maps in case the struct was zero-valued.
func (h *LoopDetectionHook) ensureInit() {
	if h.editCounts == nil {
		h.editCounts = make(map[string]int)
	}
	if h.errorCounts == nil {
		h.errorCounts = make(map[string]int)
	}
	if h.EditThreshold == 0 {
		h.EditThreshold = defaultEditThreshold
	}
	if h.ToolCallWindow == 0 {
		h.ToolCallWindow = defaultToolCallWindow
	}
}

// detectConsecutiveCalls checks whether the last consecutiveCallThreshold
// entries in the rolling window are identical (same name + same arguments).
func (h *LoopDetectionHook) detectConsecutiveCalls(latest ToolCall) string {
	n := len(h.recentCalls)
	if n < consecutiveCallThreshold {
		return ""
	}

	// Build a canonical key for the latest call.
	latestKey := callKey(latest)

	// Walk backwards from the second-to-last entry (latest is already appended).
	consecutive := 1
	for i := n - 2; i >= 0 && consecutive < consecutiveCallThreshold; i-- {
		if callKey(h.recentCalls[i]) == latestKey {
			consecutive++
		} else {
			break
		}
	}

	if consecutive >= consecutiveCallThreshold {
		return fmt.Sprintf(
			"You are calling %q with the same arguments %d times in a row — "+
				"try a different approach.",
			latest.Name, consecutive,
		)
	}
	return ""
}

// ─────────────────────────────────────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────────────────────────────────────

// editToolNames is the set of tool names that are considered "edits" for the
// purposes of the same-resource edit threshold signal.
var editToolNames = map[string]bool{
	"write_file": true,
	"edit_file":  true,
	"patch_file": true,
	"sed":        true,
	"awk":        true,
	"truncate":   true,
}

// isEditTool reports whether the tool name represents a file-edit operation.
func isEditTool(name string) bool {
	return editToolNames[name]
}

// extractResource returns the file path / resource name from a tool call's
// arguments. It checks the most common parameter names used across tools.
func extractResource(call ToolCall) string {
	for _, key := range []string{"path", "file", "filename", "target", "resource"} {
		if v, ok := call.Arguments[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// callKey produces a stable string representation of a ToolCall for
// duplicate-detection purposes. The key includes the tool name and all
// argument key-value pairs sorted deterministically.
func callKey(call ToolCall) string {
	var sb strings.Builder
	sb.WriteString(call.Name)
	sb.WriteByte('|')

	// Sort argument keys for deterministic comparison.
	keys := make([]string, 0, len(call.Arguments))
	for k := range call.Arguments {
		keys = append(keys, k)
	}
	// Simple insertion sort — argument lists are tiny.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	for _, k := range keys {
		fmt.Fprintf(&sb, "%s=%v;", k, call.Arguments[k])
	}
	return sb.String()
}

// normalizeError trims whitespace and lowercases an error string so that
// minor formatting variations don't prevent matching.
func normalizeError(msg string) string {
	return strings.TrimSpace(strings.ToLower(msg))
}
