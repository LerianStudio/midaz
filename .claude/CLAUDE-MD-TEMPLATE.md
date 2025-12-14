# CLAUDE.md Template

> **Based on**: [Writing a good CLAUDE.md](https://www.humanlayer.dev/blog/writing-a-good-claude-md) by Kyle Mistele (HumanLayer)
>
> **Key Principle**: This file goes into EVERY conversation. Keep it concise (<300 lines, ideally <100), universally applicable, and use progressive disclosure.

---

## How to Use This Template

1. **Delete this "How to Use" section** after customizing
2. **Replace all `[PLACEHOLDERS]`** with your project-specific content
3. **Remove any sections** that don't apply to your project
4. **Keep instructions minimal** - if it's not universally applicable, move it to `docs/agents/`
5. **Prefer pointers over copies** - reference `file:line` instead of pasting code

---

# [PROJECT NAME]

<!--
  ONE-LINER: A single sentence describing what this project does.
  This helps Claude understand the domain immediately.
-->

[Brief one-sentence description of the project's purpose]

## Project Overview (WHY)

<!--
  WHY section: Help Claude understand the PURPOSE and CONTEXT.
  - What problem does this solve?
  - Who uses it?
  - What's the business domain?
  Keep this to 3-5 bullet points max.
-->

- **Purpose**: [What this project does and why it exists]
- **Domain**: [Business/technical domain - e.g., fintech, healthcare, e-commerce]
- **Users**: [Who uses this - developers, end users, other services]

## Tech Stack (WHAT)

<!--
  WHAT section: The technologies Claude needs to know about.
  Only include what's actually used - don't list every possible tool.
-->

| Layer | Technology |
|-------|------------|
| Language | [e.g., Go 1.23, TypeScript 5.x] |
| Framework | [e.g., Fiber, Next.js, FastAPI] |
| Database | [e.g., PostgreSQL 16, MongoDB 7] |
| Cache | [e.g., Redis/Valkey, Memcached] |
| Queue | [e.g., RabbitMQ, Kafka, SQS] |
| Infrastructure | [e.g., Docker, Kubernetes, AWS] |

## Project Structure (WHAT)

<!--
  Provide a HIGH-LEVEL map of the codebase.
  Don't list every file - just the important directories.
  This helps Claude know WHERE to look.
-->

```
[root]/
├── apps/           # [Description - e.g., Service applications]
│   ├── [app1]/     # [What app1 does]
│   └── [app2]/     # [What app2 does]
├── pkg/            # [Description - e.g., Shared libraries]
├── tests/          # [Description - e.g., Integration/E2E tests]
└── docs/           # [Description - e.g., Documentation]
```

## Essential Commands (HOW)

<!--
  HOW section: Only include commands that are UNIVERSALLY needed.
  These should be commands Claude will use in almost every session.
  Move task-specific commands to docs/agents/
-->

```bash
# Development
[command to start dev environment]      # e.g., make dev
[command to run tests]                  # e.g., make test
[command to lint/format]                # e.g., make lint

# Verification (run before completing tasks)
[command to verify changes]             # e.g., make check
```

## Architecture Patterns

<!--
  Only include patterns that are FUNDAMENTAL to understanding the codebase.
  These should be patterns Claude will encounter in almost every task.
  Detailed patterns go in docs/agents/architecture.md
-->

- **[Pattern 1]**: [One-line description - e.g., "Hexagonal architecture with ports/adapters"]
- **[Pattern 2]**: [One-line description - e.g., "CQRS with separate command/query handlers"]
- **[Pattern 3]**: [One-line description - e.g., "Domain events for cross-service communication"]

## Critical Rules

<!--
  ONLY include rules that:
  1. Apply to EVERY task
  2. Would cause significant problems if violated
  3. Are not enforced by linters/CI

  Keep this to 5-7 rules MAX. More rules = more ignored.
-->

1. **[Rule 1]**: [e.g., "Never use context.Background() in production code"]
2. **[Rule 2]**: [e.g., "Always return errors, never panic"]
3. **[Rule 3]**: [e.g., "Tenant ID must come from authenticated context, never from request"]
4. **[Rule 4]**: [e.g., "All logs must use structured logging with PII redaction"]
5. **[Rule 5]**: [e.g., "Database access only through repository interfaces"]

## Progressive Disclosure

<!--
  This is the KEY pattern from the article.
  Instead of putting everything in CLAUDE.md, point to detailed docs.
  Claude will read these ONLY when relevant to the current task.
-->

**Before starting work**, check if any of these are relevant to your task:

| Document | When to Read |
|----------|--------------|
| `docs/agents/architecture.md` | Understanding system design, adding new features |
| `docs/agents/testing.md` | Writing or fixing tests |
| `docs/agents/database.md` | Schema changes, migrations, queries |
| `docs/agents/api-design.md` | Creating or modifying API endpoints |
| `docs/agents/deployment.md` | CI/CD, containerization, infrastructure |
| `docs/agents/security.md` | Authentication, authorization, data protection |
| `docs/agents/[domain].md` | Working on specific domain/bounded context |

<!--
  Create these files in your project's docs/agents/ directory.
  Each file should contain DETAILED instructions for that specific topic.
  Use file:line references instead of code snippets where possible.
-->

## Code Conventions

<!--
  DO NOT put style guidelines here - use linters!
  Only include conventions that:
  1. Can't be enforced by tooling
  2. Are critical for code consistency
  3. Apply universally

  If you have many conventions, move them to docs/agents/conventions.md
-->

- **Naming**: [e.g., "Use descriptive names; abbreviations only for well-known terms"]
- **Errors**: [e.g., "Wrap errors with context using errors.Wrap()"]
- **Comments**: [e.g., "Only comment WHY, not WHAT - code should be self-documenting"]

## Verification Checklist

<!--
  What should Claude verify before considering a task complete?
  Keep this minimal - ideally a single command that runs all checks.
-->

Before completing any task, run:
```bash
[single verification command]  # e.g., make verify
```

This runs: [list what it includes - e.g., "tests, linting, type checking, build"]

---

# Template: docs/agents/ Files

<!--
  Below are TEMPLATES for the docs/agents/ files referenced above.
  Create these as separate files in your project.
  DELETE this section from your actual CLAUDE.md.
-->

## docs/agents/architecture.md (Template)

```markdown
# Architecture Guide

## System Overview
[Detailed system architecture description]
[Diagram references or ASCII art]

## Key Components
- **[Component 1]**: [Purpose, location: `path/to/component`]
- **[Component 2]**: [Purpose, location: `path/to/component`]

## Data Flow
[How data flows through the system]

## Design Patterns
### [Pattern Name]
- **What**: [Description]
- **Where**: See `path/to/example.go:42`
- **When to use**: [Guidance]

## Adding New Features
1. [Step-by-step guidance]
2. [Reference to existing examples]
```

## docs/agents/testing.md (Template)

```markdown
# Testing Guide

## Test Structure
- Unit tests: `*_test.go` next to source
- Integration tests: `tests/integration/`
- E2E tests: `tests/e2e/`

## Running Tests
```bash
# Unit tests only
[command]

# Integration tests (requires infrastructure)
[command]

# Specific test
[command]
```

## Writing Tests
- [Testing conventions]
- [Mocking strategy]
- [Example reference: `path/to/good_test.go`]

## Common Test Patterns
- **Table-driven tests**: See `path/to/example_test.go:15`
- **Mocking external services**: See `path/to/mock_example.go:30`
```

## docs/agents/database.md (Template)

```markdown
# Database Guide

## Schema Overview
[Description of database structure]

## Migrations
```bash
# Create migration
[command]

# Run migrations
[command]

# Rollback
[command]
```

## Migration Rules
1. [e.g., "Always use TIMESTAMPTZ, never TIMESTAMP"]
2. [e.g., "Include IF NOT EXISTS for idempotency"]
3. [e.g., "Every .up.sql must have matching .down.sql"]

## Query Patterns
- [Repository pattern reference]
- [Transaction handling]
```

## docs/agents/api-design.md (Template)

```markdown
# API Design Guide

## Endpoint Structure
[URL patterns, versioning strategy]

## Request/Response Patterns
- [Standard response format]
- [Error response format]
- [Pagination pattern]

## Adding New Endpoints
1. [Step-by-step process]
2. [Where to add handlers]
3. [How to document with OpenAPI]

## Authentication
[How auth works, what headers are required]

## Examples
- See `path/to/handler.go:50` for standard CRUD
- See `path/to/handler.go:120` for complex operations
```

---

# Anti-Patterns to Avoid in CLAUDE.md

<!--
  DELETE this section from your actual CLAUDE.md.
  This is guidance for template users.
-->

## DON'T include:

1. **Code style guidelines** - Use linters instead
2. **Every possible command** - Only universally needed ones
3. **Code snippets** - They become outdated; use file:line references
4. **Task-specific instructions** - Move to docs/agents/
5. **Implementation details** - Claude will discover these from the code
6. **Warnings about edge cases** - These belong in code comments or docs
7. **Historical context** - "We used to do X but now Y" is noise

## DO include:

1. **Project purpose** - Why does this exist?
2. **Tech stack** - What technologies are used?
3. **Project map** - Where are things located?
4. **Essential commands** - How to build, test, verify?
5. **Critical rules** - What must NEVER be violated?
6. **Pointers to detailed docs** - Where to find more info?

---

# Checklist Before Using

- [ ] Removed all template instructions and comments
- [ ] Replaced all [PLACEHOLDERS] with actual content
- [ ] File is under 300 lines (ideally under 100)
- [ ] Every instruction is universally applicable
- [ ] Created docs/agents/ files for detailed topics
- [ ] No code snippets (using file:line references instead)
- [ ] No style guidelines (using linters instead)
- [ ] Critical rules are limited to 5-7 items
- [ ] Verification command is a single, simple command
