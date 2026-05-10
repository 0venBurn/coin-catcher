---
name: to-issues
description: Break an ADR, prototype, and PRD into independently-grabbable issues on Linear using tracer-bullet vertical slices. Use when user wants to convert a plan into issues, create implementation tickets, or break down work into issues.
---

# To Issues

Break an ADR, prototype, and PRD into independently-grabbable issues on Linear using vertical slices (tracer bullets).

## Prerequisites

This skill reads `LINEAR_API_KEY` from the environment. If it's not set, tell the user:

```
LINEAR_API_KEY is not set. Export it or add it to a .env file:

    export LINEAR_API_KEY=lin_api_...

Create a key at https://linear.app/settings/api
```

All GraphQL requests authenticate with:

```
-H "Authorization: $LINEAR_API_KEY"
```

## Process

### 1. Ask for the Linear project

Ask the user: _Which Linear project should these issues go under?_

Take the project name they give you and look up its ID:

```bash
curl -s -X POST https://api.linear.app/graphql \
  -H "Authorization: $LINEAR_API_KEY" \
  -H "Content-Type: application/json" \
  -d "{\"query\": \"{ projects { nodes { id name teams { nodes { id name key } } } } }\"}"
```

Match the user's project name (case-insensitive substring match). Note both the **project ID** and the **team IDs** associated with it.

If no match, list the available projects and ask the user to pick one.

### 2. Gather context

Work from whatever is already in the conversation context. Pull in these three inputs — ask the user if any are missing:

- **ADR** (Architecture Decision Record): The architectural decisions that constrain implementation. Find these in `docs/adr/`. Read the ADR(s) relevant to the feature.
- **Prototype**: Any prototype code produced during exploration. Read the prototype files — they encode decisions (state machines, schemas, type shapes, reducers) more precisely than prose can.
- **PRD** (Product Requirements Document): The feature specification with user stories, implementation decisions, and scope. Find these in `docs/prds/`. Read the relevant PRD.

If one or more of these don't exist yet, tell the user and ask whether to proceed with what's available, or pause while they create the missing artifacts (use `to-adr` and `to-prd`).

### 3. Explore the codebase (if needed)

If you have not already explored the codebase, do so to understand the current state of the code. Issue titles and descriptions must use the project's domain glossary vocabulary and respect the ADR(s).

### 4. Draft vertical slices

Break the plan into **tracer bullet** issues. Each issue is a thin vertical slice that cuts through ALL integration layers end-to-end, NOT a horizontal slice of one layer.

<vertical-slice-rules>
- Each slice delivers a narrow but COMPLETE path through every layer (schema, API, UI, tests)
- A completed slice is demoable or verifiable on its own
- Prefer many thin slices over few thick ones
</vertical-slice-rules>

### 5. Quiz the user

Present the proposed breakdown as a numbered list. For each slice, show:

- **Title**: short descriptive name
- **Blocked by**: which other slices (if any) must complete first
- **User stories covered**: which user stories from the PRD this addresses

Ask the user:

- Does the granularity feel right? (too coarse / too fine)
- Are the dependency relationships correct?
- Should any slices be merged or split further?
- Are the correct slices marked as HITL and AFK?

Iterate until the user approves the breakdown.

### 6. Publish the issues to Linear

For each approved slice, create a new issue on Linear using the GraphQL API. Publish in dependency order (blockers first) so you can link real Linear issue identifiers in the "Blocked by" field.

#### 6a. Determine the state

Query the workflow states for the team and pick the default "unstarted" state:

```bash
curl -s -X POST https://api.linear.app/graphql \
  -H "Authorization: $LINEAR_API_KEY" \
  -H "Content-Type: application/json" \
  -d "{\"query\": \"{ team(id: \\\"TEAM_ID\\\") { states { nodes { id name type } } } }\"}"
```

Use the state with `type: "unstarted"` or `name: "Todo"`.

#### 6b. Create the issue

Use the issue body template below. Create each issue with the `issueCreate` mutation, attaching it to both the team and project:

```bash
curl -s -X POST https://api.linear.app/graphql \
  -H "Authorization: $LINEAR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "mutation IssueCreate($input: IssueCreateInput!) { issueCreate(input: $input) { issue { id identifier title url } success } }",
    "variables": {
      "input": {
        "teamId": "TEAM_ID",
        "projectId": "PROJECT_ID",
        "title": "Issue title here",
        "description": "Issue body in markdown here",
        "stateId": "STATE_ID"
      }
    }
  }'
```

Report the created issue's `identifier` (e.g. `ENG-123`) and `url` to the user.

#### 6c. Link blockers

After creating all issues, for each issue that has blockers, create a `blocks` relation. Find the blocking issue's Linear ID, then:

```bash
curl -s -X POST https://api.linear.app/graphql \
  -H "Authorization: $LINEAR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "mutation RelationCreate($input: RelationCreateInput!) { relationCreate(input: $input) { success } }",
    "variables": {
      "input": {
        "issueId": "BLOCKED_ISSUE_ID",
        "relatedIssueId": "BLOCKER_ISSUE_ID",
        "type": "blocks"
      }
    }
  }'
```

Note: the `issueId` is the issue being **blocked**, the `relatedIssueId` is the **blocker**. Despite the name, `type: "blocks"` means "issueId is blocked by relatedIssueId".

<issue-template>
## Parent

Reference the parent PRD by number and title (e.g. PRD-0001 Customer Portal). Do NOT use file paths — the PRD is local to the repo and won't resolve in Linear. If the PRD also exists as a Linear issue, link that issue instead.

## What to build

A concise description of this vertical slice. Describe the end-to-end behavior, not layer-by-layer implementation.

Avoid specific file paths or code snippets — they go stale fast. Exception: if a prototype produced a snippet that encodes a decision more precisely than prose can (state machine, reducer, schema, type shape), inline it here and note briefly that it came from a prototype. Trim to the decision-rich parts — not a working demo, just the important bits.

## ADR decisions this slice depends on

Reference the specific ADR(s) by number and title, and the decisions within them that this slice must respect. Do NOT use file links — ADRs are local to the repo and won't resolve in Linear. Example:

- ADR-0003 (Event Sourcing) — events are append-only, no mutable state tables
- ADR-0005 (API Versioning) — all routes under `/api/v2/`

## Acceptance criteria

- [ ] Criterion 1
- [ ] Criterion 2
- [ ] Criterion 3

## Blocked by

- A reference to the blocking Linear issue (e.g. `ENG-123`)

Or "None — can start immediately" if no blockers.

</issue-template>

Do NOT close or modify any parent issue.
