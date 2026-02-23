# Unified Artifact Policy Part 2 (Verification, Ergonomics, Hardening) Implementation Plan

> **For Claude:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend Part 1 with policy-driven artifact verification, improved diagnostics, and authoring ergonomics while keeping core Attractor behavior unchanged.

**Architecture:** Build on the Part 1 resolved policy model by adding a `verify` sub-policy and a typed `verify.artifacts` handler. Keep deterministic execution semantics by emitting standard `runtime.Outcome` fields and routing through existing engine retry/failure-class mechanics. Add optional auto profile detection and skill/template updates only after verification is stable.

**Tech Stack:** Go (`internal/attractor/engine`, `internal/attractor/validate`), DOT templates, YAML run templates, `doublestar` glob matching, git porcelain parsing.

---

## Scope Check

This is intentionally a second plan because it touches additional subsystems (handler registry, DOT generation guidance, template guardrails, validation hardening). It depends on Part 1 being merged first.

## Dependencies

- Requires: `docs/superpowers/plans/2026-02-23-unified-artifact-policy-part-1-core.md` complete and merged.
- Requires resolved policy runtime field and checkpoint snapshot restore from Part 1.

## Guardrails

- Do not modify `docs/strongdm/attractor/attractor-spec.md`.
- Keep deterministic gate semantics (`max_retries=0` for verification gate defaults).
- Do not reintroduce language-specific hacks in shared engine code.
- Verification rules must come from resolved run policy, not ad hoc DOT regex commands.

## File Structure Map

- Modify: `internal/attractor/engine/config.go`  
  Responsibility: add `artifact_policy.verify` and optional profile mode schema.
- Create: `internal/attractor/engine/artifact_verify_handler.go`  
  Responsibility: typed policy-driven verifier with deterministic diagnostics.
- Create: `internal/attractor/engine/artifact_verify_handler_test.go`  
  Responsibility: verifier pass/fail/retry behavior tests.
- Modify: `internal/attractor/engine/handlers.go`  
  Responsibility: register `verify.artifacts` handler.
- Modify: `internal/attractor/engine/codergen_router.go`  
  Responsibility: ensure typed handler resolution remains deterministic.
- Modify: `internal/attractor/engine/artifact_policy_resolve.go`  
  Responsibility: optional `profiles.mode` resolution and verify rule normalization.
- Modify: `internal/attractor/engine/artifact_policy_resolve_test.go`  
  Responsibility: auto/explicit mode tests and verify precedence tests.
- Modify: `internal/attractor/engine/archive.go`  
  Responsibility: ensure policy-managed roots are excluded from run archive payloads.
- Modify: `skills/english-to-dotfile/SKILL.md`  
  Responsibility: declarative guidance to generate consistent DOT + YAML artifact policy.
- Modify: `skills/english-to-dotfile/reference_template.dot`  
  Responsibility: canonical `verify.artifacts` usage instead of ad hoc regex tool gate.
- Modify: `skills/english-to-dotfile/reference_run_template.yaml`  
  Responsibility: include `artifact_policy.verify` and optional profile mode examples.
- Modify: `internal/attractor/validate/reference_template_guardrail_test.go`  
  Responsibility: enforce template invariants around `verify.artifacts` usage.

## Chunk 1: Verification Contract and Handler

### Task 1: Add `artifact_policy.verify` Schema and Validation

**Files:**
- Modify: `internal/attractor/engine/config.go`
- Test: `internal/attractor/engine/config_runtime_policy_test.go`

- [ ] **Step 1: Write failing config tests for verify policy**

```go
func TestValidateConfig_ArtifactPolicyVerifyRejectsEmptyDenyAndAllow(t *testing.T) {
	cfg := validMinimalRunConfigForTest()
	cfg.ArtifactPolicy.Verify.DenyGlobs = nil
	cfg.ArtifactPolicy.Verify.AllowGlobs = nil
	if err := validateConfig(cfg); err == nil {
		t.Fatal("expected validation error when verify rules are empty")
	}
}

func TestValidateConfig_ArtifactPolicyVerifyAcceptsDenyOnly(t *testing.T) {
	cfg := validMinimalRunConfigForTest()
	cfg.ArtifactPolicy.Verify.DenyGlobs = []string{"**/node_modules/**"}
	if err := validateConfig(cfg); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/attractor/engine -run 'ArtifactPolicyVerify' -count=1`  
Expected: FAIL with missing verify schema/validation.

- [ ] **Step 3: Implement verify sub-policy in config contract**

