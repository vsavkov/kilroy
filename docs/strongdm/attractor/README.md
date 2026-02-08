# Attractor 

This repository contains nlspecs to build your own version of Attractor to create your own software factory.

Although bringing your own agentic loop and unified LLM SDK is not required to build your own Attractor, we highly recommend controlling the stack so you have a strong foundation.

## Specs

- [Attractor Specification](./attractor-spec.md)
- [Coding Agent Loop Specification](./coding-agent-loop-spec.md)
- [Unified LLM Client Specification](./unified-llm-spec.md)

## Runbook Notes

- Canonical `status.json` contract:
  - `status` values `fail` and `retry` must include a non-empty `failure_reason`.
  - Legacy worktree payloads that only provide `outcome` + `details` are normalized by the runtime decoder, but emitters should still write canonical `failure_reason` directly.
- OpenAI codex CLI invocation:
  - Default args use `codex exec --json --sandbox workspace-write ...`.
  - Deprecated `--ask-for-approval` is intentionally not used.
- Codex schema behavior:
  - Structured output schema is strict (`required: ["final","summary"]`, `additionalProperties: false`).
  - If codex rejects schema validation (`invalid_json_schema`-class errors), Attractor retries once without `--output-schema` and records fallback metadata in stage artifacts.
- Loop safety:
  - Use `loop_restart=true` on retry-loop edges that jump back to earlier stages.
  - Set graph-level `max_restarts` to bound cycle count and prevent unbounded runs.
