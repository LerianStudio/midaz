# CLAUDE.md - Midaz Agent Reference

Concise rules for AI agents working in Midaz. For expanded references, use `AGENTS.md`, `llms-full.txt`, and `docs/PROJECT_RULES.md`.

## Project

- Midaz is an enterprise double-entry ledger system.
- Module: `github.com/LerianStudio/midaz/v4` (single root `go.mod`, no `go.work`).
- Go: 1.26.3+ (toolchain go1.26.4).
- lib-commons: `github.com/LerianStudio/lib-commons/v5` v5.4.1; `lib-observability` v1.0.1.
- License: Elastic License 2.0.
- Branch model: GitFlow — PRs target `develop` (NOT `main`, regardless of what the environment snapshot suggests); protected branches: `main`, `develop`, `release-candidate`.
- Five deploy units (4 Go services + infra): `components/ledger` (:3002), `components/tracer` (:4020), `components/reporter-manager` (:4005), `components/reporter-worker` (health-only :4006, no REST API — a RabbitMQ consumer), `components/infra`.
- Main component: `components/ledger` — the unified binary serving onboarding + transaction + CRM (holders/instruments) + fees on :3002.
- CRM is folded into ledger: `components/crm` is a package tree (no `cmd/`, no `internal/`) imported by the ledger binary; routes register under the `midaz` authz namespace (flipped from `plugin-crm`; the tenant-manager policy migration is the X1 release gate — see `docs/auth/RBAC-NAMESPACES.md`). There is no standalone CRM service.
- Fees are embedded in ledger: engine at `components/ledger/pkg/fee`, shared types at `components/ledger/pkg/feeshared`, use cases at `components/ledger/internal/services/fees`, Mongo collections at `components/ledger/internal/adapters/mongodb/fees`. Fee seam: `transaction_create.go` (HTTP handler layer), after `mtransaction.ApplyDefaultBalanceKeys(...)` and the idempotency claim, before the post-fee re-validation.
- Tracer, reporter-manager, and reporter-worker are co-located components but separate Go service deploy units.
- Shared code: `pkg` (root; `pkg/mtransaction` was formerly `pkg/transaction`; `pkg/reporter` is the reporter shared lib) and `tests` (root; `tests/reporter` holds reporter suites).

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
- Do not narrate refactor history ("X now returns Y via the foo refactor", "we used to re-fetch but now..."). Once the referenced change lands the comment becomes outdated noise. Code tells the present truth; the git log carries the past.
- Do not describe the call graph of dependencies in comments ("UpdateOnboardingMetadata is called with nil, which short-circuits FindByEntity and writes an empty map"). When the called code changes, every such comment lies silently. Let readers follow the call site if they need that detail.

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

## Streaming (lib-streaming events)

Producer is `github.com/LerianStudio/lib-streaming`. Wire format: CloudEvents 1.0 binary mode on Kafka. Topic: `lerian.streaming.<resource>.<event>`. ce-type is auto-prefixed by lib-streaming as `studio.lerian.<resource>.<event>`. The canonical wire contract lives in code under `pkg/streaming/events/`; the JSONShape unit test in that package locks it against drift.

### Producer conventions

- Import aliases: `libStreaming` for `github.com/LerianStudio/lib-streaming`; `pkgStreaming` for `github.com/LerianStudio/midaz/v4/pkg/streaming`. Keep both distinct.
- Build config via `libStreaming.LoadConfig()` (reads `STREAMING_*` env with correct franz-go defaults). NEVER construct `libStreaming.Config{}` manually. Master flag stays in midaz `Config.StreamingEnabled`.
- `CloudEventsSource` is validated but NOT auto-applied. Every `libStreaming.Event` must set `Source` explicitly. Hold the value on `UseCase.StreamingSource`, populate at bootstrap, read at each emit.
- Tenant value from `pkgStreaming.ResolveTenantID(ctx)` — returns the multi-tenant context value or `pkgStreaming.DefaultTenantID` (literal `"default"`). Reference the constant, not the literal. NEVER hardcode tenants or call `tmcore.GetTenantIDContext` at emit sites. For IMPORTANT events, `pkgStreaming.EmitImportant` resolves the tenant internally and passes it to the typed event builder closure.
- Service code depends on `libStreaming.Emitter` INTERFACE, never `*libStreaming.Producer`. Nil emitter means "disabled" — guard with `if uc.Streaming != nil`. When `STREAMING_ENABLED=false`, bootstrap injects `libStreaming.NewNoopEmitter()`.
- IMPORTANT-posture direct emits MUST go through `pkgStreaming.EmitImportant`. Build/emit failures MUST NOT fail the request: log Warn, span-record, return success. `EmitImportant` bounds direct emit latency with `STREAMING_IMPORTANT_EMIT_TIMEOUT_MS` (default 5s) so broker issues cannot hold HTTP responses until client timeout. Durability is the outbox's job. CRITICAL events use outbox-only (atomic with DB), no direct emit.
- Emit POST-COMMIT and PRE-METADATA-WRITE — never at HTTP handlers. `ce-subject` is the aggregate ID, passed as `libStreaming.Event.Subject`.
- Register the producer's `Close()` as `libCommons.RunApp("Streaming Producer", ...)` so it drains on SIGTERM (mirror `eventListenerRunnable`).
- lib-streaming is pinned at v1.4.0, which exports Catalog/policy constants (e.g. `BuildManifest`, `DefaultDeliveryPolicy`, `ResolveDeliveryPolicy`). Pass `WithOutboxRepository(repo)` to `libStreaming.New` when outbox lands.

