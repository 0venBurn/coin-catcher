---
name: test
description: Write tests for a component, feature, bug fix, or existing behavior without implementing production code. Use this skill when the user asks for TDD tests, tests for a new feature, regression tests that prove a bug, or coverage for existing code. Triggers include "write tests for", "add test coverage", "test this component", "write failing tests", "TDD", or any task where tests are the deliverable rather than the implementation.
---

# Write Tests

Write focused tests that match user intent: TDD for new behavior, regression tests for bugs, or coverage for existing behavior.

## Process

### 1. Read testing conventions

Read `@docs/testing.md` first. If `docs/testing.md` does not exist, note it and use standard conventions for the detected stack (e.g. pytest for Python, Jest/Vitest for TypeScript, built-in test runner for Go).

Do not write test code before understanding project test structure, utilities, naming, and commands.

### 2. Infer expected test result from intent

Use the user's wording and surrounding context:

- If the user asks for **TDD**, **new feature tests**, or **tests before implementation**, write tests that should fail until implementation exists.
- If the user asks to **prove a bug**, **reproduce a bug**, or **write a regression test**, write a test that should fail before the bug fix and pass after the fix.
- If the user asks for **coverage**, **tests for existing behavior**, or **add tests around this code**, write tests that should pass against current behavior.

If expected pass/fail state is unclear and matters, ask one question. Otherwise infer from context and state the expectation when running tests.

### 3. Find existing tests

Search for existing tests covering the subject. If found:

- Extend existing tests before creating new files
- Match existing style, setup, naming, and assertion patterns

If no existing tests are found, create a new test file following `@docs/testing.md` or detected project convention.

### 4. Write focused tests

Cover cases specified by the user. When relevant, include:

| Case type       | What to cover                                         |
| --------------- | ----------------------------------------------------- |
| **Happy path**  | Expected successful behavior                          |
| **Error cases** | Known failure paths, thrown errors, rejected promises |
| **Edge cases**  | Boundaries, empty inputs, nulls, extremes             |

Each test should be self-contained and clearly named so failure output explains the behavior.

Keep tests focused on the requested behavior. Do not add broad unrelated coverage.

### 5. Run tests

Run tests using the command documented in `@docs/testing.md` or the closest project-specific command.

Confirm result matches intent:

- TDD/new feature tests fail for missing behavior.
- Bug reproduction tests fail before fix, proving the bug.
- Coverage tests pass against existing behavior.

If result does not match intent, investigate and fix the test until it accurately represents the requested behavior.

### 6. Commit only when required

If current workflow or human asks for commits, commit with a descriptive message following project convention. Check `@docs/conventions.md` if unsure.

Example:

```text
test: add [component] tests
```

## Constraints

- **Tests only.** Do not implement production code.
- **Intent controls expected result.** Do not force failing tests for coverage work; do not force passing tests for TDD work.
- **Extend before creating.** Prefer existing test files when appropriate.
- **Read docs first.** Do not assume test runner or project utilities when docs or nearby tests exist.
- **No fake verification.** Only report test results that were actually run.
