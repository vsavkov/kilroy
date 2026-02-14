# Detached Run CWD Independence Hardening Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Eliminate detached-run failures caused by deleted launcher working directories by removing ambient CWD dependencies from process launch and schema compilation paths.

**Architecture:** The fix hardens two layers: process launch boundaries and in-process schema compilation. Detached child processes must start with an explicit stable working directory and absolute run inputs. JSON schema compilation must use in-memory absolute URIs so it never calls `getwd`. Failover logic must classify local bootstrap errors as non-failoverable to prevent provider ping-pong loops.

**Tech Stack:** Go (`cmd/kilroy`, `internal/agent`, `internal/llm`, `internal/attractor/engine`), `github.com/santhosh-tekuri/jsonschema/v5`, existing `go test` suites.

---

## Problem Statement

The run failed with:

- `tool read_file schema: getwd: no such file or directory`
- then repeated provider failover and deterministic cycle-break abort

Evidence:

- `/home/user/.local/state/kilroy/attractor/runs/01KHDAZD6QY9VTFGRMEMJC4GAY/run.out`
- `/home/user/.local/state/kilroy/attractor/runs/01KHDAZD6QY9VTFGRMEMJC4GAY/progress.ndjson`

Root-cause chain:

1. Detached launcher does not set `cmd.Dir`, so child inherits caller CWD (`cmd/kilroy/run_detach.go`).
2. Caller CWD can disappear (ephemeral worktree path removed later).
3. Agent session registration compiles tool JSON schemas (`internal/agent/tool_registry.go`).
4. jsonschema v5 resolves relative schema URLs via `filepath.Abs(...)`, which depends on `getwd`.
5. `getwd` fails when inherited CWD no longer exists, causing schema compile failure before tool execution.

## Why This Is the Right Solution

1. **Architectural ownership:** CWD validity is runtime infrastructure, not dotfile content. Fix belongs in launcher/runtime code, not graph generation skills.
2. **Defense in depth:** Explicit `cmd.Dir` + absolute run paths prevent process-level CWD drift; in-memory absolute schema URIs prevent library-level CWD lookup.
3. **Correct failure semantics:** Local deterministic bootstrap failures should not trigger provider failover. This avoids noisy failover loops and surfaces actionable errors faster.
4. **Low-risk, high-leverage:** Changes are localized to launch path, schema compile helpers, and failover classification, with regression tests at each layer.

---

### Task 1: Add a CWD-Independent Schema Compiler Utility

**Files:**
- Create: `internal/jsonschemautil/compile.go`
- Test: `internal/jsonschemautil/compile_test.go`

**Step 1: Write the failing test (deleted CWD regression)**

```go
func TestCompileMapSchema_DoesNotDependOnProcessCWD(t *testing.T) {
    temp := t.TempDir()
    oldWD, err := os.Getwd()
    if err != nil {
        t.Fatalf("getwd: %v", err)
    }
    if err := os.Chdir(temp); err != nil {
        t.Fatalf("chdir temp: %v", err)
    }
    t.Cleanup(func() { _ = os.Chdir(oldWD) })
    if err := os.RemoveAll(temp); err != nil {
        t.Fatalf("remove temp: %v", err)
    }

    schema := map[string]any{
        "type": "object",
        "properties": map[string]any{
            "file_path": map[string]any{"type": "string"},
        },
        "required": []string{"file_path"},
    }

    compiled, err := CompileMapSchema(schema, nil)
    if err != nil {
        t.Fatalf("CompileMapSchema: %v", err)
    }
    if err := compiled.Validate(map[string]any{"file_path": "x"}); err != nil {
        t.Fatalf("validate: %v", err)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/jsonschemautil -run TestCompileMapSchema_DoesNotDependOnProcessCWD -v`  
Expected: FAIL (function does not exist yet).

**Step 3: Write minimal implementation**

