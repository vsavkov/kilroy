package ingest

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildCLIArgs(t *testing.T) {
	// Create a temp skill file so --append-system-prompt is exercised.
	skillDir := t.TempDir()
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("test skill content"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		opts      Options
		wantExe   string
		checkArgs func(*testing.T, []string)
	}{
		{
			name: "basic invocation",
			opts: Options{
				Model:        "claude-sonnet-4-5",
				SkillPath:    skillPath,
				Requirements: "Build a solitaire game",
			},
			wantExe: "claude",
			checkArgs: func(t *testing.T, args []string) {
				assertNotContains(t, args, "-p")
				assertNotContains(t, args, "--output-format")
				assertNotContains(t, args, "--disallowedTools")
				assertContains(t, args, "--model")
				assertContains(t, args, "claude-sonnet-4-5")
				assertContains(t, args, "--append-system-prompt")
				assertContains(t, args, "--max-turns")
				assertContains(t, args, "--dangerously-skip-permissions")
			},
		},
		{
			name: "custom model",
			opts: Options{
				Model:        "claude-opus-4-6",
				SkillPath:    skillPath,
				Requirements: "Build DTTF",
			},
			checkArgs: func(t *testing.T, args []string) {
				assertContains(t, args, "claude-opus-4-6")
			},
		},
		{
			name: "custom max turns",
			opts: Options{
				Model:        "claude-sonnet-4-5",
				SkillPath:    skillPath,
				Requirements: "Build something",
				MaxTurns:     5,
			},
			checkArgs: func(t *testing.T, args []string) {
				assertContains(t, args, "5")
			},
		},
		{
			name: "no skill path omits flag",
			opts: Options{
				Model:        "claude-sonnet-4-5",
				Requirements: "Build something",
			},
			checkArgs: func(t *testing.T, args []string) {
				assertNotContains(t, args, "--append-system-prompt")
			},
		},
		{
			name: "relative repo path resolved to absolute",
			opts: Options{
				Model:        "claude-sonnet-4-5",
				Requirements: "Build something",
				RepoPath:     "../relative/path",
			},
			checkArgs: func(t *testing.T, args []string) {
				for i, a := range args {
					if a == "--add-dir" && i+1 < len(args) {
						if !filepath.IsAbs(args[i+1]) {
							t.Errorf("--add-dir value %q is not absolute", args[i+1])
						}
						return
					}
				}
				t.Error("--add-dir not found in args")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exe, args, tmpDir, err := buildCLIArgs(tt.opts)
			if err != nil {
				t.Fatalf("buildCLIArgs: %v", err)
			}
			defer os.RemoveAll(tmpDir)
			if tt.wantExe != "" && exe != tt.wantExe {
				t.Errorf("exe = %q, want %q", exe, tt.wantExe)
			}
			if tt.checkArgs != nil {
				tt.checkArgs(t, args)
			}
		})
	}
}

func TestRunIngestRequiresSkill(t *testing.T) {
	_, err := Run(context.Background(), Options{
		Requirements: "Build something",
		SkillPath:    "/nonexistent/SKILL.md",
		Model:        "claude-sonnet-4-5",
	})
	if err == nil {
		t.Fatal("expected error for missing skill file")
	}
}

func assertContains(t *testing.T, slice []string, want string) {
	t.Helper()
	for _, s := range slice {
		if s == want {
			return
		}
	}
	t.Errorf("args %v does not contain %q", slice, want)
}

func assertNotContains(t *testing.T, slice []string, unwanted string) {
	t.Helper()
	for _, s := range slice {
		if s == unwanted {
			t.Errorf("args %v should not contain %q", slice, unwanted)
			return
		}
	}
}
