package engine

import (
	"fmt"
	"strings"
)

type ArtifactPolicyConfig struct {
	Profiles   []string                 `json:"profiles,omitempty" yaml:"profiles,omitempty"`
	Env        ArtifactPolicyEnv        `json:"env,omitempty" yaml:"env,omitempty"`
	Checkpoint ArtifactPolicyCheckpoint `json:"checkpoint,omitempty" yaml:"checkpoint,omitempty"`
}

type ArtifactPolicyEnv struct {
	ManagedRoots map[string]string            `json:"managed_roots,omitempty" yaml:"managed_roots,omitempty"`
	Overrides    map[string]map[string]string `json:"overrides,omitempty" yaml:"overrides,omitempty"`
}

type ArtifactPolicyCheckpoint struct {
	ExcludeGlobs []string `json:"exclude_globs,omitempty" yaml:"exclude_globs,omitempty"`
}

var allowedArtifactPolicyProfiles = map[string]struct{}{
	"generic": {},
	"rust":    {},
	"go":      {},
	"node":    {},
	"python":  {},
	"java":    {},
}

func applyArtifactPolicyDefaults(cfg *RunConfigFile) {
	if cfg == nil {
		return
	}

	cfg.ArtifactPolicy.Profiles = normalizeArtifactPolicyProfiles(cfg.ArtifactPolicy.Profiles)
	if len(cfg.ArtifactPolicy.Profiles) == 0 {
		cfg.ArtifactPolicy.Profiles = []string{"generic"}
	}

	if cfg.ArtifactPolicy.Env.ManagedRoots == nil {
		cfg.ArtifactPolicy.Env.ManagedRoots = map[string]string{}
	}
	if cfg.ArtifactPolicy.Env.Overrides == nil {
		cfg.ArtifactPolicy.Env.Overrides = map[string]map[string]string{}
	}

	cfg.ArtifactPolicy.Checkpoint.ExcludeGlobs = trimNonEmpty(cfg.ArtifactPolicy.Checkpoint.ExcludeGlobs)
	if len(cfg.ArtifactPolicy.Checkpoint.ExcludeGlobs) == 0 {
		cfg.ArtifactPolicy.Checkpoint.ExcludeGlobs = []string{
			"**/.cargo-target*/**",
			"**/.cargo_target*/**",
			"**/.wasm-pack/**",
			"**/.tmpbuild/**",
		}
	}
}

func normalizeArtifactPolicyProfiles(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, p := range in {
		s := strings.ToLower(strings.TrimSpace(p))
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func validateArtifactPolicyConfig(cfg *RunConfigFile) error {
	if cfg == nil {
		return nil
	}
	for _, p := range cfg.ArtifactPolicy.Profiles {
		if _, ok := allowedArtifactPolicyProfiles[p]; !ok {
			return fmt.Errorf("artifact_policy.profiles contains unsupported profile %q", p)
		}
	}
	if len(cfg.Git.CheckpointExcludeGlobs) > 0 {
		return fmt.Errorf("git.checkpoint_exclude_globs is deprecated; use artifact_policy.checkpoint.exclude_globs")
	}
	return nil
}
