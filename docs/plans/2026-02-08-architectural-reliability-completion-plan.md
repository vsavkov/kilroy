# Architectural Reliability Completion Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ensure deterministic failures fail fast without consuming retry budget, and surface provider CLI contract/model issues before stage execution with deterministic preflight artifacts.

**Architecture:** Use one shared failure-policy contract for classification and retry decisions across stage retry, loop restart, and fan-in. Classify failures at the source (especially provider CLI adapters), then consume normalized classes in retry logic. Add deterministic provider CLI preflight in `RunWithConfig` before CXDB health checks and always persist `preflight_report.json` (pass and fail paths).

**Tech Stack:** Go 1.25, stdlib `testing`, existing Attractor engine integration tests, existing CXDB harness, shell guardrail script.

---

## Baseline (Revalidated 2026-02-08)

Run and confirm green before starting:

1. `go test ./internal/attractor/runtime -count=1`
2. `go test ./internal/attractor/engine -count=1`
3. `go test ./cmd/kilroy -count=1`
4. `go test ./internal/llm/providers/... -count=1`
5. `bash scripts/e2e-guardrail-matrix.sh`

Known reliability gaps this plan closes:

1. Deterministic stage failures still retry until attempts are exhausted.
2. Provider CLI contract mismatches are discovered during stage execution instead of preflight.
3. Provider model-availability failures are not consistently class-tagged at source.
4. Targeted `go test -run ...` commands can silently pass as `[no tests to run]` if names drift.

---

## Mandatory Test-Run Rule (All Tasks)

For every targeted `go test -run` command in this plan:

1. Use anchored regexes (`'^TestName$'` or explicit alternation with `^...$`).
2. Treat output containing `[no tests to run]` as a hard failure.

Example pattern:

```bash
out="$(go test ./internal/attractor/engine -run '^TestRun_DeterministicFailure_DoesNotRetry$' -count=1 2>&1)"
printf '%s\n' "$out"
if grep -q '\[no tests to run\]' <<<"$out"; then
  echo "ERROR: targeted test did not execute"
  exit 1
fi
```

---

### Task 1: Add Shared Failure-Policy Contract Tests

**Files:**
- Create: `internal/attractor/engine/failure_policy_test.go`
- Reuse: `internal/attractor/engine/loop_restart_policy.go`

**Step 1: Write failing normalization + retry-policy tests**

```go
func TestFailurePolicy_NormalizeFailureClass(t *testing.T) {
    cases := map[string]string{
        "transient":      failureClassTransientInfra,
        "transient_infra": failureClassTransientInfra,
        "permanent":      failureClassDeterministic,
        "":               "",
    }
    for in, want := range cases {
        if got := normalizedFailureClass(in); got != want {
            t.Fatalf("normalizedFailureClass(%q)=%q want %q", in, got, want)
        }
    }
}

func TestFailurePolicy_ShouldRetryOutcome_ClassGated(t *testing.T) {
    if !shouldRetryOutcome(runtime.Outcome{Status: runtime.StatusFail}, failureClassTransientInfra) {
        t.Fatalf("expected transient fail to retry")
    }
    if shouldRetryOutcome(runtime.Outcome{Status: runtime.StatusFail}, failureClassDeterministic) {
        t.Fatalf("expected deterministic fail to not retry")
    }
}
```

**Step 2: Run tests to verify red**

Run:

```bash
out="$(go test ./internal/attractor/engine -run '^TestFailurePolicy_NormalizeFailureClass$|^TestFailurePolicy_ShouldRetryOutcome_ClassGated$' -count=1 2>&1)"
printf '%s\n' "$out"
! grep -q '\[no tests to run\]' <<<"$out"
```

Expected: FAIL because `shouldRetryOutcome` does not exist yet.

**Step 3: Commit failing tests**

```bash
git add internal/attractor/engine/failure_policy_test.go
git commit -m "test(engine): add shared failure-policy contract coverage"
```

---

### Task 2: Implement Shared Failure-Policy Helpers

**Files:**
- Modify: `internal/attractor/engine/loop_restart_policy.go`
- Optionally create: `internal/attractor/engine/failure_policy.go`
- Test: `internal/attractor/engine/failure_policy_test.go`

**Step 1: Add class-gated retry helper in shared policy layer**

```go
func shouldRetryOutcome(out runtime.Outcome, failureClass string) bool {
    if out.Status != runtime.StatusFail && out.Status != runtime.StatusRetry {
        return false
    }
    return normalizedFailureClassOrDefault(failureClass) == failureClassTransientInfra
}
```

