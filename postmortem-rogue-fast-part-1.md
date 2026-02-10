# Postmortem: rogue-fast (Part 1)

## Scope

This postmortem is strictly scoped to **one run only**:

- `run_id`: `rogue-fast-20260210T131933Z`
- `logs_root`: `/home/user/.local/state/kilroy/attractor/runs/rogue-fast-20260210T131933Z`
- `graph`: `demo/rogue/rogue_fast.dot`
- `config`: `demo/rogue/run-fast.yaml` (materialized as `run_config.json` in the run dir)

Everything below was derived from this run's own artifacts (top-level + its `parallel/fanout/*` subdirectories). No other run IDs were used in analysis.

## Executive Summary

The run did not fail fast and did not complete. It entered a long-lived internal failure loop after fanout:

1. Two fanout branches completed successfully.
2. Two fanout branches hit a stall watchdog timeout once.
3. After that first timeout, those branches repeatedly failed **immediately** with the same timeout reason thousands of times.
4. Top-level orchestration did not converge to a terminal outcome (`final.json` never written).
5. Process remained alive for hours until manually killed.

This is not a single timeout-tuning problem. It is a multi-layer reliability failure across timeout semantics, retry/loop control, parallel orchestration, and run-finalization behavior.

## Run Facts (Observed)

