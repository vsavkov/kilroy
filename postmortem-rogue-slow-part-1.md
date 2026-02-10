# Postmortem: rogue-slow (Part 1)

## Scope

This postmortem is strictly scoped to **one run only**:

- `run_id`: `01KH3DN4STSZ3MVKB58YY6RE57`
- `logs_root`: `/home/user/.local/state/kilroy/attractor/runs/01KH3DN4STSZ3MVKB58YY6RE57`
- `graph`: `graph.dot` inside that run directory
- `config`: `run_config.json` inside that run directory

Everything below was derived from artifacts under that single run directory (including `parallel/fanout/*` subdirectories). No other run IDs were used.

## Executive Summary

The run did not complete and never produced `final.json`.

This was not a single "slow provider" issue. It was a layered reliability incident:

1. The run spent most of its wall-clock time in top-level scaffold retry loops before fanout.
2. Fanout started, but branch outcomes diverged:
3. One branch succeeded but kept emitting stale heartbeats long after completion.
4. One branch did real work but failed due status-contract mismatch (`status.json` vs `.ai/status.json`).
5. Two branches repeatedly hit turn limits and check-node failure propagation errors.
6. Top-level orchestration never converged to terminal state and remained in `fanout` with stale heartbeats until manually killed.

## Run Facts (Observed)

- Start: `2026-02-10T09:22:03.948370657Z`
- Last top-level event: `2026-02-10T17:10:00.10969594Z`
- Wall-clock span in artifacts: `7h 47m`
- Pre-fanout span: `6h 59m 45s` (`~89.7%` of observed runtime)
- Fanout span before last top-level event: `48m 10s` (`~10.3%`)
- Top-level `final.json`: missing
- Top-level `live.json`: `stage_heartbeat` on `verify_scaffold` (stale vs current node `fanout`)
- Top-level `checkpoint.json` current node: `check_scaffold` (stale once fanout began)
- Runtime policy (`run_config.json`):
- `stage_timeout_ms=0`
- `stall_timeout_ms=1800000` (30m)
- `stall_check_interval_ms=5000`
- Providers (`run_config.json`):
- CLI: `anthropic`, `openai`
- API: `kimi`, `zai`
- Force-models (`manifest.json`):
- `anthropic=opus`
- `openai=gpt-5.3-codex`
- Preflight (`preflight_report.json`): `pass=16`, `warn=0`, `fail=0`, including prompt probes for forced CLI models.

## Timeline (UTC)

1. `09:22:03` run starts.
2. `09:22` to `10:27` start/toolchain/spec/analysis/architecture eventually pass.
3. `10:27:08` top-level enters scaffold phase.
4. `10:27` to `16:21` scaffold loop repeats (`impl_scaffold -> verify_scaffold -> check_scaffold`) 36 times.
5. `16:21:49` fanout starts.
6. `16:42:11` branch `01-impl_dungeon` reaches check success.
7. `16:38` to `17:06` branch `02-impl_combat_items` alternates success work and deterministic fail (`missing status.json`).
8. `16:35` to `17:05` branch `03-impl_monsters` repeats turn-limit retries.
9. `16:44` to `17:09` branch `04-impl_player_io` repeats turn-limit retries.
10. `17:10:00` last top-level event is still a stale heartbeat; no terminal artifact.
11. Run was manually killed later (PID dead, no `final.json`).

## Deep Dive

## Layer 0: Observable Incident

- Top-level entered `fanout` but never emitted `stage_attempt_end` for `fanout`.
- Top-level generated `7944` heartbeats after last non-heartbeat event.
- Last non-heartbeat top-level event remained `fanout` start.
- No top-level terminal artifact (`final.json` missing).

## Layer 1: Proximate Causes by Phase

### 1.1 Pre-fanout was dominated by scaffold churn