```go
package jsonschemautil

import (
    "bytes"
    "encoding/json"

    "github.com/santhosh-tekuri/jsonschema/v5"
)

func CompileMapSchema(schema map[string]any, draft jsonschema.Draft) (*jsonschema.Schema, error) {
    c := jsonschema.NewCompiler()
    if draft != nil {
        c.Draft = draft
    }
    b, err := json.Marshal(schema)
    if err != nil {
        return nil, err
    }
    uri := "mem://schema/inline.json"
    if err := c.AddResource(uri, bytes.NewReader(b)); err != nil {
        return nil, err
    }
    return c.Compile(uri)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/jsonschemautil -v`  
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/jsonschemautil/compile.go internal/jsonschemautil/compile_test.go
git commit -m "attractor: add cwd-independent in-memory jsonschema compiler utility"
```

---

### Task 2: Migrate Existing Schema Compilation Call Sites

**Files:**
- Modify: `internal/agent/tool_registry.go`
- Modify: `internal/llm/generate.go`
- Modify: `internal/llm/generate_object.go`
- Modify: `internal/agent/tool_registry_test.go`
- Modify: `internal/llm/generate_object_test.go`
- Modify: `internal/llm/generate_test.go`

**Step 1: Write failing tests at current call sites**

Add tests that compile schemas while CWD is deleted:

```go
func TestCompileSchema_DoesNotDependOnGetwd(t *testing.T) { ... }
```

Place one in:

- `internal/agent/tool_registry_test.go` for `compileSchema`
- `internal/llm/generate_object_test.go` for `compileJSONSchema`
- `internal/llm/generate_test.go` for `compileSchema`

**Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/agent -run DoesNotDependOnGetwd -v
go test ./internal/llm -run DoesNotDependOnGetwd -v
```

Expected: FAIL with a `getwd`-related compile error before migration.

**Step 3: Replace relative `"schema.json"` compile flow with shared utility**

In each callsite, replace:

```go
c.AddResource("schema.json", ...)
return c.Compile("schema.json")
```

with:

```go
return jsonschemautil.CompileMapSchema(params, jsonschema.Draft2020) // llm
return jsonschemautil.CompileMapSchema(params, nil)                  // agent
return jsonschemautil.CompileMapSchema(schema, jsonschema.Draft2020) // llm GenerateObject compileJSONSchema
```

Keep behavior otherwise identical.

**Step 4: Run tests to verify pass**

Run:

```bash
go test ./internal/agent ./internal/llm -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/agent/tool_registry.go internal/agent/tool_registry_test.go internal/llm/generate.go internal/llm/generate_object.go internal/llm/generate_test.go internal/llm/generate_object_test.go
git commit -m "attractor: remove getwd-dependent schema compilation from agent and llm paths"
```

---

### Task 3: Harden Detached Launch Process Boundary

**Files:**
- Modify: `cmd/kilroy/main.go`
- Modify: `cmd/kilroy/run_detach.go`
- Create: `cmd/kilroy/detach_paths.go`
- Create: `cmd/kilroy/detach_paths_test.go`
- Create: `cmd/kilroy/run_detach_test.go`

**Step 1: Write failing tests**

Add path normalization test:

```go
func TestResolveDetachedPaths_ConvertsRelativeToAbsolute(t *testing.T) {
    tempDir := t.TempDir()
    oldWD, err := os.Getwd()
    if err != nil {
        t.Fatalf("getwd: %v", err)
    }
    if err := os.Chdir(tempDir); err != nil {
        t.Fatalf("chdir temp: %v", err)
    }
    t.Cleanup(func() { _ = os.Chdir(oldWD) })
    gotGraph, gotConfig, gotLogs, err := resolveDetachedPaths("g.dot", "run.yaml", "logs")
    // assert filepath.IsAbs(...) for all
}
```

Add detach launcher working-dir test by injecting exec command factory:

```go
func TestLaunchDetached_SetsCmdDirToLogsRoot(t *testing.T) {
    // use injected command that writes pwd to file and exits
    // assert pwd == logsRoot
}
```

**Step 2: Run tests to verify fail**

Run:

```bash
go test ./cmd/kilroy -run "ResolveDetachedPaths|LaunchDetached_SetsCmdDirToLogsRoot" -v
```

Expected: FAIL (helpers/injection not implemented).

**Step 3: Implement launch hardening**

1. Add `resolveDetachedPaths(...)` helper and call it in detached branch of `mainRunAttractor`.
2. In `launchDetached`, set:

```go
cmd.Dir = logsRoot
```

3. Add small test seam in `run_detach.go`:

```go
var detachedExecCommand = exec.Command
```

Use it in place of direct `exec.Command` for deterministic tests.

**Step 4: Run tests to verify pass**

Run:

```bash
go test ./cmd/kilroy -run "ResolveDetachedPaths|LaunchDetached_SetsCmdDirToLogsRoot|Detached" -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/kilroy/main.go cmd/kilroy/run_detach.go cmd/kilroy/detach_paths.go cmd/kilroy/detach_paths_test.go cmd/kilroy/run_detach_test.go
git commit -m "attractor: normalize detached run paths and pin child cwd to logs root"
```

