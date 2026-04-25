---
name: implement
description: Implement a feature, bug fix, refactor, or code change, then verify before finishing. Use this skill when the user asks to implement, add, build, change, refactor, make tests pass, or complete task-list work.
---

# Implement

Implement the requested change with enough context, minimal ceremony, and real verification.

## Process

### 1. Understand intent

Use the human prompt, issue, failing test, or current task-list section as the brief.

Identify:

- Requested outcome
- Files or behavior likely affected
- Tests or checks that prove completion
- Any blocking ambiguity

Ask one focused question only when a missing decision would change the implementation. Otherwise proceed.

### 2. Gather context

Read enough context for the request:

- For tiny/local changes, read directly relevant files and nearby tests.
- For normal features, bug fixes, refactors, or test-backed work, read relevant docs, code paths, and tests.
- For risky or broad work, read relevant `docs/` and `specs/[feature-name]/` before editing.

Prefer these docs when present and relevant:

- `docs/testing.md`
- `docs/system_patterns.md`
- `docs/conventions.md`
- feature-specific docs or specs

If docs do not exist, note it and use detected project conventions plus nearby patterns.

### 3. Implement change

Make the smallest correct change that satisfies the brief.

Follow discovered project patterns for:

- file placement and naming
- data flow and boundaries
- error handling
- tests and fixtures
- formatting and style

Do not rewrite unrelated code. Do not broaden scope without human approval.

### 4. Verify

Run checks that prove the change works:

- For tiny/local changes, run the narrowest meaningful check if available; otherwise explain manual inspection.
- For normal code changes, run targeted tests for touched behavior; run lint/format when documented or relevant.
- For risky or broad changes, run stronger relevant test coverage, lint, formatter, and review the diff before finishing.

If verification fails because of the implementation, fix the implementation and rerun relevant checks.

Do not modify tests only to make them pass unless requested behavior legitimately changed expected output.

### 5. Review before finish

Check before responding:

- [ ] Relevant tests/checks passed, or skipped with reason
- [ ] Lint/format run when required or relevant
- [ ] Code follows project conventions and nearby patterns
- [ ] No debug code, stray logs, or temporary comments remain
- [ ] Scope stayed within requested change

For business logic, auth, UI, data handling, or other non-trivial changes, review own diff before final response.

### 6. Task-list sections only

When working from a task list section, follow `AGENTS.md` section workflow:

1. Read relevant `specs/[feature-name]/` documents
2. Implement current section only
3. Verify
4. Review when required
5. Commit
6. Mark section complete (`- [ ]` → `- [x]`)

Never start next section before current section commit.

On unexpected failure mid-section, stop and report current state. Do not attempt recovery without instruction.

## Constraints

- **Enough context first.** Read docs/code appropriate to the request before editing.
- **No planning ceremony by default.** PRDs, tech specs, and task lists are only for explicit human request.
- **No fake verification.** Only claim checks passed when actually run.
- **No unrelated edits.** Keep change focused on requested outcome.
