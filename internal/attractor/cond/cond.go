package cond

import (
	"fmt"
	"strings"

	"github.com/strongdm/kilroy/internal/attractor/runtime"
)

// Evaluate evaluates a minimal AND-only condition language used on edges.
//
// Grammar (per attractor-spec.md Section 10):
//
//	ConditionExpr ::= Clause ( '&&' Clause )*
//	Clause        ::= Key Operator Literal
//	Key           ::= 'outcome' | 'preferred_label' | 'context.' Path
//	Operator      ::= '=' | '!='
//
// Missing keys resolve to empty string. Comparisons are exact string comparisons.
func Evaluate(condition string, outcome runtime.Outcome, ctx *runtime.Context) (bool, error) {
	condition = strings.TrimSpace(condition)
	if condition == "" {
		return true, nil
	}
	clauses := strings.Split(condition, "&&")
	for _, clause := range clauses {
		clause = strings.TrimSpace(clause)
		if clause == "" {
			continue
		}
		ok, err := evalClause(clause, outcome, ctx)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func evalClause(clause string, outcome runtime.Outcome, ctx *runtime.Context) (bool, error) {
	if strings.Contains(clause, "!=") {
		parts := strings.SplitN(clause, "!=", 2)
		if len(parts) != 2 {
			return false, fmt.Errorf("invalid clause: %q", clause)
		}
		k := strings.TrimSpace(parts[0])
		want := strings.TrimSpace(parts[1])
		got := resolveKey(k, outcome, ctx)
		want = canonicalizeCompareValue(k, want)
		return got != want, nil
	}
	if strings.Contains(clause, "=") {
		parts := strings.SplitN(clause, "=", 2)
		if len(parts) != 2 {
			return false, fmt.Errorf("invalid clause: %q", clause)
		}
		k := strings.TrimSpace(parts[0])
		want := strings.TrimSpace(parts[1])
		got := resolveKey(k, outcome, ctx)
		want = canonicalizeCompareValue(k, want)
		return got == want, nil
	}
	// Bare key: truthy if non-empty and not "false"/"0" (best-effort).
	got := resolveKey(strings.TrimSpace(clause), outcome, ctx)
	if got == "" {
		return false, nil
	}
	switch strings.ToLower(got) {
	case "false", "0", "no":
		return false, nil
	default:
		return true, nil
	}
}

func resolveKey(key string, outcome runtime.Outcome, ctx *runtime.Context) string {
	switch key {
	case "outcome":
		co, err := outcome.Canonicalize()
		if err != nil {
			return string(outcome.Status)
		}
		return string(co.Status)
	case "preferred_label":
		return outcome.PreferredLabel
	}
	if strings.HasPrefix(key, "context.") {
		if ctx != nil {
			if v, ok := ctx.Get(key); ok && v != nil {
				return fmt.Sprint(v)
			}
			// Also try without "context." prefix for convenience.
			short := strings.TrimPrefix(key, "context.")
			if v, ok := ctx.Get(short); ok && v != nil {
				return fmt.Sprint(v)
			}
		}
		return ""
	}
	if ctx != nil {
		if v, ok := ctx.Get(key); ok && v != nil {
			return fmt.Sprint(v)
		}
	}
	return ""
}

// canonicalizeCompareValue normalizes the comparison value for outcome conditions
// so that aliases like "skip"/"skipped" and "failure"/"fail" match correctly.
func canonicalizeCompareValue(key, value string) string {
	if key != "outcome" {
		return value
	}
	if canonical, err := runtime.ParseStageStatus(value); err == nil {
		return string(canonical)
	}
	return value
}
