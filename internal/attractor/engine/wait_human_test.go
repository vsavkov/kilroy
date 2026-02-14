package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/danshapiro/kilroy/internal/attractor/runtime"
)

func TestRun_WaitHuman_RoutesOnQueueInterviewerSelection(t *testing.T) {
	repo := t.TempDir()
	runCmd(t, repo, "git", "init")
	runCmd(t, repo, "git", "config", "user.name", "tester")
	runCmd(t, repo, "git", "config", "user.email", "tester@example.com")
	_ = os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644)
	runCmd(t, repo, "git", "add", "-A")
	runCmd(t, repo, "git", "commit", "-m", "init")

	dot := []byte(`
digraph G {
  graph [goal="test"]
  start [shape=Mdiamond]
  gate  [shape=hexagon, label="Gate"]
  approve [shape=parallelogram, tool_command="echo approve"]
  fix     [shape=parallelogram, tool_command="echo fix"]
  exit  [shape=Msquare]

  start -> gate
  gate -> approve [label="[A] Approve"]
  gate -> fix     [label="[F] Fix"]
  approve -> exit
  fix -> exit
}
`)
	g, _, err := Prepare(dot)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	logsRoot := t.TempDir()
	opts := RunOptions{RepoPath: repo, RunID: "human", LogsRoot: logsRoot}
	if err := opts.applyDefaults(); err != nil {
		t.Fatalf("applyDefaults: %v", err)
	}

	eng := &Engine{
		Graph:           g,
		Options:         opts,
		DotSource:       append([]byte{}, dot...),
		LogsRoot:        opts.LogsRoot,
		WorktreeDir:     opts.WorktreeDir,
		Context:         runtime.NewContext(),
		Registry:        NewDefaultRegistry(),
		Interviewer:     &QueueInterviewer{Answers: []Answer{{Value: "F"}}},
		CodergenBackend: &SimulatedCodergenBackend{},
	}
	eng.RunBranch = fmt.Sprintf("%s/%s", opts.RunBranchPrefix, opts.RunID)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	_, err = eng.run(ctx)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// Selected branch should execute "fix" and skip "approve".
	if _, err := os.Stat(filepath.Join(logsRoot, "fix", "status.json")); err != nil {
		t.Fatalf("expected fix to execute: %v", err)
	}
	if _, err := os.Stat(filepath.Join(logsRoot, "approve", "status.json")); err == nil {
		t.Fatalf("expected approve to be skipped")
	}

	// Human gate outcome should include selection in context_updates.
	b, err := os.ReadFile(filepath.Join(logsRoot, "gate", "status.json"))
	if err != nil {
		t.Fatalf("read gate status.json: %v", err)
	}
	out, err := runtime.DecodeOutcomeJSON(b)
	if err != nil {
		t.Fatalf("decode gate status.json: %v", err)
	}
	if got := fmt.Sprint(out.ContextUpdates["human.gate.selected"]); got != "fix" {
		t.Fatalf("human.gate.selected: %v", got)
	}
}

