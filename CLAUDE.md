# CLAUDE.md - Midaz Agent Reference

Concise rules for AI agents working in Midaz. For expanded references, use `AGENTS.md`, `llms-full.txt`, and `docs/PROJECT_RULES.md`.

## Project

- Midaz is an enterprise double-entry ledger system.
- Module: `github.com/LerianStudio/midaz/v3`.
- Go: 1.25+.
- License: Elastic License 2.0.
- Main component: `components/ledger`.
- CRM component: `components/crm`.
- Shared code: `pkg`.

## Architecture

Flow: HTTP handlers -> command/query use cases -> repository interfaces -> adapters.

- Handlers: `components/ledger/internal/adapters/http/in`.
- Write use cases: `components/ledger/internal/services/command`.
- Read use cases: `components/ledger/internal/services/query`.
- PostgreSQL adapters: `components/ledger/internal/adapters/postgres`.
- Metadata adapters: MongoDB repositories.
- Domain models live in `pkg/mmodel`; do not create `/internal/domain`.
- Interfaces are defined where used. Repository interfaces usually sit in the adapter or service package that owns the contract.
- Dependencies flow inward; do not import outer layers from inner layers.
- Do not put domain logic in handlers or repositories.

## Key Files

- Composition root/config: `components/ledger/internal/bootstrap/config.go`.
- Routes: `components/ledger/internal/adapters/http/in/routes.go`.
- Error codes: `pkg/constant/errors.go`.
- Entity constants: `pkg/constant/entity.go`.
- Error factories/types: `pkg/errors.go`.
- Coding standards: `docs/PROJECT_RULES.md`.
- Full API/env reference: `llms-full.txt`.

## Coding Rules

- Use `any`, never `interface{}`.
- Use `uuid.UUID` for IDs, not strings.
- Context is always the first parameter; check `ctx.Err()` before expensive work.
- Validate in this order: normalize -> defaults -> validate -> execute.
- Business errors return directly; technical errors wrap with `%w` where adding context is useful.
- Use `constant.Entity*` instead of `reflect.TypeOf(mmodel.Foo{}).Name()`.
- Error sentinels must be unique and defined in `pkg/constant/errors.go`.
- Source files use `snake_case.go`; imports are stdlib -> external -> internal.
- Capture `time.Now()` once on create and reuse for `CreatedAt` and `UpdatedAt`.
- Do not use `time.Now()` in tests; use fixed times/utilities.
- PATCH optional `*string` fields: use `!= nil`, not `IsNilOrEmpty`, so empty strings can clear values.
- HTTP methods use `http.Method*` constants, not string literals.
- Metadata is flat only: no nesting, key max 100, value max 2000.
- Soft delete uses `DeletedAt` and status `DELETED` semantics.
- Pagination for new endpoints is cursor/page constrained by max limit 100; do not introduce offset pagination.

## Declaration And Docs

Within files, prefer this order:

1. Exported interface.
2. Exported types.
3. Constructor.
4. Exported methods.
5. Unexported helpers.

Documentation rules:

- Put repository/service method comments on the interface contract.
- Do not duplicate interface method comments on implementations unless implementation-specific behavior needs explanation.
- Keep comments short and behavioral; avoid comments that restate obvious code.

## SQL And Repositories

- Use `squirrel` for SQL construction: SELECT, INSERT, UPDATE, DELETE.
- Do not manually concatenate SQL placeholders with `strconv.Itoa`.
- Use `PlaceholderFormat(squirrel.Dollar)` for PostgreSQL.
- After `ToSql()` succeeds, optionally log assembled SQL at `Debug`; log query string only, never args.
- Check `RowsAffected()` for update/delete operations that should report not found; zero rows should map to the repository sentinel expected by the service layer.
- Repository/adapter code should be covered with integration tests using real dependencies/testcontainers. Unit tests are appropriate only for pure helpers/business branches.

## Logging

Use structured logs:

```go
logger.Log(ctx, libLog.LevelError, "Failed to execute query", libLog.Err(err))
logger.Log(ctx, libLog.LevelWarn, "Operation route not found", libLog.String("operation_route_id", id.String()))
```

Do not use:

```go
logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to execute query: %v", err))
```

Log levels:

