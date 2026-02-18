package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/danshapiro/kilroy/internal/attractor/runtime"
)

func TestRun_NoMatchingFailEdge_FallsBackToRetryTarget(t *testing.T) {
	repo := t.TempDir()
	runCmd(t, repo, "git", "init")
	runCmd(t, repo, "git", "config", "user.name", "tester")
	runCmd(t, repo, "git", "config", "user.email", "tester@example.com")
	_ = os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644)
	runCmd(t, repo, "git", "add", "-A")
	runCmd(t, repo, "git", "commit", "-m", "init")

	// "review" fails, has only a condition="outcome=yes" edge to exit.
	// No condition="outcome=fail" edge exists.
	// Graph-level retry_target points to "fix", which succeeds.
	// Engine should fall back to retry_target instead of dying.
	dot := []byte(`
digraph G {
  graph [goal="test", retry_target="fix"]
  start  [shape=Mdiamond]
  exit   [shape=Msquare]
  review [
    shape=parallelogram,
    tool_command="echo fail; exit 1"
  ]
  fix [
    shape=parallelogram,
    tool_command="echo fixed > fixed.txt"
  ]
  start -> review
  review -> exit [condition="outcome=yes"]
  review -> fix  [condition="outcome=__never__"]
  fix -> exit
}
`)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	res, err := Run(ctx, dot, RunOptions{RepoPath: repo})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if res.FinalStatus != runtime.FinalSuccess {
		t.Fatalf("final status: got %q want %q", res.FinalStatus, runtime.FinalSuccess)
	}
}

// TestRun_NoMatchingFailEdge_NoRetryTarget_FallsBackToAnyEdge verifies that
// when a node fails and no condition matches (only a "success" edge exists)
// and there is no retry_target, the engine falls back to ALL edges per spec §3.3
// ("Fallback: any edge"). The single edge (condition="outcome=yes") is selected
// as fallback, routing the pipeline to exit.
func TestRun_NoMatchingFailEdge_NoRetryTarget_FallsBackToAnyEdge(t *testing.T) {
	repo := t.TempDir()
	runCmd(t, repo, "git", "init")
	runCmd(t, repo, "git", "config", "user.name", "tester")
	runCmd(t, repo, "git", "config", "user.email", "tester@example.com")
	_ = os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644)
	runCmd(t, repo, "git", "add", "-A")
	runCmd(t, repo, "git", "commit", "-m", "init")

	// No retry_target, no matching edge. The only edge has condition="outcome=yes"
	// but the node fails. Spec §3.3 fallback: the engine selects the only edge
	// (condition="outcome=yes") as fallback, routing to exit.
	dot := []byte(`
digraph G {
  graph [goal="test", default_max_retry=0]
  start  [shape=Mdiamond]
  exit   [shape=Msquare]
  review [
    shape=parallelogram,
    tool_command="echo fail; exit 1"
  ]
  start -> review
  review -> exit [condition="outcome=yes"]
}
`)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := Run(ctx, dot, RunOptions{RepoPath: repo})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Fallback routing: pipeline reaches exit via the only available edge.
	if res.FinalStatus != runtime.FinalSuccess {
		t.Fatalf("final status: got %q want %q (fallback should route to exit)", res.FinalStatus, runtime.FinalSuccess)
	}
}
