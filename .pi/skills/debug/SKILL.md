---
name: debug
description: Diagnose and prove the root cause of a bug without fixing production code. Use this skill when the user asks to find root cause, debug, diagnose, prove an issue, explain why a bug happens, or reproduce a failure. If the user asks to fix the bug too, diagnose first, then use the implement skill for the fix.
---

# Debug

Diagnose bugs by proving root cause before any fix is attempted.

## Process

Follow these steps in order.

### 1. Understand failure

Read the bug description and working context carefully. Gather the smallest useful set of files, logs, tests, docs, or commands needed to understand the failing path.

Identify:

- Where the failure occurs
- What expected behavior is
- What actual behavior is
- What input, state, or environment triggers it

### 2. Form one hypothesis

Choose the most likely root cause first. Consider:

- What invariant or assumption is being violated?
- Is this a data issue, logic issue, timing issue, environment issue, or interface mismatch?
- What changed recently in the affected path?

Do not chase multiple hypotheses at once. Commit to one, then prove or disprove it.

### 3. Prove root cause

Use the strongest practical evidence available:

- Targeted unit/integration test
- Existing failing test
- Minimal reproduction
- Runtime trace, log, or command output
- Direct code-path evidence when tests are not practical

If writing a test, consult `@docs/testing.md` first. If docs do not exist, use detected project conventions.

A proof test should be minimal and directly exercise the suspected failure path. It should fail or expose the issue when the hypothesis is correct.

Do not write the fix. Do not refactor.

### 4. Iterate if needed

If evidence disproves the hypothesis, use the new observation to form the next most likely hypothesis and repeat.

Continue until root cause is proven or blocked by missing information/tooling.

### 5. Report

Output concise diagnosis:

```text
ROOT CAUSE: [Brief description]
EVIDENCE: [Test/command/file/line evidence proving it]
NEXT STEP: [Smallest fix direction, or what is blocked]
```

If user asked to fix too, stop after diagnosis and state that implementation should happen through `implement` using the proven cause.

## Constraints

- **Diagnose only.** Do not edit production code or apply fixes in this skill.
- **Do not skip proof.** Use a targeted test, reproduction, command output, logs, or clear code-path evidence.
- **One hypothesis at a time.** Avoid shotgun debugging.
- **No fake verification.** Only cite evidence actually gathered.
