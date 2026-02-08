package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/strongdm/kilroy/internal/attractor/model"
	"github.com/strongdm/kilroy/internal/attractor/runtime"
)

func TestRun_FinalizationWritesFinalJSON_WhenFailingBeforeTerminal(t *testing.T) {
	repo := t.TempDir()
	runCmd(t, repo, "git", "init")
	runCmd(t, repo, "git", "config", "user.name", "tester")
	runCmd(t, repo, "git", "config", "user.email", "tester@example.com")
	_ = os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644)
	runCmd(t, repo, "git", "add", "-A")
	runCmd(t, repo, "git", "commit", "-m", "init")

	dot := []byte(`
digraph G {
  graph [goal="fatal before terminal"]
  start [shape=Mdiamond]
  exit  [shape=Msquare]
  work  [shape=box, llm_provider=openai, llm_model=gpt-5.2, prompt="do work"]
  start -> work
  work -> exit [condition="outcome=success"]
}
`)

	backend := &countingBackend{
		fn: func(ctx context.Context, exec *Execution, node *model.Node, prompt string) (string, *runtime.Outcome, error) {
			return "fail", &runtime.Outcome{Status: runtime.StatusFail, FailureReason: "unknown flag: --verbose"}, nil
		},
	}
	g, _, err := Prepare(dot)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	logsRoot := t.TempDir()
	eng := &Engine{
		Graph:           g,
		Options:         RunOptions{RepoPath: repo, RunID: "fatal-before-terminal", LogsRoot: logsRoot, WorktreeDir: filepath.Join(logsRoot, "worktree"), RunBranchPrefix: "attractor/run", RequireClean: true},
		DotSource:       dot,
		LogsRoot:        logsRoot,
		WorktreeDir:     filepath.Join(logsRoot, "worktree"),
		Context:         runtime.NewContext(),
		Registry:        NewDefaultRegistry(),
		Interviewer:     &AutoApproveInterviewer{},
		CodergenBackend: backend,
	}
	eng.RunBranch = "attractor/run/fatal-before-terminal"

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	_, err = eng.run(ctx)
	if err == nil {
		t.Fatalf("expected run error")
	}

	finalPath := filepath.Join(logsRoot, "final.json")
	b, readErr := os.ReadFile(finalPath)
	if readErr != nil {
		t.Fatalf("read final.json: %v", readErr)
	}
	var final runtime.FinalOutcome
	if err := json.Unmarshal(b, &final); err != nil {
		t.Fatalf("unmarshal final.json: %v", err)
	}
	if final.Status != runtime.FinalFail {
		t.Fatalf("final.status=%q want=%q", final.Status, runtime.FinalFail)
	}
	if strings.TrimSpace(final.FailureReason) == "" {
		t.Fatalf("final.failure_reason must be non-empty")
	}
}

func TestRun_FinalizationWritesFinalJSON_WhenLoopRestartCircuitBreaks(t *testing.T) {
	repo := t.TempDir()
	runCmd(t, repo, "git", "init")
	runCmd(t, repo, "git", "config", "user.name", "tester")
	runCmd(t, repo, "git", "config", "user.email", "tester@example.com")
	_ = os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644)
	runCmd(t, repo, "git", "add", "-A")
	runCmd(t, repo, "git", "commit", "-m", "init")

	dot := []byte(`
digraph G {
  graph [goal="fatal loop restart circuit", max_restarts="10", restart_signature_limit="2"]
  start [shape=Mdiamond]
  exit  [shape=Msquare]
  work  [shape=box, llm_provider=openai, llm_model=gpt-5.2, prompt="do work"]
  check [shape=diamond]
  start -> work
  work -> check
  check -> exit [condition="outcome=success"]
  check -> work [condition="outcome=fail", loop_restart=true]
}
`)

	backend := &countingBackend{
		fn: func(ctx context.Context, exec *Execution, node *model.Node, prompt string) (string, *runtime.Outcome, error) {
			return "fail", &runtime.Outcome{Status: runtime.StatusFail, FailureReason: "request timeout after 10s"}, nil
		},
	}
	g, _, err := Prepare(dot)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	logsRoot := t.TempDir()
	eng := &Engine{
		Graph:           g,
		Options:         RunOptions{RepoPath: repo, RunID: "fatal-loop-circuit", LogsRoot: logsRoot, WorktreeDir: filepath.Join(logsRoot, "worktree"), RunBranchPrefix: "attractor/run", RequireClean: true},
		DotSource:       dot,
		LogsRoot:        logsRoot,
		WorktreeDir:     filepath.Join(logsRoot, "worktree"),
		Context:         runtime.NewContext(),
		Registry:        NewDefaultRegistry(),
		Interviewer:     &AutoApproveInterviewer{},
		CodergenBackend: backend,
	}
	eng.RunBranch = "attractor/run/fatal-loop-circuit"

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	_, err = eng.run(ctx)
	if err == nil {
		t.Fatalf("expected run error")
	}

	finalPath := filepath.Join(eng.LogsRoot, "final.json")
	b, readErr := os.ReadFile(finalPath)
	if readErr != nil {
		t.Fatalf("read final.json: %v", readErr)
	}
	var final runtime.FinalOutcome
	if err := json.Unmarshal(b, &final); err != nil {
		t.Fatalf("unmarshal final.json: %v", err)
	}
	if final.Status != runtime.FinalFail {
		t.Fatalf("final.status=%q want=%q", final.Status, runtime.FinalFail)
	}
	if strings.TrimSpace(final.FailureReason) == "" {
		t.Fatalf("final.failure_reason must be non-empty")
	}
	if !strings.Contains(final.FailureReason, "failure_signature") {
		t.Fatalf("final.failure_reason should include circuit details, got: %q", final.FailureReason)
	}
}
