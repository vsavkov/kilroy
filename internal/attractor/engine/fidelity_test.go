package engine

import (
	"testing"

	"github.com/danshapiro/kilroy/internal/attractor/model"
)

func TestResolveFidelityMode_ContextFidelityDefault(t *testing.T) {
	g := model.NewGraph("test")
	g.Attrs["context_fidelity_default"] = "truncate"
	mode := resolveFidelityMode(g, nil, nil)
	if mode != "truncate" {
		t.Errorf("resolveFidelityMode() = %q, want %q", mode, "truncate")
	}
}

func TestResolveFidelityMode_DefaultFidelityTakesPrecedence(t *testing.T) {
	g := model.NewGraph("test")
	g.Attrs["default_fidelity"] = "full"
	g.Attrs["context_fidelity_default"] = "truncate"
	mode := resolveFidelityMode(g, nil, nil)
	if mode != "full" {
		t.Errorf("resolveFidelityMode() = %q, want %q", mode, "full")
	}
}

func TestResolveThreadKey_ContextThreadDefault(t *testing.T) {
	g := model.NewGraph("test")
	g.Attrs["context_thread_default"] = "my-thread"
	n := model.NewNode("mynode")
	key := resolveThreadKey(g, nil, n)
	if key != "my-thread" {
		t.Errorf("resolveThreadKey() = %q, want %q", key, "my-thread")
	}
}

func TestResolveThreadKey_ThreadIDTakesPrecedence(t *testing.T) {
	g := model.NewGraph("test")
	g.Attrs["thread_id"] = "canonical"
	g.Attrs["context_thread_default"] = "alias"
	n := model.NewNode("mynode")
	key := resolveThreadKey(g, nil, n)
	if key != "canonical" {
		t.Errorf("resolveThreadKey() = %q, want %q", key, "canonical")
	}
}
