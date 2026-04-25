---
name: generate-prd
description: Generate a Product Requirements Document for a new feature. Use this skill when the user wants to define what to build and why before writing any code. Triggers include "generate a PRD", "write product requirements", or any request to document a feature's goals, user stories, and scope before implementation.
---

# Generate PRD

Produce a Product Requirements Document that defines what to build and why.

## Process

### 1. Ask clarifying questions

Ask only the most critical questions needed to write a clear PRD. Focus on areas where the initial prompt is ambiguous or missing essential context. Common areas that may need clarification:

- **Problem/Goal:** "What problem does this feature solve for the user?"
- **Core Functionality:** "What are the key actions a user should be able to perform?"
- **Scope/Boundaries:** "Are there specific things this feature should NOT do?"
- **Success Criteria:** "How will we know when this feature is successfully implemented?"

Only ask when the answer isn't reasonably inferable from the prompt.

### Formatting Requirements

- **Number all questions** (1, 2, 3, etc.)
- **List options for each question as A, B, C, D, etc.** for easy reference
- Make it simple for the user to respond with selections like "1A, 2C, 3B"

### Example Format

```
1. What is the primary goal of this feature?
   A. Improve user onboarding experience
   B. Increase user retention
   C. Reduce support burden
   D. Generate additional revenue

2. Who is the target user for this feature?
   A. New users only
   B. Existing users only
   C. All users
   D. Admin users only

```

### 2. Generate the PRD

Using the prompt and answers, produce a PRD with these sections:

1. **Introduction/Overview** — feature description, problem it solves, goal
2. **Goals** — specific, measurable objectives
3. **User Stories** — narratives describing feature usage and benefit
4. **Functional Requirements** — numbered list of specific must-have functionalities
5. **Non-Goals** — explicit scope boundaries
6. **Design Considerations** _(optional)_ — mockup links, UI/UX notes, relevant components
7. **Technical Considerations** _(optional)_ — known constraints, dependencies, integration points
8. **Success Metrics** — how success is measured
9. **Open Questions** — anything still unresolved

## Target Audience

Assume the primary reader of the PRD is a **junior developer**. Therefore, requirements should be explicit, unambiguous, and avoid jargon where possible. Provide enough detail for them to understand the feature's purpose and core logic.

### 3. Save

Save as `specs/[feature-name]/prd.md`. Create the feature folder if it doesn't exist.

## Constraints

- Do NOT start implementing.
- Make sure to ask the user clarifying questions
- Take the user's answers to the clarifying questions and improve the PRD