```go
type ArtifactPolicyVerify struct {
	DenyGlobs  []string `json:"deny_globs,omitempty" yaml:"deny_globs,omitempty"`
	AllowGlobs []string `json:"allow_globs,omitempty" yaml:"allow_globs,omitempty"`
}

// In ArtifactPolicyConfig:
Verify ArtifactPolicyVerify `json:"verify,omitempty" yaml:"verify,omitempty"`
```

Validation rules:
- Trim empties and normalize path separators.
- Deny rules must be non-empty.
- Allow rules may be empty.

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/attractor/engine -run 'ArtifactPolicyVerify' -count=1`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/attractor/engine/config.go internal/attractor/engine/config_runtime_policy_test.go
git commit -m "engine/config: add artifact_policy.verify schema and validation"
```

### Task 2: Implement `verify.artifacts` Handler

**Files:**
- Create: `internal/attractor/engine/artifact_verify_handler.go`
- Create: `internal/attractor/engine/artifact_verify_handler_test.go`
- Modify: `internal/attractor/engine/handlers.go`
- Modify: `internal/attractor/engine/codergen_router.go`

- [ ] **Step 1: Write failing handler tests**

```go
func TestArtifactVerifyHandler_PassesWhenNoDeniedPaths(t *testing.T) {}
func TestArtifactVerifyHandler_FailsWithOffendingPathsAndRules(t *testing.T) {}
func TestArtifactVerifyHandler_ReturnsRetryOnGitStatusError(t *testing.T) {}
```

Detailed expectations:
- PASS: `runtime.StatusSuccess`, no failure metadata.
- FAIL: `runtime.StatusFail`, `FailureReason="artifact_policy_violation"`, `Details["offending_paths"]` contains exact paths.
- RETRY: `runtime.StatusRetry` with `Meta["failure_class"]="transient_infra"`.

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/attractor/engine -run 'ArtifactVerifyHandler' -count=1`  
Expected: FAIL because handler is not implemented/registered.

- [ ] **Step 3: Implement handler and registration**

Implementation requirements:
- Read changed paths with `git status --porcelain=v1 -z --untracked-files=all`.
- Parse NUL records robustly (including rename/copy formats).
- Evaluate deny first, then allow carve-outs.
- Emit deterministic signature and rule-match details for diagnostics.
- Register under handler type `verify.artifacts`.

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/attractor/engine -run 'ArtifactVerifyHandler' -count=1`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/attractor/engine/artifact_verify_handler.go \
  internal/attractor/engine/artifact_verify_handler_test.go \
  internal/attractor/engine/handlers.go \
  internal/attractor/engine/codergen_router.go
git commit -m "engine/handlers: add policy-driven verify.artifacts handler"
```

## Chunk 2: Optional Profile Auto-Detection

### Task 3: Add `profiles.mode` and Marker-Based Detection

**Files:**
- Modify: `internal/attractor/engine/config.go`
- Modify: `internal/attractor/engine/artifact_policy_resolve.go`
- Modify: `internal/attractor/engine/artifact_policy_resolve_test.go`

- [ ] **Step 1: Write failing tests for explicit/auto/disabled modes**

```go
func TestResolveArtifactPolicy_ExplicitModeUsesOnlyConfiguredProfiles(t *testing.T) {}
func TestResolveArtifactPolicy_AutoModeDetectsProfilesFromRepoMarkers(t *testing.T) {}
func TestResolveArtifactPolicy_DisabledModeReturnsEmptyProfileEnvDefaults(t *testing.T) {}
```

Marker examples:
- Rust: `Cargo.toml`, `Cargo.lock`
- Go: `go.mod`
- Node: `package.json`, lock files
- Python: `pyproject.toml`, `requirements.txt`
- Java: `pom.xml`, `build.gradle`

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/attractor/engine -run 'ResolveArtifactPolicy_(ExplicitMode|AutoMode|DisabledMode)' -count=1`  
Expected: FAIL until mode-aware resolution exists.

- [ ] **Step 3: Implement mode-aware resolver behavior**

Mode contract:
- `explicit`: use only `profiles.explicit` list.
- `auto`: detect from repo markers with deterministic lexical ordering.
- `disabled`: apply no profile defaults; still honor explicit run-config env/checkpoint/verify lists.