- Top-level `edge_selected` counts:
- `impl_scaffold -> verify_scaffold`: `36`
- `verify_scaffold -> check_scaffold`: `36`
- `check_scaffold -> impl_scaffold` (`outcome=fail`, retry): `35`
- Stage outcomes:
- `verify_scaffold fail`: `35`
- `check_scaffold fail`: `35`
- `impl_scaffold success`: `33`
- Attempt duration stats (derived from start/end timestamps):
- `impl_scaffold`: min `171s`, median `294s`, max `1188s`, total `~3.62h`
- `verify_scaffold`: min `132s`, median `230s`, max `299s`, total `~2.25h`
- Failure-reason fragmentation:
- `verify_scaffold` fail reasons: `35` total, `31` unique strings.
- Deterministic cycle checks never reached breaker threshold:
- `deterministic_failure_cycle_check` max `signature_count=2`, `signature_limit=3`.

Implication: repeated equivalent failure class (`deterministic`) was represented as too many distinct strings/signatures, so cycle protection never tripped.

### 1.2 Fanout branch outcomes were mixed and non-convergent

#### Branch `01-impl_dungeon` (OpenAI CLI, forced `gpt-5.3-codex`)

- Stage end statuses: `impl success`, `verify success`, `check success`.
- Last non-heartbeat: `2026-02-10T16:42:11.818881199Z`.
- Heartbeats continued until `2026-02-10T17:09:53.173452309Z` (`1661s`, `140` extra heartbeats).
- `final.json` missing in branch directory.

Interpretation: branch completed work but telemetry kept emitting stale heartbeats.

#### Branch `02-impl_combat_items` (Anthropic CLI, forced `opus`)

- Branch warnings show force override applied on both impl and verify nodes.
- CLI invocations were long and successful from process perspective:
- `impl_combat_items`: `534529ms`, exit `0`.
- `verify_combat_items`: `268578ms`, exit `0`.
- Event logs report substantial work and known cost:
- `impl_combat_items` result: `104` turns, `~$4.8338`.
- `verify_combat_items` result: `44` turns, `~$3.0504`.
- Branch failed anyway due status-contract mismatch:
- Stage failure reason: `missing status.json (auto_status=false)`.
- Worktree contains `.ai/status.json` with `outcome=success`.
- Check node then failed with generic validator error:
- `failure_reason must be non-empty when status="fail"`.

Interpretation: meaningful work was produced but lost by a strict/ambiguous status-file contract.

#### Branch `03-impl_monsters` (ZAI API, `glm-4.7`)

- Repeated deterministic retry-blocked reasons:
- `impl_monsters`: `turn limit reached (max_turns=25)` x4
- `verify_monsters`: `turn limit reached (max_turns=8)` x4
- `check_monsters` failed x4 with:
- `failure_reason must be non-empty when status="fail"`.
- Tool errors in stage event stream include malformed patch format:
- `apply_patch: expected '*** Begin Patch'`
- `apply_patch: unexpected line: "--- src/monsters.rs"`
- Quantified stage-stream errors:
- `impl_monsters`: `4` tool-call errors (`3` apply-patch format, `1` missing `.ai/rogue_architecture.md`)
- `verify_monsters`: `1` tool-call error (compile error output), then turn limit
- Branch `checkpoint.json` has `node_retries={}` despite multiple retries.

Interpretation: retries were happening, but with poor tool adaptation and broken failure propagation/telemetry.

#### Branch `04-impl_player_io` (Kimi API, `kimi-k2.5`)

- Repeated deterministic retry-blocked reasons:
- `impl_player_io`: `turn limit reached (max_turns=25)` x2
- `verify_player_io`: `turn limit reached (max_turns=8)` x1
- `check_player_io` failed with:
- `failure_reason must be non-empty when status="fail"`.
- Event stream shows repeated apply-patch format failures plus large compile-error outputs.
- Quantified stage-stream errors:
- `impl_player_io`: `6` tool-call errors (`2` apply-patch format, `3` compile-error outputs, `1` missing `.ai/rogue_architecture.md`)
- `verify_player_io` never reached a complete attempt (only `SESSION_START` + `USER_INPUT` before run kill).
- Branch `checkpoint.json` also has `node_retries={}`.

Interpretation: long attempt budgets + low patch-tool compliance + incomplete verify execution produced low progress and weak diagnostics.

### 1.3 Top-level telemetry diverged from real active work

