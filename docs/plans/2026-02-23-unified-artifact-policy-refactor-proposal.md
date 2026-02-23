# Unified Artifact Policy Refactor Proposal

Date: 2026-02-23
Status: Proposed
Owner: Attractor engine/runtime

## 1. Executive Summary

Replace fragmented, partly language-specific artifact handling with one engine-level, run-resolved `artifact_policy` contract.

Today artifact behavior is split across:
- environment shaping,
- checkpoint staging excludes,
- graph-embedded artifact checks.

This causes deterministic failure loops with weak diagnostics. The fix is to centralize policy resolution once per run and apply it uniformly everywhere artifact behavior matters.

This is an architectural correction, not a compatibility shim. We are optimizing for idiomatic long-term design.

## 2. What We Agreed

Hard decisions:
- Language-specific hacks in shared core engine logic are wrong.
- Artifact behavior must be derived from one policy contract.
- DOT should define workflow topology and stage intent; runtime policy belongs in run config.
- Human intent should map automatically to the right artifacts (`.dot`, run YAML, or both); users should not have to decide file-level routing.
- We are not prioritizing backward-compatibility scaffolding for this refactor.

Principle:
- Attractor is orchestration-layer infrastructure. Toolchain-specific behavior belongs in pluggable/profiled layers, not core shared runtime branches.

## 3. Failure Class We Are Solving

Class label:
- Opaque deterministic artifact-gate failure.

Observed pattern:
- `verify_artifacts` and `check_artifacts` fail repeatedly with generic `exit status 1` / `artifact_paths_detected`.
- Retry cycles and loop restarts repeat deterministically.
- Engine lacks path-precise diagnostics at the failure point.

Recent evidence:
- Fail run: `~/.local/state/kilroy/attractor/runs/01KJ1G4NFDE9HTVH7FF00ADJR6`
  - `verify_artifacts/tool_invocation.json` uses ad hoc regex gate.
  - `verify_artifacts/stdout.log` only reports `artifact_paths_detected`.
  - `verify_artifacts/diff.patch` shows concrete paths (for example `.cargo-target/...`) but these are not surfaced as structured failure payload.
  - `progress.ndjson` shows repeated `verify_artifacts`/`check_artifacts` failure cycles ending in `deterministic_failure_cycle_breaker`.
- Success comparator: `~/.local/state/kilroy/attractor/runs/01KJ0MV257J7E1ENEC1BPH2SYH`
  - same command path,
  - clean pass,
  - demonstrates that gate semantics are workload/state dependent but diagnostics remain low information.

Implication:
- The class is cross-language and engine-level. Rogue/Rust exposed it; design flaw is general.

## 4. Current State Defects

1. Policy fragmentation
- Artifact behavior is split across multiple surfaces with different semantics.

2. Ad hoc graph policy
- Artifact deny logic exists as inline regex commands in graphs instead of an engine contract.

3. Core language coupling
- Shared runtime includes language-specific branches (for example Rust-specific env/preflight behavior).

4. Weak observability
- Failure output is generic and not path-precise at the point where recovery routing decisions are made.

5. Retry churn
- Deterministic failures consume retries and loop restarts because cause data is insufficient for targeted remediation routing.

## 5. Target Architecture

## 5.1 Run-Level Contract: `artifact_policy`

Add `artifact_policy` to run config as the single source of truth.

Proposed shape:
- `profiles`: `auto` or explicit list (`rust`, `go`, `node`, `python`, `java`, ...)
- `managed_roots`: run-local roots intentionally outside repo/worktree
- `env_overrides`: profile-scoped environment mappings for build/cache outputs
- `checkpoint_exclude_globs`: globs excluded from checkpoint staging
- `verify_deny_globs`: paths forbidden in feature diff unless allowed
- `verify_allow_globs`: explicit allowlist carve-outs for intentional deliverables

Notes:
- `profiles=auto` resolves from repo/run context.
- explicit profiles override auto-detection.

## 5.2 Policy Resolution

Resolve exactly once per run:
- input: run config + repo context + selected profiles,
- output: immutable `ResolvedArtifactPolicy` attached to runtime context.

All handlers consume the same resolved object.

## 5.3 Single-Source Application Points

Use `ResolvedArtifactPolicy` in exactly these engine paths:

1. Node environment shaping
- `buildBaseNodeEnv` applies resolved `env_overrides` and `managed_roots` mapping.
- Remove language-specific branches from shared env plumbing.

2. Checkpoint staging
- Stage with `checkpoint_exclude_globs` from resolved policy.
- Remove hardcoded language-specific default excludes from global defaults.

