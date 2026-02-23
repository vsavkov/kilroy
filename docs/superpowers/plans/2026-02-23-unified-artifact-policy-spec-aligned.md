# Unified Artifact Policy (Spec-Aligned, No Spec Changes) Implementation Plan

> **For Claude:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement one run-resolved artifact policy that drives env shaping, checkpoint staging, and artifact verification while remaining fully consistent with `docs/strongdm/attractor/attractor-spec.md` as written.

**Architecture:** Keep Attractor core contracts intact: handlers execute nodes, engine routes on `Outcome`, checkpoint/resume stays deterministic. Add a Kilroy run-config extension (`artifact_policy`) with modular sub-policies (`env`, `checkpoint`, `verify`), resolve it once at run start, serialize the resolved snapshot for resume determinism, and consume it through existing engine/handler seams. Replace ad hoc regex `verify_artifacts` tool commands with a built-in `verify.artifacts` handler that returns existing Go runtime `Outcome` fields (`status`, `failure_reason`, `details`, `meta`, `context_updates`).

**Tech Stack:** Go (`internal/attractor/engine`, `internal/attractor/runtime`), YAML run config (`RunConfigFile`), DOT templates, `doublestar` glob matching, Git porcelain/pathspec, Go test (`go test ./...`).

---

## Scope Check

This is one subsystem-level refactor (artifact policy contract + engine integration + authoring surface updates). It should remain one plan because the changes are tightly coupled: config schema, resolver semantics, handler behavior, checkpoint/resume, and template/skill generation must all land together to keep runs deterministic.

## Non-Negotiable Constraints

- Do not modify `docs/strongdm/attractor/attractor-spec.md`.
- Preserve Attractor idioms from the existing spec:
  - handler contract (spec-level abstract contract + Go implementation contract `Execute(ctx context.Context, exec *Execution, node *model.Node) (runtime.Outcome, error)`),
  - core execution loop order,
  - `Outcome` field semantics,
  - checkpoint/resume model.
- No language-specific hacks in shared engine paths.
- No compatibility shim work for legacy ad hoc artifact regex behavior.

## Issue-Closure Matrix (from Fresh-Eyes Review)

- Missing spec anchor: solved by explicitly treating `artifact_policy` as Kilroy run-config extension; no Attractor spec changes.
- Undefined execution integration point: solved by implementing verification as a registered handler type (`verify.artifacts`) and keeping engine loop unchanged.
- Outcome contract mismatch: solved by using existing `Outcome` fields only (`failure_reason`, `details`, `meta`, `context_updates`).
- Resolve-once vs resume ambiguity: solved by serializing resolved policy snapshot into checkpoint `Extra` and restoring on resume.
- Auto profile detection undefined: solved by deterministic detector algorithm + explicit precedence rules.
- Env / managed roots semantics vague: solved by explicit merge order and path rules.
- Deny/allow precedence undefined: solved by deterministic rule order (`deny` first, then `allow` carve-out).
- Core cleanup ambiguity: solved by preserving current invariants through profile defaults before deleting Rust-specific hooks.
- Template migration gap: solved by updating reference DOT + demo DOT + skill + run YAML template in same change set.
- Missing tests: solved by dedicated unit/integration/resume regression tasks below.
- Monolithic policy risk: solved by sub-policy split (`env`, `checkpoint`, `verify`) under one root.

## File Structure Map

- Create: `internal/attractor/engine/artifact_policy.go`
- Create: `internal/attractor/engine/artifact_policy_profiles.go`
- Create: `internal/attractor/engine/artifact_policy_resolve.go`
- Create: `internal/attractor/engine/artifact_verify_handler.go`
- Create: `internal/attractor/engine/artifact_policy_test.go`
- Create: `internal/attractor/engine/artifact_policy_resolve_test.go`
- Create: `internal/attractor/engine/artifact_verify_handler_test.go`
- Create: `internal/attractor/engine/artifact_policy_resume_test.go`
- Create: `internal/attractor/engine/artifact_policy_test_helpers_test.go`
- Modify: `internal/attractor/engine/config.go`
- Modify: `internal/attractor/engine/config_runtime_policy_test.go`
- Modify: `internal/attractor/engine/engine.go`
- Modify: `internal/attractor/engine/run_with_config.go`
- Modify: `internal/attractor/engine/resume.go`
- Modify: `internal/attractor/engine/node_env.go`
- Modify: `internal/attractor/engine/node_env_test.go`
- Modify: `internal/attractor/engine/api_env_parity_test.go`
- Modify: `internal/attractor/engine/handlers.go`
- Modify: `internal/attractor/engine/codergen_router.go`
- Modify: `internal/attractor/engine/tool_hooks.go`
- Modify: `internal/attractor/engine/checkpoint_exclude_test.go`
- Modify: `internal/attractor/engine/archive.go`
- Modify: `skills/english-to-dotfile/reference_template.dot`
- Modify: `demo/rogue/rogue.dot`
- Modify: `skills/english-to-dotfile/SKILL.md`
- Modify: `skills/english-to-dotfile/reference_run_template.yaml`
- Modify: `internal/attractor/validate/reference_template_guardrail_test.go`
- Modify: `docs/plans/2026-02-23-unified-artifact-policy-refactor-proposal.md`

## Dependency Order

1. Contract/types/config validation.
2. Policy resolver + auto profile detection + precedence.
3. Engine lifecycle integration (run start + checkpoint/resume state).
4. Env + checkpoint consumers.
5. Verify handler + handler registry wiring.
6. DOT/template/skill migration.
7. Integration and regression tests.

