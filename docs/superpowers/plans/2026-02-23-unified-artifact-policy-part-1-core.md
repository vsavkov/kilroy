# Unified Artifact Policy Part 1 (Core Run-Config Integration) Implementation Plan

> **For Claude:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make run config the single source of truth for artifact env and checkpoint hygiene, and remove language-specific hardcoding (including Rust) from shared engine logic.

**Architecture:** Add a minimal `artifact_policy` contract to run config, resolve it once at run start, and restore the resolved snapshot on resume for determinism. Apply the resolved policy in exactly two consumers: base node env shaping and checkpoint staging excludes. Keep Attractor spec semantics intact by using existing engine lifecycle seams and checkpoint extension fields.

**Tech Stack:** Go (`internal/attractor/engine`), YAML run config, git staging/pathspec behavior, Go test.

---

## Scope Check

This is a standalone, shippable phase. It intentionally excludes artifact verification and profile auto-detection so we can validate the architectural pivot (run-config authority + no Rust hacks) with minimal blast radius.

## Guardrails

- Do not modify `docs/strongdm/attractor/attractor-spec.md`.
- Do not add language-specific conditionals in shared engine paths.
- No compatibility shim for legacy Rust hardcoding.
- `artifact_policy` is a Kilroy run-config extension; Attractor spec contracts remain unchanged.
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
- Create: `internal/attractor/engine/artifact_policy_test_helpers_test.go`  
  Responsibility: shared test helpers used by new config/resolver/env tests.
- Modify: `internal/attractor/engine/config.go`  
  Responsibility: run-config schema, defaults, validation hooks.
- Modify: `internal/attractor/engine/config_runtime_policy_test.go`  
  Responsibility: config defaults/validation tests (including checkpoint exclude migration assertions).
- Modify: `internal/attractor/engine/checkpoint_exclude_test.go`  
  Responsibility: migrate integration test input from legacy git excludes to artifact policy excludes in Chunk 1 so early commits stay green.
- Modify: `internal/attractor/engine/engine.go`  
  Responsibility: migrate checkpoint exclude consumer to new source in Chunk 1, then add runtime field to hold resolved artifact policy in Chunk 2.
- Modify: `internal/attractor/engine/run_with_config.go`  
  Responsibility: resolve policy at run start and attach to engine.
- Modify: `internal/attractor/engine/resume.go`  
  Responsibility: restore resolved policy from checkpoint extension.
- Modify: `internal/attractor/engine/node_env.go`  
  Responsibility: apply resolved env vars only; remove hardcoded Rust/Go branches.
- Modify: `internal/attractor/engine/node_env_test.go`  
  Responsibility: env behavior regression tests (Rust and non-Rust).
- Modify: `internal/attractor/engine/tool_hooks.go`  
  Responsibility: pass resolved policy into env builders at call sites.
- Modify: `internal/attractor/engine/handlers.go`  
  Responsibility: update `buildBaseNodeEnv` caller for new signature.
- Modify: `internal/attractor/engine/codergen_router.go`  
  Responsibility: update env-builder callers and agent-loop override invocation for policy-aware signatures.
- Modify: `internal/attractor/engine/api_env_parity_test.go`  
  Responsibility: keep API/CLI env parity tests aligned after `buildAgentLoopOverrides` changes.
- Modify: `skills/english-to-dotfile/reference_run_template.yaml`  
  Responsibility: minimal authoring surface for new run-config contract.

## Chunk 1: Run-Config Contract

### Task 1: Add Minimal `artifact_policy` Schema, Defaults, and Validation

**Files:**
- Create: `internal/attractor/engine/artifact_policy.go`
- Create: `internal/attractor/engine/artifact_policy_test_helpers_test.go`
- Modify: `internal/attractor/engine/config.go`
- Modify: `internal/attractor/engine/engine.go`
- Test: `internal/attractor/engine/config_runtime_policy_test.go`
- Test: `internal/attractor/engine/checkpoint_exclude_test.go`

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

