package style

import (
	"testing"

	"github.com/danshapiro/kilroy/internal/attractor/model"
)

func TestStylesheet_ParseAndApply(t *testing.T) {
	ss := `
* { llm_model: claude-sonnet-4-5; llm_provider: anthropic; }
box { reasoning_effort: low; }
.code { llm_model: claude-opus-4-6; }
#n1 { llm_provider: openai; reasoning_effort: high; }
`
	rules, err := ParseStylesheet(ss)
	if err != nil {
		t.Fatalf("ParseStylesheet error: %v", err)
	}
	g := model.NewGraph("G")
	n1 := model.NewNode("n1")
	n1.Attrs["shape"] = "box"
	n1.Attrs["class"] = "code"
	n2 := model.NewNode("n2")
	n2.Attrs["shape"] = "diamond"
	n2.Attrs["llm_model"] = "explicit-model"
	if err := g.AddNode(n1); err != nil {
		t.Fatalf("AddNode n1: %v", err)
	}
	if err := g.AddNode(n2); err != nil {
		t.Fatalf("AddNode n2: %v", err)
	}

	if err := ApplyStylesheet(g, rules); err != nil {
		t.Fatalf("ApplyStylesheet error: %v", err)
	}

	if got := g.Nodes["n1"].Attrs["llm_model"]; got != "claude-opus-4-6" {
		t.Fatalf("n1 llm_model: got %q", got)
	}
	if got := g.Nodes["n1"].Attrs["llm_provider"]; got != "openai" {
		t.Fatalf("n1 llm_provider: got %q", got)
	}
	if got := g.Nodes["n1"].Attrs["reasoning_effort"]; got != "high" {
		t.Fatalf("n1 reasoning_effort: got %q", got)
	}

	if got := g.Nodes["n2"].Attrs["llm_model"]; got != "explicit-model" {
		t.Fatalf("n2 llm_model should not be overridden: got %q", got)
	}
	if got := g.Nodes["n2"].Attrs["llm_provider"]; got != "anthropic" {
		t.Fatalf("n2 llm_provider: got %q", got)
	}
}
