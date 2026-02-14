# Capability Escalation & Skill Hardening Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add capability-based model escalation to the Attractor engine and harden the dotfile creation skill with patterns that prevent common pipeline failures (compile-fix loops, cross-branch divergence, incomplete scaffolds).

**Architecture:** Five changes across two systems. Engine gets new failure classes (`budget_exhausted`, `compilation_loop`) that unlock retries for capability failures, plus a node-level `escalation_models` attribute that cycles through progressively more capable models on retry. The dotfile skill gets four new prompt/pattern guidelines. All changes are backward-compatible — existing dotfiles and run configs work unchanged.

**Tech Stack:** Go (engine), Markdown (skill docs), existing test harness (`go test ./internal/attractor/engine/...`)

---

### Task 1: Add new failure class constants

**Files:**
- Modify: `internal/attractor/engine/loop_restart_policy.go:12-17`
- Test: `internal/attractor/engine/loop_restart_policy_test.go` (existing, will add cases)

**Step 1: Write failing tests for the new failure classes**

Add to the existing test file (or create a new focused test file). These test that `normalizedFailureClass` recognizes the new classes and that `classifyFailureClass` uses the new heuristic hints.

```go
// In a new file: internal/attractor/engine/failure_class_expansion_test.go
package engine

import (
	"testing"

	"github.com/danshapiro/kilroy/internal/attractor/runtime"
)

func TestNormalizedFailureClass_BudgetExhausted(t *testing.T) {
	cases := []string{"budget_exhausted", "budget-exhausted", "budget exhausted"}
	for _, c := range cases {
		if got := normalizedFailureClass(c); got != failureClassBudgetExhausted {
			t.Errorf("normalizedFailureClass(%q) = %q, want %q", c, got, failureClassBudgetExhausted)
		}
	}
}

func TestNormalizedFailureClass_CompilationLoop(t *testing.T) {
	cases := []string{"compilation_loop", "compilation-loop", "compilation loop", "compile_loop"}
	for _, c := range cases {
		if got := normalizedFailureClass(c); got != failureClassCompilationLoop {
			t.Errorf("normalizedFailureClass(%q) = %q, want %q", c, got, failureClassCompilationLoop)
		}
	}
}

func TestClassifyFailureClass_TurnLimitHint(t *testing.T) {
	out := runtime.Outcome{
		Status:        runtime.StatusFail,
		FailureReason: "turn limit reached (max_turns=60)",
	}
	if got := classifyFailureClass(out); got != failureClassBudgetExhausted {
		t.Errorf("classifyFailureClass(turn limit) = %q, want %q", got, failureClassBudgetExhausted)
	}
}

func TestClassifyFailureClass_MaxTokensHint(t *testing.T) {
	out := runtime.Outcome{
		Status:        runtime.StatusFail,
		FailureReason: "max tokens exceeded for session",
	}
	if got := classifyFailureClass(out); got != failureClassBudgetExhausted {
		t.Errorf("classifyFailureClass(max tokens) = %q, want %q", got, failureClassBudgetExhausted)
	}
}

func TestClassifyFailureClass_ExplicitHintOverridesHeuristic(t *testing.T) {
	out := runtime.Outcome{
		Status:        runtime.StatusFail,
		FailureReason: "turn limit reached",
		Meta:          map[string]any{"failure_class": "deterministic"},
	}
	// Explicit hint should win over heuristic
	if got := classifyFailureClass(out); got != failureClassDeterministic {
		t.Errorf("classifyFailureClass(explicit deterministic) = %q, want %q", got, failureClassDeterministic)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /home/user/code/kilroy && go test ./internal/attractor/engine/ -run "TestNormalizedFailureClass_Budget|TestNormalizedFailureClass_Compilation|TestClassifyFailureClass_TurnLimit|TestClassifyFailureClass_MaxTokens|TestClassifyFailureClass_ExplicitHint" -v`
Expected: FAIL — `failureClassBudgetExhausted` and `failureClassCompilationLoop` undefined

**Step 3: Add the new constants and recognition logic**

In `internal/attractor/engine/loop_restart_policy.go`:

Add constants (after line 15):
```go
const (
	failureClassTransientInfra       = "transient_infra"
	failureClassDeterministic        = "deterministic"
	failureClassCanceled             = "canceled"
	failureClassBudgetExhausted      = "budget_exhausted"
	failureClassCompilationLoop      = "compilation_loop"
	defaultLoopRestartSignatureLimit = 3
)
```

