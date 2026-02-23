# Unified Artifact Policy Part 1 (Core Run-Config Integration) Implementation Plan

> **For Claude:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make run config the single source of truth for artifact env and checkpoint hygiene, and remove hardcoded Rust behavior from shared engine logic.

**Architecture:** Add a minimal `artifact_policy` contract to run config, resolve it once at run start, and restore the resolved snapshot on resume for determinism. Apply the resolved policy in exactly two consumers: base node env shaping and checkpoint staging excludes. Keep Attractor spec semantics intact by using existing engine lifecycle seams and checkpoint extension fields.

**Tech Stack:** Go (`internal/attractor/engine`), YAML run config, git staging/pathspec behavior, Go test.

---

## Scope Check

This is a standalone, shippable phase. It intentionally excludes artifact verification and profile auto-detection so we can validate the architectural pivot (run-config authority + no Rust hacks) with minimal blast radius.

## Guardrails

- Do not modify `docs/strongdm/attractor/attractor-spec.md`.
- Do not add language-specific conditionals in shared engine paths.
- No compatibility shim for legacy Rust hardcoding.
- `artifact_policy` values come from run config and resolved policy snapshot, not ad hoc runtime heuristics.

## File Structure Map

- Create: `internal/attractor/engine/artifact_policy.go`  
  Responsibility: core config and resolved-policy types for Part 1.
- Create: `internal/attractor/engine/artifact_policy_resolve.go`  
  Responsibility: deterministic policy resolution for `profiles`, env vars, managed roots, checkpoint excludes.
- Create: `internal/attractor/engine/artifact_policy_resolve_test.go`  
  Responsibility: resolver precedence/path materialization tests.
- Create: `internal/attractor/engine/artifact_policy_resume_test.go`  
  Responsibility: checkpoint snapshot restore/fallback behavior tests.
- Modify: `internal/attractor/engine/config.go`  
  Responsibility: run-config schema, defaults, validation hooks.
- Modify: `internal/attractor/engine/config_runtime_policy_test.go`  
  Responsibility: config defaults/validation tests.
- Modify: `internal/attractor/engine/engine.go`  
  Responsibility: runtime field to hold resolved artifact policy.
- Modify: `internal/attractor/engine/run_with_config.go`  
  Responsibility: resolve policy at run start and attach to engine.
- Modify: `internal/attractor/engine/resume.go`  
  Responsibility: restore resolved policy from checkpoint extension.
- Modify: `internal/attractor/engine/node_env.go`  
  Responsibility: apply resolved env vars only; remove hardcoded Rust branches.
- Modify: `internal/attractor/engine/node_env_test.go`  
  Responsibility: env behavior regression tests (Rust and non-Rust).
- Modify: `internal/attractor/engine/tool_hooks.go`  
  Responsibility: checkpoint staging exclusions sourced from resolved policy.
- Modify: `internal/attractor/engine/checkpoint_exclude_test.go`  
  Responsibility: checkpoint exclusion behavior regression tests.
- Modify: `skills/english-to-dotfile/reference_run_template.yaml`  
  Responsibility: minimal authoring surface for new run-config contract.

## Chunk 1: Run-Config Contract

### Task 1: Add Minimal `artifact_policy` Schema, Defaults, and Validation

**Files:**
- Modify: `internal/attractor/engine/config.go`
- Test: `internal/attractor/engine/config_runtime_policy_test.go`

- [ ] **Step 1: Write the failing tests**

```go
func TestApplyConfigDefaults_ArtifactPolicyInitialized(t *testing.T) {
	cfg := &RunConfigFile{}
	applyConfigDefaults(cfg)

	if cfg.ArtifactPolicy.Env.ManagedRoots == nil {
		t.Fatal("artifact_policy.env.managed_roots must be initialized")
	}
	if cfg.ArtifactPolicy.Env.Overrides == nil {
		t.Fatal("artifact_policy.env.overrides must be initialized")
	}
}

func TestApplyConfigDefaults_ArtifactPolicyProfilesDefault(t *testing.T) {
	cfg := &RunConfigFile{}
	applyConfigDefaults(cfg)

	if len(cfg.ArtifactPolicy.Profiles) == 0 {
		t.Fatal("artifact_policy.profiles must default to a non-empty explicit list")
	}
}

func TestValidateConfig_ArtifactPolicyRejectsUnknownProfile(t *testing.T) {
	cfg := validMinimalRunConfigForTest()
	cfg.ArtifactPolicy.Profiles = []string{"fortran77"}

	err := validateConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error for unknown artifact_policy profile")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/attractor/engine -run 'ArtifactPolicy.*(Initialized|ProfilesDefault|RejectsUnknownProfile)' -count=1`  
Expected: FAIL with missing fields/defaults/validation.

- [ ] **Step 3: Implement minimal schema and validation in `config.go`**