**Step 2: Keep fail-closed default for unknown class**

Ensure unknown/empty class defaults to deterministic via existing `normalizedFailureClassOrDefault`.

**Step 3: Run tests to verify green**

Run:

```bash
out="$(go test ./internal/attractor/engine -run '^TestFailurePolicy_NormalizeFailureClass$|^TestFailurePolicy_ShouldRetryOutcome_ClassGated$' -count=1 2>&1)"
printf '%s\n' "$out"
! grep -q '\[no tests to run\]' <<<"$out"
```

Expected: PASS.

**Step 4: Commit**

```bash
git add internal/attractor/engine/loop_restart_policy.go internal/attractor/engine/failure_policy.go internal/attractor/engine/failure_policy_test.go
git commit -m "feat(engine): centralize class normalization and retry gating policy"
```

---

### Task 3: Add Stage Retry-Gating Red Tests

**Files:**
- Create: `internal/attractor/engine/retry_failure_class_test.go`
- Reuse helpers: `internal/attractor/engine/engine_test.go`

**Step 1: Add deterministic no-retry test**

```go
func TestRun_DeterministicFailure_DoesNotRetry(t *testing.T) {
    // Handler returns StatusFail + Meta.failure_class=deterministic.
    // Graph default_max_retry=3.
    // Expect exactly one stage_attempt_end for node and zero stage_retry_sleep events.
}
```

**Step 2: Add transient retry test**

```go
func TestRun_TransientFailure_StillRetries(t *testing.T) {
    // Attempt1: StatusFail + Meta.failure_class=transient_infra.
    // Attempt2: success.
    // Expect at least one stage_retry_sleep and eventual success.
}
```

**Step 3: Run tests to verify red**

Run:

```bash
out="$(go test ./internal/attractor/engine -run '^TestRun_DeterministicFailure_DoesNotRetry$|^TestRun_TransientFailure_StillRetries$' -count=1 2>&1)"
printf '%s\n' "$out"
! grep -q '\[no tests to run\]' <<<"$out"
```

Expected: FAIL before retry-loop wiring.

**Step 4: Commit failing tests**

```bash
git add internal/attractor/engine/retry_failure_class_test.go
git commit -m "test(engine): reproduce missing class-gated stage retry behavior"
```

---

### Task 4: Gate `executeWithRetry` By Shared Failure Policy

**Files:**
- Modify: `internal/attractor/engine/engine.go`
- Test: `internal/attractor/engine/retry_failure_class_test.go`
- Regressions: `internal/attractor/engine/retry_policy_test.go`, `internal/attractor/engine/retry_on_retry_status_test.go`

**Step 1: Use source-classified failure class when available**

In retry loop:

```go
failureClass := classifyFailureClass(out) // respects Meta/ContextUpdates hints first
if attempt < maxAttempts && shouldRetryOutcome(out, failureClass) {
    // existing sleep path
} else if attempt < maxAttempts {
    e.appendProgress(map[string]any{
        "event":           "stage_retry_blocked",
        "node_id":         node.ID,
        "status":          string(out.Status),
        "failure_class":   normalizedFailureClassOrDefault(failureClass),
        "failure_reason":  out.FailureReason,
        "attempt":         attempt,
        "max":             maxAttempts,
    })
    break
}
```

**Step 2: Preserve existing allow-partial and terminal-fail semantics**

Do not change end-of-loop status canonicalization behavior.

**Step 3: Run focused tests**

```bash
go test ./internal/attractor/engine -run '^TestRun_DeterministicFailure_DoesNotRetry$|^TestRun_TransientFailure_StillRetries$|^TestRun_RetriesOnFail_ThenSucceeds$|^TestRun_RetriesOnRetryStatus$|^TestRun_AllowPartialAfterRetryExhaustion$' -count=1
```

Expected: PASS.

**Step 4: Commit**

```bash
git add internal/attractor/engine/engine.go internal/attractor/engine/retry_failure_class_test.go
git commit -m "fix(engine): apply class-gated retry policy in executeWithRetry"
```

---

### Task 5: Add Provider CLI Error-Classification Red Tests

**Files:**
- Create: `internal/attractor/engine/provider_error_classification_test.go`
- Reuse: `internal/attractor/engine/codergen_process_test.go`

**Step 1: Add Anthropic contract mismatch classifier test**

