# Postmortem: rogue-slow (Part 2)

## Scope and Method

This falsification pass is strictly scoped to the same run as Part 1:

- `run_id`: `01KH3DN4STSZ3MVKB58YY6RE57`
- `logs_root`: `/home/user/.local/state/kilroy/attractor/runs/01KH3DN4STSZ3MVKB58YY6RE57`

Method:

1. Re-check Part 1 conclusions against run artifacts under this run ID only.
2. Trace engine code paths to verify (or disprove) each causal claim.
3. Mark each claim as `Supported`, `Partially Supported`, or `Falsified/Corrected`.

---

## Falsification Matrix

### 1) "The run hung in fanout and never reached terminal state on its own"

**Verdict: Supported**

Evidence:

- Top-level `progress.ndjson` has exactly one `stage_attempt_start` for `fanout` and zero `stage_attempt_end` for `fanout`.
- Last non-heartbeat top-level event is fanout start (`2026-02-10T16:21:49.878354873Z`).
- No `stall_watchdog_timeout` event exists at top level.
- Top-level `final.json` is absent.

Interpretation:

- The run stayed in fanout orchestration until external termination.

---

### 2) "Stale heartbeats were a major liveness false-positive"

**Verdict: Supported (strongly)**

Evidence from run:

- Top-level emitted `7944` heartbeats after fanout start, with no non-heartbeat progress.
- Top-level `live.json` ended on heartbeat for stale node `verify_scaffold`.
- Heartbeats continued for nodes long after their last `stage_attempt_end`:
  - `verify_scaffold`: thousands of post-completion heartbeats.
  - Branch `01-impl_dungeon`: `140` heartbeats after branch's last non-heartbeat event.
  - Branch `02-impl_combat_items`: stale heartbeats from `impl_combat_items` after attempt end.

Static code support:

- `internal/attractor/engine/codergen_router.go:960-989`: heartbeat goroutine exits only on `ctx.Done()`, not on CLI process exit.
- `internal/attractor/engine/progress.go:44`: every appended event updates `lastProgressAt`.
- `internal/attractor/engine/engine.go:1336-1366`: stall watchdog uses global `lastProgressAt`.

Interpretation:

- This is a concrete root-cause class, not just a symptom.

---

### 3) "Parent telemetry diverged from real execution during fanout"

**Verdict: Supported**

Evidence:

- Parent stream after fanout start has only heartbeats from pre-fanout node IDs.
- Parent checkpoint remains `current_node=check_scaffold` and timestamped pre-fanout.
- Branches have their own progress streams under `parallel/fanout/*/progress.ndjson`.

Static code support:

- `internal/attractor/engine/parallel_handlers.go:92-110`: branches run in worker goroutines.
- `internal/attractor/engine/parallel_handlers.go:205-227`: each branch uses its own `Engine` with its own `LogsRoot`.
- No mechanism in parallel handler mirrors branch progress into parent progress stream.

Interpretation:

- Parent-level observability is insufficient for fanout health by itself.

---

### 4) "Status contract mismatch caused branch 02 false failures"

**Verdict: Partially Supported (important correction)**

What is supported in this run:

- Branch `02` repeatedly failed with `missing status.json (auto_status=false)`.
- Same branch worktree contains `.ai/status.json` with success content.
- Branch `02` event stream explicitly references writing/reading `.ai/status.json`.

What is falsified/corrected from Part 1 framing:

- The engine does not simply "require stage-local `status.json` and ignore `.ai/status.json`".
- `internal/attractor/engine/handlers.go:166-171, 232-247` explicitly checks and ingests both:
  - `<worktree>/status.json`
  - `<worktree>/.ai/status.json`
- Tests already cover this path:
  - `internal/attractor/engine/status_json_worktree_test.go:95-175`

Interpretation:

- The failure mode is real in this run, but root cause is not "engine has no `.ai/status.json` support".
- More likely a lifecycle/timing/path mismatch in this run path (needs targeted instrumentation), not a missing feature.

---

### 5) "Check-node failure_reason propagation is broken in fanout branches"

**Verdict: Supported**

Evidence from run:

- Branch check nodes repeatedly fail with `failure_reason must be non-empty when status="fail"`:
  - `check_combat_items`, `check_monsters`, `check_player_io`.
- Branch checkpoints show `context_values.failure_reason = null` and `failure_class = null`.

Static code support:

- Conditional node is pass-through and depends on context:
  - `internal/attractor/engine/handlers.go:124-139`
- Subgraph path does not set `failure_reason`/`failure_class` into context:
  - `internal/attractor/engine/subgraph.go:57-60`
- Main loop does set these fields:
  - `internal/attractor/engine/engine.go:488-490`
- Validator requires non-empty reason on fail/retry:
  - `internal/attractor/runtime/status.go:93-95`
- Engine coercion creates the generic validator message:
  - `internal/attractor/engine/engine.go:839-845`

Interpretation:

- This is a validated causal bug in fanout/subgraph semantics.

---

### 6) "Deterministic cycle protection did not protect fanout branches"

**Verdict: Supported**

