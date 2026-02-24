---
name: create-dotfile
description: Use when authoring or repairing Kilroy Attractor DOT graphs from requirements, with template-first topology, routing guardrails, and validator-clean output.
---

# Create Dotfile

## Scope

This skill owns DOT graph authoring and repair for Attractor pipelines.

In scope:
- Turning requirements/spec/DoD into a runnable `.dot` graph.
- Defining topology, node prompts, routing, model assignments, and validation behavior.
- Enforcing DOT-specific guardrails and validator compatibility.

Out of scope:
- Run config (`run.yaml` / `run.json`) authoring and backend policy details. Use `create-runfile` for that.

## Overview

Core principle:
- Prefer validated template topology over ad-hoc graph design.
- Compose prompt text from project evidence; do not copy stale boilerplate.
- Optimize for reliable execution and recoverability, not novelty.

Default topology source:
- `skills/create-dotfile/reference_template.dot`

Model defaults source:
- `skills/create-dotfile/preferences.yaml`

## Workflow

1. Determine mode and hard constraints.
- If non-interactive/programmatic, do not ask follow-up questions.
- Extract explicit constraints (`no fanout`, model/provider requirements, deliverable paths).

2. Gather repo evidence.
- Read the authoritative spec/DoD sources if provided.
- Use repo docs and files to resolve ambiguity before making assumptions.

3. Choose topology from template first.
- Start from `reference_template.dot` for node shapes, routing, and loop structure.
- If user says `no fanout` or `single path`, remove fan-out/fan-in branch families.

4. Set model/provider resolution in `model_stylesheet`.
- Ensure every `shape=box` node resolves provider + model via attrs or stylesheet.
- Keep backend choice (`cli` vs `api`) out of DOT; backend belongs in run config.

5. Compose node prompts and handoffs.
- Every `shape=box` prompt must include both `$KILROY_STAGE_STATUS_PATH` and `$KILROY_STAGE_STATUS_FALLBACK_PATH`.
- Require explicit success/fail/retry behavior. For fail/retry include `failure_reason` and `details` (and `failure_class` where applicable).
- Keep `.ai/*` producer/consumer paths exact; no filename drift.
- `shape=parallelogram` nodes must use `tool_command`.

6. Enforce routing guardrails.
- Do not bypass actionable outcomes with unconditional pass-through edges.
- For nodes with conditional edges, include one unconditional fallback edge.
- Use only supported condition operators: `=`, `!=`, `&&`.
- Use `loop_restart=true` only for `context.failure_class=transient_infra`.

7. Preserve authoritative text contracts.
- If user explicitly provides goal/spec/DoD text, keep it verbatim (DOT-escape only).
- `expand_spec` must include the full user input verbatim in a delimited block.

8. Validate and repair before emit.
- Verify no unresolved placeholders (`DEFAULT_MODEL`, etc.).
- Run syntax + semantic validation loops, applying minimal fixes until clean.

## Non-Negotiable Guardrails

- Programmatic output is DOT only (`digraph ... }`), no markdown fences or sentinel text.
- `shape=diamond` nodes route outcomes only; do not attach execution prompts.
- Keep prerequisite/tool gates real: route success/failure explicitly.
- Add deterministic checks for explicit deliverable paths named in requirements.
- For semantic verify stages, include a content-addressable `failure_signature` when failing repeated acceptance checks.

## References

- `docs/strongdm/attractor/ingestor-spec.md`
- `docs/strongdm/attractor/attractor-spec.md`
- `docs/strongdm/attractor/coding-agent-loop-spec.md`
- `skills/create-dotfile/reference_template.dot`
- `skills/create-dotfile/preferences.yaml`
