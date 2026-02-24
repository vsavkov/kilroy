---
name: create-runfile
description: Use when authoring or repairing Kilroy run config YAML/JSON files, including DOT-to-provider backend alignment and runtime policy defaults.
---

# Create Runfile

## Scope

This skill owns run config authoring (`run.yaml` / `run.json`) for `kilroy attractor run` and `resume`.

In scope:
- Building config structure (`version: 1` schema).
- Aligning provider backends with DOT model/provider usage.
- Setting runtime, preflight, modeldb, git, and CXDB defaults.

Out of scope:
- DOT graph authoring and routing design. Use `create-dotfile` for graph work.

## Overview

Core principle:
- Keep execution policy in run config, not in DOT topology.
- Align run config with what the graph needs to execute now.
- Favor explicit, deterministic defaults over implicit behavior.

Default run-config source:
- `skills/create-runfile/reference_run_template.yaml`

## Workflow

1. Collect inputs.
- Read the target DOT graph and detect provider usage (`llm_provider` attrs and `model_stylesheet`).
- Capture user constraints for production/test mode and backend policy.

2. Choose run mode explicitly.
- Production mode: `llm.cli_profile: real` and no test-shim flags.
- Test mode: `llm.cli_profile: test_shim` with shim-compatible provider config.

3. Start from the template and fill required fields.
- Required: `version`, `repo.path`, `cxdb.binary_addr`, `cxdb.http_base_url`, `modeldb.openrouter_model_info_path`.
- Keep absolute paths for repo/modeldb/script entries.

4. Align providers with DOT.
- For every provider used by DOT, set `llm.providers.<provider>.backend` (`api` or `cli`).
- Do not edit DOT to force backend execution strategy.

5. Apply runtime defaults and safety guardrails.
- Set `git.run_branch_prefix`, `git.commit_per_node`, and `git.require_clean` intentionally.
- Keep `runtime_policy` explicit (`stage_timeout_ms`, `stall_timeout_ms`, retry cap).
- Enable `preflight.prompt_probes` and use a non-aggressive timeout baseline for real-provider runs.

6. Preserve local-run robustness.
- In this repo, keep `cxdb.autostart` launcher wiring when generating local CXDB configs.
- Keep artifact/checkpoint hygiene settings where relevant (for example managed tool-cache roots).

7. Validate alignment before handoff.
- Confirm every DOT provider has a run-config backend entry.
- Confirm mode consistency (`real` vs `test_shim`) with intended command flags.
- Confirm config has no unresolved placeholder paths.

## Non-Negotiable Guardrails

- Backend policy lives in run config; do not encode it in DOT structure.
- Do not omit providers that are referenced by the graph.
- Do not use fragile preflight probe timeouts for real-provider runs.
- Do not emit local CXDB configs without `cxdb.autostart` wiring in this repository context.

## References

- `docs/strongdm/attractor/attractor-spec.md`
- `docs/strongdm/attractor/unified-llm-spec.md`
- `README.md`
- `skills/create-runfile/reference_run_template.yaml`
