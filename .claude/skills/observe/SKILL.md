---
name: observe
description: Explore and understand an existing codebase, then either summarize findings or update docs when the user asks. Use this skill when the user wants to explore the codebase, understand how something works, map architecture, document existing behavior, or update docs from observed code. Do not use for implementation tasks.
---

# Observe

Understand what exists. Summarize it, or document it when the user asks for docs to be updated.

## Process

### 1. Determine requested output from intent

Use the user's wording:

- If the user asks to **observe**, **explore**, **understand**, **map**, or **explain**, explore and provide a summary. Do not edit files.
- If the user asks to **update docs**, **document this**, **write findings to docs**, or **observe and update documents**, explore and update `docs/` with findings.
- If the user asks reconnaissance before possible work, report relevant files, patterns, risks, and questions. Do not create a full plan unless asked.

If output is unclear, default to summary without edits.

### 2. Explore codebase

Work systematically. Start broad, then go deep where relevant.

Look for:

- **Patterns and design decisions** — repeated approaches, deliberate choices
- **Component relationships** — dependencies, module boundaries, ownership
- **Architecture** — layers, entry points, data ownership, separation of concerns
- **Data flows** — where data starts, transforms, persists, and leaves
- **Conventions** — naming, file structure, errors, tests, formatting
- **Module interactions** — APIs, events, queues, imports, side effects

Read existing `docs/` when relevant, especially before updating docs.

### 3. Summarize findings when no doc update requested

Report concise findings without editing files.

Include whichever are relevant:

- Key files and entry points
- Important flows or architecture
- Existing conventions and patterns
- Risks, unknowns, or questions
- Suggested next files to inspect if more depth is needed

Describe what is. Do not prescribe changes unless user asked for recommendations.

### 4. Update docs when requested

If user asks to update documents, check whether `docs/` exists.

- **If `docs/` exists:** read relevant docs first, identify gaps, then add only new or corrected information. Place findings in the most relevant existing doc. Create a new doc only when no suitable doc exists.
- **If `docs/` does not exist:** create `docs/architecture.md` as the initial document. Note that it was bootstrapped from observed code.

Do not duplicate existing docs. Do not rewrite or restructure docs unless user explicitly asks.

### 5. Report doc changes

When docs were updated, summarize:

- Files changed
- What information was added or corrected
- Any important unknowns left undocumented

## Constraints

- **No production code changes.** Observe and docs only.
- **Intent controls output.** Summary by default; docs updated only when requested.
- **No plans/tasks unless asked.** Recon may include risks and questions only.
- **Extend, don't rewrite.** Preserve existing docs structure unless told otherwise.
- **Describe what is, not what should be.**
