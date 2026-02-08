package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/strongdm/kilroy/internal/attractor/model"
	"github.com/strongdm/kilroy/internal/attractor/runtime"
)

func TestRunCLI_FailureClassification(t *testing.T) {
	tests := []struct {
		name          string
		stderrLine    string
		wantClass     string
		wantReasonSub string
	}{
		{
			name:          "deterministic_contract_error",
			stderrLine:    "unknown flag: --verbose",
			wantClass:     string(failureClassDeterministic),
			wantReasonSub: "unknown flag",
		},
		{
			name:          "transient_transport_error",
			stderrLine:    "connection reset by peer",
			wantClass:     string(failureClassTransientInfra),
			wantReasonSub: "connection reset",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stageLogs := t.TempDir()
			worktree := t.TempDir()

			cli := filepath.Join(t.TempDir(), "codex")
			if err := os.WriteFile(cli, []byte(`#!/usr/bin/env bash
set -euo pipefail
echo '`+tc.stderrLine+`' >&2
exit 1
`), 0o755); err != nil {
				t.Fatalf("write cli: %v", err)
			}
			t.Setenv("KILROY_CODEX_PATH", cli)

			r := NewCodergenRouter(nil, nil)
			node := model.NewNode("a")
			eng := &Engine{
				Options:  RunOptions{RunID: "cli-failure"},
				LogsRoot: stageLogs,
				Context:  runtime.NewContext(),
			}
			execCtx := &Execution{
				LogsRoot:    stageLogs,
				WorktreeDir: worktree,
				Engine:      eng,
			}

			_, out, err := r.runCLI(context.Background(), execCtx, node, "openai", "gpt-5.2-codex", "hello")
			if err != nil {
				t.Fatalf("runCLI error: %v", err)
			}
			if out == nil {
				t.Fatalf("expected failure outcome, got nil")
			}
			if out.Status != runtime.StatusFail {
				t.Fatalf("status=%q want=%q", out.Status, runtime.StatusFail)
			}
			if !strings.Contains(strings.ToLower(out.FailureReason), "openai") {
				t.Fatalf("failure reason should include provider name: %q", out.FailureReason)
			}
			if !strings.Contains(strings.ToLower(out.FailureReason), strings.ToLower(tc.wantReasonSub)) {
				t.Fatalf("failure reason missing stderr detail %q: %q", tc.wantReasonSub, out.FailureReason)
			}

			classVal := strings.TrimSpace(anyToString(out.Meta[failureMetaClass]))
			if classVal != tc.wantClass {
				t.Fatalf("meta[%s]=%q want=%q", failureMetaClass, classVal, tc.wantClass)
			}
			sigVal := strings.TrimSpace(anyToString(out.Meta[failureMetaSignature]))
			if sigVal == "" {
				t.Fatalf("meta[%s] should be non-empty", failureMetaSignature)
			}
		})
	}
}
