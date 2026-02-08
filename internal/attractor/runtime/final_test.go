package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFinalOutcome_Save_WritesJSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "nested", "final.json")
	fo := &FinalOutcome{
		Timestamp:         time.Unix(123, 0).UTC(),
		Status:            FinalSuccess,
		RunID:             "r1",
		FinalGitCommitSHA: "abc",
		CXDBContextID:     "c1",
		CXDBHeadTurnID:    "t1",
	}
	if err := fo.Save(p); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
}

func TestFinalOutcome_Save_PersistsFailureReason(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "final.json")
	fo := &FinalOutcome{
		Timestamp:         time.Unix(123, 0).UTC(),
		Status:            FinalFail,
		RunID:             "r1",
		FinalGitCommitSHA: "abc",
		FailureReason:     "loop_restart limit exceeded",
		CXDBContextID:     "c1",
		CXDBHeadTurnID:    "t1",
	}
	if err := fo.Save(p); err != nil {
		t.Fatalf("Save: %v", err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got["failure_reason"] != "loop_restart limit exceeded" {
		t.Fatalf("failure_reason=%v", got["failure_reason"])
	}
}
