package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/danshapiro/kilroy/internal/attractor/dot"
	"github.com/danshapiro/kilroy/internal/attractor/model"
	"github.com/danshapiro/kilroy/internal/attractor/runtime"
)

type panicHandler struct{}

func (h *panicHandler) Execute(ctx context.Context, exec *Execution, node *model.Node) (runtime.Outcome, error) {
	_ = ctx
	_ = exec
	_ = node
	panic("boom")
}

func TestEngine_ConvertsHandlerPanicsToFailOutcomeAndWritesStatusJSON(t *testing.T) {
	g, err := dot.Parse([]byte(`
digraph G {
  start [shape=Mdiamond]
  boom  [shape=box, type="panic"]
  exit  [shape=Msquare]
  start -> boom -> exit
}
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	logsRoot := t.TempDir()
	eng := &Engine{
		Graph:       g,
		LogsRoot:    logsRoot,
		WorktreeDir: t.TempDir(),
		Context:     runtime.NewContext(),
		Registry:    NewDefaultRegistry(),
	}
	eng.Registry.Register("panic", &panicHandler{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := eng.executeNode(ctx, g.Nodes["boom"])
	if err != nil {
		t.Fatalf("executeNode returned error: %v", err)
	}
	if out.Status != runtime.StatusFail {
		t.Fatalf("status: got %q want %q", out.Status, runtime.StatusFail)
	}
	if !strings.Contains(out.FailureReason, "panic") {
		t.Fatalf("failure_reason: %q", out.FailureReason)
	}

	b, err := os.ReadFile(filepath.Join(logsRoot, "boom", "status.json"))
	if err != nil {
		t.Fatalf("read status.json: %v", err)
	}
	got, err := runtime.DecodeOutcomeJSON(b)
	if err != nil {
		t.Fatalf("DecodeOutcomeJSON: %v", err)
	}
	if got.Status != runtime.StatusFail {
		t.Fatalf("status.json status: got %q want %q", got.Status, runtime.StatusFail)
	}
	if !strings.Contains(got.FailureReason, "panic") {
		t.Fatalf("status.json failure_reason: %q", got.FailureReason)
	}
	if _, err := os.Stat(filepath.Join(logsRoot, "boom", "panic.txt")); err != nil {
		t.Fatalf("expected panic.txt to exist: %v", err)
	}
}