Detection performance guard:
- Skip heavy directories: `.git`, `node_modules`, `target`, `.cargo-target*`.

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/attractor/engine -run 'ResolveArtifactPolicy_(ExplicitMode|AutoMode|DisabledMode)' -count=1`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/attractor/engine/config.go \
  internal/attractor/engine/artifact_policy_resolve.go \
  internal/attractor/engine/artifact_policy_resolve_test.go
git commit -m "engine/policy: add optional profile mode and auto detection"
```

## Chunk 3: Authoring Ergonomics and Template Guardrails

### Task 4: Align DOT/YAML Authoring with Typed Verification

**Files:**
- Modify: `skills/english-to-dotfile/SKILL.md`
- Modify: `skills/english-to-dotfile/reference_template.dot`
- Modify: `skills/english-to-dotfile/reference_run_template.yaml`
- Modify: `internal/attractor/validate/reference_template_guardrail_test.go`

- [ ] **Step 1: Write failing template guardrail tests**

```go
func TestReferenceTemplate_UsesVerifyArtifactsHandlerType(t *testing.T) {}
func TestReferenceTemplate_DoesNotUseAdHocArtifactRegexToolCommand(t *testing.T) {}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/attractor/validate -run 'ReferenceTemplate_.*Artifact' -count=1`  
Expected: FAIL with current template guidance.

- [ ] **Step 3: Update templates and skill instructions**

Required outcomes:
- DOT template uses `type="verify.artifacts"` node.
- Run YAML template includes `artifact_policy.verify` examples.
- Skill guidance explicitly states: run config carries policy; DOT gate invokes typed verify handler.

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/attractor/validate -run 'ReferenceTemplate_.*Artifact' -count=1`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add skills/english-to-dotfile/SKILL.md \
  skills/english-to-dotfile/reference_template.dot \
  skills/english-to-dotfile/reference_run_template.yaml \
  internal/attractor/validate/reference_template_guardrail_test.go
git commit -m "skills/template: align artifact policy authoring with verify.artifacts handler"
```

## Chunk 4: Hardening and End-to-End Regression

### Task 5: Add Key Validation, Archive Hygiene, and Cross-Stack Regression Tests

**Files:**
- Modify: `internal/attractor/engine/config.go`
- Modify: `internal/attractor/engine/archive.go`
- Modify: `internal/attractor/engine/artifact_policy_resolve_test.go`
- Modify: `internal/attractor/engine/run_with_config_integration_test.go`

- [ ] **Step 1: Write failing hardening tests**

```go
func TestValidateConfig_ArtifactPolicyManagedRootsRejectsUnknownKeys(t *testing.T) {}
func TestArchive_ExcludesPolicyManagedRoots(t *testing.T) {}
func TestRunWithConfig_ArtifactPolicyRegression_RustAndNode(t *testing.T) {}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/attractor/engine -run '(ManagedRootsRejectsUnknownKeys|ExcludesPolicyManagedRoots|ArtifactPolicyRegression_RustAndNode)' -count=1`  
Expected: FAIL.

- [ ] **Step 3: Implement hardening changes**

Hardening requirements:
- `artifact_policy.env.managed_roots` unknown keys fail validation.
- Archives exclude `logs_root/policy-managed-roots/**`.
- Add integration coverage proving policy behavior in at least Rust and Node sample repos.

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/attractor/engine -run '(ManagedRootsRejectsUnknownKeys|ExcludesPolicyManagedRoots|ArtifactPolicyRegression_RustAndNode)' -count=1`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/attractor/engine/config.go \
  internal/attractor/engine/archive.go \
  internal/attractor/engine/artifact_policy_resolve_test.go \
  internal/attractor/engine/run_with_config_integration_test.go
git commit -m "engine/hardening: validate managed roots and add artifact policy regressions"
```

## Part 2 Exit Criteria

- Artifact verification uses a typed handler driven by run policy.
- Failures show exact offending paths and matched deny/allow rules.
- Optional profile auto mode is deterministic and bounded.
- Authoring surfaces (skill + templates + guardrails) produce consistent DOT/YAML outputs.
- Archive hygiene avoids policy-managed cache bloat.

## Full Validation

- `go test ./internal/attractor/engine -count=1`
- `go test ./internal/attractor/validate -count=1`
- `go test ./...`

## Handoff

Plan path: `docs/superpowers/plans/2026-02-23-unified-artifact-policy-part-2-verify-ergonomics.md`  
Prerequisite plan: `docs/superpowers/plans/2026-02-23-unified-artifact-policy-part-1-core.md`  
Prerequisite spec reference: `docs/strongdm/attractor/attractor-spec.md`
