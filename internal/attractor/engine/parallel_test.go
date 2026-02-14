package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danshapiro/kilroy/internal/attractor/runtime"
)

func TestRun_ParallelFanOutAndFanIn_FastForwardsWinner(t *testing.T) {
	repo := t.TempDir()
	runCmd(t, repo, "git", "init")
	runCmd(t, repo, "git", "config", "user.name", "tester")
	runCmd(t, repo, "git", "config", "user.email", "tester@example.com")
	_ = os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644)
	runCmd(t, repo, "git", "add", "-A")
	runCmd(t, repo, "git", "commit", "-m", "init")

	dot := []byte(`
digraph P {
  graph [goal="test"]
  start [shape=Mdiamond]
  par [shape=component]
  a [shape=box, llm_provider=openai, llm_model=gpt-5.2, prompt="a"]
  b [shape=box, llm_provider=openai, llm_model=gpt-5.2, prompt="b"]
  join [shape=tripleoctagon]
  exit [shape=Msquare]

  start -> par
  par -> a
  par -> b
  a -> join
  b -> join
  join -> exit
}
`)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	res, err := Run(ctx, dot, RunOptions{RepoPath: repo})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	assertExists(t, filepath.Join(res.LogsRoot, "par", "parallel_results.json"))
	assertExists(t, filepath.Join(res.LogsRoot, "join", "status.json"))

	// Winner should deterministically be branch "a" (lexical tie-break).
	b, err := os.ReadFile(filepath.Join(res.LogsRoot, "join", "status.json"))
	if err != nil {
		t.Fatalf("read join status.json: %v", err)
	}
	out, err := runtime.DecodeOutcomeJSON(b)
	if err != nil {
		t.Fatalf("decode join status.json: %v", err)
	}
	best, ok := out.ContextUpdates["parallel.fan_in.best_id"]
	if !ok {
		t.Fatalf("missing parallel.fan_in.best_id in context updates")
	}
	if strings.TrimSpace(strings.ToLower(fmt.Sprint(best))) != "a" {
		t.Fatalf("best branch: got %v, want a", best)
	}

	// Base + 5 node commits (start, par, a, join, exit) => 6 total.
	count := strings.TrimSpace(runCmdOut(t, repo, "git", "rev-list", "--count", res.RunBranch))
	if count != "6" {
		t.Fatalf("commit count: got %s, want 6 (base+5 nodes on winning path)", count)
	}
}