```go
func TestClassifyProviderCLIError_AnthropicStreamJSONRequiresVerbose(t *testing.T) {
    stderr := "Error: When using --print, --output-format=stream-json requires --verbose"
    got := classifyProviderCLIError("anthropic", stderr, errors.New("exit status 1"))
    // Expect deterministic + stable provider_contract signature.
}
```

**Step 2: Add Gemini model-not-found classifier test**

```go
func TestClassifyProviderCLIError_GeminiModelNotFound(t *testing.T) {
    stderr := "ModelNotFoundError: Requested entity was not found."
    got := classifyProviderCLIError("google", stderr, errors.New("exit status 1"))
    // Expect deterministic + provider_model_unavailable signature.
}
```

**Step 3: Add Codex idle-timeout classifier test using runErr signal**

```go
func TestClassifyProviderCLIError_CodexIdleTimeout_RunErrSignal(t *testing.T) {
    runErr := errors.New("codex idle timeout after 2m0s with no output")
    got := classifyProviderCLIError("openai", "", runErr)
    // Expect transient_infra.
}
```

**Step 4: Run tests to verify red**

```bash
out="$(go test ./internal/attractor/engine -run '^TestClassifyProviderCLIError_AnthropicStreamJSONRequiresVerbose$|^TestClassifyProviderCLIError_GeminiModelNotFound$|^TestClassifyProviderCLIError_CodexIdleTimeout_RunErrSignal$' -count=1 2>&1)"
printf '%s\n' "$out"
! grep -q '\[no tests to run\]' <<<"$out"
```

Expected: FAIL before classifier implementation.

**Step 5: Commit failing tests**

```bash
git add internal/attractor/engine/provider_error_classification_test.go
git commit -m "test(engine): reproduce missing provider CLI failure classification"
```

---

### Task 6: Implement Provider Classifier + Source-Level Outcome Metadata

**Files:**
- Create: `internal/attractor/engine/provider_error_classification.go`
- Modify: `internal/attractor/engine/codergen_router.go`
- Modify: `internal/attractor/engine/codergen_cli_invocation_test.go`
- Test: `internal/attractor/engine/provider_error_classification_test.go`

**Step 1: Implement deterministic-first provider classifier**

```go
type providerCLIClassifiedError struct {
    FailureClass     string
    FailureSignature string
    FailureReason    string
}

func classifyProviderCLIError(provider string, stderr string, runErr error) providerCLIClassifiedError {
    // 1) Provider-specific deterministic signatures (contract/model unavailable)
    // 2) Transient infra hints (timeouts/network/429/5xx)
    // 3) Fallback deterministic
}
```

**Step 2: Attach class/signature where CLI failures are produced**

In `runCLI`, when returning failure outcomes for CLI execution failure:

```go
classified := classifyProviderCLIError(providerKey, string(stderrBytes), runErr)
return outStr, &runtime.Outcome{
    Status:        runtime.StatusFail,
    FailureReason: classified.FailureReason,
    Meta: map[string]any{
        "failure_class":     classified.FailureClass,
        "failure_signature": classified.FailureSignature,
    },
    ContextUpdates: map[string]any{
        "failure_class": classified.FailureClass,
    },
}, nil
```

**Step 3: Fix Anthropic default CLI invocation contract**

```go
case "anthropic":
    exe = envOr("KILROY_CLAUDE_PATH", "claude")
    args = []string{"-p", "--output-format", "stream-json", "--verbose", "--model", modelID}
```

**Step 4: Add/adjust invocation tests**

Add test:

```go
func TestDefaultCLIInvocation_AnthropicIncludesVerboseForStreamJSON(t *testing.T) {
    _, args := defaultCLIInvocation("anthropic", "claude-opus", "/tmp/worktree")
    if !hasArg(args, "--verbose") { t.Fatalf("...") }
}
```

**Step 5: Run focused tests**

```bash
go test ./internal/attractor/engine -run '^TestClassifyProviderCLIError_AnthropicStreamJSONRequiresVerbose$|^TestClassifyProviderCLIError_GeminiModelNotFound$|^TestClassifyProviderCLIError_CodexIdleTimeout_RunErrSignal$|^TestDefaultCLIInvocation_AnthropicIncludesVerboseForStreamJSON$|^TestCodexCLIInvocation_StateRootIsAbsolute$' -count=1
```

Expected: PASS.

**Step 6: Commit**

