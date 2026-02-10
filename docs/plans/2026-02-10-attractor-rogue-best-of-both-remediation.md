# Attractor Rogue Best-of-Both Reliability Remediation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Eliminate the failure modes found in rogue-fast and rogue-slow postmortems by hardening status ingestion, liveness/watchdog logic, cancellation and cycle handling, failure propagation/classification, and terminal artifact guarantees.

**Architecture:** Implement in strict dependency order: artifact and status contract first, then liveness/cancellation/cycle controls, then failure taxonomy and failover policy, then observability and spec deltas. Every behavior change is test-first and committed in small deltas so regressions are isolated quickly.

**Tech Stack:** Go (`internal/attractor/engine`, `internal/attractor/runtime`, `internal/llm`), markdown specs (`docs/strongdm/attractor`), Go test tooling (`go test`).

---

### Task 1: Atomic Artifact Write Primitive (runtime)

**Files:**
- Create: `internal/attractor/runtime/atomic_write.go`
- Modify: `internal/attractor/runtime/final.go`
- Modify: `internal/attractor/runtime/checkpoint.go`
- Test: `internal/attractor/runtime/final_test.go`
- Test: `internal/attractor/runtime/checkpoint_test.go`

**Step 1: Write the failing test for atomic writer behavior**

```go
func TestWriteJSONAtomic_ReplacesTargetWithoutPartialFile(t *testing.T) {
    dir := t.TempDir()
    target := filepath.Join(dir, "final.json")

    require.NoError(t, os.WriteFile(target, []byte("{\"status\":\"old\"}"), 0o644))
    err := writeJSONAtomic(target, map[string]any{"status": "new"})
    require.NoError(t, err)

    b, err := os.ReadFile(target)
    require.NoError(t, err)
    require.Contains(t, string(b), "new")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/attractor/runtime -run 'WriteJSONAtomic_ReplacesTargetWithoutPartialFile' -count=1`
Expected: FAIL with `undefined: writeJSONAtomic`.

**Step 3: Write minimal implementation**

```go
func writeJSONAtomic(path string, v any) error {
    if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
        return err
    }
    tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*.json")
    if err != nil {
        return err
    }
    tmpName := tmp.Name()
    defer os.Remove(tmpName)

    enc := json.NewEncoder(tmp)
    enc.SetIndent("", "  ")
    if err := enc.Encode(v); err != nil {
        _ = tmp.Close()
        return err
    }
    if err := tmp.Sync(); err != nil {
        _ = tmp.Close()
        return err
    }
    if err := tmp.Close(); err != nil {
        return err
    }
    return os.Rename(tmpName, path)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/attractor/runtime -run 'WriteJSONAtomic|Final|Checkpoint' -count=1`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/attractor/runtime/atomic_write.go internal/attractor/runtime/final.go internal/attractor/runtime/checkpoint.go internal/attractor/runtime/final_test.go internal/attractor/runtime/checkpoint_test.go
