# Semgrep Rules for Midaz

This directory contains custom Semgrep rules for enforcing code quality and safety standards in the Midaz codebase.

## Rules Overview

### go-runtime-safety.yml

Enforces panic hardening patterns:

| Rule ID | Severity | Description |
|---------|----------|-------------|
| `go-no-raw-goroutine` | ERROR | Prevents raw `go` statements; use `mruntime.SafeGo()` instead |
| `go-no-recover-outside-boundary` | ERROR | Restricts `recover()` to boundary packages only |
| `go-no-panic-in-production` | WARNING | Prevents `panic()` calls outside test files |
| `go-no-bare-recover` | WARNING | Ensures `recover()` calls log the panic value |

### Exclusions

The following paths are excluded from certain rules:

- `pkg/mruntime/` - The runtime safety package itself (allowed to use raw `go` and `recover()`)
- `components/*/internal/adapters/http/` - HTTP boundary layer (allowed to use `recover()`)
- `components/*/internal/adapters/grpc/` - gRPC boundary layer (allowed to use `recover()`)
- `components/*/internal/adapters/rabbitmq/` - Message queue boundary layer (allowed to use `recover()`)
- `components/*/internal/bootstrap/` - Worker bootstrap with message consumers (allowed to use `recover()`)
- `**/*_test.go` - Test files (allowed to use all patterns for testing purposes)

## Local Testing

Run Semgrep locally to check for violations:

```bash
# Check all rules
semgrep scan --config .semgrep/ ./components/ ./pkg/

# Check a specific rule
semgrep scan --config .semgrep/go-runtime-safety.yml ./components/

# Dry run (show matches without failing)
semgrep scan --config .semgrep/ --dry-run ./components/ ./pkg/
```

## CI Integration

These rules are automatically run on pull requests via a **Semgrep job added to** `.github/workflows/go-combined-analysis.yml`.

## Adding New Rules

1. Add rules to the appropriate YAML file or create a new one
2. Test locally with `semgrep --config .semgrep/<file>.yml ./...`
3. Document the rule in this README
4. Verify CI passes before merging
