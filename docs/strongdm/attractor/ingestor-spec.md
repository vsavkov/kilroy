# Ingestor v1 Specification

**The Ingestor** converts English requirements into validated `.dot` pipeline files for Kilroy's Attractor engine. It bridges the gap between human intent and machine-executable pipelines by invoking Claude Code with the `english-to-dotfile` skill.

---

## 1. Input

### 1.1 Requirements Text

A positional argument containing English-language requirements. Ranges from a single vague sentence to a reference to a detailed spec file:

```
"solitaire plz"
"Build a Go CLI link checker with robots.txt support and JSON output"
"Build DTTF per the spec at demo/dttf/dttf-v1.md"
```

Multiple positional words are joined with spaces.

### 1.2 Skill File

A markdown file (default: `skills/english-to-dotfile/SKILL.md`) containing instructions that teach the LLM how to produce valid `.dot` pipeline files. The skill is appended to Claude Code's system prompt via `--append-system-prompt`, preserving Claude Code's built-in tool instructions.

### 1.3 Repository Context

The ingestor gives Claude Code read access to the repository root (default: current directory) via `--add-dir`. This gives the LLM access to spec files, existing code, and project structure referenced in the requirements. Relative paths are resolved to absolute before passing to Claude.

---

## 2. Output

### 2.1 Format

A valid DOT digraph conforming to Kilroy's Attractor DSL. The output starts with `digraph` and ends with the closing `}`.

### 2.2 Delivery

- If `--output` is specified: written to the given file path. A confirmation message is printed to stderr: `wrote <path> (<N> bytes)`.
- If `--output` is omitted: printed to stdout.

### 2.3 Validation

By default, the output is validated through `engine.Prepare()` before delivery. Validation checks DOT syntax, node shapes, edge conditions, and graph structure. Validation diagnostics are emitted as warnings to stderr. Validation can be skipped with `--no-validate`.

---

## 3. Pipeline

```
Requirements text (English)
    |
    v
Claude Code CLI invocation (interactive)
    claude \
      --model <model> \
      --max-turns <n> \
      --dangerously-skip-permissions \
      --append-system-prompt <skill content> \
      --add-dir <repo path> \
      "<ingest prompt with requirements>"
    |
    v
Claude writes pipeline.dot to its working directory (a temp dir)
    |
    v
Kilroy reads pipeline.dot after Claude exits
    |
    v
engine.Prepare() — structural validation
    |
    v
.dot file written to disk or stdout
```

### 3.1 Claude Code Invocation

The ingestor spawns Claude Code as an interactive subprocess. Claude runs in a temporary working directory and writes `pipeline.dot` there. Key flags:

| Flag | Value | Purpose |
|------|-------|---------|
| `--model` | Configurable (default: `claude-sonnet-4-5`) | LLM model for generation |
| `--max-turns` | `15` (default) | Max agentic turns |
| `--dangerously-skip-permissions` | — | Skip interactive permission prompts |
| `--append-system-prompt` | Skill file content | Loads skill as system prompt addition |
| `--add-dir` | Repository root (absolute) | Gives Claude read access to the repo |

The ingest prompt (from `ingest_prompt.tmpl`) is passed as the final positional argument. It instructs Claude to follow the skill exactly and write the `.dot` file to `pipeline.dot`.

The executable defaults to `claude` but can be overridden via the `KILROY_CLAUDE_PATH` environment variable.

Stdin, stdout, and stderr are connected to the terminal for interactive use. The invocation has a 15-minute context timeout.

### 3.2 Output Collection

After Claude exits, the ingestor reads `pipeline.dot` from the temporary working directory. The content is trimmed and validated. The temp directory is cleaned up via `defer os.RemoveAll`.

### 3.3 Validation

When enabled (default), the extracted `.dot` content is parsed and validated by `engine.Prepare()`. This catches:
- DOT syntax errors
- Invalid node shapes
- Missing required graph attributes
- Malformed edge conditions
- Structural issues (missing start/exit nodes, disconnected subgraphs)

Validation errors cause the command to exit with a non-zero status. Validation warnings are emitted to stderr but do not prevent output.

---

## 4. Architecture

### 4.1 Language

Go. Part of the `kilroy` binary.

### 4.2 Package Structure