```go
type ArtifactPolicyConfig struct {
	Profiles   []string               `json:"profiles,omitempty" yaml:"profiles,omitempty"`
	Env        ArtifactPolicyEnv      `json:"env,omitempty" yaml:"env,omitempty"`
	Checkpoint ArtifactPolicyCheckpoint `json:"checkpoint,omitempty" yaml:"checkpoint,omitempty"`
}

type ArtifactPolicyEnv struct {
	ManagedRoots map[string]string            `json:"managed_roots,omitempty" yaml:"managed_roots,omitempty"`
	Overrides    map[string]map[string]string `json:"overrides,omitempty" yaml:"overrides,omitempty"`
}

type ArtifactPolicyCheckpoint struct {
	ExcludeGlobs []string `json:"exclude_globs,omitempty" yaml:"exclude_globs,omitempty"`
}

func applyArtifactPolicyDefaults(cfg *RunConfigFile) {
	if len(cfg.ArtifactPolicy.Profiles) == 0 {
		cfg.ArtifactPolicy.Profiles = []string{"generic"}
	}
	if cfg.ArtifactPolicy.Env.ManagedRoots == nil {
		cfg.ArtifactPolicy.Env.ManagedRoots = map[string]string{}
	}
	if cfg.ArtifactPolicy.Env.Overrides == nil {
		cfg.ArtifactPolicy.Env.Overrides = map[string]map[string]string{}
	}
}
```

Profile whitelist for Part 1: `generic`, `rust`, `go`, `node`, `python`, `java`.

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/attractor/engine -run 'ArtifactPolicy.*(Initialized|ProfilesDefault|RejectsUnknownProfile)' -count=1`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/attractor/engine/config.go \
  internal/attractor/engine/config_runtime_policy_test.go \
  internal/attractor/engine/artifact_policy.go
git commit -m "engine/config: add minimal artifact_policy run-config contract"
```

## Chunk 2: Resolver and Engine Lifecycle Wiring

### Task 2: Resolve Policy Once Per Run and Restore from Checkpoint

**Files:**
- Create: `internal/attractor/engine/artifact_policy_resolve.go`
- Create: `internal/attractor/engine/artifact_policy_resolve_test.go`
- Create: `internal/attractor/engine/artifact_policy_resume_test.go`
- Modify: `internal/attractor/engine/engine.go`
- Modify: `internal/attractor/engine/run_with_config.go`
- Modify: `internal/attractor/engine/resume.go`

- [ ] **Step 1: Write failing resolver tests**

```go
func TestResolveArtifactPolicy_RelativeManagedRootsUseLogsRoot(t *testing.T) {
	logsRoot := t.TempDir()
	cfg := validMinimalRunConfigForTest()
	cfg.ArtifactPolicy.Profiles = []string{"rust"}
	cfg.ArtifactPolicy.Env.ManagedRoots = map[string]string{"tool_cache_root": "managed"}

	rp, err := ResolveArtifactPolicy(cfg, ResolveArtifactPolicyInput{LogsRoot: logsRoot})
	if err != nil {
		t.Fatal(err)
	}
	if got := rp.ManagedRoots["tool_cache_root"]; !strings.HasPrefix(got, filepath.Join(logsRoot, "policy-managed-roots")) {
		t.Fatalf("tool_cache_root=%q not under logs root policy-managed-roots", got)
	}
}

func TestResolveArtifactPolicy_OSOverridesProfileDefaults(t *testing.T) {
	t.Setenv("CARGO_TARGET_DIR", "/tmp/from-os")
	cfg := validMinimalRunConfigForTest()
	cfg.ArtifactPolicy.Profiles = []string{"rust"}

	rp, err := ResolveArtifactPolicy(cfg, ResolveArtifactPolicyInput{LogsRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if got := rp.Env.Vars["CARGO_TARGET_DIR"]; got != "/tmp/from-os" {
		t.Fatalf("CARGO_TARGET_DIR=%q want /tmp/from-os", got)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/attractor/engine -run 'ResolveArtifactPolicy_(RelativeManagedRootsUseLogsRoot|OSOverridesProfileDefaults)' -count=1`  
Expected: FAIL with missing resolver.

- [ ] **Step 3: Implement resolver and runtime wiring**

Resolver contract for Part 1:
- Resolution happens once in `run_with_config.go`.
- Precedence for env values: OS env > run-config `env.overrides` (active profiles) > profile defaults.
- Relative `managed_roots.*` values are rooted at `{logs_root}/policy-managed-roots/<value>`.
- Checkpoint excludes are computed from `artifact_policy.checkpoint.exclude_globs`.

Runtime wiring:
- Add `ArtifactPolicy ResolvedArtifactPolicy` to engine runtime state.
- Persist resolved snapshot in checkpoint extension (`checkpoint.Extra["artifact_policy_resolved"]`).
- On resume, restore from snapshot when present; otherwise resolve from run config.

- [ ] **Step 4: Write and run resume regression tests**

```go
func TestResume_RestoresResolvedArtifactPolicySnapshot(t *testing.T) {
	// Build checkpoint with Extra["artifact_policy_resolved"], resume, assert engine policy equals snapshot.
}

func TestResume_ResolvesPolicyWhenSnapshotMissing(t *testing.T) {
	// Older checkpoint path: no snapshot, config provided, resolver runs once and populates engine policy.
}
```

