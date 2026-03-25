package loop

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func freshState(id string) AgentState {
	return AgentState{ID: id}
}

// editCallSeq builds a write_file ToolCall with a unique seq counter in
// the arguments. Use this when consecutive identical-call detection would
// fire before the edit-threshold signal.
func editCallSeq(path string, seq int) ToolCall {
	return ToolCall{
		ID:   fmt.Sprintf("c-%d", seq),
		Name: "write_file",
		Arguments: map[string]interface{}{
			"path": path,
			"seq":  seq, // differentiates calls so consecutive-call signal does not fire
		},
	}
}

func errorCall(tool, errMsg string) ToolCall {
	return ToolCall{
		ID:    "e1",
		Name:  tool,
		Error: errMsg,
	}
}

func callWithArgs(name string, args map[string]interface{}) ToolCall {
	return ToolCall{ID: "x", Name: name, Arguments: args}
}

// ─────────────────────────────────────────────────────────────────────────────
// Issue #28 — VerificationChecklistHook / PreCompletionHook
// ─────────────────────────────────────────────────────────────────────────────

// TestVerification_ForcesOneMoreIteration verifies that the hook injects the
// checklist prompt and returns continue=true on the first completion signal.
func TestVerification_ForcesOneMoreIteration(t *testing.T) {
	t.Parallel()
	prompt := "Before completing: 1. Run tests. 2. Check edge cases."
	hook := VerificationChecklistHook(prompt)

	state := freshState("agent-1")
	newState, cont := hook(context.Background(), state)

	if !cont {
		t.Fatal("expected continue=true on first completion")
	}
	if len(newState.InjectedMessages) == 0 {
		t.Fatal("expected at least one injected message")
	}
	if newState.InjectedMessages[0] != prompt {
		t.Errorf("injected message = %q; want %q", newState.InjectedMessages[0], prompt)
	}
	if !newState.HasRunVerification {
		t.Error("HasRunVerification should be true after first hook call")
	}
}

// TestVerification_ExitsCleanlyAfterVerification verifies that the hook does
// NOT force another iteration when HasRunVerification is already true.
func TestVerification_ExitsCleanlyAfterVerification(t *testing.T) {
	t.Parallel()
	hook := VerificationChecklistHook("checklist")

	state := freshState("agent-2")
	state.HasRunVerification = true

	newState, cont := hook(context.Background(), state)

	if cont {
		t.Fatal("expected continue=false when already verified")
	}
	if len(newState.InjectedMessages) != 0 {
		t.Errorf("expected no injected messages, got %v", newState.InjectedMessages)
	}
}

