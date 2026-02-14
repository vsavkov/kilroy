package engine

import (
	"testing"

	"github.com/danshapiro/kilroy/internal/attractor/dot"
	"github.com/danshapiro/kilroy/internal/attractor/runtime"
)

func TestSelectNextEdge_ConditionBeatsUnconditionalWeight(t *testing.T) {
	g, err := dot.Parse([]byte(`
digraph G {
  start [shape=Mdiamond]
  exit  [shape=Msquare]
  a [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  b [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  c [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  start -> a
  a -> b [condition="outcome=success", weight=0]
  a -> c [weight=100]
  b -> exit
  c -> exit
}
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out := runtime.Outcome{Status: runtime.StatusSuccess}
	ctx := runtime.NewContext()
	e, err := selectNextEdge(g, "a", out, ctx)
	if err != nil {
		t.Fatalf("selectNextEdge: %v", err)
	}
	if e == nil || e.To != "b" {
		t.Fatalf("edge: got %+v want to=b", e)
	}
}

func TestSelectNextEdge_PreferredLabelBeatsWeightAmongUnconditional(t *testing.T) {
	g, err := dot.Parse([]byte(`
digraph G {
  start [shape=Mdiamond]
  exit  [shape=Msquare]
  a [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  b [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  c [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  start -> a
  a -> b [label="[A] Approve", weight=0]
  a -> c [label="[F] Fix", weight=100]
  b -> exit
  c -> exit
}
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out := runtime.Outcome{Status: runtime.StatusSuccess, PreferredLabel: "Approve"}
	ctx := runtime.NewContext()
	e, err := selectNextEdge(g, "a", out, ctx)
	if err != nil {
		t.Fatalf("selectNextEdge: %v", err)
	}
	if e == nil || e.To != "b" {
		t.Fatalf("edge: got %+v want to=b", e)
	}
}

func TestSelectNextEdge_SuggestedNextIDsBeatsWeightAmongUnconditional(t *testing.T) {
	g, err := dot.Parse([]byte(`
digraph G {
  start [shape=Mdiamond]
  exit  [shape=Msquare]
  a [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  b [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  c [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  start -> a
  a -> b [weight=100]
  a -> c [weight=0]
  b -> exit
  c -> exit
}
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out := runtime.Outcome{Status: runtime.StatusSuccess, SuggestedNextIDs: []string{"c"}}
	ctx := runtime.NewContext()
	e, err := selectNextEdge(g, "a", out, ctx)
	if err != nil {
		t.Fatalf("selectNextEdge: %v", err)
	}
	if e == nil || e.To != "c" {
		t.Fatalf("edge: got %+v want to=c", e)
	}
}

func TestSelectNextEdge_WeightThenLexicalThenOrder(t *testing.T) {
	g, err := dot.Parse([]byte(`
digraph G {
  start [shape=Mdiamond]
  exit  [shape=Msquare]
  a [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  b [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  c [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  d [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  start -> a
  a -> d [weight=2]
  a -> c [weight=2]
  a -> b [weight=2]
  b -> exit
  c -> exit
  d -> exit
}
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out := runtime.Outcome{Status: runtime.StatusSuccess}
	ctx := runtime.NewContext()
	e, err := selectNextEdge(g, "a", out, ctx)
	if err != nil {
		t.Fatalf("selectNextEdge: %v", err)
	}
	// All weights tied; lexical by to_node chooses "b".
	if e == nil || e.To != "b" {
		t.Fatalf("edge: got %+v want to=b", e)
	}
}

func TestSelectAllEligibleEdges_MultipleUnconditional(t *testing.T) {
	g, err := dot.Parse([]byte(`
digraph G {
  graph [goal="test"]
  start [shape=Mdiamond]
  exit  [shape=Msquare]
  a [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  b [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  c [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  d [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  start -> a
  a -> b
  a -> c
  a -> d
  b -> exit
  c -> exit
  d -> exit
}
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out := runtime.Outcome{Status: runtime.StatusSuccess}
	ctx := runtime.NewContext()
	edges, err := selectAllEligibleEdges(g, "a", out, ctx)
	if err != nil {
		t.Fatalf("selectAllEligibleEdges: %v", err)
	}
	if len(edges) != 3 {
		t.Fatalf("got %d edges, want 3", len(edges))
	}
	targets := map[string]bool{}
	for _, e := range edges {
		targets[e.To] = true
	}
	for _, want := range []string{"b", "c", "d"} {
		if !targets[want] {
			t.Fatalf("missing target %q", want)
		}
	}
}

func TestSelectAllEligibleEdges_MultipleMatchingConditions(t *testing.T) {
	g, err := dot.Parse([]byte(`
digraph G {
  graph [goal="test"]
  start [shape=Mdiamond]
  exit  [shape=Msquare]
  a [shape=diamond]
  b [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  c [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  d [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  start -> a
  a -> b [condition="outcome=success"]
  a -> c [condition="outcome=success"]
  a -> d [condition="outcome=fail"]
  b -> exit
  c -> exit
  d -> exit
}
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out := runtime.Outcome{Status: runtime.StatusSuccess}
	ctx := runtime.NewContext()
	edges, err := selectAllEligibleEdges(g, "a", out, ctx)
	if err != nil {
		t.Fatalf("selectAllEligibleEdges: %v", err)
	}
	if len(edges) != 2 {
		t.Fatalf("got %d edges, want 2 (b and c)", len(edges))
	}
}

func TestSelectAllEligibleEdges_SingleEdge(t *testing.T) {
	g, err := dot.Parse([]byte(`
digraph G {
  graph [goal="test"]
  start [shape=Mdiamond]
  exit  [shape=Msquare]
  a [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  b [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  start -> a
  a -> b
  b -> exit
}
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out := runtime.Outcome{Status: runtime.StatusSuccess}
	ctx := runtime.NewContext()
	edges, err := selectAllEligibleEdges(g, "a", out, ctx)
	if err != nil {
		t.Fatalf("selectAllEligibleEdges: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("got %d edges, want 1", len(edges))
	}
	if edges[0].To != "b" {
		t.Fatalf("got %q, want b", edges[0].To)
	}
}

func TestSelectAllEligibleEdges_PreferredLabelNarrowsToOne(t *testing.T) {
	g, err := dot.Parse([]byte(`
digraph G {
  graph [goal="test"]
  start [shape=Mdiamond]
  exit  [shape=Msquare]
  a [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  b [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  c [shape=box, llm_provider=openai, llm_model=gpt-5.2]
  start -> a
  a -> b [label="approve"]
  a -> c [label="reject"]
  b -> exit
  c -> exit
}
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out := runtime.Outcome{Status: runtime.StatusSuccess, PreferredLabel: "approve"}
	ctx := runtime.NewContext()
	edges, err := selectAllEligibleEdges(g, "a", out, ctx)
	if err != nil {
		t.Fatalf("selectAllEligibleEdges: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("got %d edges, want 1 (preferred label narrows)", len(edges))
	}
	if edges[0].To != "b" {
		t.Fatalf("got %q, want b", edges[0].To)
	}
}

