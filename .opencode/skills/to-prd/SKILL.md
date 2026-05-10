---
name: to-prd
description: Generate a Product Requirements Document from the current conversation context and write it to docs/prds/ with sequential numbering. Use when a feature spec has emerged that should be captured as a permanent artifact before implementation.
---

# To PRD

Generate a PRD from the current conversation context and save it to `docs/prds/NNNN-slug.md`.

Do NOT interview the user — just synthesize what you already know from the conversation.

## Process

### 1. Explore the codebase (if needed)

Understand the current state of the code, if you haven't already. Use the project's domain glossary vocabulary throughout the PRD, and respect any ADRs in the area you're touching.

### 2. Gather the spec from context

Extract these from the current conversation:

- **What problem are we solving?** From the user's perspective. What's the pain?
- **What's the proposed solution?** The user-facing outcome, not the implementation.
- **What user stories emerged?** Even rough ones — "as a X, I want Y, so that Z."
- **What implementation decisions were discussed?** Modules, schemas, API contracts, architectural notes.
- **What's explicitly out of scope?** Things discussed and deliberately deferred.

If any of these are fuzzy, synthesize what you can and flag gaps.

### 3. Draft the modules

Sketch the major modules to build or modify. Look for opportunities to extract deep modules that can be tested in isolation.

A deep module (as opposed to a shallow module) is one which encapsulates a lot of functionality in a simple, testable interface which rarely changes.

Ask the user: _Do these modules match your expectations? Which modules do you want tests written for?_

### 4. Determine the number and slug

Scan `docs/prds/` for the highest existing number:

```bash
ls docs/prds/ 2>/dev/null | grep '^[0-9]' | sort -n | tail -1
```

Increment by one and zero-pad to 4 digits (e.g. `0001`, `0002`, ..., `0042`).

Derive a slug from the feature name: lowercase, hyphens for spaces, strip punctuation. Keep it short — 3-6 words. Examples:

- `customer-portal`
- `payment-retry-logic`
- `api-rate-limiting`

The filename is `NNNN-slug.md`.

### 5. Propose the PRD outline to the user

Show a preview:

```
## Proposed PRD: NNNN-slug.md

**Problem:** {1-2 sentences}
**Solution:** {1-2 sentences}
**Modules:** {list of modules to build/modify}
**User stories:** {count} stories identified
**Out of scope:** {what's not included}
```

Ask: _Does this scope look right? Anything missing or over-scoped?_

Iterate until the user approves.

### 6. Write the PRD file

Create `docs/prds/` if it doesn't exist:

```bash
mkdir -p docs/prds
```

Write using this template.

<prd-template>
```md
# {Feature name}

## Problem Statement

The problem that the user is facing, from the user's perspective.

## Solution

The solution to the problem, from the user's perspective.

## User Stories

A LONG, numbered list of user stories in the format:

1. As an <actor>, I want a <feature>, so that <benefit>

Cover all aspects of the feature exhaustively.

## Implementation Decisions

A list of implementation decisions that were made. Include:

- The modules that will be built/modified
- The interfaces of those modules
- Technical clarifications from the developer
- Architectural decisions
- Schema changes
- API contracts
- Specific interactions

Do NOT include specific file paths or code snippets — they go stale fast.

Exception: if a prototype produced a snippet that encodes a decision more precisely than prose can (state machine, reducer, schema, type shape), inline it within the relevant decision and note briefly that it came from a prototype. Trim to the decision-rich parts — not a working demo, just the important bits.

## Testing Decisions

- What makes a good test (only test external behavior, not implementation details)
- Which modules will be tested
- Prior art for the tests (i.e. similar types of tests in the codebase)

## Out of Scope

Things that are explicitly out of scope for this PRD.

## Further Notes

Any further notes about the feature.
```
</prd-template>

### 7. Confirm

```
Wrote docs/prds/NNNN-slug.md
```
