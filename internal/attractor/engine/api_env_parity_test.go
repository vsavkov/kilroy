package engine

import (
	"testing"
)

func TestBuildAgentLoopOverrides_UsesBaseNodeEnvContract(t *testing.T) {
	worktree := t.TempDir()
	rp := ResolvedArtifactPolicy{
		Env: ResolvedArtifactEnv{
			Vars: map[string]string{
				"CARGO_TARGET_DIR": "/tmp/policy-target",
			},
		},
	}
	env := buildAgentLoopOverrides(worktree, rp, map[string]string{"KILROY_STAGE_STATUS_PATH": "/tmp/status.json"})

	if env["CARGO_TARGET_DIR"] != "/tmp/policy-target" {
		t.Fatal("CARGO_TARGET_DIR must come from resolved artifact policy for API agent_loop path")
	}
	if env["KILROY_STAGE_STATUS_PATH"] != "/tmp/status.json" {
		t.Fatal("stage status env must be preserved")
	}
}
