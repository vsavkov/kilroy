package server

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

func TestPipelineRegistry_RegisterAndGet(t *testing.T) {
	r := NewPipelineRegistry()

	ps := &PipelineState{RunID: "run-1"}
	if err := r.Register("run-1", ps); err != nil {
		t.Fatalf("register: %v", err)
	}

	got, ok := r.Get("run-1")
	if !ok {
		t.Fatal("expected to find pipeline")
	}
	if got.RunID != "run-1" {
		t.Fatalf("unexpected run ID: %s", got.RunID)
	}
}

func TestPipelineRegistry_DuplicateRegister(t *testing.T) {
	r := NewPipelineRegistry()

	ps := &PipelineState{RunID: "run-1"}
	if err := r.Register("run-1", ps); err != nil {
		t.Fatalf("register: %v", err)
	}

	err := r.Register("run-1", ps)
	if err == nil {
		t.Fatal("expected error on duplicate register")
	}
}

func TestPipelineRegistry_GetNotFound(t *testing.T) {
	r := NewPipelineRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestPipelineRegistry_List(t *testing.T) {
	r := NewPipelineRegistry()
	r.Register("a", &PipelineState{RunID: "a"})
	r.Register("b", &PipelineState{RunID: "b"})

	ids := r.List()
	if len(ids) != 2 {
		t.Fatalf("expected 2 pipelines, got %d", len(ids))
	}
}

func TestPipelineRegistry_CancelAll(t *testing.T) {
	r := NewPipelineRegistry()

	canceled := make([]string, 0)
	var mu sync.Mutex

	for _, id := range []string{"a", "b", "c"} {
		_, cancel := context.WithCancelCause(context.Background())
		localID := id
		r.Register(id, &PipelineState{
			RunID: id,
			Cancel: func(err error) {
				mu.Lock()
				canceled = append(canceled, localID)
				mu.Unlock()
				cancel(err)
			},
		})
	}

	r.CancelAll("test shutdown")

	mu.Lock()
	defer mu.Unlock()
	if len(canceled) != 3 {
		t.Fatalf("expected 3 cancellations, got %d", len(canceled))
	}
}

func TestPipelineState_Status(t *testing.T) {
	ps := &PipelineState{RunID: "test-run"}

	// Before completion.
	status := ps.Status()
	if status.State != "running" {
		t.Fatalf("expected running, got %s", status.State)
	}

	// After error.
	ps.SetResult(nil, fmt.Errorf("something failed"))
	status = ps.Status()
	if status.State != "fail" {
		t.Fatalf("expected fail, got %s", status.State)
	}
	if status.FailureReason != "something failed" {
		t.Fatalf("unexpected failure reason: %s", status.FailureReason)
	}
}
