package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/strongdm/kilroy/internal/attractor/model"
	"github.com/strongdm/kilroy/internal/attractor/runtime"
)

func TestRunCLI_RelativeStateEnvBecomesAbsolute(t *testing.T) {
	stageLogs := t.TempDir()
	worktree := t.TempDir()

	cli := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(cli, []byte(`#!/usr/bin/env bash
set -euo pipefail

dump="${KILROY_TEST_ENV_DUMP:-}"
if [[ -n "$dump" ]]; then
  printf "%s" "${CODEX_HOME:-}" > "$dump"
fi

out=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -o|--output)
      out="$2"
      shift 2
      ;;
    --output-schema)
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done

if [[ -n "$out" ]]; then
  cat > "$out" <<'JSON'
{"final":"ok","summary":"ok"}
JSON
fi

echo '{"type":"done","text":"ok"}'
`), 0o755); err != nil {
		t.Fatalf("write codex cli: %v", err)
	}
	t.Setenv("KILROY_CODEX_PATH", cli)

	dumpPath := filepath.Join(t.TempDir(), "codex-home.txt")
	t.Setenv("KILROY_TEST_ENV_DUMP", dumpPath)

	relStatePath := filepath.Join("relative", "codex-home")
	t.Setenv("CODEX_HOME", relStatePath)

	r := &CodergenRouter{}
	node := model.NewNode("a")
	eng := &Engine{
		Options:  RunOptions{RunID: "env-path"},
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
	if out != nil && out.Status == runtime.StatusFail {
		t.Fatalf("runCLI outcome failure: %+v", *out)
	}

	dumped, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("read dump path: %v", err)
	}
	got := strings.TrimSpace(string(dumped))
	if got == relStatePath {
		t.Fatalf("CODEX_HOME was not absolutized: %q", got)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("CODEX_HOME should be absolute, got %q", got)
	}
	if !strings.HasSuffix(got, filepath.FromSlash("relative/codex-home")) {
		t.Fatalf("CODEX_HOME absolute value unexpected: %q", got)
	}

	b, err := os.ReadFile(filepath.Join(stageLogs, "a", "cli_invocation.json"))
	if err != nil {
		t.Fatalf("read cli_invocation.json: %v", err)
	}
	var inv map[string]any
	if err := json.Unmarshal(b, &inv); err != nil {
		t.Fatalf("unmarshal cli_invocation.json: %v", err)
	}
	envOverrides, _ := inv["env_path_overrides"].(map[string]any)
	if envOverrides == nil {
		t.Fatalf("missing env_path_overrides in invocation: %v", inv)
	}
	if strings.TrimSpace(anyToString(envOverrides["CODEX_HOME"])) == "" {
		t.Fatalf("expected CODEX_HOME override in invocation: %v", envOverrides)
	}
}