- Started: `2026-02-10T13:20:08.018873872Z`
- Top-level `final.json`: missing
- Top-level `run.out`: empty
- Top-level latest `live.json` event: `stall_watchdog_timeout` at `2026-02-10T14:14:43.518233644Z`
- Top-level `progress.ndjson` ends at the same timestamp (`2026-02-10T14:14:43.518233644Z`)
- Runtime policy (from this run's `run_config.json`):
  - `stage_timeout_ms: 0`
  - `stall_timeout_ms: 1800000` (30 min)
  - `stall_check_interval_ms: 5000`
- Graph-level command timeouts (from this run's `graph.dot`):
  - `default_command_timeout_ms=300000` (5 min)
  - `max_command_timeout_ms=1800000` (30 min)
- Preflight in this run passed (`11 pass, 0 warn, 0 fail`).

## Timeline (UTC)

- `13:20:08` Run starts.
- `13:20` to `13:44` Top-level phases succeed through `check_scaffold`.
- `13:44:39` Top-level enters `fanout`.
- `13:55:36` Branch `01-impl_dungeon` completes success path.
- `14:12:42` Branch `02-impl_combat_items` completes success path.
- `14:14:43` First stall timeout appears in both remaining branches:
  - `03-impl_monsters`
  - `04-impl_player_io`
- `14:14:43` Top-level records `stall_watchdog_timeout` and stops emitting top-level progress.
- `14:14:43` to `17:09:46` failing branches continue high-rate retry/fail loops.
- Run process was manually terminated later (no top-level terminal artifact was written).

## What Worked

- Environment and provider preflight checks were healthy for this run.
- Early serial phases completed (`start` -> `check_scaffold`).
- Parallel branch `01-impl_dungeon` finished cleanly:
  - 3 starts, 3 ends, all success.
- Parallel branch `02-impl_combat_items` finished cleanly:
  - 3 starts, 3 ends, all success.

## What Failed (Layered)

### Layer 1: Observable Failure

Two branches entered endless deterministic looping with no convergence:

- `03-impl_monsters`
  - `stage_attempt_end`: `retry=5298`, `fail=2649`
  - first stall: `2026-02-10T14:14:43.526272472Z`
  - last event: `2026-02-10T17:09:45.998623734Z`
- `04-impl_player_io`
  - `stage_attempt_end`: `retry=5312`, `fail=2656`
  - first stall: `2026-02-10T14:14:43.526570801Z`
  - last event: `2026-02-10T17:09:46.04862514Z`

Loop cadence was ~1 cycle every ~3.9 seconds for ~2h55m after first stall.

### Layer 2: Mechanism Failures

#### 2.1 Timeout state appears sticky across attempts (inference from artifacts)

After first 30-minute stall, subsequent attempts in failing branches ended almost immediately with the same failure reason:

- `stall watchdog timeout after 30m0s with no progress`

Evidence:

- Tail sequences repeat rapid pattern:
  - `impl_* start -> impl_* retry(stall)`
  - `verify_* start -> verify_* retry(stall)`
  - `check_* start -> check_* fail(stall)`
- Stage backend artifacts did **not** refresh like a real fresh model attempt:
  - `events.ndjson` stayed at first-timeout mtime (`06:14 PST`)
  - `response.md` and `provider_used.json` were missing on failing branches
  - only `prompt.md` and `status.json` kept updating during loops

Inference: once stall fired, the same timeout condition was effectively reused on subsequent attempts instead of being reset/re-evaluated per new attempt.

#### 2.2 Retry edges allow unbounded deterministic cycles

The graph routes branch failure as:

- `check_monsters -> impl_monsters [condition="outcome=fail", label="retry"]`
- `check_player_io -> impl_player_io [condition="outcome=fail", label="retry"]`

There is no branch-level breaker for repeated deterministic signatures. With immediate fail conditions, these edges produce endless cycles.

#### 2.3 Top-level orchestration lost convergence while children kept running

Top-level state froze at fanout-timeout:

- top-level `progress.ndjson` stopped at `14:14:43Z`
- top-level `checkpoint.json` remained on `check_scaffold`
- no top-level `final.json`

But child branch logs continued for hours. This indicates missing/incorrect cancellation and terminal-state propagation between parent run and parallel children.

#### 2.4 Checkpoint quality degraded under hot-loop conditions

In failing branches, checkpoints showed:

- `completed_nodes` lengths near 8k with repeated triplets
- `node_retries: {}`
- context fields largely `null`

This erodes debuggability exactly when reliability problems occur.

### Layer 3: Source-Level Design Gaps

1. Stall watchdog semantics are not attempt-scoped enough.
2. Deterministic failure signatures are not centrally circuit-broken in cyclic edges.
3. Parallel parent/child cancellation/finalization contracts are not strong enough.
4. Artifact model is attempt-overwriting rather than attempt-indexed (harder to diagnose).
5. Top-level telemetry does not include child branch activity, creating false "idle" visibility.

## Why This Matters

- The system can burn hours/cost while making zero forward progress.
- Operators can see a "running" process with no terminal outcome.
- Postmortem and triage become harder due partial/stale artifacts and checkpoint drift.
- Reliability regressions can hide behind timeout tuning unless root mechanisms are fixed.

## Improvement Plan (Exhaustive, Multi-Level)

## A. Immediate Containment (Stop Bleeding)

1. Add deterministic-loop circuit breaker at engine level.
- Detect repeated `(node_id, failure_signature)` cycles over threshold in a moving window.
- Force terminal branch failure with explicit reason once threshold exceeded.
- Acceptance: failing branch above is capped within bounded retries, not 7k+ loops.

2. On parent `stall_watchdog_timeout`, cancel all active fanout children and finalize parent.
- Ensure `final.json` is always written when run is no longer making progress.
- Acceptance: reproducing this scenario yields top-level terminal fail within minutes, no orphan child loops.

3. Add run-level liveness guard.
- If top-level progress is idle but child logs are mutating, mark state as "child-active" not idle.
- Acceptance: no false idle diagnosis when fanout children are active.

## B. Fix Source of Timeouts (Not Just Response Policy)

1. Reset stall watchdog state per stage attempt.
- New attempt must not inherit previous attempt's stale watchdog deadline/state.
- Acceptance: after a timeout, next attempt is a genuine attempt (new backend events), not immediate timeout replay.

2. Distinguish timeout classes accurately.
- Separate `true provider stall` from `engine scheduling/heartbeat stall` from `state-machine deadlock`.
- Avoid collapsing to generic deterministic API failure signature (`api_deterministic|api|unknown`).
- Acceptance: failure signatures are specific and route to appropriate recovery behavior.

3. Ensure retry does real work.
- Retries should require evidence of fresh backend invocation (`provider_used.json` and new `events.ndjson`/`response.md`) unless explicitly short-circuited with reason code.
- Acceptance: no retry loop where only `prompt.md`/`status.json` changes.

## C. Parallel Orchestration Correctness

1. Parent-child lifecycle contract.
- Parent must track child run states as first-class and aggregate into top-level progress stream.
- If any child enters terminal fail beyond budget, parent chooses retry/fail path deterministically.

2. Fanout fan-in completion criteria hardening.
- Explicitly require 4/4 branch terminal states before fanin decision, with bounded wait and deterministic failure path.

3. Cancellation propagation guarantees.
- On parent stop/fail/timeout: cancel children, wait join, emit terminal artifact.

## D. Retry and Graph Semantics

1. Add graph/engine policy to prevent unbounded fail-edge cycling for deterministic errors.
- Could be edge-level option or global policy keyed by failure class/signature.

2. Optional auto-`loop_restart=true` recommendation/lint for long back-edges.
- Especially in fanout branches with expensive nodes.

3. Incorporate branch-level retry budgets independent from node-local `max_retries`.
- Current pattern can bypass effective limits via `check_* -> impl_*` cycles.

## E. Observability and Forensics

1. Attempt-indexed artifacts.
- Store per-attempt directories (`impl_monsters/attempt-0001/...`) instead of overwriting shared files.
- Preserve exact evidence for each retry.

2. Child events in top-level stream.
- Mirror child event summaries into parent `progress.ndjson` with branch labels.

3. Checkpoint fidelity fixes.
- Ensure branch `node_retries` and context fields are populated even under rapid cycles.
- Prevent unbounded `completed_nodes` duplication from becoming the only trace.

4. Built-in run doctor command.
- Example: `kilroy attractor diagnose --run-id ...` that reports loop signatures, stale artifacts, and missing terminalization.

## F. Testing and Validation Gaps to Close

1. Repro test: fanout with 2 success branches + 2 timeout branches.
- Assert bounded retries and proper terminalization.

2. Repro test: first stall then retry should perform fresh backend attempt.
- Assert new backend artifacts per retry.

3. Repro test: parent timeout must cancel children and write `final.json`.

4. Repro test: deterministic signature repetition triggers circuit breaker.

5. Repro test: top-level progress reflects child activity while in fanout.

## Prioritized Action Backlog

### P0 (must ship first)

1. Deterministic loop circuit breaker.
2. Parent timeout -> child cancellation -> guaranteed finalization.
3. Attempt-scoped watchdog reset.

### P1

1. Branch retry budgets and fail-fast deterministic policy.
2. Top-level aggregation of child events.
3. Attempt-indexed artifact layout.

### P2

1. Better failure signature taxonomy for timeout classes.
2. Checkpoint data integrity under high-frequency loops.
3. `diagnose --run-id` tooling.

## Part 2 Investigation Plan

Part 1 is artifact-driven diagnosis. Part 2 should instrument and reproduce to confirm source-code-level hypotheses:

1. Add debug logs around stall watchdog state lifecycle (create/reset/trigger).
2. Reproduce with minimal synthetic fanout graph.
3. Verify whether timeout state is reused across node transitions.
4. Patch + rerun until branch retries produce fresh backend artifacts.
5. Add regression tests listed above.

## Appendix: Run-Scoped Evidence Highlights

- Top-level stall event: `2026-02-10T14:14:43.518233644Z`
- Branch first stalls:
  - monsters: `2026-02-10T14:14:43.526272472Z`
  - player_io: `2026-02-10T14:14:43.526570801Z`
- Branch totals at stop:
  - monsters: `7947` starts, `7947` ends, `5298 retry`, `2649 fail`
  - player_io: `7968` starts, `7968` ends, `5312 retry`, `2656 fail`
- Successful branches:
  - dungeon: 3/3 success
  - combat_items: 3/3 success
- Top-level terminal artifact absent: `final.json` missing

