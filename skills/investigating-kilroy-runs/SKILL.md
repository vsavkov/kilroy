---
name: investigating-kilroy-runs
description: "Diagnose active, stuck, or failed Kilroy Attractor runs by inspecting run artifacts (`manifest.json`, `live.json`, `checkpoint.json`, `final.json`, `progress.ndjson`), resolving run IDs/log roots, identifying model/provider routing, and isolating failure causes. Includes CXDB operations: launch/probe CXDB, open the CXDB UI, and query run context turns. Use when asked to investigate run status, debug retries/failures, explain model usage, or inspect CXDB-backed event history."
---

# Investigating Kilroy Runs

Use this workflow to inspect a run quickly and produce a precise status report.

## Resolve Run Root

1. Prefer an explicit `--logs-root` path if provided.
2. Otherwise find the newest run under `~/.local/state/kilroy/attractor/runs`.
3. Treat that directory as `RUN_ROOT`.

```bash
RUNS="$HOME/.local/state/kilroy/attractor/runs"
RUN_ID="$(find "$RUNS" -mindepth 1 -maxdepth 1 -type d -printf '%T@ %f\n' | sort -nr | head -n1 | awk '{print $2}')"
RUN_ROOT="$RUNS/$RUN_ID"
echo "$RUN_ID"
```

## CXDB: Launch, UI, and Query

Resolve CXDB fields from run metadata first:

```bash
CXDB_URL="$(jq -r '.cxdb.http_base_url' "$RUN_ROOT/manifest.json")"
CONTEXT_ID="$(jq -r '.cxdb.context_id' "$RUN_ROOT/manifest.json")"
echo "cxdb_url=$CXDB_URL context_id=$CONTEXT_ID"
```

Start/probe CXDB and print UI endpoint:

```bash
./scripts/start-cxdb.sh
./scripts/start-cxdb-ui.sh
```

Open UI in a browser when needed:

```bash
KILROY_CXDB_OPEN_UI=1 ./scripts/start-cxdb-ui.sh
```

Follow run events directly from CXDB (preferred for live runs):

```bash
./kilroy attractor status --logs-root "$RUN_ROOT" --follow --cxdb
./kilroy attractor status --logs-root "$RUN_ROOT" --follow --cxdb --raw
```

Direct HTTP queries (for ad-hoc debugging):

```bash
# Health endpoint may be /healthz even when /health returns 404.
curl -fsS "$CXDB_URL/health" || curl -fsS "$CXDB_URL/healthz"
curl -fsS "$CXDB_URL/v1/contexts"
curl -fsS "$CXDB_URL/v1/contexts/$CONTEXT_ID"
curl -fsS "$CXDB_URL/v1/contexts/$CONTEXT_ID/turns?limit=20"
curl -fsS "$CXDB_URL/v1/contexts/$CONTEXT_ID/turns?view=typed&limit=20"
```

## Read Canonical Files

Inspect files in this order:

1. `manifest.json`: run identity, graph name, repo, worktree, `started_at`.
2. `live.json`: most recent event.
3. `checkpoint.json`: last completed node and failure context.
4. `final.json`: if present, run is finished (`success` or `fail`).
5. `progress.ndjson`: full event timeline.

```bash
sed -n '1,200p' "$RUN_ROOT/manifest.json"
sed -n '1,200p' "$RUN_ROOT/live.json"
[ -f "$RUN_ROOT/checkpoint.json" ] && sed -n '1,200p' "$RUN_ROOT/checkpoint.json"
[ -f "$RUN_ROOT/final.json" ] && sed -n '1,200p' "$RUN_ROOT/final.json"
tail -n 80 "$RUN_ROOT/progress.ndjson"
```

## Determine Current State

Classify run state:

- Running: `final.json` missing and `live.json`/`progress.ndjson` still changing.
- Finished: `final.json` present.
- Likely stalled: no `progress.ndjson` updates for longer than configured stall timeout.

Useful checks:

```bash
ls -la "$RUN_ROOT/final.json"
tail -n 1 "$RUN_ROOT/progress.ndjson"
```

## Identify Models and Providers

Use both static and runtime evidence:

1. Static routing from graph `model_stylesheet` and node classes.
2. Runtime events from `progress.ndjson` (`llm_retry`, `llm_call_*`, provider/model fields).
3. Provider availability from `run_config.json`.

```bash
rg -n 'model_stylesheet|llm_model|llm_provider|class=' "$RUN_ROOT/graph.dot"
rg -n '"event":"llm_|"provider":"|"model":"' "$RUN_ROOT/progress.ndjson"
sed -n '1,220p' "$RUN_ROOT/run_config.json"
```

## Triage Common Failures

- `missing status.json`: codergen node did not emit required status signal.
- `llm retry` with `429`/rate-limit: provider quota or backoff pressure.
- `deterministic_failure_cycle_check`: repeated deterministic failure at same node.
- Toolchain/setup errors: check `setup_command_*` events and stage `stderr.log`.

```bash
rg -n 'missing status.json|llm_retry|deterministic_failure_cycle_check|setup_command_|failure_reason' "$RUN_ROOT/progress.ndjson"
```

## Report Format

Return findings in this order:

1. `run_id`, `run_root`, started time.
2. Current node/state and whether run is still active.
3. Models/providers configured and observed.
4. Top failure signals with exact file references.
5. One next action (continue waiting, resume, or fix specific failure cause).