func TestValidateConfig_ArtifactPolicyRejectsLegacyGitCheckpointExcludes(t *testing.T) {
	cfg := validMinimalRunConfigForTest()
	cfg.Git.CheckpointExcludeGlobs = []string{"**/tmp-build/**"}
	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected legacy git.checkpoint_exclude_globs validation error")
	}
}

func TestApplyConfigDefaults_LegacyGitCheckpointExcludesNotAutoPopulated(t *testing.T) {
	cfg := &RunConfigFile{}
	applyConfigDefaults(cfg)
	if len(cfg.Git.CheckpointExcludeGlobs) != 0 {
		t.Fatal("git.checkpoint_exclude_globs must remain empty after migration to artifact_policy")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/attractor/engine -run '(ArtifactPolicy.*(Initialized|ProfilesDefault|RejectsUnknownProfile|RejectsLegacyGitCheckpointExcludes)|LegacyGitCheckpointExcludesNotAutoPopulated)' -count=1`  
Expected: FAIL with missing fields/defaults/validation.

- [ ] **Step 3: Implement minimal schema/types, add `RunConfigFile` field, and add validation**

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

type RunConfigFile struct {
	// ... existing fields ...
	ArtifactPolicy ArtifactPolicyConfig `json:"artifact_policy,omitempty" yaml:"artifact_policy,omitempty"`
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
	if len(cfg.ArtifactPolicy.Checkpoint.ExcludeGlobs) == 0 {
		cfg.ArtifactPolicy.Checkpoint.ExcludeGlobs = []string{
			"**/.cargo-target*/**",
			"**/.cargo_target*/**",
			"**/.wasm-pack/**",
			"**/.tmpbuild/**",
		}
	}
}

func validateArtifactPolicyConfig(cfg *RunConfigFile) error {
	allowedProfiles := map[string]struct{}{
		"generic": {},
		"rust":    {},
		"go":      {},
		"node":    {},
		"python":  {},
		"java":    {},
	}
	for _, p := range cfg.ArtifactPolicy.Profiles {
		if _, ok := allowedProfiles[p]; !ok {
			return fmt.Errorf("artifact_policy.profiles contains unsupported profile %q", p)
		}
	}
	if len(cfg.Git.CheckpointExcludeGlobs) > 0 {
		return fmt.Errorf("git.checkpoint_exclude_globs is deprecated; use artifact_policy.checkpoint.exclude_globs")
	}
	return nil
}
```

```go
// artifact_policy_test_helpers_test.go
func validMinimalRunConfigForTest() *RunConfigFile {
	cfg := &RunConfigFile{}
	applyConfigDefaults(cfg)
	cfg.Version = 1
	cfg.Repo.Path = "/tmp/repo"
	cfg.CXDB.BinaryAddr = "127.0.0.1:9009"
	cfg.CXDB.HTTPBaseURL = "http://127.0.0.1:9010"
	cfg.ModelDB.OpenRouterModelInfoPath = "/tmp/catalog.json"
	return cfg
}

func containsEnv(env []string, item string) bool {
	for _, v := range env {
		if v == item {
			return true
		}
	}
	return false
}

func findEnvPrefix(env []string, prefix string) string {
	for _, v := range env {
		if strings.HasPrefix(v, prefix) {
			return v
		}
	}
	return ""
}
```

- [ ] **Step 4: Migrate checkpoint exclude defaults and consumer source in the same commit**

Migration sequencing requirement:
- In `applyConfigDefaults`, stop auto-populating `cfg.Git.CheckpointExcludeGlobs`.
- Keep the legacy field empty by default.
- Populate defaults in `cfg.ArtifactPolicy.Checkpoint.ExcludeGlobs` instead.
- In the same chunk, update `Engine.checkpointExcludeGlobs()` in `internal/attractor/engine/engine.go` to read from `e.RunConfig.ArtifactPolicy.Checkpoint.ExcludeGlobs` so checkpoint behavior remains correct immediately after migration (before `Engine.ArtifactPolicy` exists in Chunk 2).
- Replace `TestApplyConfigDefaults_CheckpointExcludeGlobs` in `internal/attractor/engine/config_runtime_policy_test.go` with:

```go
func TestApplyConfigDefaults_ArtifactPolicyCheckpointExcludeGlobs(t *testing.T) {
	cfg := &RunConfigFile{}
	applyConfigDefaults(cfg)
	if len(cfg.ArtifactPolicy.Checkpoint.ExcludeGlobs) == 0 {
		t.Fatal("expected non-empty artifact_policy.checkpoint.exclude_globs defaults")
	}
	if len(cfg.Git.CheckpointExcludeGlobs) != 0 {
		t.Fatal("legacy git.checkpoint_exclude_globs must remain empty")
	}
}
```

- In the same commit, migrate `internal/attractor/engine/checkpoint_exclude_test.go` setup from:
  - `cfg.Git.CheckpointExcludeGlobs = ...`
  to:
  - `cfg.ArtifactPolicy.Checkpoint.ExcludeGlobs = ...`
  so Chunk 1 remains test-green.

Profile whitelist for Part 1: `generic`, `rust`, `go`, `node`, `python`, `java`.

- [ ] **Step 5: Run tests to verify pass**

Run: `go test ./internal/attractor/engine -run '(ArtifactPolicy.*(Initialized|ProfilesDefault|RejectsUnknownProfile|RejectsLegacyGitCheckpointExcludes)|LegacyGitCheckpointExcludesNotAutoPopulated)' -count=1`  
Expected: PASS.

Run: `go test ./internal/attractor/engine -run 'CheckpointExcludesConfiguredArtifacts' -count=1`  
Expected: PASS (ensures Chunk 1 migration did not break existing integration coverage).

- [ ] **Step 6: Commit**

```bash
git add internal/attractor/engine/config.go \
  internal/attractor/engine/artifact_policy.go \
  internal/attractor/engine/artifact_policy_test_helpers_test.go \
  internal/attractor/engine/config_runtime_policy_test.go \
  internal/attractor/engine/engine.go \
  internal/attractor/engine/checkpoint_exclude_test.go
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

func TestResolveArtifactPolicy_CheckpointExcludesMirrorConfig(t *testing.T) {
	cfg := validMinimalRunConfigForTest()
	cfg.ArtifactPolicy.Checkpoint.ExcludeGlobs = []string{"**/.cargo-target*/**"}
	rp, err := ResolveArtifactPolicy(cfg, ResolveArtifactPolicyInput{LogsRoot: t.TempDir(), WorktreeDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(rp.Checkpoint.ExcludeGlobs) != 1 || rp.Checkpoint.ExcludeGlobs[0] != "**/.cargo-target*/**" {
		t.Fatalf("checkpoint excludes mismatch: %+v", rp.Checkpoint.ExcludeGlobs)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/attractor/engine -run 'ResolveArtifactPolicy_(RelativeManagedRootsUseLogsRoot|OSOverridesProfileDefaults|CheckpointExcludesMirrorConfig)' -count=1`  
Expected: FAIL with missing resolver.

- [ ] **Step 3: Implement resolver and runtime wiring**

Resolver contract for Part 1:
- Resolution happens once in `run_with_config.go`.
- Precedence for env values: OS env > run-config `env.overrides` (active profiles) > profile defaults.
- Relative `managed_roots.*` values are rooted at `{logs_root}/policy-managed-roots/<value>`.
- Profile default templates like `{managed_roots.tool_cache_root}/cargo-target` must be expanded after managed-root resolution and before OS-env precedence is applied.
- Checkpoint excludes are computed from `artifact_policy.checkpoint.exclude_globs`.

Built-in profile defaults must be explicit in `artifact_policy_resolve.go`:

```go
type ResolvedArtifactPolicy struct {
	Profiles   []string
	ManagedRoots map[string]string
	Env        ResolvedArtifactEnv
	Checkpoint ResolvedArtifactCheckpoint
}

type ResolvedArtifactEnv struct {
	Vars map[string]string
}

type ResolvedArtifactCheckpoint struct {
	ExcludeGlobs []string
}

type ResolveArtifactPolicyInput struct {
	LogsRoot    string
	WorktreeDir string
}

var profileDefaultEnv = map[string]map[string]string{
	"generic": {},
	"rust": {
		"CARGO_HOME":       "{managed_roots.tool_cache_root}/cargo-home",
		"RUSTUP_HOME":      "{managed_roots.tool_cache_root}/rustup-home",
		"CARGO_TARGET_DIR": "{managed_roots.tool_cache_root}/cargo-target",
	},
	"go": {
		"GOPATH":     "{managed_roots.tool_cache_root}/go-path",
		"GOMODCACHE": "{managed_roots.tool_cache_root}/go-path/pkg/mod",
	},
	"node":   {},
	"python": {},
	"java":   {},
}
```

Runtime wiring:
- Add `ArtifactPolicy ResolvedArtifactPolicy` to engine runtime state.
- After this field exists, switch `Engine.checkpointExcludeGlobs()` from `e.RunConfig.ArtifactPolicy...` to `e.ArtifactPolicy.Checkpoint...`.
- Preserve behavior across the source switch by asserting `ResolvedArtifactPolicy.Checkpoint.ExcludeGlobs` exactly mirrors config values in Part 1.
- Persist resolved snapshot in checkpoint extension (`checkpoint.Extra["artifact_policy_resolved"]`) with explicit struct tags/versioned envelope.
- Implement helper `restoreArtifactPolicyForResume(cp *runtime.Checkpoint, cfg *RunConfigFile, in ResolveArtifactPolicyInput) (ResolvedArtifactPolicy, error)` and call it from `resume.go`.
- On resume, restore from snapshot when present; otherwise resolve from run config.

- [ ] **Step 4: Write and run resume regression tests**

```go
func TestRestoreArtifactPolicyForResume_UsesCheckpointSnapshot(t *testing.T) {
	cp := runtime.NewCheckpoint()
	cp.Extra = map[string]any{
		"artifact_policy_resolved": map[string]any{
			"profiles": []any{"rust"},
			"env": map[string]any{"vars": map[string]any{"CARGO_TARGET_DIR": "/tmp/policy-target"}},
		},
	}
	cfg := validMinimalRunConfigForTest()
	rp, err := restoreArtifactPolicyForResume(cp, cfg, ResolveArtifactPolicyInput{LogsRoot: t.TempDir(), WorktreeDir: t.TempDir()})
	if err != nil {
		t.Fatalf("restoreArtifactPolicyForResume: %v", err)
	}
	if got := rp.Env.Vars["CARGO_TARGET_DIR"]; got != "/tmp/policy-target" {
		t.Fatalf("CARGO_TARGET_DIR=%q want /tmp/policy-target", got)
	}
}

func TestRestoreArtifactPolicyForResume_FallsBackToResolverWhenSnapshotMissing(t *testing.T) {
	cp := runtime.NewCheckpoint() // no artifact_policy_resolved in Extra
	cfg := validMinimalRunConfigForTest()
	cfg.ArtifactPolicy.Profiles = []string{"rust"}
	rp, err := restoreArtifactPolicyForResume(cp, cfg, ResolveArtifactPolicyInput{LogsRoot: t.TempDir(), WorktreeDir: t.TempDir()})
	if err != nil {
		t.Fatalf("restoreArtifactPolicyForResume: %v", err)
	}
	if len(rp.Profiles) == 0 {
		t.Fatal("expected resolver fallback to populate artifact policy from run config")
	}
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
- Modify: `internal/attractor/engine/handlers.go`
- Modify: `internal/attractor/engine/codergen_router.go`
- Modify: `internal/attractor/engine/api_env_parity_test.go`
- Modify: `internal/attractor/engine/checkpoint_exclude_test.go`

- [ ] **Step 1: Write failing regression tests for Rust de-specialization**

```go
func TestBuildBaseNodeEnv_RustVarsComeFromResolvedPolicy(t *testing.T) {
	rp := ResolvedArtifactPolicy{}
	rp.Env.Vars = map[string]string{
		"CARGO_TARGET_DIR": "/tmp/policy-target",
	}
	env := buildBaseNodeEnv(t.TempDir(), rp)
	if !containsEnv(env, "CARGO_TARGET_DIR=/tmp/policy-target") {
		t.Fatal("expected CARGO_TARGET_DIR from resolved artifact policy")
	}
}

func TestBuildBaseNodeEnv_NoImplicitRustOrGoInjectionWithoutPolicy(t *testing.T) {
	env := buildBaseNodeEnv(t.TempDir(), ResolvedArtifactPolicy{})
	if findEnvPrefix(env, "CARGO_TARGET_DIR=") != "" {
		t.Fatal("unexpected implicit Rust env injection")
	}
	if findEnvPrefix(env, "GOPATH=") != "" {
		t.Fatal("unexpected implicit Go env injection")
	}
}

func TestRunWithConfig_CheckpointExcludesConfiguredArtifactsFromArtifactPolicy(t *testing.T) {
	repo := initTestRepo(t)
	logsRoot := t.TempDir()
	pinned := writePinnedCatalog(t)
	cxdbSrv := newCXDBTestServer(t)

	cfg := &RunConfigFile{Version: 1}
	cfg.Repo.Path = repo
	cfg.CXDB.BinaryAddr = cxdbSrv.BinaryAddr()
	cfg.CXDB.HTTPBaseURL = cxdbSrv.URL()
	cfg.ModelDB.OpenRouterModelInfoPath = pinned
	cfg.ModelDB.OpenRouterModelInfoUpdatePolicy = "pinned"
	cfg.Git.RunBranchPrefix = "attractor/run"
	cfg.ArtifactPolicy.Checkpoint.ExcludeGlobs = []string{"**/.cargo_target_local/**"}

	dot := []byte(`digraph G {
  graph [goal="checkpoint exclude from artifact policy"]
  start [shape=Mdiamond]
  exit  [shape=Msquare]
  write_files [shape=parallelogram, tool_command="mkdir -p src .cargo_target_local/obj && echo ok > src/ok.txt && echo temp > .cargo_target_local/obj/a.bin"]
  start -> write_files -> exit
}`)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	res, err := RunWithConfig(ctx, dot, cfg, RunOptions{RunID: "checkpoint-exclude-artifact-policy", LogsRoot: logsRoot})
	if err != nil {
		t.Fatalf("RunWithConfig: %v", err)
	}
	files := gitLsFiles(t, res.WorktreeDir)
	if containsPath(files, ".cargo_target_local/obj/a.bin") {
		t.Fatal("excluded artifact should not be checkpointed")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/attractor/engine -run '(BuildBaseNodeEnv|CheckpointExcludesConfiguredArtifactsFromArtifactPolicy)' -count=1`  
Expected: FAIL while Rust hardcoded branches are still present.

- [ ] **Step 3: Implement consumer migration and remove hardcoded Rust code**

Required edits:
- `node_env.go`: change `buildBaseNodeEnv` signature to accept resolved policy (`buildBaseNodeEnv(worktreeDir string, rp ResolvedArtifactPolicy)`), delete Rust/Go-specific env branch logic, and merge `rp.Env.Vars` deterministically.
- `node_env.go`: replace hardcoded `buildAgentLoopOverrides` keep-map with policy-driven forwarding derived from resolved env vars.
- `node_env.go`: change `buildAgentLoopOverrides` signature to `buildAgentLoopOverrides(worktreeDir string, rp ResolvedArtifactPolicy, contractEnv map[string]string)` so policy is threaded explicitly, including its internal call to `buildBaseNodeEnv`.
- `tool_hooks.go`, `handlers.go`, and `codergen_router.go`: update all direct `buildBaseNodeEnv` call sites to pass resolved policy.
- `codergen_router.go` and `api_env_parity_test.go`: update `buildAgentLoopOverrides` call/expectation to use policy-aware signature (`codergen_router.go` has call sites in both API and CLI paths).
- ensure deterministic ordering of excludes for stable tests.
- `node_env_test.go`: explicitly update existing tests that rely on implicit Rust/Go pinning (`TestBuildBaseNodeEnv_PreservesToolchainPaths`, `TestBuildBaseNodeEnv_InfersGoPathsFromHOME`, `TestBuildBaseNodeEnv_GoModCacheUsesFirstGOPATHEntry`, `TestBuildBaseNodeEnv_SetsCargoTargetDirToWorktree`, `TestBuildBaseNodeEnv_DoesNotOverrideExplicitCargoTargetDir`, `TestBuildBaseNodeEnv_InfersToolchainPathsFromHOME`, `TestToolHandler_UsesBaseNodeEnv`, `TestBuildCodexIsolatedEnv_PreservesToolchainPaths`, `TestBuildCodexIsolatedEnvWithName_RetryPreservesToolchainPaths`) to inject policy explicitly in setup/expectations.
- `node_env_test.go`: also update `TestBuildBaseNodeEnv_StripsClaudeCode` for the new function signature.

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/attractor/engine -run '(BuildBaseNodeEnv|CheckpointExcludesConfiguredArtifactsFromArtifactPolicy)' -count=1`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/attractor/engine/node_env.go \
  internal/attractor/engine/node_env_test.go \
  internal/attractor/engine/tool_hooks.go \
  internal/attractor/engine/handlers.go \
  internal/attractor/engine/codergen_router.go \
  internal/attractor/engine/api_env_parity_test.go \
  internal/attractor/engine/checkpoint_exclude_test.go
git commit -m "engine: remove hardcoded Rust/Go artifact handling in env and checkpoint paths"
```

## Chunk 4: Authoring Surface and Phase Exit

### Task 4: Add Minimal Run Template Support for Part 1 Contract

**Files:**
- Modify: `skills/english-to-dotfile/reference_run_template.yaml`

- [ ] **Step 1: Update template with minimal Part 1 block**

```yaml
artifact_policy:
  profiles: ["rust"]
  env:
    managed_roots:
      tool_cache_root: "managed"
    overrides:
      rust:
        CARGO_TARGET_DIR: "{managed_roots.tool_cache_root}/cargo-target"
  checkpoint:
    exclude_globs:
      - "**/.cargo-target*/**"
      - "**/.cargo_target*/**"
```

Note: `"{managed_roots.tool_cache_root}/cargo-target"` is an intentional literal resolver template, not YAML interpolation.

Run-config migration note for Part 1:
- `git.checkpoint_exclude_globs` is rejected by validation.
- `artifact_policy.checkpoint.exclude_globs` is the single staging exclude source.

- [ ] **Step 2: Add/adjust parse test for template-compatible config**

Run: `go test ./internal/attractor/engine -run 'Config' -count=1`  
Expected: PASS.

Run: `go test ./internal/attractor/engine -run 'LoadRunConfigFile_YAMLAndJSON' -count=1`  
Expected: PASS (template-compatible YAML parses with new schema).

- [ ] **Step 3: Commit**

```bash
git add skills/english-to-dotfile/reference_run_template.yaml
git commit -m "skills: add minimal part-1 artifact_policy run template"
```

## Part 1 Exit Criteria

- Shared engine code has no hardcoded Rust/Go artifact env/staging behavior.
- Run config controls artifact env and checkpoint excludes.
- Resolved policy is restored on resume deterministically.
- Rust behavior is achieved only by profile selection in run config.

## Full Validation

- `go test ./internal/attractor/engine -count=1`
- `go test ./...`

## Handoff

Plan path: `docs/superpowers/plans/2026-02-23-unified-artifact-policy-part-1-core.md`  
Prerequisite spec reference: `docs/strongdm/attractor/attractor-spec.md`