## Terminology Boundary

- `artifact_policy` in this plan means run-time hygiene policy for build/cache/temp byproducts (env shaping, checkpoint excludes, verification).
- `ArtifactStore` in `attractor-spec.md` §5.5 remains unchanged and still means named storage for large stage outputs.
- `checkpoint.Extra` usage in this plan is a Kilroy implementation extension through `internal/attractor/runtime/checkpoint.go` forward-compat field. Core spec checkpoint fields remain unchanged; this plan adds extension data without changing spec-required checkpoint semantics.

## Chunk 1: Contracts and Resolution

### Task 1: Add Modular `artifact_policy` Run-Config Contract

**Files:**
- Modify: `internal/attractor/engine/config.go`
- Create: `internal/attractor/engine/artifact_policy.go`
- Create: `internal/attractor/engine/artifact_policy_test_helpers_test.go`
- Test: `internal/attractor/engine/config_runtime_policy_test.go`
- Test: `internal/attractor/engine/artifact_policy_test.go`

- [ ] **Step 1: Write failing config contract tests**

```go
func TestApplyConfigDefaults_ArtifactPolicy_Defaults(t *testing.T) {
    cfg := &RunConfigFile{}
    applyConfigDefaults(cfg)

    if cfg.ArtifactPolicy.Profiles.Mode != "auto" {
        t.Fatalf("profiles.mode: got %q want auto", cfg.ArtifactPolicy.Profiles.Mode)
    }
    if len(cfg.ArtifactPolicy.Checkpoint.ExcludeGlobs) == 0 {
        t.Fatal("expected non-empty artifact_policy.checkpoint.exclude_globs")
    }
    if len(cfg.ArtifactPolicy.Verify.DenyGlobs) == 0 {
        t.Fatal("expected non-empty artifact_policy.verify.deny_globs")
    }
}

func TestValidateConfig_ArtifactPolicy_InvalidMode(t *testing.T) {
    cfg := validMinimalRunConfigForTest()
    cfg.ArtifactPolicy.Profiles.Mode = "sometimes"
    if err := validateConfig(cfg); err == nil {
        t.Fatal("expected validation failure for artifact_policy.profiles.mode")
    }
}

func TestValidateConfig_ArtifactPolicy_RejectsLegacyCheckpointExcludeField(t *testing.T) {
    cfg := validMinimalRunConfigForTest()
    cfg.Git.CheckpointExcludeGlobs = []string{"**/legacy-cache/**"}
    if err := validateConfig(cfg); err == nil {
        t.Fatal("expected validation failure when git.checkpoint_exclude_globs is set (legacy field is forbidden)")
    }
}
```

Add missing shared test helpers used across tasks:

```go
// artifact_policy_test_helpers_test.go
func validMinimalRunConfigForTest() *RunConfigFile { /* fills required v1 fields */ }
func setupRepoWithFiles(t *testing.T, relPaths []string) string { /* init git repo + create files */ }
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/attractor/engine -run 'ArtifactPolicy|CheckpointExcludeGlobs' -count=1`
Expected: FAIL with unknown fields/missing defaults.

- [ ] **Step 3: Implement schema and defaults with split sub-policies**

```go
// artifact_policy.go

type ArtifactPolicyConfig struct {
    Profiles   ArtifactProfilesConfig   `json:"profiles,omitempty" yaml:"profiles,omitempty"`
    Env        ArtifactEnvConfig        `json:"env,omitempty" yaml:"env,omitempty"`
    Checkpoint ArtifactCheckpointConfig `json:"checkpoint,omitempty" yaml:"checkpoint,omitempty"`
    Verify     ArtifactVerifyConfig     `json:"verify,omitempty" yaml:"verify,omitempty"`
}

type ArtifactProfilesConfig struct {
    Mode     string   `json:"mode,omitempty" yaml:"mode,omitempty"` // auto|explicit|disabled
    Explicit []string `json:"explicit,omitempty" yaml:"explicit,omitempty"`
}

type ArtifactEnvConfig struct {
    ManagedRoots map[string]string            `json:"managed_roots,omitempty" yaml:"managed_roots,omitempty"`
    Overrides    map[string]map[string]string `json:"overrides,omitempty" yaml:"overrides,omitempty"` // profile -> env map
}

type ArtifactCheckpointConfig struct {
    ExcludeGlobs []string `json:"exclude_globs,omitempty" yaml:"exclude_globs,omitempty"`
}

type ArtifactVerifyConfig struct {
    DenyGlobs  []string `json:"deny_globs,omitempty" yaml:"deny_globs,omitempty"`
    AllowGlobs []string `json:"allow_globs,omitempty" yaml:"allow_globs,omitempty"`
}
```

- [ ] **Step 4: Remove old config default coupling and route defaults through new policy**

