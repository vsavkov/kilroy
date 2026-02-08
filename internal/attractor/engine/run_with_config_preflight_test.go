package engine

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunWithConfig_Preflight_FailsWhenProviderCLIUnavailable(t *testing.T) {
	repo := initTestRepo(t)
	logsRoot := t.TempDir()
	pinned := writePinnedCatalog(t)

	t.Setenv("KILROY_CODEX_PATH", filepath.Join(t.TempDir(), "missing-codex"))

	cfg := &RunConfigFile{Version: 1}
	cfg.Repo.Path = repo
	cfg.CXDB.BinaryAddr = "127.0.0.1:65530"
	cfg.CXDB.HTTPBaseURL = "http://127.0.0.1:65531"
	cfg.LLM.Providers = map[string]struct {
		Backend BackendKind `json:"backend" yaml:"backend"`
	}{
		"openai": {Backend: BackendCLI},
	}
	cfg.ModelDB.LiteLLMCatalogPath = pinned
	cfg.ModelDB.LiteLLMCatalogUpdatePolicy = "pinned"
	cfg.Git.RunBranchPrefix = "attractor/run"

	dot := []byte(`
digraph G {
  graph [goal="preflight"]
  start [shape=Mdiamond]
  exit  [shape=Msquare]
  a [shape=box, llm_provider=openai, llm_model=gpt-5.2, prompt="say hi"]
  start -> a -> exit
}
`)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err := RunWithConfig(ctx, dot, cfg, RunOptions{RunID: "preflight-run", LogsRoot: logsRoot})
	if err == nil {
		t.Fatalf("expected preflight error, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "provider cli preflight") {
		t.Fatalf("expected provider cli preflight error, got: %v", err)
	}
}