Run: `go test ./internal/attractor/engine -run 'Resume_.*ArtifactPolicy' -count=1`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/attractor/engine/artifact_policy_resolve.go \
  internal/attractor/engine/artifact_policy_resolve_test.go \
  internal/attractor/engine/artifact_policy_resume_test.go \
  internal/attractor/engine/engine.go \
  internal/attractor/engine/run_with_config.go \
  internal/attractor/engine/resume.go
git commit -m "engine/runtime: resolve artifact policy once and restore on resume"
```

## Chunk 3: Consumer Migration and Rust Hack Removal

### Task 3: Use Resolved Policy in Env + Checkpoint and Delete Rust-Specific Branches

**Files:**
- Modify: `internal/attractor/engine/node_env.go`
- Modify: `internal/attractor/engine/node_env_test.go`
- Modify: `internal/attractor/engine/tool_hooks.go`
- Modify: `internal/attractor/engine/checkpoint_exclude_test.go`

- [ ] **Step 1: Write failing regression tests for Rust de-specialization**

```go
func TestBuildBaseNodeEnv_RustVarsComeFromResolvedPolicy(t *testing.T) {
	exec := newExecutionForEnvTests(t)
	exec.Engine.ArtifactPolicy.Env.Vars = map[string]string{
		"CARGO_TARGET_DIR": "/tmp/policy-target",
	}
	env := buildBaseNodeEnv(exec)
	if !containsEnv(env, "CARGO_TARGET_DIR=/tmp/policy-target") {
		t.Fatal("expected CARGO_TARGET_DIR from resolved artifact policy")
	}
}

func TestBuildBaseNodeEnv_NoRustProfileNoRustInjection(t *testing.T) {
	exec := newExecutionForEnvTests(t)
	exec.Engine.ArtifactPolicy.Env.Vars = map[string]string{}
	env := buildBaseNodeEnv(exec)
	if findEnvPrefix(env, "CARGO_TARGET_DIR=") != "" {
		t.Fatal("unexpected implicit Rust env injection")
	}
}

func TestCheckpointExcludes_ComesFromResolvedArtifactPolicy(t *testing.T) {
	exec := newExecutionForCheckpointTests(t)
	exec.Engine.ArtifactPolicy.Checkpoint.ExcludeGlobs = []string{"**/.cargo-target*/**"}
	args := checkpointGitAddArgs(exec)
	if !slices.Contains(args, ":(exclude)**/.cargo-target*/**") {
		t.Fatal("expected checkpoint excludes from resolved artifact policy")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/attractor/engine -run '(BuildBaseNodeEnv|CheckpointExcludes_).*' -count=1`  
Expected: FAIL while Rust hardcoded branches are still present.

- [ ] **Step 3: Implement consumer migration and remove hardcoded Rust code**

Required edits:
- `node_env.go`: delete Rust-specific env branch logic; merge `Engine.ArtifactPolicy.Env.Vars` deterministically.
- `tool_hooks.go`: source checkpoint `:(exclude)` pathspec globs from `Engine.ArtifactPolicy.Checkpoint.ExcludeGlobs`.
- ensure deterministic ordering of excludes for stable tests.

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/attractor/engine -run '(BuildBaseNodeEnv|CheckpointExcludes_).*' -count=1`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/attractor/engine/node_env.go \
  internal/attractor/engine/node_env_test.go \
  internal/attractor/engine/tool_hooks.go \
  internal/attractor/engine/checkpoint_exclude_test.go
git commit -m "engine: remove hardcoded Rust artifact handling in env and checkpoint paths"
```

## Chunk 4: Authoring Surface and Phase Exit

### Task 4: Add Minimal Run Template Support for Part 1 Contract

**Files:**
- Modify: `skills/english-to-dotfile/reference_run_template.yaml`

- [ ] **Step 1: Update template with minimal Part 1 block**

```yaml
artifact_policy:
  profiles: ["generic"]
  env:
    managed_roots:
      tool_cache_root: "managed"
    overrides:
      rust:
        CARGO_TARGET_DIR: "{managed_roots.tool_cache_root}/cargo-target"
  checkpoint:
    exclude_globs:
      - "**/.cargo-target*/**"
      - "**/node_modules/**"
```

- [ ] **Step 2: Add/adjust parse test for template-compatible config**

Run: `go test ./internal/attractor/engine -run 'Config' -count=1`  
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add skills/english-to-dotfile/reference_run_template.yaml
git commit -m "skills: add minimal part-1 artifact_policy run template"
```

## Part 1 Exit Criteria

- Shared engine code has no hardcoded Rust artifact env/staging behavior.
- Run config controls artifact env and checkpoint excludes.
- Resolved policy is restored on resume deterministically.
- Rust behavior is achieved only by profile selection in run config.

## Full Validation

- `go test ./internal/attractor/engine -count=1`
- `go test ./...`

## Handoff

Plan path: `docs/superpowers/plans/2026-02-23-unified-artifact-policy-part-1-core.md`  
Prerequisite spec reference: `docs/strongdm/attractor/attractor-spec.md`
