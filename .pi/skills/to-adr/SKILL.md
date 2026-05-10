---
name: to-adr
description: Generate an Architecture Decision Record from the current conversation context and write it to docs/adr/ with sequential numbering. Use when a decision has crystallised that is hard to reverse, surprising without context, and the result of a real trade-off.
---

# To ADR

Generate an ADR from the current conversation context and save it to `docs/adr/NNNN-slug.md`.

Do NOT create an ADR unless the decision meets all three criteria:

1. **Hard to reverse** — the cost of changing your mind later is meaningful
2. **Surprising without context** — a future reader will look at the code and wonder "why on earth did they do it this way?"
3. **The result of a real trade-off** — there were genuine alternatives and you picked one for specific reasons

If a decision is easy to reverse, skip it. If it's not surprising, nobody will wonder why. If there was no real alternative, there's nothing to record beyond "we did the obvious thing."

## Process

### 1. Gather the decision from context

Extract these from the current conversation:

- **What was the context?** What problem were we solving, what constraints were we under?
- **What did we decide?** The specific choice made.
- **Why this over the alternatives?** The reason this won — trade-offs, constraints, data that tipped the balance.
- **What were the alternatives considered?** Only if the rejection is non-obvious and worth remembering.

If any of these are fuzzy, ask the user to clarify before proceeding.

### 2. Check whether an ADR is warranted

Re-apply the three criteria from above.

Examples of decisions that typically qualify:

- **Architectural shape.** "We're using a monorepo." "The write model is event-sourced, the read model is projected into Postgres."
- **Integration patterns between contexts.** "Ordering and Billing communicate via domain events, not synchronous HTTP."
- **Technology choices that carry lock-in.** Database, message bus, auth provider, deployment target. Not every library — just the ones that would take a quarter to swap out.
- **Boundary and scope decisions.** "Customer data is owned by the Customer context; other contexts reference it by ID only."
- **Deliberate deviations from the obvious path.** "We're using manual SQL instead of an ORM because X."
- **Constraints not visible in the code.** "We can't use AWS because of compliance."
- **Rejected alternatives when the rejection is non-obvious.** If you considered GraphQL and picked REST for subtle reasons.

If any criterion fails, tell the user this doesn't warrant an ADR and explain why. Skip the rest.

### 3. Determine the number and slug

Scan `docs/adr/` for the highest existing number:

```bash
ls docs/adr/ 2>/dev/null | grep '^[0-9]' | sort -n | tail -1
```

Increment by one and zero-pad to 4 digits (e.g. `0001`, `0002`, ..., `0042`).

Derive a slug from the decision: lowercase, hyphens for spaces, strip punctuation. Keep it short — 3-6 words. Examples:

- `event-sourced-orders`
- `postgres-for-write-model`
- `api-versioning-v2`

The filename is `NNNN-slug.md`.

### 4. Propose the ADR to the user

Show a preview:

```
## Proposed ADR: NNNN-slug.md

**Title:** {Short title}
**Context:** {1-2 sentences on the problem/constraints}
**Decision:** {what we chose}
**Why:** {why this over alternatives}

(Optional: Status, Considered Options, Consequences)
```

Ask: _Does this capture the decision accurately? Anything to add, remove, or clarify?_

Iterate until the user approves.

### 5. Write the ADR file

Create `docs/adr/` if it doesn't exist:

```bash
mkdir -p docs/adr
```

Write using this template. Keep it concise — an ADR can be a single paragraph. Only include optional sections when they add genuine value.

<adr-template>
```md
# {Short title of the decision}

{1-3 sentences: what's the context, what did we decide, and why.}
```

## Optional sections (only when they add genuine value)

If `Status` is included, add it as frontmatter:

```
---
status: proposed | accepted | deprecated | superseded by ADR-NNNN
---
```

If alternatives are worth remembering:

```
## Considered Options

- **Option A**: {description}. Rejected because {reason}.
- **Option B**: {description}. Rejected because {reason}.
```

If downstream effects are non-obvious:

```
## Consequences

- {Effect 1}
- {Effect 2}
```
</adr-template>

### 6. Confirm

```
Wrote docs/adr/NNNN-slug.md
```