```go
func applyArtifactPolicyDefaults(cfg *RunConfigFile) {
    if strings.TrimSpace(cfg.ArtifactPolicy.Profiles.Mode) == "" {
        cfg.ArtifactPolicy.Profiles.Mode = "auto"
    }

    cfg.ArtifactPolicy.Checkpoint.ExcludeGlobs = trimNonEmpty(cfg.ArtifactPolicy.Checkpoint.ExcludeGlobs)
    if len(cfg.ArtifactPolicy.Checkpoint.ExcludeGlobs) == 0 {
        cfg.ArtifactPolicy.Checkpoint.ExcludeGlobs = []string{
            "**/.cargo-target*/**", "**/.cargo_target*/**", "**/.wasm-pack/**", "**/.tmpbuild/**",
            "**/node_modules/**", "**/dist/**", "**/build/**", "**/__pycache__/**",
        }
    }
    cfg.ArtifactPolicy.Verify.DenyGlobs = trimNonEmpty(cfg.ArtifactPolicy.Verify.DenyGlobs)
    if len(cfg.ArtifactPolicy.Verify.DenyGlobs) == 0 {
        cfg.ArtifactPolicy.Verify.DenyGlobs = append([]string{}, cfg.ArtifactPolicy.Checkpoint.ExcludeGlobs...)
    }
    cfg.ArtifactPolicy.Verify.AllowGlobs = trimNonEmpty(cfg.ArtifactPolicy.Verify.AllowGlobs)

    // Zero-compatibility stance: validation rejects legacy field usage instead
    // of silently merging or ignoring values.
}

func validateArtifactPolicyConfig(cfg *RunConfigFile) error {
    if len(trimNonEmpty(cfg.Git.CheckpointExcludeGlobs)) > 0 {
        return fmt.Errorf("git.checkpoint_exclude_globs is legacy and unsupported; use artifact_policy.checkpoint.exclude_globs")
    }
    return nil
}
```

- [ ] **Step 5: Run tests to verify pass**

Run: `go test ./internal/attractor/engine -run 'ArtifactPolicy|CheckpointExcludeGlobs' -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/attractor/engine/config.go \
  internal/attractor/engine/artifact_policy.go \
  internal/attractor/engine/artifact_policy_test_helpers_test.go \
  internal/attractor/engine/config_runtime_policy_test.go \
  internal/attractor/engine/artifact_policy_test.go
git commit -m "engine/config: add modular artifact_policy schema and defaults"
```

### Task 2: Implement Deterministic Profile Detection and Resolution Rules

**Files:**
- Create: `internal/attractor/engine/artifact_policy_profiles.go`
- Create: `internal/attractor/engine/artifact_policy_resolve.go`
- Test: `internal/attractor/engine/artifact_policy_resolve_test.go`

Built-in profile detection contract (must be implemented exactly):
- `rust` markers: `Cargo.toml`, `rust-toolchain.toml`, `rust-toolchain`
- `go` markers: `go.mod`, `go.work`
- `node` markers: `package.json`, `pnpm-lock.yaml`, `yarn.lock`, `package-lock.json`
- `python` markers: `pyproject.toml`, `requirements.txt`, `setup.py`
- `java` markers: `pom.xml`, `build.gradle`, `settings.gradle`
- detection scope: repository root + recursive scan to depth 6, deterministic lexical ordering
- no markers found: fallback profile `generic`
- `profiles.mode=explicit`: use only `profiles.explicit` after validation
- `profiles.mode=disabled`: disable profile-derived env defaults only; preserve explicit verify/checkpoint rules from config

Built-in profile env contract (default vars when not explicitly set in OS env):
- `rust`: `CARGO_HOME`, `RUSTUP_HOME`, `CARGO_TARGET_DIR`
- `go`: `GOPATH`, `GOMODCACHE` (derived from first GOPATH entry + `/pkg/mod`)
- `node`: `npm_config_cache`, `PNPM_HOME`
- `python`: `PIP_CACHE_DIR`
- `java`: `GRADLE_USER_HOME`
- `java` Maven rule: preserve existing `MAVEN_OPTS`; append `-Dmaven.repo.local=<managed_root>/m2` only when that key is not already present (never overwrite existing flags)

- [ ] **Step 1: Write failing resolver tests for auto/explicit behavior**