```bash
git add internal/attractor/engine/provider_error_classification.go internal/attractor/engine/codergen_router.go internal/attractor/engine/provider_error_classification_test.go internal/attractor/engine/codergen_cli_invocation_test.go
git commit -m "feat(engine): classify provider CLI failures at source and fix anthropic invocation contract"
```

---

### Task 7: Add Deterministic Provider CLI Preflight + `preflight_report.json`

**Files:**
- Create: `internal/attractor/engine/provider_preflight.go`
- Modify: `internal/attractor/engine/run_with_config.go`
- Modify: `internal/attractor/engine/provider_preflight_test.go`
- Optionally modify: `internal/attractor/engine/run_with_config_integration_test.go`

**Step 1: Add preflight red tests (binary missing, contract mismatch, report always written)**

```go
func TestRunWithConfig_PreflightFails_WhenProviderCLIBinaryMissing(t *testing.T) {}
func TestRunWithConfig_PreflightFails_WhenAnthropicHelpMissingVerboseFlag(t *testing.T) {}
func TestRunWithConfig_WritesPreflightReport_Always(t *testing.T) {}
```

Test fixtures must ensure provider/model pairs are valid in catalog when testing CLI binary/contract checks (to avoid failing earlier in model validation).

**Step 2: Run tests to verify red**

```bash
out="$(go test ./internal/attractor/engine -run '^TestRunWithConfig_PreflightFails_WhenProviderCLIBinaryMissing$|^TestRunWithConfig_PreflightFails_WhenAnthropicHelpMissingVerboseFlag$|^TestRunWithConfig_WritesPreflightReport_Always$' -count=1 2>&1)"
printf '%s\n' "$out"
! grep -q '\[no tests to run\]' <<<"$out"
```

Expected: FAIL before preflight implementation.

**Step 3: Implement preflight model + writer (always writes)**

```go
type preflightReport struct {
    Timestamp string           `json:"timestamp"`
    Checks    []preflightCheck `json:"checks"`
    Summary   preflightSummary `json:"summary"`
}
```

In `RunWithConfig`:

1. After `opts.applyDefaults()`, create logs root (`os.MkdirAll(opts.LogsRoot, 0o755)`).
2. Execute CLI preflight before CXDB `Health` and before stage execution.
3. Use deferred finalization to persist `preflight_report.json` on both pass and fail paths.

**Step 4: Implement provider-specific capability checks using real invocation path**

- Preflight only providers used by graph nodes with `backend=cli`.
- Required command checks:
  1. OpenAI: `<codex_exe> exec --help` includes `--json` and `-m`.
  2. Anthropic: `<claude_exe> --help` includes `-p`, `--output-format`, `--verbose`, `--model`.
  3. Google: `<gemini_exe> --help` includes `-p`, `--output-format`, `--yolo`, `--model`.

**Step 5: Return deterministic, class-aware preflight errors**

```go
return fmt.Errorf("preflight[deterministic]: provider=%s check=%s: %w", provider, checkName, err)
```

**Step 6: Run focused tests**

```bash
go test ./internal/attractor/engine -run '^TestRunWithConfig_PreflightFails_WhenProviderCLIBinaryMissing$|^TestRunWithConfig_PreflightFails_WhenAnthropicHelpMissingVerboseFlag$|^TestRunWithConfig_WritesPreflightReport_Always$|^TestRunWithConfig_FailsFast_WhenCLIModelNotInCatalogForProvider$' -count=1
```

Expected: PASS.

**Step 7: Commit**

```bash
git add internal/attractor/engine/provider_preflight.go internal/attractor/engine/run_with_config.go internal/attractor/engine/provider_preflight_test.go internal/attractor/engine/run_with_config_integration_test.go
git commit -m "feat(engine): add deterministic provider CLI preflight and always-persisted preflight report"
```

---

### Task 8: Add Fan-In Aggregate Failure Classification

**Files:**
- Modify: `internal/attractor/engine/parallel_handlers.go`
- Modify: `internal/attractor/engine/parallel_guardrails_test.go`
- Create: `internal/attractor/engine/fanin_failure_class_test.go`

**Step 1: Add fan-in classification red tests**

```go
func TestFanIn_AllParallelBranchesFail_DeterministicClass(t *testing.T) {}
func TestFanIn_AllParallelBranchesFail_TransientClass(t *testing.T) {}
```

Rules:

