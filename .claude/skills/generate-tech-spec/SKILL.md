---
name: generate-tech-spec
description: Generate a Technical Specification for a feature — defining how it's built, including data models, interfaces, data flows, and infrastructure. Use this skill after a PRD exists, or when the user wants to define implementation details before writing code. Triggers include "generate a tech spec", "write a technical specification", "spec out how to build this", or when called as part of the planning pipeline after generate-prd.
---

# Generate Tech Spec

Produce a Technical Specification that defines how the feature is built — explicit enough that implementation choices are unambiguous.

## Process

### 1. Read existing specs

Check `specs/[feature-name]/prd.md`. If it exists, use it as the primary input. If not, note the absence and proceed from the user's description directly.

Also check `/docs/system_patterns.md` and `/docs/conventions.md` for existing architectural patterns and conventions to follow.

### 2. Ask clarifying questions

Ask only questions about missing technical details — limit to 3–5. Skip anything already answered in the PRD. Number all questions and list options as A, B, C, D.

Common gaps to probe:

- **Data Model:** "What are the core entities and their relationships?"
- **Infrastructure:** "Are there specific database, caching, or queue requirements?"
- **Integration Points:** "What existing services or modules does this connect to?"
- **Performance/Scale:** "Are there specific throughput, latency, or retention requirements?"
- **Constraints:** "Are there existing patterns or conventions this must follow?"

### 3. Generate the tech spec

Using the PRD and answers, produce a tech spec with these sections:

1. **Overview** — what is being built and why; reference the PRD
2. **Architecture Diagram** _(optional)_ — ASCII or description of component connections
3. **Data Models** — for each entity: table name, columns with types/constraints/defaults, indexes, relationships, any TimescaleDB specifics
4. **API / Interface Design** _(if applicable)_ — endpoints or function signatures, request/response shapes, error cases
5. **Integration Points** — how this connects to existing services, modules, or external systems
6. **Infrastructure & Config** — database, caching, queues, environment variables, migrations
7. **Data Flow** — step-by-step description of how data moves through the system for the core use case
8. **Error Handling & Edge Cases** — known failure modes and how to handle them
9. **Performance Considerations** — indexing strategy, query patterns, expected load, retention
10. **Security Considerations** — auth requirements, data sensitivity, access control
11. **Open Questions** — anything still unresolved that could affect implementation

### 4. Save

Save as `specs/[feature-name]/tech-spec.md`. Create the feature folder if it doesn't exist.

## Constraints

- Do NOT start implementing.