```go
func TestResolveArtifactPolicy_AutoDetectsMultipleProfiles(t *testing.T) {
    repo := setupRepoWithFiles(t, []string{"Cargo.toml", "package.json", "go.mod"})
    cfg := validMinimalRunConfigForTest()
    cfg.Repo.Path = repo
    cfg.ArtifactPolicy.Profiles.Mode = "auto"

    rp, err := ResolveArtifactPolicy(cfg, ResolveArtifactPolicyInput{LogsRoot: t.TempDir(), WorktreeDir: repo})
    if err != nil { t.Fatal(err) }

    want := []string{"go", "node", "rust"}
    if !reflect.DeepEqual(rp.ActiveProfiles, want) {
        t.Fatalf("profiles: got %v want %v", rp.ActiveProfiles, want)
    }
}

func TestResolveArtifactPolicy_DenyAllowPrecedence(t *testing.T) {
    rp := ResolvedArtifactPolicy{
        Verify: ResolvedArtifactVerify{
            DenyGlobs:  []string{"**/dist/**"},
            AllowGlobs: []string{"demo/rogue/dist/keep-me.txt"},
        },
    }
    v := evaluateArtifactPaths(rp, []string{"demo/rogue/dist/keep-me.txt", "web/dist/app.js"})
    if len(v.OffendingPaths) != 1 || v.OffendingPaths[0] != "web/dist/app.js" {
        t.Fatalf("unexpected offending paths: %v", v.OffendingPaths)
    }
}

func TestResolveArtifactPolicy_ExplicitModeUsesOnlyRequestedProfiles(t *testing.T) {
    repo := setupRepoWithFiles(t, []string{"Cargo.toml", "package.json", "go.mod"})
    cfg := validMinimalRunConfigForTest()
    cfg.Repo.Path = repo
    cfg.ArtifactPolicy.Profiles.Mode = "explicit"
    cfg.ArtifactPolicy.Profiles.Explicit = []string{"go"}
    rp, err := ResolveArtifactPolicy(cfg, ResolveArtifactPolicyInput{LogsRoot: t.TempDir(), WorktreeDir: repo})
    if err != nil { t.Fatal(err) }
    if !reflect.DeepEqual(rp.ActiveProfiles, []string{"go"}) {
        t.Fatalf("profiles: got %v want [go]", rp.ActiveProfiles)
    }
}

func TestResolveArtifactPolicy_DisabledModeKeepsVerifyRulesButDisablesProfileEnv(t *testing.T) {
    cfg := validMinimalRunConfigForTest()
    cfg.ArtifactPolicy.Profiles.Mode = "disabled"
    cfg.ArtifactPolicy.Verify.DenyGlobs = []string{"**/dist/**"}
    rp, err := ResolveArtifactPolicy(cfg, ResolveArtifactPolicyInput{LogsRoot: t.TempDir(), WorktreeDir: cfg.Repo.Path})
    if err != nil { t.Fatal(err) }
    if len(rp.Env.Vars) != 0 {
        t.Fatalf("expected no profile-derived env vars in disabled mode, got %v", rp.Env.Vars)
    }
    if !reflect.DeepEqual(rp.Verify.DenyGlobs, []string{"**/dist/**"}) {
        t.Fatalf("verify deny globs not preserved in disabled mode: %v", rp.Verify.DenyGlobs)
    }
}

func TestResolveArtifactPolicy_ResolveRootFallsBackToRepoPathWhenWorktreeMissing(t *testing.T) {
    repo := setupRepoWithFiles(t, []string{"go.mod"})
    cfg := validMinimalRunConfigForTest()
    cfg.Repo.Path = repo
    rp, err := ResolveArtifactPolicy(cfg, ResolveArtifactPolicyInput{
        LogsRoot:   t.TempDir(),
        WorktreeDir: filepath.Join(t.TempDir(), "missing-worktree"),
    })
    if err != nil { t.Fatal(err) }
    if !reflect.DeepEqual(rp.ActiveProfiles, []string{"go"}) {
        t.Fatalf("profiles: got %v want [go]", rp.ActiveProfiles)
    }
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/attractor/engine -run 'ResolveArtifactPolicy|DenyAllowPrecedence' -count=1`
Expected: FAIL because resolver/detector does not exist.

- [ ] **Step 3: Implement resolver algorithm and precedence contract**

```go
type ResolvedArtifactPolicy struct {
    ActiveProfiles []string
    Env            ResolvedArtifactEnv
    ManagedRoots   map[string]string
    Checkpoint     ResolvedArtifactCheckpoint
    Verify         ResolvedArtifactVerify
}

type ResolvedArtifactEnv struct {
    Vars map[string]string
}

type ResolvedArtifactCheckpoint struct {
    ExcludeGlobs []string
}

type ResolvedArtifactVerify struct {
    DenyGlobs  []string
    AllowGlobs []string
}

// Resolution precedence:
// 1) explicit OS env value
// 2) profile override in artifact_policy.env.overrides
// 3) managed root default derived under {logs_root}/policy-managed-roots
// 4) built-in per-profile defaults (cargo/go/python/node/java cache roots)
//
// Note: `github.com/bmatcuk/doublestar/v4` is already in this repo's module graph,
// so no new dependency bootstrap step is required for glob evaluation.
//
// Managed root path rule:
// - absolute value => use as-is
// - relative value => resolve under {logs_root}/policy-managed-roots/{value}

func ResolveArtifactPolicy(cfg *RunConfigFile, in ResolveArtifactPolicyInput) (ResolvedArtifactPolicy, error) {
    resolveRoot := strings.TrimSpace(in.WorktreeDir)
    if resolveRoot != "" {
        if st, err := os.Stat(resolveRoot); err != nil || !st.IsDir() {
            resolveRoot = ""
        }
    }
    if resolveRoot == "" {
        candidate := strings.TrimSpace(cfg.Repo.Path)
        if st, err := os.Stat(candidate); err == nil && st.IsDir() {
            resolveRoot = candidate
        }
    }
    if resolveRoot == "" {
        return ResolvedArtifactPolicy{}, fmt.Errorf("artifact policy resolve root missing: neither worktree nor repo path is a readable directory")
    }

    verify := normalizeVerifyRules(cfg.ArtifactPolicy.Verify)
    checkpoint := trimNonEmpty(cfg.ArtifactPolicy.Checkpoint.ExcludeGlobs)

    if strings.EqualFold(strings.TrimSpace(cfg.ArtifactPolicy.Profiles.Mode), "disabled") {
        return ResolvedArtifactPolicy{
            ActiveProfiles: nil,
            Env:            ResolvedArtifactEnv{Vars: map[string]string{}},
            ManagedRoots:   map[string]string{},
            Checkpoint:     ResolvedArtifactCheckpoint{ExcludeGlobs: checkpoint},
            Verify:         verify, // keep explicit verify rules; only profile-derived env is disabled
        }, nil
    }

    profiles, err := resolveProfiles(cfg.ArtifactPolicy.Profiles, resolveRoot)
    if err != nil { return ResolvedArtifactPolicy{}, err }

    env, managedRoots := resolveEnvPolicy(profiles, cfg.ArtifactPolicy.Env, in.LogsRoot, in.WorktreeDir)

    return ResolvedArtifactPolicy{
        ActiveProfiles: profiles,
        Env:            env,
        ManagedRoots:   managedRoots,
        Checkpoint:     ResolvedArtifactCheckpoint{ExcludeGlobs: checkpoint},
        Verify:         verify,
    }, nil
}

func (rp ResolvedArtifactPolicy) Hash() (string, error) {
    b, err := json.Marshal(rp) // deterministic struct encoding for checkpoints
    if err != nil {
        return "", err
    }
    sum := sha256.Sum256(b)
    return fmt.Sprintf("%x", sum[:]), nil
}

func decodeResolvedArtifactPolicy(raw any) (ResolvedArtifactPolicy, error) {
    b, err := json.Marshal(raw) // map[string]any -> JSON
    if err != nil {
        return ResolvedArtifactPolicy{}, err
    }
    var rp ResolvedArtifactPolicy
    if err := json.Unmarshal(b, &rp); err != nil {
        return ResolvedArtifactPolicy{}, err
    }
    return rp, nil
}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/attractor/engine -run 'ResolveArtifactPolicy|DenyAllowPrecedence' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/attractor/engine/artifact_policy_profiles.go \
  internal/attractor/engine/artifact_policy_resolve.go \
  internal/attractor/engine/artifact_policy_resolve_test.go
git commit -m "engine/artifact-policy: add deterministic profile detection and resolver precedence"
```

