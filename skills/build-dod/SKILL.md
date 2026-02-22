---
name: build-dod
description: Use when converting a spec, requirements document, or goal statement into a Definition of Done with acceptance criteria and paired verification steps
---

# Build DoD

A DoD converts a spec into pass/fail gates. Its power is in **verification steps** — checks that prove each criterion is met by testing the delivered artifact directly.

## Core Principle

Every acceptance criterion is paired with a verification step that catches the specific failure mode it guards against.

## Process

1. Read the full spec
2. List deliverables — the artifacts that exist when done
3. Write acceptance criteria — one observable assertion per row
4. Pair each AC with a verification step that tests the delivered artifact
5. Crosscheck — confirm each verification would catch its AC being violated

## Acceptance Criteria

Each AC is a single, testable assertion using observable language: "exists", "returns", "displays", "produces", "exits 0".

Group by concern (e.g. Build, Output, Behavior, Integration). Number hierarchically: AC-1.1, AC-1.2, AC-2.1.

## Verification Steps

**Pair every AC with its verification in the same table row.** This ensures every criterion has a check and every check maps to a criterion.

**Prefer deterministic checks** — commands that exit 0 on pass, non-zero on fail.

**Test the delivered artifact directly:**
- Browser app → serve it and confirm it loads and runs
- CLI tool → invoke it and check exit code and output
- Library → import it and call its public API
- Data file → validate its schema and contents

**Verify outputs exist and have expected properties.** A source file that references an output is evidence of intent; confirm the output itself is present and valid. When one artifact references another, verify both the reference and the referenced artifact's existence (e.g. confirm a config mentions a data file AND confirm the data file is present).

**For checks that require judgment**, write a concrete semantic verification with:
- The question to answer
- The expected answer
- The evidence to examine (file paths, commands, artifacts)

## The Crosscheck

After writing all AC/verification pairs, review each row:

1. Confirm the verification catches the failure mode the AC guards against
2. Confirm the verification tests the delivered artifact
3. Look for semantic checks that can become deterministic — convert them

## Output Format

```markdown
# [Project] — Definition of Done

## Scope

### In Scope
[What the deliverable covers]

### Out of Scope
[Explicit exclusions]

### Assumptions
[Prerequisites and environment]

## Deliverables

| Artifact | Location | Description |
|----------|----------|-------------|
| ... | ... | ... |

## Acceptance Criteria

### [Concern Area]

| ID | Criterion | Verification |
|----|-----------|--------------|
| AC-N.M | [Observable assertion] | `command` or semantic: Q → A via [evidence] |
```
