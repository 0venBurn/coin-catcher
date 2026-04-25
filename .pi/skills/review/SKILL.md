---
name: review
description: Review code changes for critical issues only — bugs, performance, security, correctness, and pattern violations. Use this skill when the user wants a code review of a diff, branch, or set of changes. Triggers include "review this", "review the diff", "check my changes", "review this PR", or any request to evaluate code changes before merging. Always go deep into surrounding context, not just the diff itself. Do NOT use for general code exploration or implementation tasks.
---

# Review

A focused code review process targeting critical issues only. Style preferences and non-critical nitpicks are out of scope.

## Process

### 1. Get Changes — Read the diff

Choose review target from user intent:

- Branch review: `git diff main...HEAD` (substitute base ref if not `main`)
- Staged review: `git diff --staged`
- Unstaged review: `git diff`
- File review: diff/read the specified files

If unclear, default to branch review when on a feature branch; otherwise ask which target to review.

### 2. Understand Context — Go beyond the diff

Do not review the diff in isolation. Read the surrounding code to understand:

- What does the changed code interact with?
- What invariants or assumptions does the broader system rely on?
- How does data flow through the affected area?

Also read `@docs/` to understand established patterns and conventions before evaluating whether changes conform to them. If `docs/` does not exist, note it and review against standard conventions for the detected stack.

### 3. Review — Critical issues only

Flag issues in these categories only:

| Category           | What to look for                                                |
| ------------------ | --------------------------------------------------------------- |
| **Bugs**           | Logic errors, null/undefined handling, edge cases               |
| **Performance**    | N+1 queries, inefficient algorithms, memory leaks               |
| **Security**       | SQL injection, XSS, auth bypasses, data exposure                |
| **Correctness**    | Business logic errors, data integrity violations                |
| **Pattern Breaks** | Violations of established architectural patterns or conventions |

If an issue doesn't fall into one of these categories, do not raise it.

## Output Format

For each critical issue found:

```
[SEVERITY][CATEGORY] Brief title
Location: file:line
Issue: What is wrong and why it matters.
```

Severity: `HIGH`, `MEDIUM`, or `LOW`. Only include `LOW` when still critical enough to block/repair.

If no critical issues are found, say so plainly. Do not pad the review.

## Constraints

- **Critical issues only.** No style comments, no suggestions, no "consider doing X instead."
- **Context is mandatory.** Do not flag something as a pattern break without first verifying what the pattern actually is.
- **Be specific.** Vague concerns ("this might be slow") are not reviews — point to the exact code and explain why.