- After `fanout` start, top-level heartbeat node IDs were stale pre-fanout nodes:
- `impl_scaffold` (`3514`)
- `verify_scaffold` (`3470`)
- plus older nodes (`impl_architecture`, `verify_architecture`, etc.)
- No fanout child node IDs appeared in top-level progress.
- Top-level checkpoint stayed on pre-fanout node (`check_scaffold`).

Interpretation: parent telemetry cannot reliably tell whether fanout is active, complete, or deadlocked.

## Layer 2: Mechanism Failures

### 2.1 Terminalization and lifecycle contract failure

- Parent started fanout but never emitted fanout end/terminal run state.
- Child branches had their own terminal-like states, but parent never finalized.
- Missing `final.json` at both top-level and branches confirms absent end-state convergence.

### 2.2 Heartbeat source integrity failure

- Heartbeats continued for nodes that were no longer current.
- Successful branch (`01`) still emitted heartbeats after last non-heartbeat by ~27 minutes.
- This creates "hung vs working" ambiguity and blocks reliable operators/tools.

### 2.3 Status contract mismatch (`status.json` vs `.ai/status.json`)

- Branch `02` produced `.ai/status.json` success while engine required stage-local `status.json`.
- Because `auto_status=false`, engine interpreted successful work as deterministic fail.
- Contract mismatch cascaded into check-node generic fail reason.

### 2.4 Check-node failure_reason propagation bug

- Multiple branches failed checks with:
- `failure_reason must be non-empty when status="fail"`.
- This strips true causal reason and degrades retry policy and diagnostics.

### 2.5 Deterministic loop control ineffective where needed

- Top-level cycle checks existed but failed to trip because signatures fragmented.
- Fanout branches (`03`, `04`) had no observed deterministic cycle-check events.
- Back-edges (`check_* -> impl_*`) enabled repeat loops without strong branch-level circuit breakers.

### 2.6 Context and checkpoint quality degradation

- Top-level checkpoint `completed_nodes`: `123` entries, only `12` unique.
- Branch prompts included large repeated context payloads:
- `03 impl USER_INPUT`: `8497` chars
- `04 impl USER_INPUT`: `9417` chars
- Branch checkpoints for heavy-retry branches had `node_retries={}` despite observed retries.

### 2.7 Provider/tool adaptation gap

- Kimi/ZAI streams repeatedly emitted malformed `apply_patch` calls using non-required patch format.
- This consumed turns without producing valid edits and increased probability of turn-limit exits.
- OpenAI path showed frequent schema fallback warnings:
- `codex schema validation failed; retrying once without --output-schema` x`81`
- `codex state-db discrepancy...fresh state root` x`3`

## Layer 3: Source Causes (What Needs to Improve)

1. Engine currently optimizes for per-node local retries, not end-to-end run convergence under fanout.
2. Failure-reason normalization and canonicalization are too weak to support cycle breakers.
3. Status handoff contract across providers/tools is brittle and under-specified.
4. Telemetry model (heartbeat/checkpoint/live/progress) allows stale data to masquerade as liveness.
5. Prompt assembly keeps appending duplicated history instead of bounded/canonical state summaries.
6. Patch-tool contracts are not robustly enforced/adapted for all providers in mixed runs.

## Improvement Plan (Exhaustive, Multi-Level)

## P0: Must Fix First

1. **Guaranteed terminalization contract**
- Parent run must always write `final.json` on stop/fail/kill path.
- Fanout parent must join/cancel children deterministically.
- Acceptance: this scenario ends with explicit top-level terminal state, never orphaned running-without-final.

2. **Heartbeat correctness**
- Emit heartbeats only for active attempts/nodes.
- On stage completion, old heartbeat stream must stop immediately.
- Acceptance: zero heartbeats after branch/node completion timestamp.

3. **Status contract hardening**
- Accept one canonical status location or provide strict adapter logic.
- If `.ai/status.json` is produced by provider workflow, ingest/normalize it or fail with clear actionable contract error.
- Acceptance: branch `02`-style run maps produced status to deterministic engine outcome without false fail.

