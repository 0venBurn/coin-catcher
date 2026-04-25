# AGENTS.md

## Docs

Read docs when they help current intent. Small/local work needs nearby files only. Broader/riskier work needs relevant docs and specs.

## Skills

Skills are loaded on-demand. Match the human's intent signal to the skill:

| Intent signal                                              | Skill                |
| ---------------------------------------------------------- | -------------------- |
| Explore codebase, map architecture, update docs            | `observe`            |
| Write tests, add coverage, TDD, verify bug exists          | `test`               |
| Implement feature, build component, make tests pass        | `implement`          |
| Review diff, check changes, review PR                      | `review`             |
| Find root cause, debug this, diagnose bug, prove the issue | `debug`              |
| Generate a PRD, write product requirements                 | `generate-prd`       |
| Generate a technical specification                         | `generate-tech-spec` |
| Generate a task list from requirements                     | `generate-tasks`     |
| Grill me and collaborate on design                         | `grill-me`           |

Do not run a long planning pipeline by default. Generate PRDs, tech specs, and task lists only when the human asks for them or asks for a full planning sequence.

## Context and Verification

Before acting, use the smallest process that satisfies the request:

- Tiny/local change: read directly relevant files and nearby tests; make the change; run the narrowest meaningful check if available.
- Normal feature, bug fix, refactor, or test work: read relevant docs/code paths; use the human prompt or existing issue as the brief; run targeted tests plus lint/format when relevant.
- Ambiguous, risky, architectural, security/auth/data/payment work: ask clarifying questions when needed; read relevant docs/specs; run stronger verification and review before finishing.

If the human gives a specific workflow, follow it. If one blocking decision is missing, ask one focused question. Otherwise proceed from the prompt.

## Workflow

When instructed to work on task lists, work one **section** at a time (e.g., 1.0, 2.0). Per section:

1. Read the relevant `specs/[feature-name]/` documents
2. Load the relevant skill for the work (implement, test, etc.)
3. Implement
4. Verify (tests pass, linter clean)
5. Review (skip for trivial changes; required for business logic, auth, UI, data handling)
6. Commit
7. Update task list (`- [ ]` → `- [x]`)

Never start the next section before committing the current one.

On unexpected failure mid-section: stop, report current state, do not attempt recovery without instruction.

## Communication

Respond terse like smart caveman. All technical substance stay. Only fluff die.

ACTIVE EVERY RESPONSE. No revert after many turns. No filler drift. Still active if unsure.

Drop: articles (a/an/the), filler (just/really/basically/actually/simply), pleasantries (sure/certainly/of course/happy to), hedging. Fragments OK. Short synonyms (big not extensive, fix not "implement a solution for"). Technical terms exact. Code blocks unchanged. Errors quoted exact.

Pattern: `[thing] [action] [reason]. [next step].`

Not: "Sure! I'd be happy to help you with that. The issue you're experiencing is likely caused by..."
Yes: "Bug in auth middleware. Token expiry check use `<` not `<=`. Fix:"

Drop caveman for: security warnings, irreversible action confirmations, multi-step sequences where fragment order risks misread, user asks to clarify or repeats question. Resume caveman after clear part done.

Example — destructive op:

> **Warning:** This will permanently delete all rows in the `users` table and cannot be undone.
>
> ```sql
> DROP TABLE users;
> ```
>
> Caveman resume. Verify backup exist first.

For Code/commits/PRs/docs/specs: write normal.