Add heuristic hints — create a new `budgetExhaustedReasonHints` slice after the existing `transientInfraReasonHints` (after line 49):
```go
	budgetExhaustedReasonHints = []string{
		"turn limit",
		"max_turns",
		"max turns",
		"token limit",
		"max tokens",
		"max_tokens",
		"context length exceeded",
		"context window exceeded",
		"budget exhausted",
	}
```

Update `classifyFailureClass` to check budget hints AFTER transient infra but BEFORE the default deterministic fallback (between lines 75-76):
```go
	for _, hint := range budgetExhaustedReasonHints {
		if strings.Contains(reason, hint) {
			return failureClassBudgetExhausted
		}
	}
```

Update `normalizedFailureClass` to recognize the new classes (add cases before the default):
```go
	case "budget_exhausted", "budget-exhausted", "budget exhausted", "budget":
		return failureClassBudgetExhausted
	case "compilation_loop", "compilation-loop", "compilation loop", "compile_loop", "compile-loop":
		return failureClassCompilationLoop
```

**Step 4: Run tests to verify they pass**

Run: `cd /home/user/code/kilroy && go test ./internal/attractor/engine/ -run "TestNormalizedFailureClass_Budget|TestNormalizedFailureClass_Compilation|TestClassifyFailureClass_TurnLimit|TestClassifyFailureClass_MaxTokens|TestClassifyFailureClass_ExplicitHint" -v`
Expected: PASS

**Step 5: Run all existing tests to verify no regressions**

