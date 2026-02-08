package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResume_Preflight_FailsWhenProviderCLIUnavailable(t *testing.T) {
	repo := t.TempDir()
	runCmd(t, repo, "git", "init")
	runCmd(t, repo, "git", "config", "user.name", "tester")
	runCmd(t, repo, "git", "config", "user.email", "tester@example.com")
	_ = os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o644)
	runCmd(t, repo, "git", "add", "-A")
	runCmd(t, repo, "git", "commit", "-m", "init")

	logsRoot := t.TempDir()
	pinned := writePinnedCatalog(t)
	cxdbSrv := newCXDBTestServer(t)

	cli := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(cli, []byte(`#!/usr/bin/env bash
set -euo pipefail
echo '{"type":"done","text":"ok"}'
`), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KILROY_CODEX_PATH", cli)

	cfg := &RunConfigFile{Version: 1}
	cfg.Repo.Path = repo
	cfg.CXDB.BinaryAddr = cxdbSrv.BinaryAddr()
	cfg.CXDB.HTTPBaseURL = cxdbSrv.URL()
	cfg.LLM.Providers = map[string]struct {
		Backend BackendKind `json:"backend" yaml:"backend"`
	}{"openai": {Backend: BackendCLI}}
	cfg.ModelDB.LiteLLMCatalogPath = pinned
	cfg.ModelDB.LiteLLMCatalogUpdatePolicy = "pinned"
	cfg.Git.RunBranchPrefix = "attractor/run"

	dot := []byte(`
digraph G {
  graph [goal="resume preflight"]
  start [shape=Mdiamond]
  exit  [shape=Msquare]
  a [shape=box, llm_provider=openai, llm_model=gpt-5.2, prompt="say hi"]
  start -> a -> exit
}
`)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := RunWithConfig(ctx, dot, cfg, RunOptions{RunID: "resume-preflight", LogsRoot: logsRoot}); err != nil {
		t.Fatalf("RunWithConfig: %v", err)
	}

	// Simulate provider CLI contract drift after the original run.
	t.Setenv("KILROY_CODEX_PATH", filepath.Join(t.TempDir(), "missing-codex"))

	_, err := Resume(ctx, logsRoot)
	if err == nil {
		t.Fatalf("expected resume preflight error, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "provider cli preflight") {
		t.Fatalf("expected provider cli preflight error, got: %v", err)
	}
}