### Task 3: Resolve-Once Semantics with Checkpoint/Resume Persistence

**Files:**
- Modify: `internal/attractor/engine/engine.go`
- Modify: `internal/attractor/engine/run_with_config.go`
- Modify: `internal/attractor/engine/resume.go`
- Create: `internal/attractor/engine/artifact_policy_resume_test.go`

- [ ] **Step 1: Write failing resume determinism tests**

```go
func TestResume_UsesCheckpointedResolvedArtifactPolicy(t *testing.T) {
    // 1) run once with auto profiles
    // 2) mutate repo files to change detectable language mix
    // 3) resume from checkpoint
    // 4) assert resumed engine keeps checkpointed active profiles, not new detection
}

func TestResume_FailsWhenSnapshottedRunConfigIsInvalid(t *testing.T) {
    // corrupt logs_root/run_config.json and assert Resume returns error
    // instead of silently falling back to simulated backend.
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/attractor/engine -run 'Resume_UsesCheckpointedResolvedArtifactPolicy' -count=1`
Expected: FAIL because checkpoint does not carry resolved artifact policy.

- [ ] **Step 3: Add engine fields and checkpoint serialization**

```go
type Engine struct {
    // ... existing fields ...
    ArtifactPolicy ResolvedArtifactPolicy
}

// checkpoint save path (engine.go)
// Kilroy extension note: cp.Extra is the forward-compat extension field in
// runtime.Checkpoint; base checkpoint semantics are unchanged.
cp.Extra["artifact_policy_resolved"] = e.ArtifactPolicy
if h, err := e.ArtifactPolicy.Hash(); err == nil {
    cp.Extra["artifact_policy_resolved_sha256"] = h
} else {
    return "", fmt.Errorf("checkpoint: hash resolved artifact policy: %w", err)
}
```

- [ ] **Step 4: Restore resolved policy on resume with deterministic fallback**

```go
// resume.go
if _, statErr := os.Stat(cfgPath); statErr == nil {
    loaded, loadErr := LoadRunConfigFile(cfgPath)
    if loadErr != nil {
        return nil, fmt.Errorf("resume: load run config snapshot: %w", loadErr)
    }
    cfg = loaded
}

if raw := cp.Extra["artifact_policy_resolved"]; raw != nil {
    restored, err := decodeResolvedArtifactPolicy(raw)
    if err != nil {
        return nil, fmt.Errorf("resume: decode artifact policy snapshot: %w", err)
    }
    if want := strings.TrimSpace(fmt.Sprint(cp.Extra["artifact_policy_resolved_sha256"])); want != "" {
        got, hashErr := restored.Hash()
        if hashErr != nil {
            return nil, fmt.Errorf("resume: hash restored artifact policy: %w", hashErr)
        }
        if got != want {
            return nil, fmt.Errorf("resume: artifact policy snapshot hash mismatch")
        }
    }
    eng.ArtifactPolicy = restored
} else {
    // fallback for old checkpoints only
    rp, err := ResolveArtifactPolicy(cfg, ResolveArtifactPolicyInput{LogsRoot: logsRoot, WorktreeDir: eng.WorktreeDir})
    if err != nil { return nil, err }
    eng.ArtifactPolicy = rp
}
```

- [ ] **Step 5: Run tests to verify pass**

Run: `go test ./internal/attractor/engine -run 'Resume_UsesCheckpointedResolvedArtifactPolicy|resume|checkpoint' -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/attractor/engine/engine.go \
  internal/attractor/engine/run_with_config.go \
  internal/attractor/engine/resume.go \
  internal/attractor/engine/artifact_policy_resume_test.go
git commit -m "engine/resume: persist and restore resolved artifact policy for deterministic resume"
```

## Chunk 2: Engine Consumers and Verifier

### Task 4: Replace Shared Env Hardcoding with Policy-Driven Env Resolution