Run: `cd /home/user/code/kilroy && go test ./internal/attractor/engine/ -v -count=1 2>&1 | tail -30`
Expected: all existing tests PASS (the new constants don't affect any existing code paths because `shouldRetryOutcome` still only checks for `transient_infra`)

**Step 6: Commit**

```bash
git add internal/attractor/engine/loop_restart_policy.go internal/attractor/engine/failure_class_expansion_test.go
git commit -m "attractor: add budget_exhausted and compilation_loop failure classes

Add two new failure classification constants for capability-class failures
that are distinct from deterministic (permanent) and transient_infra
(availability) failures. These represent cases where the model ran out of
budget (turn/token limits) or got stuck in a compile-fix-regress loop.

New heuristic hints detect 'turn limit', 'max_turns', 'token limit', etc.
in failure reasons and classify them as budget_exhausted.

normalizedFailureClass now recognizes both new classes from explicit hints.

No behavioral change yet — shouldRetryOutcome still only retries
transient_infra. Retry gating update follows in the next commit."
```

---

### Task 2: Update retry gating for capability failures

**Files:**
- Modify: `internal/attractor/engine/failure_policy.go:5-10`
- Test: `internal/attractor/engine/failure_policy_test.go`

**Step 1: Write failing tests for new retry behavior**

Add test cases to the existing `TestShouldRetryOutcome_ClassGated` in `failure_policy_test.go`:

```go
		{
			name:  "fail budget_exhausted retries",
			out:   runtime.Outcome{Status: runtime.StatusFail, FailureReason: "turn limit reached"},
			class: failureClassBudgetExhausted,
			want:  true,
		},
		{
			name:  "fail compilation_loop retries",
			out:   runtime.Outcome{Status: runtime.StatusFail, FailureReason: "same 3 errors after 20 turns"},
			class: failureClassCompilationLoop,
			want:  true,
		},
		{
			name:  "retry budget_exhausted retries",
			out:   runtime.Outcome{Status: runtime.StatusRetry, FailureReason: "budget spent"},
			class: failureClassBudgetExhausted,
			want:  true,
		},
```

**Step 2: Run tests to verify they fail**

Run: `cd /home/user/code/kilroy && go test ./internal/attractor/engine/ -run "TestShouldRetryOutcome" -v`
Expected: FAIL on the new cases (budget_exhausted and compilation_loop currently return false)

**Step 3: Update shouldRetryOutcome**

Replace `failure_policy.go` contents:

```go
package engine

import "github.com/danshapiro/kilroy/internal/attractor/runtime"

// retryableFailureClasses lists failure classes that should trigger automatic retries.
// transient_infra: temporary infrastructure issues (API timeouts, rate limits)
// budget_exhausted: model ran out of turn/token budget (may succeed with retry or escalation)
// compilation_loop: model stuck in fix-regress cycle (may succeed with different approach on retry)
var retryableFailureClasses = map[string]bool{
	failureClassTransientInfra:  true,
	failureClassBudgetExhausted: true,
	failureClassCompilationLoop: true,
}

func shouldRetryOutcome(out runtime.Outcome, failureClass string) bool {
	if out.Status != runtime.StatusFail && out.Status != runtime.StatusRetry {
		return false
	}
	return retryableFailureClasses[normalizedFailureClassOrDefault(failureClass)]
}

// isEscalatableFailureClass returns true if the failure class should trigger
// model escalation (as opposed to same-model retry for transient issues).
func isEscalatableFailureClass(failureClass string) bool {
	cls := normalizedFailureClassOrDefault(failureClass)
	return cls == failureClassBudgetExhausted || cls == failureClassCompilationLoop
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /home/user/code/kilroy && go test ./internal/attractor/engine/ -run "TestShouldRetryOutcome" -v`
Expected: ALL cases PASS

**Step 5: Run full engine test suite**

Run: `cd /home/user/code/kilroy && go test ./internal/attractor/engine/ -v -count=1 2>&1 | tail -30`
Expected: PASS. Watch for `retry_classification_integration_test.go` and `deterministic_failure_cycle_test.go` — these test that deterministic failures DON'T retry, and our change preserves that. The new retryable classes are additive.

**Step 6: Commit**

```bash
git add internal/attractor/engine/failure_policy.go internal/attractor/engine/failure_policy_test.go
git commit -m "attractor: allow retries on budget_exhausted and compilation_loop failures

Previously only transient_infra failures triggered automatic retries.
Deterministic failures (the default) blocked retries, which meant
turn-limit-exhaustion and compile-fix loops caused immediate node failure
even when retries were available.

Now budget_exhausted and compilation_loop are retryable, enabling the
engine to try again (and, with escalation_models in the next commit,
to try with a different model).

Also adds isEscalatableFailureClass() helper for the escalation logic."
```

---

### Task 3: Implement escalation_models node attribute

This is the largest task. The engine's `executeWithRetry` learns to cycle through models on capability-class retries.

**Files:**
- Modify: `internal/attractor/engine/engine.go` (executeWithRetry function, ~lines 922-1048)
- Create: `internal/attractor/engine/escalation.go` (parsing + helpers)
- Create: `internal/attractor/engine/escalation_test.go`
- Modify: `internal/attractor/engine/failure_policy.go` (already has isEscalatableFailureClass from Task 2)

**Step 1: Write tests for escalation chain parsing**

Create `internal/attractor/engine/escalation_test.go`:

```go
package engine

import "testing"

func TestParseEscalationModels_Empty(t *testing.T) {
	chain := parseEscalationModels("")
	if len(chain) != 0 {
		t.Fatalf("expected empty chain, got %d entries", len(chain))
	}
}

func TestParseEscalationModels_SingleEntry(t *testing.T) {
	chain := parseEscalationModels("kimi:kimi-k2.5")
	if len(chain) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(chain))
	}
	if chain[0].Provider != "kimi" || chain[0].Model != "kimi-k2.5" {
		t.Fatalf("got %+v", chain[0])
	}
}

func TestParseEscalationModels_MultipleEntries(t *testing.T) {
	chain := parseEscalationModels("kimi:kimi-k2.5, google:gemini-pro, anthropic:claude-opus-4-6")
	if len(chain) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(chain))
	}
	expected := []providerModel{
		{Provider: "kimi", Model: "kimi-k2.5"},
		{Provider: "google", Model: "gemini-pro"},
		{Provider: "anthropic", Model: "claude-opus-4-6"},
	}
	for i, e := range expected {
		if chain[i].Provider != e.Provider || chain[i].Model != e.Model {
			t.Errorf("entry %d: got %+v, want %+v", i, chain[i], e)
		}
	}
}

func TestParseEscalationModels_WhitespaceHandling(t *testing.T) {
	chain := parseEscalationModels("  kimi : kimi-k2.5 ,  google : gemini-pro  ")
	if len(chain) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(chain))
	}
	if chain[0].Provider != "kimi" || chain[1].Provider != "google" {
		t.Fatalf("providers: %q, %q", chain[0].Provider, chain[1].Provider)
	}
}

func TestParseEscalationModels_InvalidEntrySkipped(t *testing.T) {
	chain := parseEscalationModels("kimi:kimi-k2.5, badentry, google:gemini-pro")
	if len(chain) != 2 {
		t.Fatalf("expected 2 entries (invalid skipped), got %d", len(chain))
	}
}

func TestRetriesBeforeEscalation_Default(t *testing.T) {
	got := retriesBeforeEscalation(nil)
	if got != defaultRetriesBeforeEscalation {
		t.Fatalf("got %d, want %d", got, defaultRetriesBeforeEscalation)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /home/user/code/kilroy && go test ./internal/attractor/engine/ -run "TestParseEscalationModels|TestRetriesBeforeEscalation" -v`
Expected: FAIL — functions not defined

**Step 3: Implement escalation.go**

Create `internal/attractor/engine/escalation.go`:

```go
package engine

import (
	"strings"

	"github.com/danshapiro/kilroy/internal/attractor/model"
)

const defaultRetriesBeforeEscalation = 2

// parseEscalationModels parses a comma-separated list of "provider:model" pairs
// from the escalation_models node attribute. Invalid entries (missing colon) are skipped.
func parseEscalationModels(raw string) []providerModel {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	var chain []providerModel
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		idx := strings.Index(part, ":")
		if idx < 0 {
			continue // skip malformed entries
		}
		prov := strings.TrimSpace(part[:idx])
		mod := strings.TrimSpace(part[idx+1:])
		if prov == "" || mod == "" {
			continue
		}
		chain = append(chain, providerModel{Provider: normalizeProviderKey(prov), Model: mod})
	}
	return chain
}

// retriesBeforeEscalation returns the number of same-model retries allowed before
// escalating to the next model in the chain. Read from the graph attribute
// "retries_before_escalation", defaulting to 2 (meaning 3 total attempts per model).
func retriesBeforeEscalation(g *model.Graph) int {
	if g == nil {
		return defaultRetriesBeforeEscalation
	}
	v := parseInt(g.Attrs["retries_before_escalation"], defaultRetriesBeforeEscalation)
	if v < 0 {
		return 0
	}
	return v
}
```

**Step 4: Run parsing tests to verify they pass**

Run: `cd /home/user/code/kilroy && go test ./internal/attractor/engine/ -run "TestParseEscalationModels|TestRetriesBeforeEscalation" -v`
Expected: PASS

**Step 5: Commit parsing logic**

```bash
git add internal/attractor/engine/escalation.go internal/attractor/engine/escalation_test.go
git commit -m "attractor: add escalation_models parser and helpers

Parses the escalation_models node attribute (comma-separated
provider:model pairs) and retrieves retries_before_escalation from
graph attributes (default: 2, meaning 3 attempts per model before
escalating to the next).

No behavioral change yet — executeWithRetry integration follows."
```

**Step 6: Write integration test for escalation in executeWithRetry**

Add to `internal/attractor/engine/escalation_test.go`:

```go
func TestExecuteWithRetry_EscalatesOnBudgetExhausted(t *testing.T) {
	// This test verifies that when a node has escalation_models and fails
	// with budget_exhausted, the engine cycles through the escalation chain.
	//
	// Setup: a node that always fails with budget_exhausted, escalation chain
	// of 2 models, retries_before_escalation=1 (2 attempts per model).
	// With max_retries=5 (6 total attempts), we expect:
	//   attempt 1-2: default model (from node attrs)
	//   attempt 3-4: first escalation model
	//   attempt 5-6: second escalation model
	//
	// We track which provider:model was used on each attempt via progress events.

	g := &model.Graph{
		Attrs: map[string]string{
			"retries_before_escalation": "1",
		},
		Nodes: map[string]*model.Node{},
	}
	node := &model.Node{
		ID: "impl_test",
		Attrs: map[string]string{
			"llm_model":         "default-model",
			"llm_provider":      "default-prov",
			"max_retries":       "5",
			"escalation_models": "esc1:esc1-model, esc2:esc2-model",
		},
	}
	g.Nodes[node.ID] = node

	// Record which model was on the node when executeNode was called.
	var attemptModels []string
	e := newTestEngine(g, func(ctx context.Context, n *model.Node) runtime.Outcome {
		attemptModels = append(attemptModels, n.Attr("llm_provider", "")+":"+n.Attr("llm_model", ""))
		return runtime.Outcome{
			Status:        runtime.StatusFail,
			FailureReason: "turn limit reached (max_turns=60)",
			Meta:          map[string]any{"failure_class": "budget_exhausted"},
		}
	})

	retries := map[string]int{}
	out, _ := e.executeWithRetry(context.Background(), node, retries)

	// Should have exhausted all 6 attempts
	if out.Status != runtime.StatusFail {
		t.Fatalf("expected fail, got %s", out.Status)
	}
	if len(attemptModels) != 6 {
		t.Fatalf("expected 6 attempts, got %d: %v", len(attemptModels), attemptModels)
	}

	// Verify model progression: 2x default, 2x esc1, 2x esc2
	expected := []string{
		"default-prov:default-model",
		"default-prov:default-model",
		"esc1:esc1-model",
		"esc1:esc1-model",
		"esc2:esc2-model",
		"esc2:esc2-model",
	}
	for i, want := range expected {
		if attemptModels[i] != want {
			t.Errorf("attempt %d: got %q, want %q", i+1, attemptModels[i], want)
		}
	}

	// Verify node attrs restored to original values
	if node.Attr("llm_model", "") != "default-model" {
		t.Errorf("node llm_model not restored: %q", node.Attr("llm_model", ""))
	}
	if node.Attr("llm_provider", "") != "default-prov" {
		t.Errorf("node llm_provider not restored: %q", node.Attr("llm_provider", ""))
	}
}

func TestExecuteWithRetry_NoEscalationOnTransientInfra(t *testing.T) {
	// Transient infra failures should retry with the same model, not escalate.
	g := &model.Graph{
		Attrs: map[string]string{
			"retries_before_escalation": "0",
		},
		Nodes: map[string]*model.Node{},
	}
	node := &model.Node{
		ID: "impl_test",
		Attrs: map[string]string{
			"llm_model":         "default-model",
			"llm_provider":      "default-prov",
			"max_retries":       "3",
			"escalation_models": "esc1:esc1-model",
		},
	}
	g.Nodes[node.ID] = node

	var attemptModels []string
	e := newTestEngine(g, func(ctx context.Context, n *model.Node) runtime.Outcome {
		attemptModels = append(attemptModels, n.Attr("llm_provider", "")+":"+n.Attr("llm_model", ""))
		return runtime.Outcome{
			Status:        runtime.StatusFail,
			FailureReason: "connection timeout",
			Meta:          map[string]any{"failure_class": "transient_infra"},
		}
	})

	retries := map[string]int{}
	e.executeWithRetry(context.Background(), node, retries)

	// All attempts should use default model (no escalation for transient)
	for i, m := range attemptModels {
		if m != "default-prov:default-model" {
			t.Errorf("attempt %d: got %q, want default model (transient should not escalate)", i+1, m)
		}
	}
}

func TestExecuteWithRetry_NoEscalationWithoutAttribute(t *testing.T) {
	// Without escalation_models attribute, behavior is unchanged.
	g := &model.Graph{Attrs: map[string]string{}, Nodes: map[string]*model.Node{}}
	node := &model.Node{
		ID: "impl_test",
		Attrs: map[string]string{
			"llm_model":    "default-model",
			"llm_provider": "default-prov",
			"max_retries":  "2",
		},
	}
	g.Nodes[node.ID] = node

	var attemptModels []string
	e := newTestEngine(g, func(ctx context.Context, n *model.Node) runtime.Outcome {
		attemptModels = append(attemptModels, n.Attr("llm_provider", "")+":"+n.Attr("llm_model", ""))
		return runtime.Outcome{
			Status:        runtime.StatusFail,
			FailureReason: "turn limit reached",
			Meta:          map[string]any{"failure_class": "budget_exhausted"},
		}
	})

	retries := map[string]int{}
	e.executeWithRetry(context.Background(), node, retries)

	// All attempts use default model (no escalation chain defined)
	for i, m := range attemptModels {
		if m != "default-prov:default-model" {
			t.Errorf("attempt %d: got %q, want default model", i+1, m)
		}
	}
}
```

NOTE: The `newTestEngine` helper may need to be adapted to the existing test infrastructure. Check `retry_policy_test.go` and `retry_exhaustion_routing_test.go` for how tests construct engines with mock handlers. The test above uses a simplified pattern — the implementer should match the existing test patterns (likely using `testShimGraph` or `runTestGraph` helpers from the test suite). The intent is clear: verify model progression on escalation.

**Step 7: Run integration tests to verify they fail**

Run: `cd /home/user/code/kilroy && go test ./internal/attractor/engine/ -run "TestExecuteWithRetry_Escalat|TestExecuteWithRetry_NoEscalation" -v`
Expected: FAIL — escalation logic not yet in executeWithRetry

**Step 8: Implement escalation in executeWithRetry**

Modify `internal/attractor/engine/engine.go`, function `executeWithRetry` (starting at ~line 922). The changes are:

1. Before the retry loop, parse the escalation chain and save original model/provider:

```go
	// --- Escalation setup ---
	escalationChain := parseEscalationModels(node.Attr("escalation_models", ""))
	rbe := retriesBeforeEscalation(e.Graph)
	origModel := node.Attrs["llm_model"]
	origProvider := node.Attrs["llm_provider"]
	defer func() {
		// Always restore original attrs, even on early return.
		node.Attrs["llm_model"] = origModel
		node.Attrs["llm_provider"] = origProvider
	}()
	currentModelAttempts := 0 // attempts on the current model (reset on escalation)
	escalationIdx := -1       // -1 = using default model; 0+ = index into escalationChain
```

2. Inside the retry loop, after `failureClass := classifyFailureClass(out)` (line 984) and replacing the current canRetry logic for non-tool nodes (~lines 986-995):

```go
		canRetry := false
		if attempt < maxAttempts {
			isToolNode := strings.TrimSpace(node.Attr("tool_command", "")) != ""
			if isToolNode {
				canRetry = out.Status == runtime.StatusFail || out.Status == runtime.StatusRetry
			} else if shouldRetryOutcome(out, failureClass) {
				canRetry = true
				// Check if escalation applies (capability failures, not transient)
				if isEscalatableFailureClass(failureClass) && len(escalationChain) > 0 {
					currentModelAttempts++
					if currentModelAttempts > rbe {
						// Escalate to next model in chain
						escalationIdx++
						if escalationIdx < len(escalationChain) {
							next := escalationChain[escalationIdx]
							node.Attrs["llm_model"] = next.Model
							node.Attrs["llm_provider"] = next.Provider
							currentModelAttempts = 0
							e.appendProgress(map[string]any{
								"event":         "escalation_model_switch",
								"node_id":       node.ID,
								"attempt":       attempt,
								"from_provider": origProvider,
								"from_model":    origModel,
								"to_provider":   next.Provider,
								"to_model":      next.Model,
								"escalation_idx": escalationIdx,
								"failure_class": failureClass,
							})
						}
						// If chain exhausted, canRetry stays true — will use last
						// escalated model until max_retries is hit.
					}
				}
				// For transient_infra: no model change, just retry same model.
			}
		}
```

Note: The `origModel`/`origProvider` in the progress event should track the PREVIOUS model, not always the original. Update to use current values before the switch. Adjust as needed during implementation.

**Step 9: Run integration tests to verify they pass**

Run: `cd /home/user/code/kilroy && go test ./internal/attractor/engine/ -run "TestExecuteWithRetry_Escalat|TestExecuteWithRetry_NoEscalation" -v`
Expected: PASS

**Step 10: Run full engine test suite**

Run: `cd /home/user/code/kilroy && go test ./internal/attractor/engine/ -v -count=1 2>&1 | tail -30`
Expected: ALL PASS. Critical regressions to watch: `retry_exhaustion_routing_test.go`, `deterministic_failure_cycle_test.go`, `conditional_retry_semantics_test.go`. The node attr save/restore ensures existing tests see unmodified attrs.

**Step 11: Commit**

```bash
git add internal/attractor/engine/engine.go internal/attractor/engine/escalation.go internal/attractor/engine/escalation_test.go
git commit -m "attractor: implement escalation_models for capability-based model cycling

When a codergen node fails with budget_exhausted or compilation_loop,
the engine now checks for an escalation_models node attribute containing
a comma-separated list of provider:model pairs. After
retries_before_escalation same-model retries (default: 2), the engine
switches to the next model in the chain and continues retrying.

This separates capability escalation (model can't solve the problem)
from availability failover (model is temporarily unreachable). The
existing failover: config handles availability; escalation_models
handles capability.

The escalation chain is ordered by cost and defined in the DOT file,
generated by the dotfile creation skill during Phase 0B model selection.

Node attributes are temporarily overridden during escalated attempts
and restored after the retry loop completes.

Transient infra failures continue to retry with the same model.
Deterministic failures continue to not retry at all."
```

---

### Task 4: Update dotfile creation skill — prompt patterns

**Files:**
- Modify: `skills/english-to-dotfile/SKILL.md`

This task adds three prompt-engineering patterns to the skill. No engine changes.

**Step 1: Add "clean build first" to implementation prompt template**

In `SKILL.md`, find the implementation prompt template (around line 648-666). Add after the "Acceptance" section and before the status JSON instruction:

```
Build-first strategy:
- FIRST MILESTONE: Achieve a clean `[BUILD_COMMAND]` with stub/skeleton implementations before filling in logic.
- If you spend more than a third of your turns on build errors without reaching a clean compile, simplify your approach: comment out broken code, add stubs, get to green, then iterate.
```

The updated template should read:

```
Goal: $goal

Implement [DESCRIPTION].

Spec: [SPEC_PATH], section [SECTION_REF].
Read: [DEPENDENCY_FILES] for types/interfaces you need.

Create/modify:
- [FILE_LIST]

Build-first strategy:
- FIRST MILESTONE: Achieve a clean `[BUILD_COMMAND]` with stub/skeleton implementations before filling in logic.
- If you spend more than a third of your turns on build errors without reaching a clean compile, simplify your approach: comment out broken code, add stubs, get to green, then iterate.

Acceptance:
- `[BUILD_COMMAND]` must pass
- `[TEST_COMMAND]` must pass

Write status JSON to `$KILROY_STAGE_STATUS_PATH` (absolute path). If unavailable, use `$KILROY_STAGE_STATUS_FALLBACK_PATH`. Do not write status.json in nested module directories after `cd`.
Write status JSON: outcome=success if all criteria pass, outcome=fail with failure_reason and details otherwise.
```

**Step 2: Add interface-pinning pattern to fanout documentation**

Find the anti-pattern #27 about overlapping fan-out write scopes (around line 863) and the existing fan-out documentation. Add a new subsection under Phase 2 or as a "Fan-out coordination pattern" section. Add before the anti-patterns section:

```markdown
#### Fan-out coordination: interface-pinning pattern

When a fan-out involves branches that implement against shared types or interfaces, add a `define_contracts` or `impl_scaffold` node BEFORE the fan-out `component` node. This node writes comprehensive shared type definitions, constants, function signatures, and interface contracts to well-known paths. Fan-out branch prompts reference these files as **read-only inputs**.

Why: Parallel branches run in isolated worktrees with no communication. If each branch independently defines shared types, they will diverge. The fan-in selects one winner and discards losers' changes, causing type mismatches at integration.

Pattern:
```
impl_scaffold -> verify_scaffold -> check_scaffold -> fanout -> [branches] -> fanin
```

The scaffold prompt should:
1. Define ALL shared types, constants, and enums comprehensively — anticipate what branches will need
2. Create stub modules with correct function signatures for every parallel branch to implement against
3. Verify the project compiles with stubs before proceeding to fan-out

Each fan-out branch prompt should include:
- "Read [SHARED_TYPE_FILES] — these are your interface contract. Do NOT modify these shared files."
- "Implement ONLY your assigned module files."
```

**Step 3: Add progressive compilation pattern to prompt complexity scaling**

Find the "Prompt complexity scaling" section (around line 712-720). Add after the complexity tiers:

```markdown
For high-turn-budget nodes (40+ turns) that produce compiled code, use the **progressive compilation pattern** to prevent late-stage build failures:

```
Implementation approach:
1. Create all files with stub/skeleton implementations (verify: [BUILD_COMMAND] must pass)
2. Implement module A logic (verify: [BUILD_COMMAND] must pass)
3. Implement module B logic (verify: [BUILD_COMMAND] must pass)
4. Write tests (verify: [TEST_COMMAND] must pass)

Do NOT proceed to the next module until the current one compiles.
```

This catches type errors and interface mismatches incrementally rather than discovering them after 50 turns of work. It naturally pairs with the build-first strategy in the implementation prompt template.
```

**Step 4: Add `escalation_models` to DSL Quick Reference**

In the node attributes table (around line 800), add a new row:

```
| `escalation_models` | Comma-separated `provider:model` pairs for capability escalation. When the node fails with `budget_exhausted` or `compilation_loop`, the engine cycles through these models after `retries_before_escalation` same-model retries. Example: `"kimi:kimi-k2.5, anthropic:claude-opus-4-6"` |
```

Also add `retries_before_escalation` to a note about graph-level attributes (near line 766 where required graph attrs are listed):

```
- Optional: `retries_before_escalation` (default: 2) — same-model retry count before escalating to the next model in a node's `escalation_models` chain. Applied globally; override with caution.
```

**Step 5: Update Phase 0B / Phase 5 to generate escalation chains**

In the Phase 0B section (model selection), add guidance for generating escalation chains:

```markdown
#### Escalation chain generation

When producing the Medium or High option, generate an `escalation_models` attribute for complex implementation nodes (class="hard"). The chain should:

1. Include all available models ordered by cost (cheapest first, most expensive last)
2. Use highest reasoning/thinking settings (the escalation is for capability, not speed)
3. Skip models that are already the node's primary model (from the stylesheet)
4. Set `max_retries` on nodes with escalation chains to accommodate the full chain: `(len(chain) + 1) * (retries_before_escalation + 1) - 1`

Example for a Medium plan with kimi-k2.5 as the primary model:
```dot
impl_core [
    shape=box, class="hard", max_retries=8,
    escalation_models="zai:glm-4.7, google:gemini-pro, anthropic:claude-opus-4-6"
]
```

For the Low option, omit `escalation_models` (single model, minimal cost).
```

**Step 6: Commit**

```bash
git add skills/english-to-dotfile/SKILL.md
git commit -m "skill(english-to-dotfile): add build-first, interface-pinning, progressive compilation, and escalation_models patterns

Four prompt/pattern improvements to the dotfile creation skill:

1. Build-first strategy: implementation prompts now instruct agents to
   achieve a clean compile with stubs before filling in logic, preventing
   compile-fix-regress loops that burn entire turn budgets.

2. Interface-pinning pattern: fanout documentation now recommends a
   scaffold/contracts node before parallel branches to prevent cross-branch
   type divergence.

3. Progressive compilation: complex nodes (40+ turns) should compile after
   each module rather than writing everything then discovering errors.

4. escalation_models: Phase 0B and DSL reference now document the new
   node attribute for capability-based model escalation, with guidance
   on chain generation and max_retries calculation."
```

---

### Task 5: Update graph validation for new attributes (optional but recommended)

**Files:**
- Modify: `internal/attractor/validate/validate.go`
- Test: existing validation tests

**Step 1: Add validation for escalation_models syntax**

If an `escalation_models` attribute is present on a node, validate that each entry is parseable as `provider:model`. Add a WARNING-level lint (not ERROR — the attribute is optional and backward-compatible):

```go
// In the lint rules section:
// "escalation_models_syntax" — WARNING: validate provider:model pairs parse correctly
```

Check that:
- Each comma-separated entry contains exactly one colon
- Provider and model are non-empty after trimming

This prevents typos like `kimi-kimi-k2.5` (missing colon) from silently failing at runtime.

**Step 2: Verify referenced providers exist in config**

If a `RunConfigFile` is available during validation, check that providers in the escalation chain are configured. This is a WARNING, not ERROR (the chain might reference providers that become available later).

**Step 3: Run validation tests**

Run: `cd /home/user/code/kilroy && go test ./internal/attractor/validate/ -v`

**Step 4: Commit**

```bash
git add internal/attractor/validate/validate.go
git commit -m "attractor: add validation lint for escalation_models syntax

Warns if escalation_models entries are malformed (missing colon,
empty provider/model). Does not error — the attribute is optional
and the engine already skips unparseable entries gracefully."
```

---

## Verification Checklist

After all tasks are complete, run the full test suite and verify:

```bash
cd /home/user/code/kilroy
go test ./internal/attractor/... -v -count=1 2>&1 | tail -50
go build -o ./kilroy ./cmd/kilroy
./kilroy attractor validate --graph demo/rogue/rogue_fast.dot
```

Then verify backward compatibility:
- Existing dotfiles without `escalation_models` behave identically
- Existing run configs without `retries_before_escalation` use default (2)
- Deterministic failures still don't retry
- Transient infra failures still retry with same model
- `budget_exhausted` failures now retry and escalate through the chain

---

## Summary of Changes

| Component | Files Changed | Nature |
|-----------|--------------|--------|
| Failure classes | `loop_restart_policy.go` | 2 new constants + heuristic hints |
| Retry gating | `failure_policy.go` | budget_exhausted + compilation_loop now retryable |
| Escalation | `escalation.go` (new), `engine.go` | Parse chain, cycle models on capability failure |
| Skill patterns | `SKILL.md` | 4 new guidelines (build-first, interface-pinning, progressive compilation, escalation_models) |
| Validation | `validate.go` | Optional lint for escalation_models syntax |
