package cond

import (
	"testing"

	"github.com/strongdm/kilroy/internal/attractor/runtime"
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