**Files:**
- Modify: `internal/attractor/engine/node_env.go`
- Test: `internal/attractor/engine/node_env_test.go`
- Test: `internal/attractor/engine/api_env_parity_test.go`
- Modify: `internal/attractor/engine/handlers.go`
- Modify: `internal/attractor/engine/codergen_router.go`
- Modify: `internal/attractor/engine/tool_hooks.go`

- [ ] **Step 1: Write failing parity tests for profile-driven env values**

```go
func TestBuildBaseNodeEnv_UsesResolvedArtifactPolicyEnv(t *testing.T) {
    rp := ResolvedArtifactPolicy{
        Env: ResolvedArtifactEnv{Vars: map[string]string{"CARGO_TARGET_DIR": "/tmp/managed/cargo-target"}},
    }
    env := buildBaseNodeEnv(t.TempDir(), rp)
    if got := envLookup(env, "CARGO_TARGET_DIR"); got != "/tmp/managed/cargo-target" {
        t.Fatalf("CARGO_TARGET_DIR: got %q", got)
    }
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/attractor/engine -run 'BuildBaseNodeEnv_UsesResolvedArtifactPolicyEnv|APIAgentLoopOverrides' -count=1`
Expected: FAIL due to old signature/behavior.

- [ ] **Step 3: Implement policy-fed env construction while preserving current invariants**

```go
func buildBaseNodeEnv(worktreeDir string, rp ResolvedArtifactPolicy) []string {
    // Keep existing toolchain pinning behavior (CARGO_HOME, RUSTUP_HOME,
    // GOPATH, GOMODCACHE, CARGO_TARGET_DIR) and layer policy env on top.
    // `buildLegacyPinnedToolchainEnv` is the current pre-refactor function body
    // moved verbatim to avoid behavior drift.
    // Policy merge order:
    //   1) explicit OS env
    //   2) resolved policy vars
    //   3) existing toolchain fallback derivation
    env := buildLegacyPinnedToolchainEnv(worktreeDir)
    env = mergeEnvWithOverrides(env, rp.Env.Vars)
    return stripEnvKey(env, "CLAUDECODE")
}
```

- [ ] **Step 4: Thread resolved policy through every caller of `buildBaseNodeEnv`**

```go
// handlers.go (ToolHandler)
cmd.Env = buildBaseNodeEnv(execCtx.WorktreeDir, execCtx.Engine.ArtifactPolicy)

// codergen_router.go
baseEnv := buildBaseNodeEnv(execCtx.WorktreeDir, execCtx.Engine.ArtifactPolicy)

// tool_hooks.go
preHookEnv := buildBaseNodeEnv(execCtx.WorktreeDir, execCtx.Engine.ArtifactPolicy)  // runPreToolHook
postHookEnv := buildBaseNodeEnv(execCtx.WorktreeDir, execCtx.Engine.ArtifactPolicy) // executeToolHookForEvent

// node_env.go internal callers
func buildAgentLoopOverrides(worktreeDir string, rp ResolvedArtifactPolicy, contractEnv map[string]string) map[string]string {
    base := buildBaseNodeEnv(worktreeDir, rp)
    // ... existing key extraction logic ...
}
```

- [ ] **Step 5: Run tests to verify pass**

Run: `go test ./internal/attractor/engine -run 'BuildBaseNodeEnv_|APIAgentLoopOverrides' -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/attractor/engine/node_env.go \
  internal/attractor/engine/node_env_test.go \
  internal/attractor/engine/api_env_parity_test.go \
  internal/attractor/engine/handlers.go \
  internal/attractor/engine/codergen_router.go \
  internal/attractor/engine/tool_hooks.go
git commit -m "engine/env: drive base node env from resolved artifact policy"
```

### Task 5: Make Checkpoint Excludes Policy-Driven

**Files:**
- Modify: `internal/attractor/engine/engine.go`
- Test: `internal/attractor/engine/checkpoint_exclude_test.go`

- [ ] **Step 1: Write failing checkpoint exclude test against resolved policy**

```go
func TestCheckpoint_UsesResolvedArtifactPolicyExcludes(t *testing.T) {
    // configure artifact_policy.checkpoint.exclude_globs with a temp artifact dir
    // verify commit does not include excluded files
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/attractor/engine -run 'Checkpoint_UsesResolvedArtifactPolicyExcludes' -count=1`
Expected: FAIL because engine still reads `git.checkpoint_exclude_globs`.

- [ ] **Step 3: Switch checkpoint excludes to resolved policy source only**

```go
func (e *Engine) checkpointExcludeGlobs() []string {
    return append([]string{}, e.ArtifactPolicy.Checkpoint.ExcludeGlobs...)
}
```

- [ ] **Step 4: Run test to verify pass**

Run: `go test ./internal/attractor/engine -run 'Checkpoint_UsesResolvedArtifactPolicyExcludes|checkpoint_exclude' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/attractor/engine/engine.go internal/attractor/engine/checkpoint_exclude_test.go
git commit -m "engine/checkpoint: source commit excludes from resolved artifact policy"
```

### Task 6: Replace Ad Hoc Artifact Regex Commands with `verify.artifacts` Handler

**Files:**
- Create: `internal/attractor/engine/artifact_verify_handler.go`
- Modify: `internal/attractor/engine/handlers.go`
- Create: `internal/attractor/engine/artifact_verify_handler_test.go`

- [ ] **Step 1: Write failing handler tests for exact offending paths and rule metadata**