4. **Failure_reason propagation fix**
- Check nodes must carry upstream causal reason verbatim on fail.
- Remove generic validator fail path unless accompanied by original cause.
- Acceptance: no `failure_reason must be non-empty...` unless accompanied by exact source failure context.

5. **Branch-level deterministic circuit breaker**
- Add cycle checks for fanout branches (not just top-level serial nodes).
- Enforce bounded retries on repeating `(node, canonical_signature)` patterns.
- Acceptance: `03`/`04` style turn-limit loops terminate within configured cap.

6. **Canonical failure signature normalization**
- Normalize numeric noise, JSON formatting variance, and long free-text differences into stable signatures.
- Acceptance: scaffold-style 31/35 unique reasons collapse to a small set and breaker threshold triggers.

## P1: High Value

1. **Child activity surfaced in parent telemetry**
- Mirror child stage start/end/retry/fail summaries into top-level `progress.ndjson`.
- Acceptance: operator can determine branch health from top-level stream only.

2. **Checkpoint fidelity**
- Ensure `node_retries` reflects actual retries, especially in branch runs.
- Deduplicate `completed_nodes` and cap size growth.
- Acceptance: retries in `03`/`04` are reflected in checkpoint artifacts.

3. **Prompt-context compaction**
- Pass deduplicated compact state summaries instead of replaying entire repeated node history.
- Acceptance: USER_INPUT context shrinks materially without loss of required control-plane info.

4. **Patch tool adaptation safeguards**
- Detect malformed patch attempts and auto-rewrite to required format where safe.
- Provide provider-specific patch examples in system/tool prompts.
- Acceptance: significant drop in `apply_patch` format errors for API providers.

## P2: Strategic Hardening

1. **Attempt-indexed artifact layout**
- Preserve per-attempt logs/artifacts to avoid overwrite ambiguity.
- Acceptance: forensic replay of each retry without ambiguity.

2. **Unified cost/time accounting**
- Aggregate known provider costs and attempt durations into run-level summary artifact.
- Acceptance: operator can get total cost/time without manual branch parsing.

3. **Run doctor command**
- Add `diagnose --run-id` to report stale heartbeats, missing terminalization, contract mismatches, and loop signatures.
- Acceptance: this incident class detected automatically.

## Validation Matrix (Regression Tests)

1. Fanout lifecycle test: one success branch, one status-contract mismatch, one turn-limit branch.
- Assert parent terminalization and deterministic fail classification.

2. Heartbeat integrity test.
- Assert no post-completion heartbeats from completed nodes.

3. Status contract compatibility test.
- Feed `.ai/status.json` only and verify deterministic/expected ingestion behavior.

4. Failure-reason propagation test.
- Force upstream fail and assert check-node reason retains original cause.

5. Canonical signature + breaker test.
- Feed semantically identical but text-varied fail reasons and assert breaker activation.

6. Branch checkpoint consistency test.
- Assert retry counts appear in `node_retries` and completed-node duplication is bounded.

## Appendix: Run-Scoped Evidence Highlights

- Top-level event counts:
- `stage_heartbeat`: `39166`
- `stage_attempt_start`: `124`
- `stage_attempt_end`: `123`
- `edge_selected`: `123`
- `warning`: `84`
- `deterministic_failure_cycle_check`: `77`
- `stage_retry_blocked`: `40`

- Top-level warning counts:
- `codex schema validation failed...`: `81`
- `codex state-db discrepancy...`: `3`

- Fanout start and staleness:
- fanout start: `2026-02-10T16:21:49.878354873Z`
- last non-heartbeat top-level event: same timestamp
- heartbeats after that: `7944` over `2890s`

- Branch status snapshot:
- `01-impl_dungeon`: success path complete, stale heartbeats continued.
- `02-impl_combat_items`: meaningful work + cost, failed on missing stage status contract.
- `03-impl_monsters`: repeated turn-limit retries, check-node reason loss.
- `04-impl_player_io`: repeated turn-limit retries, patch-format and compile-error churn.

- Known explicit cost in artifacts:
- `02 impl`: `$4.83383475`
- `02 verify`: `$3.05038200`
- subtotal observed: `$7.88421675` (other providers not reporting cost in same format)
