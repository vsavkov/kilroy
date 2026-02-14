package cond

import (
	"testing"

	"github.com/danshapiro/kilroy/internal/attractor/runtime"
)

func TestEvaluate(t *testing.T) {
	ctx := runtime.NewContext()
	ctx.Set("tests_passed", true)
	ctx.Set("context.loop_state", "active")

	out := runtime.Outcome{Status: runtime.StatusSuccess, PreferredLabel: "Yes"}

	cases := []struct {
		cond string
		want bool
	}{
		{"", true},
		{"outcome=success", true},
		{"outcome!=fail", true},
		{"preferred_label=Yes", true},
		{"context.tests_passed=true", true},
		{"context.loop_state!=exhausted", true},
		{"outcome=fail", false},
		{"context.missing=foo", false},
	}
	for _, tc := range cases {
		got, err := Evaluate(tc.cond, out, ctx)
		if err != nil {
			t.Fatalf("Evaluate(%q) error: %v", tc.cond, err)
		}
		if got != tc.want {
			t.Fatalf("Evaluate(%q)=%v, want %v", tc.cond, got, tc.want)
		}
	}
}

func TestEvaluate_CustomOutcome(t *testing.T) {
	// Custom outcome values used in reference dotfiles (semport.dot: outcome=process, outcome=done).
	ctx := runtime.NewContext()
	out := runtime.Outcome{Status: runtime.StageStatus("process")}

	cases := []struct {
		cond string
		want bool
	}{
		{"outcome=process", true},
		{"outcome=done", false},
		{"outcome!=process", false},
		{"outcome!=done", true},
	}
	for _, tc := range cases {
		got, err := Evaluate(tc.cond, out, ctx)
		if err != nil {
			t.Fatalf("Evaluate(%q) error: %v", tc.cond, err)
		}
		if got != tc.want {
			t.Fatalf("Evaluate(%q)=%v, want %v", tc.cond, got, tc.want)
		}
	}
}

func TestEvaluate_OutcomeAliasesMatch(t *testing.T) {
	// Edge conditions using aliases (e.g. outcome=skip) must match the
	// canonical form produced by ParseStageStatus (e.g. "skipped").
	ctx := runtime.NewContext()

	cases := []struct {
		name   string
		status runtime.StageStatus
		cond   string
		want   bool
	}{
		// "skip" in status.json → canonical "skipped"; edge says outcome=skip
		{"skip_alias_eq", runtime.StatusSkipped, "outcome=skip", true},
		{"skip_alias_canonical", runtime.StatusSkipped, "outcome=skipped", true},
		{"skip_alias_neq", runtime.StatusSkipped, "outcome!=skip", false},
		// "failure" in edge → canonical "fail"
		{"failure_alias_eq", runtime.StatusFail, "outcome=failure", true},
		{"failure_alias_neq", runtime.StatusFail, "outcome!=failure", false},
		// "error" alias
		{"error_alias_eq", runtime.StatusFail, "outcome=error", true},
		// "ok" alias
		{"ok_alias_eq", runtime.StatusSuccess, "outcome=ok", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := runtime.Outcome{Status: tc.status}
			got, err := Evaluate(tc.cond, out, ctx)
			if err != nil {
				t.Fatalf("Evaluate(%q) error: %v", tc.cond, err)
			}
			if got != tc.want {
				t.Fatalf("Evaluate(%q) with status=%q: got %v, want %v", tc.cond, tc.status, got, tc.want)
			}
		})
	}
}
