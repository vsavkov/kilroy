package engine

import (
	"testing"

	"github.com/strongdm/kilroy/internal/attractor/runtime"
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
