package main

import (
	"testing"
)

func TestParseIngestArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
		check   func(*testing.T, *ingestOptions)
	}{
		{
			name:    "missing requirements",
			args:    []string{},
			wantErr: true,
		},
		{
			name: "requirements from positional arg",
			args: []string{"Build a solitaire game"},
			check: func(t *testing.T, o *ingestOptions) {
				if o.requirements != "Build a solitaire game" {
					t.Errorf("requirements = %q, want %q", o.requirements, "Build a solitaire game")
				}
			},
		},
		{
			name: "output flag",
			args: []string{"--output", "pipeline.dot", "Build a solitaire game"},
			check: func(t *testing.T, o *ingestOptions) {
				if o.outputPath != "pipeline.dot" {
					t.Errorf("outputPath = %q, want %q", o.outputPath, "pipeline.dot")
				}
			},
		},
		{
			name: "model flag",
			args: []string{"--model", "claude-opus-4-6", "Build a solitaire game"},
			check: func(t *testing.T, o *ingestOptions) {
				if o.model != "claude-opus-4-6" {
					t.Errorf("model = %q, want %q", o.model, "claude-opus-4-6")
				}
			},
		},
		{
			name: "skill flag",
			args: []string{"--skill", "/tmp/custom-skill.md", "Build a solitaire game"},
			check: func(t *testing.T, o *ingestOptions) {
				if o.skillPath != "/tmp/custom-skill.md" {
					t.Errorf("skillPath = %q, want %q", o.skillPath, "/tmp/custom-skill.md")
				}
			},
		},
		{
			name: "default model",
			args: []string{"Build a solitaire game"},
			check: func(t *testing.T, o *ingestOptions) {
				if o.model != "claude-sonnet-4-5" {
					t.Errorf("model = %q, want default %q", o.model, "claude-sonnet-4-5")
				}
			},
		},
		{
			name: "max-turns flag",
			args: []string{"--max-turns", "10", "Build a solitaire game"},
			check: func(t *testing.T, o *ingestOptions) {
				if o.maxTurns != 10 {
					t.Errorf("maxTurns = %d, want 10", o.maxTurns)
				}
			},
		},
		{
			name:    "max-turns missing value",
			args:    []string{"--max-turns"},
			wantErr: true,
		},
		{
			name:    "max-turns non-integer",
			args:    []string{"--max-turns", "abc", "Build a solitaire game"},
			wantErr: true,
		},
		{
			name:    "max-turns zero",
			args:    []string{"--max-turns", "0", "Build a solitaire game"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := parseIngestArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseIngestArgs() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && tt.check != nil {
				tt.check(t, opts)
			}
		})
	}
}
