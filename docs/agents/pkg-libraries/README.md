# Midaz Internal pkg/ Libraries - Index

This directory contains detailed documentation for all internal libraries in the `pkg/` directory. These are reusable packages shared across Midaz components.

## Quick Reference

| Package | Documentation | Purpose | Critical? |
|---------|---------------|---------|-----------|
| **assert** | [assert.md](./assert.md) | Domain invariant assertions for production | ⚠️ CRITICAL |
| **errors** | [errors.md](./errors.md) | Typed business errors with HTTP mapping | ⚠️ CRITICAL |
| **mruntime** | [mruntime.md](./mruntime.md) | Safe goroutine handling with panic recovery | ⚠️ REQUIRED |
| **http** | [http.md](./http.md) | Fiber handler utilities (request/response) | 🔥 High Use |
| **utils** | [utils.md](./utils.md) | Validation functions and pointer helpers | 🔥 High Use |
| **models** | [models.md](./models.md) | Domain models (mmodel) and common types | 🔥 High Use |
| **specialized** | [specialized.md](./specialized.md) | transaction, mgrpc, mongo, gold, mlint | 📦 Specialized |

## Navigation by Use Case

### I'm writing a new feature
1. Read [assert.md](./assert.md) - Validate preconditions/postconditions
2. Read [errors.md](./errors.md) - Return typed business errors
3. Read [http.md](./http.md) - Build HTTP handlers
4. Read [models.md](./models.md) - Work with domain entities

### I'm handling errors
1. Read [errors.md](./errors.md) - Typed error types
2. See `docs/agents/error-handling.md` - Error patterns
3. Read [http.md](./http.md) - HTTP error responses

### I'm launching goroutines
1. Read [mruntime.md](./mruntime.md) - Safe goroutine patterns
2. See `docs/agents/concurrency.md` - Concurrency patterns

### I'm validating input
1. Read [utils.md](./utils.md) - Validation functions
2. Read [assert.md](./assert.md) - Domain invariants

### I'm working with specialized domains
1. Read [specialized.md](./specialized.md) - Find your domain
2. Refer to specific package documentation

## Common Patterns

### Handler → Use Case → Repository Stack

```
┌─────────────────────────────────────────┐
│  HTTP Handler (Fiber)                   │
│  - pkg/net/http utilities               │
│  - pkg/assert for preconditions         │
└─────────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│  Use Case (Command/Query)               │
│  - pkg/assert for validation            │
│  - pkg/errors for business errors       │
│  - pkg/mmodel for domain models         │
└─────────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│  Repository (PostgreSQL)                │
│  - pkg/assert for nil checks            │
│  - pkg/errors for typed errors          │
└─────────────────────────────────────────┘
```

See individual files for complete code examples.

## Integration with External Libraries

`pkg/` libraries work together with `lib-commons` (external library):
- See [`docs/agents/libcommons.md`](../libcommons.md) for observability integration
- `libCommons.NewTrackingFromContext()` provides logger/tracer
- `pkg/assert` + `pkg/mruntime` handle invariants and panics

## File Organization

- **High Priority** (read first): assert.md, errors.md, mruntime.md, http.md
- **Medium Priority** (frequently used): utils.md, models.md
- **Low Priority** (specialized domains): specialized.md

## Quick Command Reference

```bash
# Find package usage
rg "pkg/assert" --type go

# Find error types
rg "EntityNotFoundError|ValidationError" --type go

# Find mruntime usage
rg "mruntime\.SafeGo" --type go

# Count package imports
rg "github.com/LerianStudio/midaz/v3/pkg/" --type go --count-matches | sort -t: -k2 -nr | head -10
```

## Contributing to Documentation

When updating package documentation:
1. Update the individual package file (e.g., `assert.md`)
2. Update this README.md if adding new packages
3. Update cross-references in related files
4. Run `make lint && make test` to verify examples
5. Update `CLAUDE.md` if adding new critical packages

## Related Documentation

- [`docs/agents/libcommons.md`](../libcommons.md) - External observability library
- [`docs/agents/error-handling.md`](../error-handling.md) - Error handling patterns
- [`docs/agents/observability.md`](../observability.md) - Observability patterns
- [`docs/agents/concurrency.md`](../concurrency.md) - Concurrency patterns

---

**Next Steps**: Click on a package file above to dive into detailed documentation with API references, usage examples, and integration patterns.