| Package | Files | Purpose |
|---------|-------|---------|
| `cmd/kilroy` | `ingest.go` | CLI arg parsing, `attractorIngest()` entry point, `runIngest()` orchestrator |
| `internal/attractor/ingest` | `extract.go` | `ExtractDigraph()` — digraph extraction (standalone utility) |
| `internal/attractor/ingest` | `ingest.go` | `Options`, `Result`, `buildCLIArgs()`, `Run()` — core invocation logic |
| `internal/attractor/ingest` | `ingest_prompt.tmpl` | Embedded prompt template for Claude |

### 4.3 Data Types

```go
// CLI-level options (cmd/kilroy)
type ingestOptions struct {
    requirements string
    outputPath   string
    model        string
    skillPath    string
    repoPath     string
    validate     bool
    maxTurns     int
}

// Package-level options (internal/attractor/ingest)
type Options struct {
    Requirements string
    SkillPath    string
    Model        string
    RepoPath     string
    Validate     bool
    MaxTurns     int
}

// Ingestion result
type Result struct {
    DotContent string
    Warnings   []string
}
```

### 4.4 API Surface

```go
// Core pipeline
func Run(ctx context.Context, opts Options) (*Result, error)

// Digraph extraction (also usable standalone)
func ExtractDigraph(text string) (string, error)
```

---

## 5. CLI Reference

```
kilroy attractor ingest [flags] <requirements>

Flags:
    -o, --output <path>     Output .dot file path (default: stdout)
    --model <model>         LLM model ID (default: claude-sonnet-4-5)
    --skill <path>          Path to skill .md file (default: auto-detect)
    --repo <path>           Repository root (default: current directory)
    --max-turns <n>         Max agentic turns for Claude (default: 15)
    --no-validate           Skip .dot validation
```

### 5.1 Skill Auto-Detection

When `--skill` is not specified, the ingestor looks for `skills/english-to-dotfile/SKILL.md` relative to the repository root. If found, it is used automatically. If not found, no skill is appended and the LLM operates without the dotfile generation instructions.

### 5.2 Examples

```bash
# Vague input, output to stdout
kilroy attractor ingest "solitaire plz"

# Spec reference, output to file
kilroy attractor ingest --output pipeline.dot "Build DTTF per the spec at demo/dttf/dttf-v1.md"

# Custom model, skip validation
kilroy attractor ingest --model claude-opus-4-6 --no-validate "Build a REST API"

# Custom skill file
kilroy attractor ingest --skill /path/to/custom-skill.md "Build a chat app"
```

---

## 6. Error Handling

| Condition | Behavior |
|-----------|----------|
| Missing requirements text | Exit 1, print usage |
| Unknown flag | Exit 1, print error and usage |
| Skill file not found | Exit 1, print path and error |
| Claude Code invocation fails | Exit 1, print error |
| Claude did not write `pipeline.dot` | Exit 1, print error |
| `pipeline.dot` is empty | Exit 1, print error |
| Validation fails | Exit 1, print validation error. Result struct still contains `DotContent` for programmatic callers |
| Validation warnings | Print warnings to stderr, continue normally |
| Context timeout (15 min) | Exit 1, Claude Code process killed |

---

## 7. Environment Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `KILROY_CLAUDE_PATH` | `claude` | Path to the Claude Code CLI executable |

---

## 8. Dependencies

### 8.1 External

- **Claude Code CLI** (`claude`): Must be installed and configured with valid API credentials. Invoked as a subprocess.

### 8.2 Internal

- **`internal/attractor/engine`**: `Prepare()` for `.dot` validation.
- **`skills/english-to-dotfile/SKILL.md`**: The skill document that teaches the LLM to generate valid pipelines.

---

## 9. Integration with Attractor

The ingestor produces `.dot` files that feed directly into the Attractor engine:

```bash
# Step 1: Ingest requirements into a pipeline
kilroy attractor ingest --output pipeline.dot "Build DTTF per the spec at demo/dttf/dttf-v1.md"

# Step 2: Execute the pipeline
kilroy attractor run --graph pipeline.dot --config run.yaml
```

The validation step ensures the generated `.dot` file is accepted by `engine.Prepare()`, the same function used by `kilroy attractor validate` and `kilroy attractor run`.

---

## 10. Not in v1

- Retry on generation failure (re-invoke Claude Code with feedback)
- Pipeline diffing (compare two generated `.dot` files)
- Multi-skill composition (combining multiple skill files)
- Cost estimation or token counting