---

### Task 4: Fix Failover Classification for Local Bootstrap Errors

**Files:**
- Modify: `internal/attractor/engine/codergen_router.go`
- Modify: `internal/attractor/engine/codergen_failover_test.go`

**Step 1: Write failing test**

```go
func TestShouldFailoverLLMError_GetwdBootstrapErrorDoesNotFailover(t *testing.T) {
    err := fmt.Errorf("tool read_file schema: getwd: no such file or directory")
    if shouldFailoverLLMError(err) {
        t.Fatalf("getwd bootstrap errors should not trigger failover")
    }
}
```

**Step 2: Run test to verify failure**

Run: `go test ./internal/attractor/engine -run GetwdBootstrapErrorDoesNotFailover -v`  
Expected: FAIL.

**Step 3: Implement classification**

Add local deterministic bootstrap detection in `shouldFailoverLLMError`:

```go
func isLocalBootstrapError(err error) bool {
    s := strings.ToLower(strings.TrimSpace(err.Error()))
    return strings.Contains(s, "getwd: no such file or directory") ||
        strings.Contains(s, "tool read_file schema: getwd:")
}
```

and short-circuit:

```go
if isLocalBootstrapError(err) {
    return false
}
```

**Step 4: Run tests to verify pass**

Run: `go test ./internal/attractor/engine -run ShouldFailoverLLMError -v`  
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/attractor/engine/codergen_router.go internal/attractor/engine/codergen_failover_test.go
git commit -m "attractor: prevent provider failover on local getwd/bootstrap errors"
```

---

### Task 5: Add End-to-End Regression for Deleted Launcher CWD

**Files:**
- Modify: `cmd/kilroy/main_detach_test.go`

**Step 1: Write failing integration test**

Add:

```go
func TestAttractorRun_DetachedMode_DeletedLauncherCWDDoesNotAbortRun(t *testing.T) {
    // launch detached from temp launcher dir
    // remove launcher dir immediately after launch returns
    // wait for final.json and assert status != fail due to getwd/schema bootstrap
}
```

Use `run.out` assertions to detect old failure signature:

```go
if strings.Contains(runOut, "tool read_file schema: getwd: no such file or directory") { t.Fatalf(...) }
```

**Step 2: Run test to verify failure on old behavior**

Run:

```bash
go test ./cmd/kilroy -run DeletedLauncherCWDDoesNotAbortRun -v
```

Expected: FAIL before fixes; PASS after Tasks 1-4.

**Step 3: Stabilize test graph/config if needed**

Use a deterministic small graph and `--allow-test-shim` to avoid external provider flake.

**Step 4: Run targeted suite**

Run:

```bash
go test ./cmd/kilroy ./internal/agent ./internal/llm ./internal/attractor/engine -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/kilroy/main_detach_test.go
git commit -m "attractor: add regression test for detached runs with deleted launcher cwd"
```

---

### Task 6: Final Verification and Clean Merge Safety

**Files:**
- Modify: none (verification only)

**Step 1: Run full test suite**

Run: `go test ./...`

Expected: PASS.

**Step 2: Validate no unintended artifact drift**

Run:

```bash
git status --short
git diff --name-only
```

Expected: Only intended files changed.

**Step 3: Optional live smoke**

Run:

```bash
./kilroy attractor run --detach --graph <abs_graph> --config <abs_config>
./kilroy attractor status --logs-root <logs_root> --json
```

Expected: No `getwd` schema bootstrap failure in `run.out`.

**Step 4: Document operational note**

Add one short note in reliability docs if needed:

- Detached run launcher now pins child CWD and absolute inputs.

**Step 5: Commit (docs-only, if added)**

```bash
git add docs/strongdm/attractor/reliability-troubleshooting.md
git commit -m "docs: note detached cwd hardening and getwd failure remediation"
```

---

## Verification Checklist

Run all:

```bash
go test ./cmd/kilroy -v
go test ./internal/jsonschemautil -v
go test ./internal/agent -v
go test ./internal/llm -v
go test ./internal/attractor/engine -v
go test ./...
```

Success criteria:

1. No test failures.
2. No run aborts with `tool read_file schema: getwd: no such file or directory`.
3. Failover logic does not switch providers for local bootstrap/CWD errors.
4. Detached runs work regardless of launcher CWD lifecycle.