3. Artifact verification
- Replace ad hoc graph regex checks with engine verifier reading `verify_deny_globs` + `verify_allow_globs`.
- Verifier emits exact offending paths and matched rules.

## 5.4 Diagnostics Contract

Artifact verification failure payload must include:
- `failure_reason`: stable machine key (for example `artifact_policy_violation`)
- `details.summary`: short human-readable summary
- `details.offending_paths`: explicit path list
- `details.matched_deny_rules`: deny rule IDs/globs that matched
- `details.allow_exceptions_evaluated`: allow rules considered
- `details.diff_source`: whether derived from tracked diff, untracked files, or both

Goal:
- make failures immediately actionable for autonomous retry routing and postmortem.

## 5.5 DOT vs YAML Responsibilities

DOT remains responsible for:
- topology,
- stage prompts,
- routing logic.

Run YAML remains responsible for:
- runtime/provider backend wiring,
- setup/runtime policy,
- artifact policy.

No artifact deny regex should be required in DOT for standard policy behavior.

## 6. Core Cleanup (Known Hotspots)

The refactor removes language-specific logic from shared runtime paths, including existing Rust-coupled surfaces such as:
- `internal/attractor/engine/node_env.go`
- `internal/attractor/engine/codergen_router.go`
- `internal/attractor/engine/rust_sandbox_preflight.go`
- language-biased defaults in `internal/attractor/engine/config.go`

Exact code movement:
- move language mapping into profile/adapters and policy resolution layer,
- keep core engine contracts language-neutral.

## 7. Skill and Authoring Ergonomics

We aligned skill behavior with this architecture:
- One intention-to-artifact routing flow.
- User should not decide "dot change" vs "run config change".
- Skill applies both when intent requires both.

Example:
- "Change model" may require DOT stylesheet/node edits and YAML provider backend updates.
- Skill resolves and applies both automatically.

This keeps human ergonomics high while preserving clean separation of concerns in engine/runtime.

## 8. Migration Stance

This refactor is a clean architectural shift.
- No effort allocated to backward-compatibility shims in this proposal.
- Existing behavior is not the design target if it conflicts with unified policy architecture.

## 9. Implementation Plan (Phased)

Phase 1: Define policy types and resolution
- Add `artifact_policy` schema to run config model.
- Implement `ResolveArtifactPolicy(...)` and runtime attachment.

Phase 2: Wire env shaping
- Replace shared Rust-specific env branches with resolved policy application.

Phase 3: Wire checkpoint excludes
- Replace global/language-hardcoded staging excludes with policy-driven excludes.

Phase 4: Engine verifier
- Implement shared artifact verifier using resolved policy.
- Integrate into execution flow and failure payload.

Phase 5: Remove ad hoc graph dependency
- Stop requiring graph-level regex artifact gate commands for standard usage.
- Keep optional graph checks only for scenario-specific custom rules, not baseline policy.

Phase 6: Remove core language-specific runtime hooks
- Delete or relocate language-specific hooks out of core engine code paths.

Phase 7: Skill integration validation
- Ensure dotfile + run YAML generation remains synchronized under intent routing.

## 10. Acceptance Criteria

Architecture:
- No language-specific branches in shared engine artifact/env/checkpoint code paths.
- One resolved artifact policy drives env, checkpoint excludes, and verifier behavior.

Behavior:
- Artifact verifier reports exact offending paths and matching rules.
- Deterministic artifact failures route with actionable context (not generic `exit status 1`).

Ergonomics:
- User intent can be fulfilled without asking them to choose dot vs yaml target.
- DOT and run YAML remain consistent when model/provider/runtime intent changes.

## 11. Test Plan

Unit tests:
- policy schema parse/validation,
- profile resolution (`auto` + explicit),
- env override merging and precedence,
- glob matching precedence (deny vs allow),
- diagnostics payload contents.

Integration tests:
- checkpoint staging excludes use policy,
- verifier catches denied paths and emits explicit offending paths,
- verifier allows explicit `verify_allow_globs` exceptions,
- multi-profile runs (`rust`, `node`, `python`) with same engine flow.

Regression tests:
- deterministic artifact loop scenario reproduces with old behavior and is resolved by unified policy-based verification + diagnostics.

## 12. Non-Goals

- Project-specific Rogue fixes.
- New language-specific hacks in core engine.
- DOT-level policy accretion for baseline artifact handling.
- Compatibility layers that preserve fragmented old behavior.

## 13. Proposal Snapshot

In one line:
- Replace fragmented, partly language-specific artifact handling with a single run-resolved artifact policy that uniformly governs env shaping, checkpoint staging, and artifact verification, with path-precise diagnostics and intent-driven DOT/YAML authoring.