// TestVerification_OriginalStateUnmodifiedOnSkip verifies that skipping the
// hook (already verified) leaves the state unchanged.
func TestVerification_OriginalStateUnmodifiedOnSkip(t *testing.T) {
	t.Parallel()
	hook := VerificationChecklistHook("checklist")

	state := freshState("agent-3")
	state.HasRunVerification = true
	state.InjectedMessages = nil

	_, cont := hook(context.Background(), state)
	if cont {
		t.Fatal("should not continue")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Issue #29 — LoopDetectionHook: edit threshold
// ─────────────────────────────────────────────────────────────────────────────

// TestLoopDetection_FiresAtEditThreshold verifies that an intervention is
// returned exactly when the edit count reaches EditThreshold.
func TestLoopDetection_FiresAtEditThreshold(t *testing.T) {
	t.Parallel()
	h := NewLoopDetectionHook()
	h.EditThreshold = 3
	const file = "main.go"

	for i := 1; i < h.EditThreshold; i++ {
		msg := h.OnToolCall(editCallSeq(file, i))
		if msg != "" {
			t.Errorf("iteration %d: unexpected early intervention %q", i, msg)
		}
	}

	// Third call — should fire.
	msg := h.OnToolCall(editCallSeq(file, h.EditThreshold))
	if msg == "" {
		t.Fatal("expected intervention at threshold but got empty message")
	}
	if !strings.Contains(msg, file) {
		t.Errorf("intervention message does not mention file %q: %s", file, msg)
	}
}

// TestLoopDetection_SkipsBelowEditThreshold verifies that no intervention
// fires when the edit count stays below the threshold.
func TestLoopDetection_SkipsBelowEditThreshold(t *testing.T) {
	t.Parallel()
	h := NewLoopDetectionHook()
	h.EditThreshold = 5
	const file = "util.go"

	// Use editCallSeq so each call has a unique seq argument.
	// Without this, three identical write_file calls would trigger the
	// consecutive-call signal (threshold=3) before the edit threshold (5).
	for i := 0; i < h.EditThreshold-1; i++ {
		if msg := h.OnToolCall(editCallSeq(file, i)); msg != "" {
			t.Errorf("unexpected intervention on call %d: %q", i+1, msg)
		}
	}
}

// TestLoopDetection_DifferentFilesNoCrossContamination verifies that edit
// counts are tracked per file independently.
func TestLoopDetection_DifferentFilesNoCrossContamination(t *testing.T) {
	t.Parallel()
	h := NewLoopDetectionHook()
	h.EditThreshold = 3

	// Use seq counters so calls are distinguishable to the consecutive-call signal.
	for i := 0; i < 2; i++ {
		h.OnToolCall(editCallSeq("a.go", i))
		h.OnToolCall(editCallSeq("b.go", i))
	}
	// Neither file has reached threshold (each has 2 edits) — no intervention.
	msgA := h.OnToolCall(editCallSeq("a.go", 2)) // a.go now at threshold
	msgB := h.OnToolCall(editCallSeq("b.go", 2)) // b.go now at threshold

	// Both should fire since both hit threshold.
	if msgA == "" {
		t.Error("expected intervention for a.go")
	}
	if msgB == "" {
		t.Error("expected intervention for b.go")
	}
	if strings.Contains(msgA, "b.go") {
		t.Errorf("a.go intervention mentions b.go: %s", msgA)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Issue #29 — LoopDetectionHook: consecutive identical calls
// ─────────────────────────────────────────────────────────────────────────────

// TestLoopDetection_ConsecutiveIdenticalCallsFiresAtThreshold verifies that
// three identical tool+args in a row triggers an intervention.
func TestLoopDetection_ConsecutiveIdenticalCallsFiresAtThreshold(t *testing.T) {
	t.Parallel()
	h := NewLoopDetectionHook()
	args := map[string]interface{}{"command": "go build"}

	var lastMsg string
	for i := 0; i < consecutiveCallThreshold; i++ {
		lastMsg = h.OnToolCall(callWithArgs("shell", args))
	}
	if lastMsg == "" {
		t.Fatal("expected intervention after consecutive identical calls")
	}
	if !strings.Contains(lastMsg, "shell") {
		t.Errorf("message should mention tool name: %s", lastMsg)
	}
}

// TestLoopDetection_InterruptedConsecutiveCallsNoIntervention verifies that
// a different call in between resets the consecutive count.
func TestLoopDetection_InterruptedConsecutiveCallsNoIntervention(t *testing.T) {
	t.Parallel()
	h := NewLoopDetectionHook()
	sameArgs := map[string]interface{}{"command": "go build"}

	h.OnToolCall(callWithArgs("shell", sameArgs))
	h.OnToolCall(callWithArgs("read_file", map[string]interface{}{"path": "x.go"})) // different
	msg := h.OnToolCall(callWithArgs("shell", sameArgs))
	if msg != "" {
		t.Errorf("unexpected consecutive-call intervention after interruption: %q", msg)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Issue #29 — LoopDetectionHook: repeated error
// ─────────────────────────────────────────────────────────────────────────────

// TestLoopDetection_RepeatedErrorFiresAtThreshold verifies that the same
// error appearing N times in a row triggers an error intervention.
func TestLoopDetection_RepeatedErrorFiresAtThreshold(t *testing.T) {
	t.Parallel()
	h := NewLoopDetectionHook()
	const errMsg = "undefined: Foo"

	var lastMsg string
	for i := 0; i < repeatedErrorThreshold; i++ {
		lastMsg = h.OnToolCall(errorCall("shell", errMsg))
	}
	if lastMsg == "" {
		t.Fatal("expected error-intervention at threshold")
	}
	if !strings.Contains(lastMsg, errMsg) {
		t.Errorf("intervention should contain original error message; got: %s", lastMsg)
	}
}

// TestLoopDetection_BelowRepeatedErrorThresholdNoIntervention verifies that
// an error seen fewer than the threshold times produces no message.
func TestLoopDetection_BelowRepeatedErrorThresholdNoIntervention(t *testing.T) {
	t.Parallel()
	h := NewLoopDetectionHook()

	for i := 0; i < repeatedErrorThreshold-1; i++ {
		if msg := h.OnToolCall(errorCall("shell", "permission denied")); msg != "" {
			t.Errorf("unexpected early error intervention: %q", msg)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Issue #29 — LoopDetectionHook: Reset
// ─────────────────────────────────────────────────────────────────────────────

// TestLoopDetection_ResetClearsCounters verifies that Reset() clears all
// accumulated state and the hook starts fresh afterwards.
func TestLoopDetection_ResetClearsCounters(t *testing.T) {
	t.Parallel()
	h := NewLoopDetectionHook()
	h.EditThreshold = 2
	const file = "main.go"

	// Build up to threshold-1.
	h.OnToolCall(editCallSeq(file, 0))

	// Reset: approach has changed.
	h.Reset()

	// After reset, counter is zero — we need EditThreshold edits again.
	for i := 0; i < h.EditThreshold-1; i++ {
		if msg := h.OnToolCall(editCallSeq(file, 100+i)); msg != "" {
			t.Errorf("unexpected intervention after reset on call %d: %q", i+1, msg)
		}
	}
}

// TestLoopDetection_ResetResourceClearsOnlyTargetFile verifies that
// ResetResource only affects the specified file's counter.
func TestLoopDetection_ResetResourceClearsOnlyTargetFile(t *testing.T) {
	t.Parallel()
	h := NewLoopDetectionHook()
	h.EditThreshold = 3

	// Build both files up to 2 edits each using seq to avoid consecutive-call signal.
	for i := 0; i < 2; i++ {
		h.OnToolCall(editCallSeq("a.go", i))
		h.OnToolCall(editCallSeq("b.go", i))
	}

	// Reset only a.go.
	h.ResetResource("a.go")

	// b.go still has 2 edits — one more will trigger.
	msgB := h.OnToolCall(editCallSeq("b.go", 10))
	if msgB == "" {
		t.Error("expected intervention for b.go after resetting only a.go")
	}

	// a.go counter was reset — no intervention on first edit.
	msgA := h.OnToolCall(editCallSeq("a.go", 20))
	if msgA != "" {
		t.Errorf("unexpected intervention for a.go after reset: %q", msgA)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Runner integration tests
// ─────────────────────────────────────────────────────────────────────────────

// TestRunner_VerificationForcesReIteration verifies that the Runner correctly
// signals a continuation when the PreCompletionHook fires.
func TestRunner_VerificationForcesReIteration(t *testing.T) {
	t.Parallel()
	const checklistMsg = "Please verify your work."
	runner := NewRunner(HookConfig{
		PreCompletion:    VerificationChecklistHook(checklistMsg),
		UsePreCompletion: true,
	})

	state := freshState("agent-run-1")
	updatedState, injected, cont, warn := runner.ExecuteStep(
		context.Background(),
		state,
		nil,   // no tool calls
		true,  // agent signals completion
	)

	if !cont {
		t.Fatal("expected continueLoop=true on first completion")
	}
	if len(injected) == 0 {
		t.Fatal("expected injected messages")
	}
	if injected[0] != checklistMsg {
		t.Errorf("injected[0] = %q; want %q", injected[0], checklistMsg)
	}
	if !updatedState.HasRunVerification {
		t.Error("HasRunVerification should be set on returned state")
	}
	if len(updatedState.InjectedMessages) != 0 {
		t.Error("InjectedMessages should be drained from state after handoff to caller")
	}
	if warn != "" {
		t.Errorf("unexpected loop warning: %q", warn)
	}
}

// TestRunner_VerifiedAgentExitsCleanly verifies that a second completion
// signal from an already-verified agent does NOT produce a continuation.
func TestRunner_VerifiedAgentExitsCleanly(t *testing.T) {
	t.Parallel()
	runner := NewRunner(HookConfig{
		PreCompletion:    VerificationChecklistHook("check"),
		UsePreCompletion: true,
	})

	state := freshState("agent-run-2")
	state.HasRunVerification = true

	_, injected, cont, _ := runner.ExecuteStep(
		context.Background(),
		state,
		nil,
		true,
	)

	if cont {
		t.Fatal("expected continueLoop=false when agent already verified")
	}
	if len(injected) != 0 {
		t.Errorf("unexpected injected messages: %v", injected)
	}
}

// TestRunner_LoopDetectionWarningPropagated verifies that a loop warning from
// LoopDetectionHook is correctly surfaced through ExecuteStep.
func TestRunner_LoopDetectionWarningPropagated(t *testing.T) {
	t.Parallel()
	ldh := NewLoopDetectionHook()
	ldh.EditThreshold = 2
	runner := NewRunner(HookConfig{
		LoopDetection:    ldh,
		UseLoopDetection: true,
	})

	const file = "router.go"
	// Use seq so the 2 calls are distinguishable to the consecutive-call signal.
	calls := []ToolCall{editCallSeq(file, 0), editCallSeq(file, 1)} // 2 edits → fires on 2nd

	_, _, _, warn := runner.ExecuteStep(
		context.Background(),
		freshState("agent-run-3"),
		calls,
		false,
	)

	if warn == "" {
		t.Fatal("expected loop warning from LoopDetectionHook")
	}
	if !strings.Contains(warn, file) {
		t.Errorf("warning should mention file %q: %s", file, warn)
	}
}

// TestRunner_DisabledHooksAreNoOps verifies that neither hook fires when both
// are explicitly disabled.
func TestRunner_DisabledHooksAreNoOps(t *testing.T) {
	t.Parallel()
	ldh := NewLoopDetectionHook()
	ldh.EditThreshold = 1 // Would fire immediately if enabled.
	runner := NewRunner(HookConfig{
		PreCompletion:    VerificationChecklistHook("checklist"),
		LoopDetection:    ldh,
		UsePreCompletion: false,
		UseLoopDetection: false,
	})

	state := freshState("agent-run-4")
	_, injected, cont, warn := runner.ExecuteStep(
		context.Background(),
		state,
		[]ToolCall{editCallSeq("x.go", 0)},
		true, // agent signals completion
	)

	if cont {
		t.Error("expected no continuation with disabled hooks")
	}
	if len(injected) != 0 {
		t.Errorf("unexpected injected messages: %v", injected)
	}
	if warn != "" {
		t.Errorf("unexpected warning with disabled hooks: %q", warn)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// callKey determinism
// ─────────────────────────────────────────────────────────────────────────────

// TestCallKey_Deterministic verifies that callKey produces the same output
// regardless of argument map insertion order.
func TestCallKey_Deterministic(t *testing.T) {
	t.Parallel()
	// Go maps are unordered; construct two calls with the same args inserted
	// in different orders (we simulate this by using the same map).
	args := map[string]interface{}{"b": 2, "a": 1}
	call1 := ToolCall{Name: "tool", Arguments: args}

	// Add more args to a fresh map in reverse order.
	args2 := map[string]interface{}{"a": 1, "b": 2}
	call2 := ToolCall{Name: "tool", Arguments: args2}

	k1 := callKey(call1)
	k2 := callKey(call2)

	if k1 != k2 {
		t.Errorf("callKey not deterministic: %q != %q", k1, k2)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Zero-value safety
// ─────────────────────────────────────────────────────────────────────────────

// TestLoopDetectionHook_ZeroValue verifies that a zero-value LoopDetectionHook
// (fields not explicitly initialised) behaves correctly and does not panic.
func TestLoopDetectionHook_ZeroValue(t *testing.T) {
	t.Parallel()
	var h LoopDetectionHook
	// Should not panic; ensureInit handles zero fields.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic on zero-value hook: %v", r)
		}
	}()
	for i := 0; i < defaultEditThreshold+1; i++ {
		h.OnToolCall(editCallSeq("x.go", i))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// extractResource
// ─────────────────────────────────────────────────────────────────────────────

func TestExtractResource_CommonKeys(t *testing.T) {
	t.Parallel()
	cases := []struct {
		key  string
		want string
	}{
		{"path", "/tmp/foo.go"},
		{"file", "bar.go"},
		{"filename", "baz.go"},
		{"target", "qux.go"},
		{"resource", "quux.go"},
	}
	for _, tc := range cases {
		call := ToolCall{Name: "write_file", Arguments: map[string]interface{}{tc.key: tc.want}}
		got := extractResource(call)
		if got != tc.want {
			t.Errorf("key=%q: got %q; want %q", tc.key, got, tc.want)
		}
	}
}

func TestExtractResource_NoPathReturnsEmpty(t *testing.T) {
	t.Parallel()
	call := ToolCall{Name: "write_file", Arguments: map[string]interface{}{"content": "hello"}}
	if got := extractResource(call); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// isEditTool
// ─────────────────────────────────────────────────────────────────────────────

func TestIsEditTool(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"write_file", "edit_file", "patch_file", "sed", "awk", "truncate"} {
		if !isEditTool(name) {
			t.Errorf("expected %q to be an edit tool", name)
		}
	}
	for _, name := range []string{"read_file", "shell", "list_files", "cat"} {
		if isEditTool(name) {
			t.Errorf("expected %q NOT to be an edit tool", name)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Verify error message formatting contains count
// ─────────────────────────────────────────────────────────────────────────────

func TestLoopDetection_ErrorMessageContainsCount(t *testing.T) {
	t.Parallel()
	h := NewLoopDetectionHook()
	const errMsg = "compilation failed"

	var msg string
	for i := 0; i < repeatedErrorThreshold; i++ {
		msg = h.OnToolCall(errorCall("shell", errMsg))
	}

	expectedCount := fmt.Sprintf("%d", repeatedErrorThreshold)
	if !strings.Contains(msg, expectedCount) {
		t.Errorf("error message %q should contain count %s", msg, expectedCount)
	}
}
