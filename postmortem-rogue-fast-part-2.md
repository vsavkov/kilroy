# Postmortem: rogue-fast (Part 2)

## Scope and Method

This investigation is strictly scoped to run:

- `run_id`: `rogue-fast-20260210T131933Z`
- `logs_root`: `~/.local/state/kilroy/attractor/runs/rogue-fast-20260210T131933Z`

Method used: **falsification-first**. I explicitly tried to disprove the Part 1 hypotheses via source tracing and run-artifact correlation.

## What Part 2 Disproved

## 1) "Provider outage / 429 caused the failure" — **DISPROVED**

Evidence:

- Branch impl event streams show sustained activity for the full 30-minute window before cancellation:
  - `parallel/fanout/03-impl_monsters/impl_monsters/events.ndjson`: `324` events
  - `parallel/fanout/04-impl_player_io/impl_player_io/events.ndjson`: `308` events
- Both sessions end at exactly watchdog-cancel time with explicit cancellation:
  - `ERROR ... context canceled`
  - followed by `SESSION_END`
- No `llm_failover`, `llm_retry`, or `stage_heartbeat` events were emitted in this run’s progress streams.

Interpretation: branches were actively executing tool calls and edits; they were not hard-failing from provider-side 429/outage behavior.

## 2) "Sticky timeout state was reused across retries" — **DISPROVED (as primary cause)**

What looked like sticky timeout was actually reused **canceled run context**:

- Parent watchdog canceled run context at `2026-02-10T14:14:43.518233644Z`.
- After that point, branch nodes repeatedly failed immediately with the same failure reason because every branch attempt inherited the already-canceled context.
- The repeated reason string (`stall watchdog timeout after 30m0s with no progress`) came from `context.Cause(ctx)` override in `executeNode`, not a stale per-attempt timeout clock.

## 3) "The branches were idle for 30 minutes" — **DISPROVED**

Branches were busy in `events.ndjson` during that interval.
What was idle was **engine progress accounting** at the parent level.

## Corrected Root Cause (Multi-Layer)

## Root cause A: Parent watchdog had no visibility into real branch activity

Top-level progress stalled from:

- `stage_attempt_start fanout` at `13:44:39.968510161Z`
- to `stall_watchdog_timeout` at `14:14:43.518233644Z`

Meanwhile branch sessions were actively producing LLM/tool events.

Why this happened in code:

- Watchdog uses `lastProgressAt` (`internal/attractor/engine/engine.go:1336-1367`), which is updated only by `appendProgress` (`internal/attractor/engine/progress.go:42-45`).
- For API agent-loop codergen, session events are captured to `events.ndjson`, but **do not call `appendProgress`** (`internal/attractor/engine/codergen_router.go:235-279`).
- Heartbeats exist only in CLI execution path (`internal/attractor/engine/codergen_router.go:960-989`), not API agent-loop path.
- Fanout branch engines write progress to branch logs, not parent logs.

Net: parent observed "no progress" for 30 minutes and canceled, despite active child work.

## Root cause B: Subgraph loop ignores canceled context and keeps cycling

After cancellation, branches should have stopped. They didn’t.

Why this happened in code:

- `runSubgraphUntil` (`internal/attractor/engine/subgraph.go:31-94`) lacks `runContextError(ctx)` checks in loop boundaries.
- It keeps executing `impl -> verify -> check -> impl` cycles using already-canceled context.
- Main `runLoop` has these guards (`internal/attractor/engine/engine.go:384-386`, `476-478`), but subgraph path does not.

Observed result:

- Branch 03: `2649` repeated fail cycles
- Branch 04: `2656` repeated fail cycles
- Until manual process kill.

## Root cause C: No deterministic-cycle breaker in subgraph path

Main run loop has deterministic failure-cycle protection (`internal/attractor/engine/engine.go:492-533`).
Subgraph path has none.

This allowed canceled-context failure signatures to repeat indefinitely in branch loops.

## Root cause D: Misclassification of cancellation as deterministic API error

Failed branch status metadata shows:

- `failure_class=deterministic`
- `failure_signature=api_deterministic|api|unknown`

But the underlying event was run cancellation (`context canceled` with cause `stall watchdog timeout ...`).
This classification is misleading and feeds wrong retry/cycle semantics.

## Additional Finding (Not causal alone, but high leverage)

`rogue_fast.dot` intentionally mixes providers/models across parallel branches:

- Branch A/B: `kimi-k2.5`
- Branch C/D: `glm-4.7` on ZAI

See run artifact graph:
- `~/.local/state/kilroy/attractor/runs/rogue-fast-20260210T131933Z/graph.dot` (`impl_monsters` and `impl_player_io` explicitly set `llm_model="glm-4.7"`, `llm_provider="zai"`).

Observed timing aligns with this split:

- Kimi branches finished before 30m watchdog window.
- ZAI branches were still actively working at 30m and got canceled by parent watchdog.

This is not the core engine bug, but it increased exposure to the watchdog-accounting flaw.

## Why top-level never finalized

- Parallel node (`fanout`) did not return because worker goroutines never converged (`wg.Wait()` in `internal/attractor/engine/parallel_handlers.go:109`).
- Therefore main run loop never advanced beyond fanout stage.
- No `final.json` was written for the top-level run.

## Prioritized Fixes (Engineering Backlog)

## P0 (must ship first)

1. **API agent-loop heartbeat/progress plumbing**
- Emit `appendProgress` periodically from API agent-loop event stream (not only CLI path).
- Include per-node elapsed time and event counters.

2. **Parent progress propagation from fanout branches**
- Child branch progress must update parent liveness (or parent watchdog must subscribe to child activity).

3. **Context-cancel guard in subgraph loop**
- Add `runContextError(ctx)` checks in `runSubgraphUntil` at loop top and post-node execution.
- Return immediately on cancel.

4. **Deterministic-cycle breaker in subgraph path**
- Reuse the main-loop signature tracker for subgraph execution.

## P1

1. **Cancellation classification cleanup**
- Classify cancellation/timeouts from canceled context as internal cancellation class, not `api_deterministic|api|unknown`.

2. **Subgraph context fidelity parity**
- Set `failure_reason` in subgraph context updates (currently only outcome/preferred_label are set).

3. **Parallel stage artifact resilience**
- Persist branch snapshots incrementally so parent fanout dir is diagnosable even if orchestration never returns.

## Required Regression Tests

1. `parallel` + watchdog + active API event stream:
- Ensure active branch event stream prevents false parent stall timeout.

2. `runSubgraphUntil` cancellation behavior:
- Cancel context; assert branch exits quickly with terminal fail, no hot loop.

3. Subgraph deterministic-cycle breaker:
- Induce repeated deterministic branch failure; assert bounded attempts and terminalization.

4. Finalization guarantee:
- In parallel cancellation scenario, assert top-level terminal artifact is written.

## Investigation Verdict

The primary failure was **engine orchestration correctness**, not provider reliability.

Specifically:

- Parent liveness accounting missed real child progress,
- watchdog canceled active work,
- subgraph execution ignored cancellation and lacked cycle protection,
- producing hours-long fail loops without terminalization.