Evidence:

- No `deterministic_failure_cycle_check` events in any branch progress stream.
- Branches repeatedly take `check_* -> impl_*` fail edges with deterministic failures.

Static code support:

- Main-loop deterministic cycle logic exists:
  - `internal/attractor/engine/engine.go:492-533`
- Subgraph execution has no equivalent cycle breaker:
  - `internal/attractor/engine/subgraph.go:31-94`

Interpretation:

- Branch loops are currently missing a key safety mechanism present in main loop.

---

### 7) "`node_retries={}` proves checkpoint/retry accounting bug"

**Verdict: Falsified/Corrected**

Why Part 1 overstated this:

- `node_retries` increments only when `canRetry` path is taken (`stage_retry_sleep`).
- In this run, many failures are `stage_retry_blocked` (deterministic/blocked), not `stage_retry_sleep`.

Static code support:

- Increment occurs only in can-retry branch:
  - `internal/attractor/engine/engine.go:953-963`
- Blocked retries do not increment:
  - `internal/attractor/engine/engine.go:972-982`

Run evidence:

- Branches `03` and `04`: multiple `stage_retry_blocked`, zero `stage_retry_sleep`.
- Therefore empty `node_retries` is expected in this run shape.

Interpretation:

- This is not, by itself, a checkpoint corruption signal.

---

### 8) "Missing `final.json` in branch dirs indicates branch terminalization failure"

**Verdict: Falsified/Corrected**

Static code support:

- Branches are executed via `runSubgraphUntil(...)` and return `parallelBranchResult`.
- Subgraph path writes checkpoints but does not persist per-branch `final.json`.
  - `internal/attractor/engine/subgraph.go:61-66`
  - `internal/attractor/engine/parallel_handlers.go:226-243`

Run artifact shape support:

- Branch dirs consistently contain `checkpoint.json`, `live.json`, `progress.ndjson` but not `final.json`.

Interpretation:

- Branch `final.json` absence is expected in current architecture and should not be used as direct failure evidence.

---

### 9) "Top-level deterministic cycle breaker under-triggered due failure-signature fragmentation"

**Verdict: Supported**

Evidence:

- `verify_scaffold` fail count: `35`, unique failure-reason strings: `31`.
- `deterministic_failure_cycle_check` max observed `signature_count=2`, `signature_limit=3`.

Interpretation:

- Part 1 diagnosis is correct: the breaker is present but signatures are too fragmented to trip reliably.

---

### 10) "Provider/tool adaptation gap (apply_patch formatting) contributed to turn-limit churn"

**Verdict: Supported**

Evidence:

- Branch `03` and `04` event streams contain repeated tool errors:
  - `apply_patch: expected '*** Begin Patch'`
  - `apply_patch: unexpected line: "--- ..."`

Static code/config support:

- OpenAI profile instructs `apply_patch (v4a)`:
  - `internal/agent/profile.go:125`
- Run config maps `kimi` and `zai` to `profile_family: "openai"`:
  - `run_config.json` for this run (`llm.providers.kimi.api.profile_family`, `llm.providers.zai.api.profile_family`).

Interpretation:

- The observed errors are real and consistent with profile/tool-contract mismatch risk.

---

### 11) "Large completed_nodes + prompt sizes are root-cause failures"

**Verdict: Partially Supported / Not proven causal**

Evidence:

- Top-level `completed_nodes`: `123` entries, `12` unique.
- Large USER_INPUT payloads are observed in branch event streams.

Correction:

- These are valid pressure indicators, but static analysis here does not prove they were primary failure causes in this run.
- They are secondary risk factors unless tied to a specific failing mechanism.

---

## Revised Causal Chain (After Falsification)

What remains as high-confidence causal chain for this run:

1. Long scaffold loops consumed most wall time.
2. Fanout started, with mixed branch outcomes and repeated fail-edge cycling.
3. Branch subgraph path dropped `failure_reason` context, causing generic check failures.
4. Branch subgraph path lacked deterministic cycle breaker parity with main loop.
5. Heartbeat goroutine lifecycle bug produced stale progress events from completed CLI stages.
6. Global watchdog progress clock was refreshed by stale heartbeats, preventing stall timeout.
7. Parent fanout never completed; run remained non-terminal until manual kill.

---

## Part 1 Corrections Summary

These Part 1 conclusions should be revised:

1. **Branch `final.json` absence** is expected for subgraph branches; not direct evidence of failed finalization.
2. **Empty `node_retries`** in this run shape is expected under `stage_retry_blocked` behavior; not automatically a checkpoint bug.
3. **Status mismatch framing** should be narrowed:
- The run does show real `missing status.json` failures.
- But engine code already supports `.ai/status.json`; root cause is subtler than "no support".

---

## Confidence Notes

- High confidence: heartbeat leak mechanism, subgraph context propagation gap, subgraph cycle-check parity gap.
- Medium confidence: exact trigger path for branch `02` `.ai/status.json` ingestion miss (needs additional instrumentation in `CodergenHandler` status-discovery path to pin sequence-level cause).