```go
func TestArtifactVerifyHandler_FailsWithPathPreciseDetails(t *testing.T) {
    // setup git worktree with forbidden path
    out, err := (&ArtifactVerifyHandler{}).Execute(ctx, exec, node)
    if err != nil { t.Fatal(err) }
    if out.Status != runtime.StatusFail { t.Fatalf("status=%s", out.Status) }
    if out.FailureReason != "artifact_policy_violation" { t.Fatalf("reason=%q", out.FailureReason) }

    details := out.Details.(map[string]any)
    paths := details["offending_paths"].([]string)
    if len(paths) == 0 { t.Fatal("expected offending_paths") }
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/attractor/engine -run 'ArtifactVerifyHandler' -count=1`
Expected: FAIL because handler type does not exist.

- [ ] **Step 3: Implement handler with spec-consistent Outcome fields**

```go
func (h *ArtifactVerifyHandler) Execute(ctx context.Context, execCtx *Execution, node *model.Node) (runtime.Outcome, error) {
    paths, err := collectChangedPaths(execCtx.WorktreeDir) // git status --porcelain=v1 -z --untracked-files=all
    if err != nil {
        return runtime.Outcome{Status: runtime.StatusRetry, FailureReason: err.Error(), Meta: map[string]any{"failure_class": failureClassTransientInfra}}, nil
    }

    verdict := evaluateArtifactPaths(execCtx.Engine.ArtifactPolicy, paths)
    if verdict.OK {
        return runtime.Outcome{Status: runtime.StatusSuccess, Notes: "artifact policy check passed"}, nil
    }

    return runtime.Outcome{
        Status:        runtime.StatusFail,
        FailureReason: "artifact_policy_violation",
        Details: map[string]any{
            "summary":                    verdict.Summary,
            "offending_paths":            verdict.OffendingPaths,
            "matched_deny_rules":         verdict.MatchedDenyRules,
            "allow_exceptions_evaluated": verdict.AllowRulesEvaluated,
            "diff_source":                verdict.DiffSource,
        },
        Meta: map[string]any{
            "failure_class":     failureClassDeterministic,
            "failure_signature": verdict.Signature,
        },
        ContextUpdates: map[string]any{"failure_class": failureClassDeterministic},
    }, nil
}
```

`collectChangedPaths` contract for this plan:
- It evaluates repository state *after* the prior node checkpoint commit.
- Therefore it is expected to surface paths still present because they are excluded from checkpoint staging or remain untracked.
- This keeps behavior consistent with the current `verify_artifacts` gate intent while moving logic into a typed handler.
- It reads `git status --porcelain=v1 -z --untracked-files=all` and parses NUL-delimited records (no whitespace/escape ambiguity).
- Rename/copy records must include destination paths in the evaluated set (and may include source paths for diagnostics).
- Non-ignored paths only; ignored files are intentionally out-of-scope.
- To avoid persistent false positives, profile env rules must direct build/cache roots outside the worktree (or projects must explicitly track/ignore them as policy intends).

- [ ] **Step 4: Register handler type and keep routing contract explicit**

```go
func NewDefaultRegistry() *HandlerRegistry {
    reg := &HandlerRegistry{handlers: map[string]Handler{}}
    // ... existing registrations ...
    reg.Register("verify.artifacts", &ArtifactVerifyHandler{})
    return reg
}
```

- [ ] **Step 5: Run tests to verify pass**

Run: `go test ./internal/attractor/engine -run 'ArtifactVerifyHandler' -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/attractor/engine/artifact_verify_handler.go \
  internal/attractor/engine/handlers.go \
  internal/attractor/engine/artifact_verify_handler_test.go
git commit -m "engine/handlers: add policy-driven verify.artifacts handler with path-precise failure details"
```

## Chunk 3: Authoring Surface, Migration, and End-to-End Verification

### Task 7: Update Canonical DOT/Skill/Run YAML to Use Policy-Driven Verification

**Files:**
- Modify: `skills/english-to-dotfile/reference_template.dot`
- Modify: `demo/rogue/rogue.dot`
- Modify: `skills/english-to-dotfile/SKILL.md`
- Modify: `skills/english-to-dotfile/reference_run_template.yaml`
- Test: `internal/attractor/validate/reference_template_guardrail_test.go`

- [ ] **Step 1: Write failing template guardrail test for handler type requirement**

```go
func TestReferenceTemplate_VerifyArtifactsUsesBuiltInHandler(t *testing.T) {
    src := loadReferenceTemplate(t)
    g, err := dot.Parse(src)
    if err != nil { t.Fatalf("parse reference template: %v", err) }
    n := g.Nodes["verify_artifacts"]
    if n == nil { t.Fatal("missing verify_artifacts node") }
    if got := n.Attr("type", ""); got != "verify.artifacts" {
        t.Fatalf("verify_artifacts.type: got %q want verify.artifacts", got)
    }
}
```

- [ ] **Step 2: Run test to verify failure**

Run: `go test ./internal/attractor/validate -run 'ReferenceTemplate_VerifyArtifactsUsesBuiltInHandler' -count=1`
Expected: FAIL.

- [ ] **Step 3: Update template and demo graph nodes**

```dot
verify_artifacts [
    shape=parallelogram,
    type="verify.artifacts",
    max_retries=1
]
```

Use `max_retries=1` specifically to permit one retry when the handler returns `status=retry` for transient `git status`/filesystem infra failures.

- [ ] **Step 4: Update `@english-to-dotfile` instructions and run template YAML**

