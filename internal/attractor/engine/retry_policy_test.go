package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/danshapiro/kilroy/internal/attractor/runtime"
)

func TestRun_RetriesOnFail_ThenSucceeds(t *testing.T) {
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
  exit  [shape=Msquare]
  t [
    shape=parallelogram,
    max_retries=1,
    tool_command="test -f .attempt && echo ok || (touch .attempt; echo fail; exit 1)"
  ]
  start -> t -> exit
}
`)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	res, err := Run(ctx, dot, RunOptions{RepoPath: repo})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(res.LogsRoot, "t", "status.json"))
	if err != nil {
		t.Fatalf("read t status.json: %v", err)
	}
	out, err := runtime.DecodeOutcomeJSON(b)
	if err != nil {
		t.Fatalf("decode t status.json: %v", err)
	}
	if out.Status != runtime.StatusSuccess {
		t.Fatalf("t outcome: got %q want %q", out.Status, runtime.StatusSuccess)
	}
}

func TestRun_AllowPartialAfterRetryExhaustion(t *testing.T) {
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
  exit  [shape=Msquare]
  t [
    shape=parallelogram,
    max_retries=1,
    allow_partial=true,
    tool_command="echo fail; exit 1"
  ]
  start -> t -> exit
}
`)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	res, err := Run(ctx, dot, RunOptions{RepoPath: repo})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(res.LogsRoot, "t", "status.json"))
	if err != nil {
		t.Fatalf("read t status.json: %v", err)
	}
	out, err := runtime.DecodeOutcomeJSON(b)
	if err != nil {
		t.Fatalf("decode t status.json: %v", err)
	}
	if out.Status != runtime.StatusPartialSuccess {
		t.Fatalf("t outcome: got %q want %q", out.Status, runtime.StatusPartialSuccess)
	}
}

