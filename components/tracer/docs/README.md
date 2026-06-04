# Tracer Documentation

**Language Policy:** All documentation MUST be in English. See [LANGUAGE_POLICY.md](./LANGUAGE_POLICY.md).

---

## Documentation Structure

### Core Standards
- **[CODING_STANDARDS.md](./CODING_STANDARDS.md)** - Coding best practices and patterns
- **[LANGUAGE_POLICY.md](./LANGUAGE_POLICY.md)** - English-only policy (code, docs, commits)
- **[PROJECT_RULES.md](./PROJECT_RULES.md)** - Architecture, conventions, and DevOps standards

### Quick Reference

**Before writing code:**
1. Read [CODING_STANDARDS.md](./CODING_STANDARDS.md) - Golden Rules section
2. Check [LANGUAGE_POLICY.md](./LANGUAGE_POLICY.md) - English only
3. Review [PROJECT_RULES.md](./PROJECT_RULES.md) - Architecture patterns

**Before committing:**
- [ ] All code in English?
- [ ] All comments in English?
- [ ] Commit message in English?
- [ ] Tests deterministic (no `time.Now()`)?
- [ ] Methods validate before mutating?
- [ ] Error wrapping uses `%w`?

**Before code review:**
- [ ] Run `make lint` - 0 issues?
- [ ] Run `make test` - all pass?
- [ ] Run `make test-integration` - all pass?
- [ ] Check [Review Checklist](./CODING_STANDARDS.md#7-review-checklist)

---

## Key Policies

### 1. Language: English Only

**ALL artifacts MUST be in English:**
- Code (variables, functions, types)
- Comments (inline, doc, block)
- Commit messages
- PR titles and descriptions
- Documentation
- Error messages
- Log messages
- Test names

See [LANGUAGE_POLICY.md](./LANGUAGE_POLICY.md) for details and enforcement.

### 2. Golden Rules

From [CODING_STANDARDS.md](./CODING_STANDARDS.md):

1. **Validate Before Mutate** - Atomicity in error-returning methods
2. **Domain Logic in Domain** - Not in service layer
3. **Deterministic Tests** - `testutil.FixedTime()`, never `time.Now()`
4. **Error Chain Preservation** - Always `%w`, never `%v`
5. **Normalize-Validate-Store** - In that order, store normalized

### 3. Testing Standards

- Use `t.Parallel()` for concurrency
- No `time.Now()`, `uuid.New()`, `rand.Intn()` in tests
- Use `testutil.FixedTime()` and `testutil.MustDeterministicUUID()`
- Integration tests use `//go:build integration` tag (optional for unit tests)

### 4. Error Handling

- Always use `%w` for error wrapping
- Capture context from `tracer.Start()`: `ctx, span := tracer.Start(...)`
- Prefer typed errors with context

---

## Automated Checks

### golangci-lint

Configuration: [../.golangci.yml](../.golangci.yml)

Enabled linters enforce:
- `errorlint` - Forces `%w` usage
- `contextcheck` - Verifies context propagation
- `govet` - Shadow detection
- `revive` - Best practices

Run: `make lint`

### CI/CD Pipeline

Tracer is a co-located deploy unit in the `midaz` monorepo; CI runs from the monorepo-root `.github/workflows/` (there is no component-local `.github/`).

---

## Quick Links

**Standards:**
- [Coding Standards](./CODING_STANDARDS.md)
- [Language Policy](./LANGUAGE_POLICY.md)
- [Project Rules](./PROJECT_RULES.md)

**External Resources:**
- [Go Error Handling](https://go.dev/blog/go1.13-errors)
- [Go Testing](https://go.dev/doc/tutorial/add-a-test)
- [Domain-Driven Design](https://martinfowler.com/bliki/DomainDrivenDesign.html)
- [OpenTelemetry Go](https://opentelemetry.io/docs/instrumentation/go/)

---

## Contributing

1. **Read** this README
2. **Review** [CODING_STANDARDS.md](./CODING_STANDARDS.md)
3. **Ensure** all code/docs in English ([LANGUAGE_POLICY.md](./LANGUAGE_POLICY.md))
4. **Follow** [PROJECT_RULES.md](./PROJECT_RULES.md)
5. **Run** `make lint && make test && make test-integration`
6. **Create** PR with English title/description

---

## Questions?

Contact the Engineering Team or refer to specific documentation:
- Code quality: [CODING_STANDARDS.md](./CODING_STANDARDS.md)
- Language: [LANGUAGE_POLICY.md](./LANGUAGE_POLICY.md)
- Architecture: [PROJECT_RULES.md](./PROJECT_RULES.md)

---

**Last Updated:** 2026-02-04  
**Maintained by:** Engineering Team