```yaml
artifact_policy:
  profiles:
    mode: auto
  env:
    managed_roots:
      tool_cache_root: "tool-cache"
  checkpoint:
    exclude_globs:
      - "**/.cargo-target*/**"
      - "**/node_modules/**"
  verify:
    deny_globs:
      - "**/.cargo-target*/**"
      - "**/node_modules/**"
    allow_globs: []
```

Resolver rule for this field:
- `artifact_policy.env.managed_roots.*` absolute values are used as-is.
- Relative values are rooted at `{logs_root}/policy-managed-roots/` during policy resolution.

- [ ] **Step 5: Run tests to verify pass**

Run: `go test ./internal/attractor/validate -run 'ReferenceTemplate_' -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add skills/english-to-dotfile/reference_template.dot \
  demo/rogue/rogue.dot \
  skills/english-to-dotfile/SKILL.md \
  skills/english-to-dotfile/reference_run_template.yaml \
  internal/attractor/validate/reference_template_guardrail_test.go
git commit -m "skills/template: migrate verify_artifacts to built-in verify.artifacts handler and artifact_policy yaml"
```

### Task 8: Add Integration and Regression Coverage for the Failure Class

**Files:**
- Create: `internal/attractor/engine/artifact_policy_integration_test.go`
- Modify: `internal/attractor/engine/run_with_config_integration_test.go`
- Modify: `internal/attractor/engine/resume_test.go`
- Modify: `internal/attractor/engine/archive.go`

- [ ] **Step 1: Write failing integration tests covering core gaps**

```go
func TestRun_ArtifactVerifyHandler_EmitsOffendingPaths(t *testing.T) {}
func TestRun_ArtifactVerifyHandler_AllowGlobCarveOut(t *testing.T) {}
func TestRun_ArtifactPolicy_AutoProfiles_MultiLanguageRepo(t *testing.T) {}
func TestResume_ArtifactPolicySnapshotStableAcrossRepoMutation(t *testing.T) {}
func TestRunArchive_ExcludesManagedRootsCaches(t *testing.T) {}
```

- [ ] **Step 2: Run targeted tests to verify failure**

Run: `go test ./internal/attractor/engine -run 'ArtifactVerifyHandler|ArtifactPolicy|Resume_ArtifactPolicy' -count=1`
Expected: FAIL.

- [ ] **Step 3: Implement minimum code updates required by failing tests**

```go
// Example assertion target for failure payload:
if out.FailureReason != "artifact_policy_violation" {
    t.Fatalf("failure_reason: got %q want artifact_policy_violation", out.FailureReason)
}
details, ok := out.Details.(map[string]any)
if !ok {
    t.Fatalf("details type: got %T want map[string]any", out.Details)
}
if _, ok := details["offending_paths"]; !ok {
    t.Fatalf("missing details.offending_paths: %v", details)
}
if got := fmt.Sprint(out.ContextUpdates["failure_class"]); got != failureClassDeterministic {
    t.Fatalf("failure_class: got %q want %q", got, failureClassDeterministic)
}

// archive.go exclusion rule:
// skip logs_root/policy-managed-roots/** so policy-managed caches never bloat run.tgz.
```

- [ ] **Step 4: Run targeted tests to verify pass**

Run: `go test ./internal/attractor/engine -run 'ArtifactVerifyHandler|ArtifactPolicy|Resume_ArtifactPolicy' -count=1`
Expected: PASS.

- [ ] **Step 5: Run full test suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/attractor/engine/artifact_policy_integration_test.go \
  internal/attractor/engine/run_with_config_integration_test.go \
  internal/attractor/engine/resume_test.go \
  internal/attractor/engine/archive.go
git commit -m "engine/tests: add artifact policy integration and resume determinism regressions"
```

### Task 9: Update Proposal Doc to Reflect Final Spec-Aligned Design Decisions

**Files:**
- Modify: `docs/plans/2026-02-23-unified-artifact-policy-refactor-proposal.md`

- [ ] **Step 1: Rewrite proposal sections that were flagged by fresh-eyes review**

```markdown
- Clarify `artifact_policy` is a Kilroy run-config extension, not an Attractor-spec primitive.
- Explicitly state verifier integration point: `verify.artifacts` handler execution.
- Define deny/allow precedence and resume snapshot behavior.
- Add migration section for reference template + skill + demo graphs.
```

- [ ] **Step 2: Verify proposal references exact implemented contracts**

Run: `rg -n "artifact_policy|verify\.artifacts|checkpoint|resume|deny|allow" docs/plans/2026-02-23-unified-artifact-policy-refactor-proposal.md`
Expected: all decisions present with no TODO markers.

- [ ] **Step 3: Commit**

```bash
git add docs/plans/2026-02-23-unified-artifact-policy-refactor-proposal.md
git commit -m "docs(plan): align artifact policy proposal with Attractor-spec-consistent implementation contract"
```

## Final Verification Checklist

- [ ] `go test ./internal/attractor/engine -count=1`
- [ ] `go test ./internal/attractor/validate -count=1`
- [ ] `go test ./...`
- [ ] `./scripts/e2e.sh`
- [ ] `./scripts/e2e-guardrail-matrix.sh`

## Execution Notes

- Keep commits narrow and frequent (one commit per task minimum).
- Use `@english-to-dotfile` when touching generation behavior, so DOT+YAML intent routing remains ergonomic.
- Use `@investigating-kilroy-runs` to validate real run artifacts for this failure class after implementation.
- If any task reveals an Attractor-spec contradiction, stop and escalate before coding; do not silently reinterpret the spec.
