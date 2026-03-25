// Package loop provides the hook-aware agent execution wrapper.
//
// Runner wraps an underlying agent execution function with opt-in middleware
// hooks (PreCompletionHook, LoopDetectionHook). It is designed to slot into
// the EvoClaw orchestrator without altering the existing ToolLoop logic:
// callers upgrade to the Runner gradually, one agent at a time, by setting
// the relevant fields in HookConfig.
//
// Wiring example (orchestrator side):
//
//	runner := loop.NewRunner(loop.HookConfig{
//	    PreCompletion:    loop.VerificationChecklistHook(cfg.Hooks.PreCompletionPrompt),
//	    LoopDetection:    loop.NewLoopDetectionHook(),
//	    UsePreCompletion: cfg.Hooks.PreCompletion,
//	    UseLoopDetection: cfg.Hooks.LoopDetection,
//	})
//	result, err := runner.Execute(ctx, state, executeFn, isCompleteFn)
package loop

import (
	"context"
)

// HookConfig holds the hook instances and their enabled flags.
// Both hooks are opt-in and default to disabled to preserve existing
// behaviour for agents that have not set [hooks] in agent.toml.
type HookConfig struct {
	// PreCompletion is the hook invoked when the agent signals completion.
	// Nil is safe — the hook is skipped when nil.
	PreCompletion PreCompletionHook
	// LoopDetection is the hook invoked after each tool call.
	// Nil is safe — the hook is skipped when nil.
	LoopDetection *LoopDetectionHook

	// UsePreCompletion enables the PreCompletion hook.
	// Maps to [hooks] pre_completion = true in agent.toml.
	UsePreCompletion bool
	// UseLoopDetection enables the LoopDetection hook.
	// Maps to [hooks] loop_detection = true in agent.toml.
	UseLoopDetection bool
}

// Runner decorates an underlying agent execution step with the configured
// middleware hooks. It is stateless across calls except for the mutable
// LoopDetectionHook (which intentionally accumulates state within a session).
type Runner struct {
	cfg HookConfig
}

// NewRunner creates a Runner with the given HookConfig.
func NewRunner(cfg HookConfig) *Runner {
	return &Runner{cfg: cfg}
}

// ExecuteStep runs one logical agent turn with hook middleware applied.
//
// Parameters:
//   - ctx is propagated to the PreCompletionHook.
//   - state is the current agent state. Hooks may modify it.
//   - toolCalls are the tool calls produced during this turn (used by
//     LoopDetectionHook). Pass nil when no tool calls were made.
//   - toolCallComplete signals that the provided toolCalls slice has
//     been fully executed and their results are available.
//   - agentComplete signals that the agent considers the task complete.
//
// Returns:
//   - updatedState is the (possibly hook-modified) agent state.
//   - injected is the list of messages the loop should prepend to the next
//     LLM prompt (non-nil only when a hook requests another iteration).
//   - continueLoop is true when a hook wants the agent to iterate again.
//   - loopWarning is a non-empty intervention message from LoopDetectionHook.
func (r *Runner) ExecuteStep(
	ctx context.Context,
	state AgentState,
	toolCalls []ToolCall,
	agentComplete bool,
) (updatedState AgentState, injected []string, continueLoop bool, loopWarning string) {
	updatedState = state

	// ── LoopDetectionHook: run after every tool call ──────────────────────
	if r.cfg.UseLoopDetection && r.cfg.LoopDetection != nil {
		for _, call := range toolCalls {
			if msg := r.cfg.LoopDetection.OnToolCall(call); msg != "" {
				loopWarning = msg
				// Only report the first warning per batch to avoid flooding.
				break
			}
		}
	}

	// ── PreCompletionHook: run only on completion signal ──────────────────
	if agentComplete && r.cfg.UsePreCompletion && r.cfg.PreCompletion != nil {
		var cont bool
		updatedState, cont = r.cfg.PreCompletion(ctx, updatedState)
		if cont {
			injected = updatedState.InjectedMessages
			// Clear injected messages from state now that they have been
			// handed to the caller; they should not be re-injected.
			updatedState.InjectedMessages = nil
			continueLoop = true
		}
	}

	return updatedState, injected, continueLoop, loopWarning
}