- `Debug`: troubleshooting details, assembled SQL, cache details, batch stats.
- `Info`: sparse production milestones only.
- `Warn`: caller/business validation failures or degraded-but-recoverable fallback.
- `Error`: infrastructure/system failures requiring operator attention.

Never log secrets, credentials, tokens, balances, financial values, PII, raw payloads, or SQL args.

Avoid repeating broad scope IDs (`organization_id`, `ledger_id`, tenant IDs) on every log when spans already carry them. Logs may include the immediate failing resource ID when useful for search, e.g. `operation_route_id`.

## Observability

Span lifecycle:

- Always `defer span.End()` immediately after `tracer.Start`.
- For child I/O spans, preserve parent context: use `_, spanExec := tracer.Start(ctx, "...")`, not `ctx, spanExec := ...`.
- Do not create child spans for in-memory mapping/validation.
- Do not add redundant "Initiating..." logs; spans already mark operation starts.

Span attributes:

- Use `app.request.*` for inputs from handlers or method arguments: IDs, query params, payload-derived values.
- Use non-request namespaces for outputs/system observations: `db.rows_affected`, `db.rows_returned`, `app.operation_route_has_transaction_route_links`.
- Do not attach sensitive data, raw payloads, SQL args, balances, financial values, or PII.

Example:

```go
span.SetAttributes(
    attribute.String("app.request.organization_id", organizationID.String()),
    attribute.String("app.request.ledger_id", ledgerID.String()),
)
spanExec.SetAttributes(attribute.Int64("db.rows_affected", rowsAffected))
```

## Errors

- API errors use typed errors from `pkg/errors.go` and constants from `pkg/constant/errors.go`.
- Use `pkg.ValidateBusinessError(constant.Err..., constant.Entity...)`.
- Not found maps to `EntityNotFoundError` / HTTP 404.
- Business rule violations map to `UnprocessableOperationError` / HTTP 422.
- Do not expose internal technical error details to API clients.
- Do not create duplicate `errors.New("code")` sentinels outside `pkg/constant/errors.go`.

## HTTP

- Framework: Fiber v2.
- All routes use `http.ProtectedRouteChain()`.
- Route protection includes auth, optional post-auth middlewares, body parsing, UUID path validation, and handler.
- Use existing route helpers and middleware patterns in `components/ledger/internal/adapters/http/in`.

## Domain Notes

- Hierarchy: Organization -> Ledger -> Assets/Portfolios/Segments -> Accounts -> Transactions -> Operations -> Balances.
- Status common codes: `ACTIVE`, `INACTIVE`, `DELETED`, `PENDING`, `CANCELLED`.
- Transaction creation modes: JSON, DSL, inflow, outflow, annotation.
- Pending transactions can be committed/cancelled; revert creates a reverse transaction.
- Async transaction processing is controlled by `RABBITMQ_TRANSACTION_ASYNC`.
- Balance fields: `Available`, `OnHold`, `Scale`, `Version`.

## Multi-Tenancy

- Enabled via `MULTI_TENANT_ENABLED=true`; auth must also be enabled.
- Tenant ID comes from JWT via auth middleware.
- Tenant DB resolution uses tenant middleware and lib-commons tenant managers.
- Modules `onboarding` and `transaction` have independent PostgreSQL and MongoDB managers.
- Multi-tenant code uses `MT` suffix for names (`NewFooMT`, `runFooMT`, `mtEnabled`, `isMTReady`). Single-tenant code uses no qualifier.

## Commands

- Setup env: `make set-env`.
- Start stack: `make up`.
- Stop stack: `make down`.
- Unit tests: `make test-unit`.
- Integration tests: `make test-integration`.
- Lint: `make lint`.
- Format: `make format`.
- Security: `make sec`.
- Component delegation: `make ledger COMMAND=<target>`.

## Do Not

- Do not overwrite `docs/PROJECT_RULES.md`.
- Do not panic; return errors.
- Do not store nested metadata.
- Do not import outer layers from inner layers.
- Do not use raw SQL string concatenation for placeholders.
- Do not use `reflect.TypeOf(mmodel.Foo{}).Name()` for entity names.
- Do not use `fmt.Sprintf` inside logger calls.
- Do not overwrite parent `ctx` with child spans.
- Do not use non-request span attributes for input data.
- Do not log SQL args, payload values, secrets, balances, financial values, or PII.