git commit -m "runtime: add atomic json writer and migrate final/checkpoint persistence"
```

### Task 2: Status Ingestion Precedence + Ownership Gate

**Files:**
- Modify: `internal/attractor/runtime/status.go`
- Modify: `internal/attractor/engine/handlers.go`
- Test: `internal/attractor/runtime/status_test.go`
- Test: `internal/attractor/engine/status_json_test.go`
- Test: `internal/attractor/engine/status_json_worktree_test.go`
- Test: `internal/attractor/engine/status_json_legacy_details_test.go`

**Step 1: Write failing decision-table tests**

```go
func TestResolveStatusSource_DecisionTable(t *testing.T) {
    cases := []struct {
        name        string
        haveCanonical bool
        haveWorktree bool
        haveAI       bool
        expectSource string
        expectErr    string
    }{
        {name: "canonical wins", haveCanonical: true, haveWorktree: true, haveAI: true, expectSource: "canonical"},
        {name: "canonical ownership mismatch fails", haveCanonical: true, expectErr: "ownership mismatch"},
        {name: "fallback worktree when canonical missing", haveWorktree: true, expectSource: "worktree"},
    }
    // execute table against resolveStatusSource(...)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/attractor/runtime ./internal/attractor/engine -run 'ResolveStatusSource|status_json' -count=1`
Expected: FAIL due to missing precedence/ownership cases.

**Step 3: Implement minimal source resolver + canonical copy contract**

```go
type statusSource string

const (
    sourceCanonical statusSource = "canonical"
    sourceWorktree  statusSource = "worktree"
    sourceAI        statusSource = "ai"
)

func resolveStatusSource(paths statusPaths, owner statusOwner) (statusSource, error) {
    // canonical first; deterministic error on canonical ownership mismatch;
    // fallback only when canonical absent.
}
```

**Step 4: Re-run targeted tests**

Run: `go test ./internal/attractor/runtime ./internal/attractor/engine -run 'ResolveStatusSource|status_json|LegacyDetails' -count=1`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/attractor/runtime/status.go internal/attractor/runtime/status_test.go internal/attractor/engine/handlers.go internal/attractor/engine/status_json_test.go internal/attractor/engine/status_json_worktree_test.go internal/attractor/engine/status_json_legacy_details_test.go
git commit -m "engine/runtime: enforce canonical status precedence and ownership-gated legacy fallback"
```

### Task 3: Attempt-Scoped Heartbeat Lifecycle

**Files:**
- Modify: `internal/attractor/engine/codergen_router.go`
- Modify: `internal/attractor/engine/progress.go`
- Test: `internal/attractor/engine/codergen_heartbeat_test.go`
- Test: `internal/attractor/engine/progress_test.go`

**Step 1: Write failing test for stale heartbeat leak**

```go
func TestHeartbeatStopsAfterAttemptEnd(t *testing.T) {
    // start attempt -> emit heartbeats -> mark attempt end -> assert no further
    // stage_heartbeat events for same (node_id, attempt_id, run_generation)
}
```

**Step 2: Run failing test**

Run: `go test ./internal/attractor/engine -run 'HeartbeatStopsAfterAttemptEnd|codergen_heartbeat' -count=1`
Expected: FAIL with post-end heartbeat observed.

**Step 3: Implement attempt lifecycle cancelation for heartbeat loop**

```go
type attemptScope struct {
    nodeID        string
    attemptID     string
    runGeneration int
    done          chan struct{}
}

func (a *attemptScope) stop() { close(a.done) }
```

**Step 4: Run targeted heartbeat/progress tests**

Run: `go test ./internal/attractor/engine -run 'Heartbeat|Progress|codergen_heartbeat' -count=1`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/attractor/engine/codergen_router.go internal/attractor/engine/progress.go internal/attractor/engine/codergen_heartbeat_test.go internal/attractor/engine/progress_test.go
git commit -m "engine: scope heartbeats to active attempt lifecycle and stop on attempt end"
```

### Task 4: Fanout-Aware Watchdog Liveness Aggregation

**Files:**
- Modify: `internal/attractor/engine/engine.go`
- Modify: `internal/attractor/engine/parallel_handlers.go`
- Test: `internal/attractor/engine/engine_stall_watchdog_test.go`
- Test: `internal/attractor/engine/parallel_guardrails_test.go`
- Test: `internal/attractor/engine/parallel_test.go`

**Step 1: Write failing tests for branch progress resetting parent watchdog**

```go
func TestStallWatchdog_AnyActiveBranchLivenessResetsTimer(t *testing.T) {}
func TestStallWatchdog_FirstSuccessIgnoresCanceledBranchAfterAck(t *testing.T) {}
```

**Step 2: Run failing watchdog tests**

Run: `go test ./internal/attractor/engine -run 'StallWatchdog|parallel_guardrails|FirstSuccess' -count=1`
Expected: FAIL with false timeout while branch events continue.

**Step 3: Implement branch-aware liveness aggregator**

```go
type watchdogLiveness struct {
    activeBranches map[string]bool
    lastTick       time.Time
}

func (w *watchdogLiveness) Accept(event progressEvent) bool {
    // count attempt start/end/heartbeat + branch_complete;
    // apply first_success cancellation acknowledgement rule.
}
```

**Step 4: Run targeted parallel/watchdog tests**

Run: `go test ./internal/attractor/engine -run 'StallWatchdog|parallel_guardrails|parallel_test' -count=1`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/attractor/engine/engine.go internal/attractor/engine/parallel_handlers.go internal/attractor/engine/engine_stall_watchdog_test.go internal/attractor/engine/parallel_guardrails_test.go internal/attractor/engine/parallel_test.go
git commit -m "engine: make stall watchdog liveness fanout-aware across active branches"
```

### Task 5: Cancellation Precedence in Subgraph/Parallel Traversal

**Files:**
- Modify: `internal/attractor/engine/subgraph.go`
- Modify: `internal/attractor/engine/parallel_handlers.go`
- Modify: `internal/attractor/engine/next_hop.go`
- Test: `internal/attractor/engine/next_hop_test.go`
- Test: `internal/attractor/engine/parallel_guardrails_test.go`

**Step 1: Write failing tests for "no new attempts after cancel"**

```go
func TestRunSubgraphUntil_CancelStopsEdgeSelection(t *testing.T) {}
func TestCancelPrecedence_IgnoreErrorPolicyStillStopsRun(t *testing.T) {}
```

**Step 2: Run failing cancellation tests**

Run: `go test ./internal/attractor/engine -run 'CancelStopsEdgeSelection|CancelPrecedence|next_hop' -count=1`
Expected: FAIL with attempt scheduling after cancellation.

**Step 3: Implement cancellation guards at loop boundaries and post-node execution**

```go
if ctx.Err() != nil {
    return runtime.StageStatusFail, runtime.FailureReasonCanceled
}
```

**Step 4: Re-run cancellation and next-hop tests**

Run: `go test ./internal/attractor/engine -run 'Cancel|next_hop|parallel_guardrails' -count=1`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/attractor/engine/subgraph.go internal/attractor/engine/parallel_handlers.go internal/attractor/engine/next_hop.go internal/attractor/engine/next_hop_test.go internal/attractor/engine/parallel_guardrails_test.go
git commit -m "engine: enforce cancellation precedence in subgraph and parallel traversal"
```

### Task 6: Deterministic Cycle Breaker Parity in Subgraph Path

**Files:**
- Modify: `internal/attractor/engine/subgraph.go`
- Modify: `internal/attractor/engine/loop_restart_policy.go`
- Test: `internal/attractor/engine/deterministic_failure_cycle_test.go`
- Test: `internal/attractor/engine/deterministic_failure_cycle_resume_test.go`
- Test: `internal/attractor/engine/loop_restart_guardrails_test.go`

**Step 1: Write failing tests for repeated-signature breaker in subgraph traversal**

```go
func TestSubgraphDeterministicFailureCycle_BreaksAtConfiguredLimit(t *testing.T) {}
```

**Step 2: Run failing cycle tests**

Run: `go test ./internal/attractor/engine -run 'SubgraphDeterministicFailureCycle|deterministic_failure_cycle' -count=1`
Expected: FAIL with repeated retries past configured limit.

**Step 3: Implement shared signature counter path used by main + subgraph**

```go
sig := normalizeFailureSignature(rawReason)
count := bumpFailureSignature(sig)
if count >= loopRestartSignatureLimit {
    return triggerLoopRestart(sig, count)
}
```

**Step 4: Re-run cycle/loop-restart tests**

Run: `go test ./internal/attractor/engine -run 'deterministic_failure_cycle|loop_restart' -count=1`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/attractor/engine/subgraph.go internal/attractor/engine/loop_restart_policy.go internal/attractor/engine/deterministic_failure_cycle_test.go internal/attractor/engine/deterministic_failure_cycle_resume_test.go internal/attractor/engine/loop_restart_guardrails_test.go
git commit -m "engine: add deterministic failure-cycle breaker parity for subgraph traversal"
```

### Task 7: Failure Causality Preservation Through Routing and Terminalization

**Files:**
- Modify: `internal/attractor/engine/handlers.go`
- Modify: `internal/attractor/engine/conditional_passthrough_test.go`
- Modify: `internal/attractor/runtime/final.go`
- Test: `internal/attractor/runtime/final_test.go`

**Step 1: Write failing tests for raw reason propagation**

```go
func TestConditionalPassthrough_PreservesFailureReason(t *testing.T) {}
func TestFinalArtifact_PreservesRawFailureReason(t *testing.T) {}
```

**Step 2: Run failing tests**

Run: `go test ./internal/attractor/engine ./internal/attractor/runtime -run 'PreservesFailureReason|conditional_passthrough|FinalArtifact' -count=1`
Expected: FAIL with genericized reason replacing causal reason.

**Step 3: Implement pass-through fields and separate normalized signature field**

```go
type failureMeta struct {
    RawReason           string `json:"failure_reason"`
    NormalizedSignature string `json:"failure_signature_normalized,omitempty"`
}
```

**Step 4: Re-run failure propagation tests**

Run: `go test ./internal/attractor/engine ./internal/attractor/runtime -run 'FailureReason|conditional_passthrough|FinalArtifact' -count=1`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/attractor/engine/handlers.go internal/attractor/engine/conditional_passthrough_test.go internal/attractor/runtime/final.go internal/attractor/runtime/final_test.go
git commit -m "engine/runtime: preserve causal failure reasons through conditionals and terminal artifact"
```

### Task 8: Terminal Artifact Guarantee Across All Controllable Exits

**Files:**
- Modify: `internal/attractor/engine/engine.go`
- Modify: `internal/attractor/runtime/final.go`
- Test: `internal/attractor/engine/engine_test.go`
- Test: `internal/attractor/engine/handler_panic_test.go`
- Test: `internal/attractor/runtime/final_test.go`

**Step 1: Write failing integration tests for missing `final.json` paths**

```go
func TestRunWritesFinalJSON_OnWatchdogTimeout(t *testing.T) {}
func TestRunWritesFinalJSON_OnContextCancel(t *testing.T) {}
func TestRunWritesFinalJSON_OnHandlerPanic(t *testing.T) {}
```

**Step 2: Run failing terminalization tests**

Run: `go test ./internal/attractor/engine ./internal/attractor/runtime -run 'WritesFinalJSON|HandlerPanic|WatchdogTimeout|ContextCancel' -count=1`
Expected: FAIL in one or more terminal paths.

**Step 3: Implement single terminalization hook invoked by every controllable exit path**

```go
func (e *Engine) finalizeRun(ctx context.Context, status runtime.FinalStatus, reason string) {
    _ = runtime.WriteFinal(e.logsRoot, runtime.Final{Status: status, FailureReason: reason, RunID: e.runID})
}
```

**Step 4: Re-run terminalization tests**

Run: `go test ./internal/attractor/engine ./internal/attractor/runtime -run 'WritesFinalJSON|final' -count=1`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/attractor/engine/engine.go internal/attractor/runtime/final.go internal/attractor/engine/engine_test.go internal/attractor/engine/handler_panic_test.go internal/attractor/runtime/final_test.go
git commit -m "engine: guarantee final.json persistence for all controllable terminal paths"
```

### Task 9: Unified-LLM Error Taxonomy Mapping Completion

**Files:**
- Modify: `internal/attractor/engine/provider_error_classification.go`
- Test: `internal/attractor/engine/provider_error_classification_test.go`
- Test: `internal/attractor/engine/retry_classification_integration_test.go`
- Test: `internal/attractor/engine/retry_failure_class_test.go`

**Step 1: Write failing table-driven mapping tests for missing types**

```go
func TestClassifyProviderError_CompleteUnifiedTaxonomy(t *testing.T) {
    cases := []struct {
        err  error
        want string
    }{
        {err: llm.InvalidToolCallError{Message: "bad"}, want: "api_deterministic"},
        {err: llm.NoObjectGeneratedError{Message: "missing"}, want: "api_deterministic"},
        {err: context.Canceled, want: "canceled"},
    }
    // assert classifier output
}
```

**Step 2: Run failing classification tests**

Run: `go test ./internal/attractor/engine -run 'ClassifyProviderError|retry_classification|retry_failure_class' -count=1`
Expected: FAIL for missing or misclassified entries.

**Step 3: Implement full mapping and explicit canceled-context class**

```go
func classifyProviderError(err error) failureClass {
    switch {
    case errors.Is(err, context.Canceled):
        return failureClassCanceled
    case llm.IsInvalidToolCallError(err), llm.IsNoObjectGeneratedError(err):
        return failureClassAPIDeterministic
    // full unified-llm mapping cases
    }
}
```

**Step 4: Re-run classifier/retry tests**

Run: `go test ./internal/attractor/engine -run 'ClassifyProviderError|retry_classification|retry_failure_class' -count=1`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/attractor/engine/provider_error_classification.go internal/attractor/engine/provider_error_classification_test.go internal/attractor/engine/retry_classification_integration_test.go internal/attractor/engine/retry_failure_class_test.go
git commit -m "engine: complete unified llm error mapping and separate canceled-context classification"
```

### Task 10: Enforce Pinned No-Failover Policy + Tool-Contract Guardrails

**Files:**
- Modify: `internal/attractor/engine/provider_runtime.go`
- Modify: `internal/attractor/engine/codergen_failover_test.go`
- Modify: `internal/attractor/engine/config.go`
- Modify: `internal/attractor/engine/config_runtime_policy_test.go`
- Modify: `internal/llm/tool_validation.go`
- Modify: `internal/llm/tool_validation_test.go`

**Step 1: Write failing tests for explicit pin/no-failover enforcement**

```go
func TestPinnedProviderModel_NoFailoverAllowed(t *testing.T) {}
func TestPinnedProviderModel_FailoverAttemptFailsLoudly(t *testing.T) {}
```

**Step 2: Run failing failover/policy tests**

Run: `go test ./internal/attractor/engine ./internal/llm -run 'NoFailover|PinnedProviderModel|ToolValidation' -count=1`
Expected: FAIL when runtime silently chooses fallback.

**Step 3: Implement enforcement path and deterministic policy violation errors**

```go
if policy.PinProviderModel && nextProvider != pinnedProvider {
    return fmt.Errorf("failover forbidden by run policy: pinned provider/model %s/%s", pinnedProvider, pinnedModel)
}
```

**Step 4: Re-run policy/tool validation tests**

Run: `go test ./internal/attractor/engine ./internal/llm -run 'NoFailover|PinnedProviderModel|ToolValidation' -count=1`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/attractor/engine/provider_runtime.go internal/attractor/engine/codergen_failover_test.go internal/attractor/engine/config.go internal/attractor/engine/config_runtime_policy_test.go internal/llm/tool_validation.go internal/llm/tool_validation_test.go
git commit -m "engine/llm: enforce pinned no-failover policy and harden tool contract validation"
```

### Task 11: Observability for Ingestion, Liveness, Cancellation, and Cycle Breaks

**Files:**
- Modify: `internal/attractor/engine/progress.go`
- Modify: `internal/attractor/engine/cxdb_events.go`
- Test: `internal/attractor/engine/progress_test.go`
- Test: `internal/attractor/engine/reference_compat_test.go`

**Step 1: Write failing event-coverage tests for new decision points**

```go
func TestProgressEmitsStatusIngestionDecision(t *testing.T) {}
func TestProgressEmitsCycleBreakSignatureAndLimit(t *testing.T) {}
func TestProgressEmitsCancellationExitPoint(t *testing.T) {}
```

**Step 2: Run failing observability tests**

Run: `go test ./internal/attractor/engine -run 'ProgressEmits|reference_compat' -count=1`
Expected: FAIL because events are missing.

**Step 3: Add structured event payloads with stable keys**

```go
emit("status_ingestion_decision", map[string]any{
    "chosen_source": source,
    "ownership_result": ownership,
    "copied_to_canonical": copied,
})
```

**Step 4: Re-run observability tests**

Run: `go test ./internal/attractor/engine -run 'ProgressEmits|reference_compat' -count=1`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/attractor/engine/progress.go internal/attractor/engine/cxdb_events.go internal/attractor/engine/progress_test.go internal/attractor/engine/reference_compat_test.go
git commit -m "engine: add structured observability events for status ingestion, cancellation, liveness, and cycle breaks"
```

### Task 12: Spec Delta Documentation (Attractor + Unified LLM alignment)

**Files:**
- Modify: `docs/strongdm/attractor/attractor-spec.md`
- Modify: `docs/strongdm/attractor/coding-agent-loop-spec.md`
- Modify: `docs/strongdm/attractor/unified-llm-spec.md`
- Modify: `postmortem-rogue-best-of-both-fixes.md`

**Step 1: Write failing doc checklist in plan comments**

```markdown
- [ ] `loop_restart_signature_limit` contract documented
- [ ] status ingestion fallback contract documented
- [ ] enum-vs-condition casing clarified
- [ ] cancellation precedence over parallel error_policy documented
```

**Step 2: Run doc consistency grep checks**

Run: `rg -n 'loop_restart_signature_limit|status.json|cancellation|StageStatus|outcome=' docs/strongdm/attractor`
Expected: Missing one or more required contracts.

**Step 3: Implement minimal spec delta text blocks**

```markdown
### Cancellation Precedence
Run-level cancellation terminates further node scheduling regardless of branch-level `error_policy`.
```

**Step 4: Re-run grep checks + markdown sanity**

Run: `rg -n 'loop_restart_signature_limit|legacy fallback|cancellation precedence|StageStatus' docs/strongdm/attractor`
Expected: All required phrases present in correct specs.

**Step 5: Commit**

```bash
git add docs/strongdm/attractor/attractor-spec.md docs/strongdm/attractor/coding-agent-loop-spec.md docs/strongdm/attractor/unified-llm-spec.md postmortem-rogue-best-of-both-fixes.md
git commit -m "docs: codify runtime contracts and spec deltas for status, cancellation, cycle-break, and taxonomy"
```

### Task 13: Full Regression Gate + Rogue-Fast Validation Run

**Files:**
- Modify: `docs/plans/2026-02-10-attractor-rogue-best-of-both-remediation.md`
- Modify: `postmortem-rogue-best-of-both-fixes.md` (release gate evidence section)

**Step 1: Add failing regression checklist entry for untouched suites**

```markdown
- [ ] `go test ./internal/attractor/...`
- [ ] `go test ./internal/llm/...`
- [ ] `go run ./cmd/kilroy attractor validate --graph demo/rogue/rogue_fast.dot`
```

**Step 2: Run full regression suite**

Run: `go test ./internal/attractor/... ./internal/llm/... -count=1`
Expected: PASS (if FAIL, return to owning task).

**Step 3: Run graph validation + one rogue-fast validation run**

Run:
- `go run ./cmd/kilroy attractor validate --graph demo/rogue/rogue_fast.dot`
- `./kilroy attractor run --detach --graph demo/rogue/rogue_fast.dot --config demo/rogue/run-fast.yaml --run-id rogue-fast-validation-$(date +%Y%m%d-%H%M%S) --logs-root ~/.local/state/kilroy/attractor`

Expected: graph validate passes; run starts cleanly with no immediate watchdog/failover misconfiguration failure.

**Step 4: Capture evidence in postmortem release gate section**

```markdown
- Regression command outputs
- Validation run_id
- `final.json` status or active-progress snapshot timestamp
```

**Step 5: Commit**

```bash
git add docs/plans/2026-02-10-attractor-rogue-best-of-both-remediation.md postmortem-rogue-best-of-both-fixes.md
git commit -m "chore: record regression gate evidence and rogue-fast validation run handoff"
```

## Execution Notes

- Keep commits delta-oriented and small; do not batch multiple tasks into one commit.
- Run the exact test command listed after each implementation step; do not skip failing-test confirmation.
- If a task touches shared engine behavior, re-run `go test ./internal/attractor/...` before committing that task.
- If a task changes error mapping or provider policy, re-run `go test ./internal/llm/...` before committing that task.

## Quick Start Command Sequence

```bash
# Task-by-task execution flow
git checkout -b plan/rogue-best-of-both-remediation-20260210
go test ./internal/attractor/runtime -run 'WriteJSONAtomic_ReplacesTargetWithoutPartialFile' -count=1
# implement Task 1, then commit
# continue task order 2 -> 13
```