1. If all branches fail and any branch is `transient_infra`, aggregate class is `transient_infra`.
2. Otherwise aggregate class is `deterministic`.

**Step 2: Run tests to verify red**

```bash
out="$(go test ./internal/attractor/engine -run '^TestFanIn_AllParallelBranchesFail_DeterministicClass$|^TestFanIn_AllParallelBranchesFail_TransientClass$' -count=1 2>&1)"
printf '%s\n' "$out"
! grep -q '\[no tests to run\]' <<<"$out"
```

Expected: FAIL before fan-in metadata implementation.

**Step 3: Implement aggregate classification in `FanInHandler.Execute`**

```go
if !okWinner {
    cls := aggregateBranchFailureClass(results)
    return runtime.Outcome{
        Status:        runtime.StatusFail,
        FailureReason: "all parallel branches failed",
        Meta: map[string]any{
            "failure_class":     cls,
            "failure_signature": "parallel_all_failed|" + cls,
        },
        ContextUpdates: map[string]any{"failure_class": cls},
    }, nil
}
```

**Step 4: Run focused tests**

```bash
go test ./internal/attractor/engine -run '^TestFanIn_AllParallelBranchesFail_DeterministicClass$|^TestFanIn_AllParallelBranchesFail_TransientClass$|^TestRun_DeterministicFailure_DoesNotRetry$' -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/attractor/engine/parallel_handlers.go internal/attractor/engine/fanin_failure_class_test.go internal/attractor/engine/parallel_guardrails_test.go
git commit -m "fix(engine): classify all-fail fan-in outcomes for downstream retry gating"
```

---

### Task 9: Harden Guardrail Matrix + Update Runbook + Run Full Gate

**Files:**
- Modify: `scripts/e2e-guardrail-matrix.sh`
- Modify: `docs/strongdm/attractor/README.md`

**Step 1: Harden guardrail script against false-pass test selection**

In script, add a helper that fails on `[no tests to run]` for targeted test commands:

```bash
run_test_checked() {
  local label="$1"
  shift
  echo "$label"
  local out
  out="$($@ 2>&1)"
  printf '%s\n' "$out"
  if grep -q '\[no tests to run\]' <<<"$out"; then
    echo "guardrail matrix: FAIL (no tests executed)"
    exit 1
  fi
}
```

**Step 2: Extend matrix with new reliability checks**

Add targeted checks for:

1. `TestRun_DeterministicFailure_DoesNotRetry`
2. `TestFanIn_AllParallelBranchesFail_DeterministicClass`
3. `TestRunWithConfig_WritesPreflightReport_Always`
4. `TestClassifyProviderCLIError_AnthropicStreamJSONRequiresVerbose`

**Step 3: Update runbook semantics**

Document:

1. Stage retry is class-gated (`transient_infra` retries by default).
2. Provider CLI failures are source-classified with `failure_class`/`failure_signature`.
3. Deterministic CLI preflight runs before CXDB and writes `preflight_report.json` on pass/fail.
4. Fan-in all-fail outcomes carry aggregated failure class.

**Step 4: Run full verification gate**

```bash
go test ./cmd/kilroy -count=1
go test ./internal/attractor/runtime -count=1
go test ./internal/attractor/engine -count=1
go test ./internal/llm/providers/... -count=1
bash scripts/e2e-guardrail-matrix.sh
```

Expected: all PASS.

**Step 5: Commit**

```bash
git add scripts/e2e-guardrail-matrix.sh docs/strongdm/attractor/README.md
git commit -m "docs+tests: enforce class-gated reliability contracts and no-false-pass targeted test gates"
```

---

## Required Green Exit Criteria

Branch is complete only when all are true:

1. Shared failure-policy tests are green.
2. Stage retry-gating tests are green and emit `stage_retry_blocked` for deterministic failures.
3. Provider classifier tests are green and `runCLI` attaches `failure_class` + `failure_signature` on CLI failures.
4. Preflight tests are green, and `RunWithConfig` writes `preflight_report.json` in both pass/fail preflight paths.
5. Fan-in all-fail classification tests are green and fan-in emits class/signature metadata.
6. Baseline suites remain green:
   - `go test ./cmd/kilroy -count=1`
   - `go test ./internal/attractor/runtime -count=1`
   - `go test ./internal/attractor/engine -count=1`
   - `go test ./internal/llm/providers/... -count=1`
7. `bash scripts/e2e-guardrail-matrix.sh` passes and fails fast if any targeted test command runs zero tests.

