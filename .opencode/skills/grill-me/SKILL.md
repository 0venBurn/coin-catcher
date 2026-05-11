---
name: grill-me
description: Stress-test a scoped plan or design decision by mapping its decision branches upfront and resolving them one by one. Use when user wants to stress-test a plan, get grilled on their design, or mentions "grill me".
---

## Setup

1. Restate the scoped question or design decision back to the user in one sentence. Get confirmation before proceeding.
2. Map out the branches of the decision tree that fall within that scope. Present them as a numbered list so the user can see the terrain and how many branches there are.
3. If the user wants to add or remove branches, adjust the map before starting.

## Grilling

- Work through the branches one at a time. Resolve each branch before moving to the next.
- For each question, provide your recommended answer.
- If a question can be answered by exploring the codebase, explore the codebase instead of asking.
- If a question would open a new branch **outside the original scope**, don't follow it. Add it to a "Parked" list and move on.
- Show progress as you go (e.g. "Branch 3 of 5").

## When all branches are resolved

Output a summary:

- **Question**: the original scoped question
- **Resolved**: what held up and the decisions reached
- **Gaps**: what needs more research or prototyping before committing
- **Parked**: out-of-scope items that surfaced and should be addressed separately
