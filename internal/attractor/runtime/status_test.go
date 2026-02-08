package runtime

import (
	"strings"
	"testing"
)

func TestParseStageStatus_CanonicalAndLegacy(t *testing.T) {
	cases := []struct {
		in   string
		want StageStatus
	}{
		{"success", StatusSuccess},
		{"partial_success", StatusPartialSuccess},
		{"retry", StatusRetry},
		{"fail", StatusFail},
		{"skipped", StatusSkipped},
		// Compatibility aliases.
		{"ok", StatusSuccess},
		{"error", StatusFail},
		{"SUCCESS", StatusSuccess},
		{"FAIL", StatusFail},
	}
	for _, tc := range cases {
		got, err := ParseStageStatus(tc.in)
		if err != nil {
			t.Fatalf("ParseStageStatus(%q) error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("ParseStageStatus(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
	if _, err := ParseStageStatus("not-a-status"); err == nil {
		t.Fatalf("expected error for invalid status")
	}
}

func TestOutcome_Validate_FailureReasonRequiredForFailAndRetry(t *testing.T) {
	if err := (Outcome{Status: StatusFail}).Validate(); err == nil {
		t.Fatalf("expected error for missing failure_reason when status=fail")
	}
	if err := (Outcome{Status: StatusRetry}).Validate(); err == nil {
		t.Fatalf("expected error for missing failure_reason when status=retry")
	}
	if err := (Outcome{Status: StatusSuccess}).Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDecodeOutcomeJSON_AcceptsCanonicalAndLegacyShapes(t *testing.T) {
	// Canonical metaspec shape.
	o1, err := DecodeOutcomeJSON([]byte(`{"status":"success","preferred_label":"x"}`))
	if err != nil {
		t.Fatalf("DecodeOutcomeJSON canonical: %v", err)
	}
	if o1.Status != StatusSuccess || o1.PreferredLabel != "x" {
		t.Fatalf("canonical decode: %+v", o1)
	}
	if o1.SuggestedNextIDs == nil || o1.ContextUpdates == nil || o1.Meta == nil {
		t.Fatalf("expected non-nil slices/maps after canonicalize: %+v", o1)
	}

	// Legacy-ish shape.
	o2, err := DecodeOutcomeJSON([]byte(`{"outcome":"SUCCESS","preferred_next_label":"Yes","suggested_next_ids":["a"],"context_updates":{"k":"v"},"notes":"n"}`))
	if err != nil {
		t.Fatalf("DecodeOutcomeJSON legacy: %v", err)
	}
	if o2.Status != StatusSuccess || o2.PreferredLabel != "Yes" {
		t.Fatalf("legacy decode: %+v", o2)
	}
	if len(o2.SuggestedNextIDs) != 1 || o2.SuggestedNextIDs[0] != "a" {
		t.Fatalf("legacy suggested_next_ids: %+v", o2.SuggestedNextIDs)
	}
	if o2.ContextUpdates["k"] != "v" {
		t.Fatalf("legacy context_updates: %+v", o2.ContextUpdates)
	}
}

func TestDecodeOutcomeJSON_LegacyFailDetails_PopulatesFailureReason(t *testing.T) {
	o, err := DecodeOutcomeJSON([]byte(`{"outcome":"fail","details":["module download blocked"],"notes":"verify step failed"}`))
	if err != nil {
		t.Fatalf("DecodeOutcomeJSON: %v", err)
	}
	if o.Status != StatusFail {
		t.Fatalf("status: got %q want %q", o.Status, StatusFail)
	}
	if strings.TrimSpace(o.FailureReason) == "" {
		t.Fatalf("expected non-empty failure_reason")
	}
	if !strings.Contains(strings.ToLower(o.FailureReason), "module") {
		t.Fatalf("failure_reason should summarize details, got: %q", o.FailureReason)
	}
}

func TestDecodeOutcomeJSON_LegacyRetryDetails_PopulatesFailureReason(t *testing.T) {
	o, err := DecodeOutcomeJSON([]byte(`{"outcome":"retry","details":"transient timeout"}`))
	if err != nil {
		t.Fatalf("DecodeOutcomeJSON: %v", err)
	}
	if o.Status != StatusRetry {
		t.Fatalf("status: got %q want %q", o.Status, StatusRetry)
	}
	if strings.TrimSpace(o.FailureReason) == "" {
		t.Fatalf("expected non-empty failure_reason")
	}
}