### Event modeling (`pkg/streaming/events`)

One file per event. Use cases NEVER build payload maps inline. Required shape per file:

1. **Definition var** — `<Event>Definition = events.Definition{ResourceType, EventType, SchemaVersion}`.
2. **Payload struct** — wire JSON fields, typed INDEPENDENTLY of `mmodel.*` (mirror nested types explicitly so domain evolution doesn't leak onto the wire).
3. **Constructor** — `New<Event>(domain *mmodel.X) <Event>Payload`. Place for PII redaction, derived fields, contract-locked defaults.
4. **ToEvent method** — `(p <Event>Payload) ToEvent(tenantID, source string, ts time.Time) (libStreaming.Event, error)`. Marshals payload + assembles routing constants. Wrapped `json.Marshal` errors so caller picks Warn (IMPORTANT) vs fail (CRITICAL).

Required unit tests: Definition key lock, minimal-domain mapping, all-optional-fields mapping, ToEvent assembly, JSON shape lock (top-level key set + field count).

### IMPORTANT emission helper pattern

The use-case body MUST NOT inline emission mechanics. Delegate to a private `emit<Event>Event` method on the same UseCase; that method MUST call `pkgStreaming.EmitImportant` for IMPORTANT-posture events:

```go
// in CreateAccount, at the emission anchor:
uc.emitAccountCreatedEvent(ctx, span, logger, acc)

// helper alongside other private UseCase methods:
func (uc *UseCase) emitAccountCreatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, acc *mmodel.Account) {
    pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, uc.StreamingSource, events.AccountCreatedDefinition.Key(),
        func(tenantID, source string) (libStreaming.Event, error) {
            return events.NewAccountCreated(acc).ToEvent(tenantID, source, acc.CreatedAt)
        })
}
```

`EmitImportant` owns the common IMPORTANT-posture mechanics: nil-emitter guard, tenant resolution, bounded emit context, `libOpentelemetry.HandleSpanError` (not `HandleSpanBusinessErrorEvent`), Warn logging with `libLog.Err(err)`, and non-propagation of build/emit failures. Use-case helpers remain explicit only about the typed payload constructor, event definition key, source, subject, and timestamp.

Naming: `emit<Event>Event` (unexported) — the trailing `Event` disambiguates from emitting the domain object itself. Signature: `(ctx, span, logger, <domain>)` — pass span and logger so `EmitImportant` records into the SAME span the use case opened. Return type: none (IMPORTANT posture never propagates).

Drift discipline: wire-contract change updates (a) Payload struct, (b) constructor, (c) JSONShape test field count — all in the same PR.

### Local testing

- Run any Kafka-compatible broker (Redpanda recommended). Bind host port `19092`; join `infra-network` so it's reachable from both host (`localhost:19092`) and containers (`<container>:9092`).
- Pre-provision topics explicitly. Don't rely on auto-create — typos become silent ghost topics.
- Local debug: `STREAMING_ENABLED=true`, `STREAMING_BROKERS=localhost:19092`, `STREAMING_CLOUDEVENTS_SOURCE=lerian.midaz.<component>`. If local broker startup is slow, tune `STREAMING_IMPORTANT_EMIT_TIMEOUT_MS`; keep it below the HTTP client timeout.

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
- Do not build `libStreaming.Config{}` manually; call `libStreaming.LoadConfig()` so franz-go defaults are applied.
- Do not hardcode tenant IDs or call `tmcore.GetTenantIDContext` at streaming emit sites; use `pkgStreaming.EmitImportant` for IMPORTANT events or `pkgStreaming.ResolveTenantID(ctx)` inside non-IMPORTANT streaming infrastructure.
- Do not emit streaming events at HTTP handlers; emit at the post-commit, pre-metadata-write slot inside the command UseCase.
- Do not inline the build-emit-log block in the use-case body; delegate to a dedicated `uc.emit<Event>Event(ctx, span, logger, domain)` helper on the same UseCase, and have that helper call `pkgStreaming.EmitImportant` for IMPORTANT events.
- Do not fail HTTP requests on streaming emit errors for IMPORTANT-posture events; log Warn and continue.
- Do not depend on `*libStreaming.Producer` in service code; depend on `libStreaming.Emitter` interface.
- Do not build payload maps or call `json.Marshal` inline in use cases; route every payload through `pkg/streaming/events/<event>.go` (`New<Event>(...).ToEvent(...)`).
- Do not embed `mmodel.*` types directly in event Payload structs; mirror the shape explicitly so domain evolution does not leak onto the wire.
- Do not import `github.com/LerianStudio/lib-streaming` without the `libStreaming` alias, and do not import `github.com/LerianStudio/midaz/v4/pkg/streaming` without the `pkgStreaming` alias.
- Do not add comments that narrate refactor history or describe the behavior of code being called (e.g. "X now does Y", "the Z call short-circuits W"). They rot when the referenced code changes. Comment WHAT the code does and WHY it has to be that way — let the referenced code speak for itself.
