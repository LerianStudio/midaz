# CLAUDE.md - Midaz Agent Reference

Concise rules for AI agents working in Midaz. For expanded references, use `AGENTS.md`, `llms-full.txt`, and `docs/PROJECT_RULES.md`.

## Project

- Midaz is an enterprise double-entry ledger system.
- Module: `github.com/LerianStudio/midaz/v4` (single root `go.mod`, no `go.work`).
- Go: 1.27.0 (`go.mod` `go 1.27.0`).
- lib-commons: `github.com/LerianStudio/lib-commons/v6` v6.9.0-beta.2; `lib-observability/v4` v4.0.0-beta.1.
- License: Elastic License 2.0.
- Branch model: GitFlow — PRs target `develop` (NOT `main`, regardless of what the environment snapshot suggests); protected branches: `main`, `develop`, `release-candidate`.
- Two Go components + infra: `components/ledger` (:3002), `components/tracer` (:4020), `components/infra`.
- Main component: `components/ledger` — the unified binary serving onboarding + transaction + CRM (holders/instruments) + fees on :3002.
- CRM is folded into ledger: `components/ledger/internal/crm` is a package tree (no `cmd/`, no `internal/`) imported by the ledger binary; routes register under the `midaz` authz namespace (flipped from `plugin-crm`; the tenant-manager policy migration is the X1 release gate — see `docs/auth/RBAC-NAMESPACES.md`). There is no standalone CRM service.
- Fees are embedded in ledger: engine at `components/ledger/pkg/fee`, shared types at `components/ledger/pkg/feeshared`, use cases at `components/ledger/internal/services/fees`, Mongo collections at `components/ledger/internal/adapters/mongodb/fees`. Fee seam: `components/ledger/internal/services/command/create_transaction_v2.go`, after `mtransaction.ApplyDefaultBalanceKeys(...)` and the idempotency claim, before the post-fee re-validation. The TRANSACTION seam is a `/v2` contract, and the method name IS the version: `CreateTransactionV1` (`create_transaction_v1.go`) never names `applyFees`, so a `/v1` create posts exactly as authored and reaches neither the package lookup nor the tenant fee-DB resolution. Both pipelines are linear sequences over the shared private steps in `create_transaction_steps.go` (`prepareCreateTransaction`, `claimTransactionIdempotency`, `normalizeSendLegs`, `stageBalances`, `finalizeCreatedTransaction`, plus the two rollbacks); no `policy`/`isRevert` boolean travels through them. Revert is split the same way (`RevertTransactionV1`/`RevertTransactionV2`, `revert_transaction.go`): a shared `prepareRevertTransaction` eligibility gate, then the version's pipeline with `action = revert` — and neither version applies fees, because `TransactionRevert` already reconstructs the reversed fee legs. The same policy gates the tracer (see `## Tracer Reservation Seam`); each seam decides for itself what the version means. This is separate from the fee ADMIN surface (packages, estimates, billing), which is served on both scopes; see `docs/api/SCOPING.md`.
- Account holder linkage is a `/v2` contract: the holder seam in `create_account.go` (`resolveAccountHolder`) — the `requireHolder` gate, the two-key `skip.holder` control, and the self-holder default that materialises `holder_id` — is threaded a `command.RouteHolderPolicy` (`HolderOffV1`/`HolderOnV2`, `account_holder_policy.go`) from the transport shell. It is the sibling of the transaction paths' version split, which encodes the same idea in the use-case name instead of a policy value; the account path threads a value because it has a single `CreateAccount` use case for both contracts. `HolderOffV1` is the FIRST gate, short-circuiting BEFORE the ledger settings read, so a `/v1` create links no holder (`holder_id` NULL, `holder_check_skipped` false) and acquires none of the seam's rejection classes. Every `/v1` account response projects onto `in.AccountV1` (`account_output_v1.go`), withholding `holderId` + `holderCheckSkipped`; `ledgerSchemaNamer` publishes that projection under the canonical `Account` component name so v1 SDKs do not churn, which puts the holder-bearing `mmodel.Account` on `AccountV2`. Composition (`/v2` only) passes `HolderOnV2`. Organization create is outside the seam on BOTH contracts: neither writes a CRM self-holder, and the idempotent backfill runner (`components/ledger/cmd/backfill`) is the only path by which an organization acquires its deterministic self-holder — the derivation (`DeriveSelfHolderID`, `holder_ports.go`) stays because the account-create default and the backfill both read it. The organization wire shape carries no holder field and no organization op is version-specific, so both contracts bind the same handler methods and differ only in the operation IDs they publish (`opSuffix`). Outside the seam on BOTH contracts: the asset-created external account (built directly via `AccountRepo.Create`, bypassing `CreateAccount`) and the account update path (`holderId` is immutable — not on `UpdateAccountInput`, and absent from the update SET list). See `docs/api/SCOPING.md`.
- Tracer is a co-located but separate Go service deploy unit at `components/tracer` (:4020); it integrates with ledger over the reservation seam (gRPC/mTLS — see `docs/architecture/`).
- Shared code: `pkg` (root; `pkg/mtransaction` was formerly `pkg/transaction`) and `tests` (root).

## Tracer Reservation Seam

The reservation lifecycle is a `/v2` contract. Three seams live in `components/ledger/internal/services/command/transaction_reservation_anchor.go`:

- `reserveTransaction` — create (all six modes) and revert. Called immediately before `ProcessBalanceOperations` on FEE-INCLUSIVE amounts. Only the `/v2` pipelines (`CreateTransactionV2`, `createRevertV2`) name it, so a `/v1` request builds no reserve request and dials nothing. Unlike `applyFees` it IS called on a revert: limits measure GROSS activity, so a `/v2` revert reserves capacity of its own and never refunds the origin's (Q9 no-refund).
- `confirmReservationsByTransaction` / `releaseReservationsByTransaction` — commit / cancel, addressed by transaction id because the create-pending handle does not survive the separate request. Only `transitionPendingV2` (`command/commit_transaction.go`) names them, after `commitPendingBalances` and before `finalizePendingTransition`; `transitionPendingV1` names neither, so a `/v1` commit or cancel builds no request and dials nothing.

Beyond the version split the seams answer three more axes: nil `TracerReserver` (`TRACER_BASE_URL` unset), per-ledger `tracer.mode` (`off`/`advisory`/`enforce`, default `off`), and an honored per-call `skip.tracer` — a field that exists ONLY on `CreateTransactionV2Input`, so a `/v1` body naming `skip` is a 400 unknown field. Under `enforce` a denial is `0177`/422 and an unavailable tracer branches on `failPosture` (`open` → proceed, `closed` → `0178`/**503**); `advisory` never blocks. There is NO tracer readiness prober in `bootstrap/readyz.go`, so an unavailable tracer under enforce+closed produces 503s with a green `/readyz`.

`app.transaction.tracer_route_eligible` is a span attribute only. Do NOT fold it into `tracer_skipped`, which is a persisted column (`tracer_skipped`, migration `000035`) recording a skip the CLIENT asked for — marking it on every `/v1` create would record a claim never made. Same rule as `fees_route_eligible`.

Known gap, documented in `docs/api/SCOPING.md`: a PENDING created on `/v2` and committed through `/v1` never receives its confirm — the by-transaction call cannot tell whether reservations exist, so `transitionPendingV1` names no seam and the TTL reaper releases the capacity instead of counting it. Mixing mounts across one lifecycle is unsupported. Closing it needs create-time reservation state persisted on the transaction row.

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

## Dependencies

- lib-commons v6 (`github.com/LerianStudio/lib-commons/v6/commons/...`, currently v6.5.1): app config, env/security/pointer helpers (`libCommons`), Redis, HTTP helpers (`libHTTP`, non-observability), circuit breaker, tenant managers (`tm*`).
- Observability is a separate module `github.com/LerianStudio/lib-observability/v4`: `log` (`libLog`), `zap` (`libZap`), `tracing` (`libOpentelemetry`), `metrics`, `middleware` (`libMid`: `NewTelemetryMiddleware`, `WithHTTPLogging`). Context helpers (`NewTrackingFromContext`, `NewLoggerFromContext`, `ContextWith*`) live in the `lib-observability` root package. `NewTrackingFromContext` returns `(log.Logger, trace.Tracer, string, *metrics.MetricsFactory)`.
- TLS enforcement: the postgres/mongo/redis/rabbitmq constructors enforce TLS by the security tier derived from `ENV_NAME` and refuse plaintext dependencies unless `ALLOW_INSECURE_TLS=true` (parsed as a bool via `commons.AllowInsecureTLS`). Set in the `.env.example` files; connection-building unit tests set it in their `TestMain`.
- MongoDB driver: `go.mongodb.org/mongo-driver/v2`. `bson/primitive` is consolidated into `bson` (`bson.ObjectID`, `bson.NewObjectID`). v2 decodes nested documents into `bson.D` (ordered), not `bson.M`; code that type-asserts nested values as `bson.M` must also handle `bson.D` (`bson.D` has no `.Map()`).
- CRM field encryption (envelope mode): `github.com/hashicorp/vault/api` v1.23.0 (Vault Transit KMS client) and `github.com/tink-crypto/tink-go/v2` v2.7.0 (Tink AEAD + PRF keysets for field encryption / search tokens). See `## CRM Field Encryption / KMS`.

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

Binding standard: `docs/standards/telemetry.md` (T6 structured logging, T7 log levels, T8 single-point logging). The rules below are the quick reference; the standard governs on any conflict.

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

Binding standard: `docs/standards/telemetry.md` (T1–T13). The span-error helper is chosen by error CLASS (T5): business/4xx errors use `HandleSpanBusinessErrorEvent` (span stays green), technical/5xx errors use `HandleSpanError` (span flips red). The rules below are the quick reference; the standard governs on any conflict.

Span lifecycle:

- Always `defer span.End()` immediately after `tracer.Start`.
- Open a child I/O span only where the driver is not auto-instrumented: MongoDB and RabbitMQ yes, PostgreSQL and Redis/Valkey no — `lib-commons` instruments both of those pools (`sqlobs`/`redisobs`), so a hand-rolled `postgres.<op>.query`/`.exec` or one-command `redis.<cmd>` span only duplicates the driver's. See T2 for the full table.
- When a child I/O span is not opened, its error handling and attributes belong on the parent domain span — the driver span carries no business-vs-technical class and does not see client-side scan/mapping failures.
- For the child I/O spans that remain, preserve parent context: use `_, spanInsert := tracer.Start(ctx, "...")`, not `ctx, spanInsert := ...`.
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
span.SetAttributes(attribute.Int64("db.rows_affected", rowsAffected))
```

## Errors

Binding standard: `docs/standards/error-handling.md` (E1–E14). One error platform in `pkg/errors.go` + `pkg/constant/errors.go`; the canonical sentinel registry is numeric only (the `FEE-`/`TRC-`/`TPL-`/`REP-` prefixed families are retired). The rules below are the quick reference; the standard governs on any conflict.

- API errors use typed errors from `pkg/errors.go` and constants from `pkg/constant/errors.go`.
- Use `pkg.ValidateBusinessError(constant.Err..., constant.Entity...)`.
- Not found maps to `EntityNotFoundError` / HTTP 404.
- Business rule violations map to `UnprocessableOperationError` / HTTP 422.
- Do not expose internal technical error details to API clients.
- Do not create duplicate `errors.New("code")` sentinels outside `pkg/constant/errors.go`.

## HTTP

- HTTP layer runs Huma v2 (OAS 3.1) over Fiber v3: Fiber is the runtime router / auth chain / middleware; Huma sits on top to generate the API contract and validate requests via typed request/response structs. Each resource is split in three: `<resource>_core.go` holds the handler struct and its transport-agnostic cores (span, service call, metric), `<resource>_handler.go` holds the Huma transport — the `<Op>Request`/`<Op>Response` envelopes and the handler methods — and `<resource>_routes.go` holds the registrars: the `Register<R>Routes` Huma terminals, the `Register<R>RoutesToApp` / `Register<R>V2RoutesToApp` mounts and the `register<R>RoutesToApp` body carrying the Fiber guard chain. A `_v2` variant is suffixed, not prefixed (`transaction_routes_v2.go`, `transaction_handler_v2.go`), so it sorts beside its v1 sibling. Cores take primitive args, so nothing transport-shaped reaches them.
- The error envelope is a function of the ROUTE VERSION, resolved by the `versionEnvelopes` registry in `components/ledger/internal/adapters/http/in/middleware/envelope.go`. `>= /v2` gets the RFC 9457 `application/problem+json` document `WithError`/`HumaProblem` produce (`type`, `title`, `status`, `detail`, `instance`, plus `code` and `entityType`); `/v1` gets the legacy `{entityType?, title, message, code, fields?}` body at `application/json`, restored because v3 clients in production parse it. The `ErrorEnvelope` middleware reshapes on the way out — producers never learn about versions. The `(code, HTTP status)` tuple is identical across versions.
- The ledger does NOT scrub `>=500` title/detail: it calls `midazhttp.DisableHighStatusScrub()` so a 5xx carries its sentinel's registry text, because several 5xx codes describe a caller mistake and `"internal error"` masked them. Tracer keeps the scrub. The bound: a `>=500` message may interpolate CALLER-supplied values only — never `err.Error()` or an internal ID.
- All routes use `http.ProtectedRouteChain()`.
- Route protection includes auth, optional post-auth middlewares, body parsing, UUID path validation, and handler.
- Use existing route helpers and middleware patterns in `components/ledger/internal/adapters/http/in`.

## Domain Notes

- Hierarchy: Organization -> Ledger -> Assets/Portfolios/Segments -> Accounts -> Transactions -> Operations -> Balances.
- Status common codes: `ACTIVE`, `INACTIVE`, `DELETED`, `PENDING`, `CANCELLED`.
- Transaction creation modes: JSON, inflow, outflow, annotation.
- Pending transactions can be committed/cancelled; revert creates a reverse transaction.
- Async transaction processing is controlled by `RABBITMQ_TRANSACTION_ASYNC`.
- Balance fields: `Available`, `OnHold`, `Scale`, `Version`.

## Streaming (lib-streaming events)

Producer is `github.com/LerianStudio/lib-streaming/v4`. Wire format: CloudEvents 1.0 binary mode on Kafka. ONE TOPIC PER PRODUCING APPLICATION: every event an application emits — every resource type, every event type, every schema version — rides `lerian.streaming.<app>`, where `<app>` is the application's ce-source, with `lerian.streaming.<app>.dlq` as its single dead-letter topic. `<app>` is `ledger` from the ledger binary (which encompasses ledger, fee, and crm events — fees keep a `fee_` ResourceType prefix, which is what namespaces them inside the application's event space) or `tracer` from the tracer service, so the topics are `lerian.streaming.ledger(.dlq)` and `lerian.streaming.tracer(.dlq)`. Topic names come from `libStreaming.AppTopic(source)` / `AppDLQTopic(source)`; there is no per-event topic and no `.v<major>` suffix — `ce-schemaversion` is the ONLY version carrier on the wire. A `lerian.streaming.<app>.commands` queue exists in the contract for service-to-service commands, but midaz emits FACTS only and has none — do not provision one. ce-type is `studio.lerian.<app>.<resource>.<event>` (e.g. `studio.lerian.ledger.account.created`, `studio.lerian.tracer.rule.created`): the app segment is what stops two services emitting byte-identical ce-type values for same-named events — a homonym collision a consumer reading only ce-type could not detect, and one a shared per-application topic makes reachable in practice. `Definition.Key()` = `<resource>.<event>` is the consumer's dispatch key inside the app stream, and `Definition.Key()` / ResourceType / EventType / ce-type are all underscore-preserving (e.g. `operation_route`, `fee_packages`, `related_party_deleted`); route keys accept underscores, so nothing is folded to hyphens anywhere. ce-source is STRICT: ONE dot-free lowercase segment matching `^[a-z0-9][a-z0-9_-]*$`, at most 223 bytes, REJECTED at startup rather than normalized — dotted (`lerian.midaz.ledger`) and URI (`//lerian.midaz/ledger`) values no longer parse. The resolved `STREAMING_CLOUDEVENTS_SOURCE` drives THREE things at once (the ce-source header, the derived topic, and what the streaming manifest advertises), so a Kafka ACL grants an application only its OWN names (its topic and `.dlq`) instead of a prefix over an open per-event namespace. It is also PINNED: `BuildStreamingEmitter` refuses boot (`pkgStreaming.RequireRosterSource`) unless the resolved source equals the roster constant (`ledger`/`tracer`), whether or not streaming is enabled. The tenant-manager grants WRITE+DESCRIBE on LITERAL topic names derived from the roster name alone — literal so a grant cannot reach a neighbouring app — so any other source, however grammar-legal, publishes to a topic that neither exists nor is granted; the IMPORTANT posture would swallow that as a Warn while pods stayed Ready. Single-tenant deployments key every event to the literal tenant `default`, so the whole app stream lands on ONE partition: a throughput ceiling (ordering only gets stronger), pending the platform partition-key default in lib-streaming. Do NOT wire a local `Builder.PartitionKey` override. The rule is unconditional and has no exceptions: `lerian.streaming.<app>` and its `.dlq` are a binary's entire write surface, and no event is routed anywhere else. The ledger and tracer binaries each serve `GET /v1/streaming/manifest` (catalog-only, manifest wire version `1.0.0`: `topic`/`dlqTopic` at DOCUMENT level, each event carrying `eventKey` + `class`, always `fact` for midaz). The canonical wire contract lives in code under `pkg/streaming/events/`; the JSONShape unit test in that package locks it against drift.

### Producer conventions

- Import aliases: `libStreaming` for `github.com/LerianStudio/lib-streaming/v4`; `pkgStreaming` for `github.com/LerianStudio/midaz/v4/pkg/streaming`. Keep both distinct.
- Build config via `libStreaming.LoadConfig()` (reads `STREAMING_*` env with correct franz-go defaults). NEVER construct `libStreaming.Config{}` manually. Master flag stays in midaz `Config.StreamingEnabled`.
- Broker security is lib-streaming's: hand the loaded config to `builder.TLSFromConfig(streamingCfg)` and `builder.SASLFromConfig(streamingCfg)`. NEVER bind `STREAMING_TLS_*` / `STREAMING_SASL_*` onto a midaz `Config` struct and NEVER build a franz-go `sasl.Mechanism` by hand — the tracer did, and the duplicate struct simply had no TLS field, so `STREAMING_TLS_ENABLED` had no reader at all and a TLS broker was unreachable while every authenticated deployment was pushed through the unsafe plaintext opt-in. With `DEPLOYMENT_MODE=saas` an ENABLED producer additionally REQUIRES `STREAMING_TLS_ENABLED=true`: a plaintext broker dial REFUSES BOOT (`ValidateSaaSStreamingTLS` in each component's bootstrap, called right after `LoadConfig` — the first point where the flag exists), matching the gate Postgres/Mongo/Redis/RabbitMQ already answer to. BYOC and local keep their plaintext brokers, and `STREAMING_ENABLED=false` never reaches the gate because no broker connection is opened.
- `STREAMING_ENABLED=true` with an empty `STREAMING_BROKERS` REFUSES BOOT (`pkgStreaming.RequireBrokers`, which validates only the broker list). The tracer additionally refuses boot on an empty event registry via a separate inline check in its `BuildStreamingEmitter`; the ledger has no such catalog gate. There is exactly ONE noop path left: `STREAMING_ENABLED=false`. The tracer's `/readyz` reports that one as `skipped`; the ledger has no streaming readiness prober, so its `/readyz` carries no streaming check in either mode. An enabled producer that fell back to `NoopEmitter` discarded every event while the noop answered the readiness probe healthy: total loss, green dashboards, the same failure class the roster source gate exists to kill.
- `CloudEventsSource` is owned by the producer Builder at construction (`libStreaming.NewBuilder().Source(cfg.CloudEventsSource)` in bootstrap), not held on the UseCase and not passed per emit. Emit sites never set `Source`; the `EmitRequest` carries only `DefinitionKey`/`TenantID`/`Subject`/`Timestamp`/`Payload`.
- Tenant value from `pkgStreaming.ResolveTenantID(ctx)` — returns the multi-tenant context value or `pkgStreaming.DefaultTenantID` (literal `"default"`). Reference the constant, not the literal. NEVER hardcode tenants or call `tmcore.GetTenantIDContext` at emit sites. For IMPORTANT events, `pkgStreaming.EmitBrokerBestEffort` resolves the tenant internally and passes it to the typed event builder closure.
- Service code depends on `libStreaming.Emitter` INTERFACE, never `*libStreaming.Producer`. Nil emitter means "disabled" — guard with `if uc.Streaming != nil`. When `STREAMING_ENABLED=false`, bootstrap injects `libStreaming.NewNoopEmitter()`.
- IMPORTANT-posture broker publication MUST go through `pkgStreaming.EmitBrokerBestEffort`. Build/emit failures MUST NOT fail the request: log Warn, span-record, return success. `EmitBrokerBestEffort` bounds the `Emitter.Emit` call with `STREAMING_IMPORTANT_EMIT_TIMEOUT_MS` (default 5s) so broker issues cannot hold HTTP responses until client timeout. It delegates policy and any configured fallback to lib-streaming; Midaz currently wires neither an outbox writer/repository nor a relay, so it provides no product-local transactional fallback or delivery guarantee.
- Emit POST-COMMIT and PRE-METADATA-WRITE — never at HTTP handlers. `ce-subject` is the aggregate ID, passed as `libStreaming.EmitRequest.Subject`.
- Register the producer's `Close()` as `libCommons.RunApp("Streaming Producer", ...)` so it drains on SIGTERM (mirror `eventListenerRunnable`).
- lib-streaming is pinned at v4.0.0-beta.4 (module path `.../lib-streaming/v4`), which exports the Catalog/policy constants (e.g. `BuildManifest`, `DefaultDeliveryPolicy`, `ResolveDeliveryPolicy`) plus the topic derivations `AppTopic` / `AppDLQTopic`. The producer is assembled with `libStreaming.NewBuilder()` (`.Source()/.Catalog()/.Routes()/.Target()`) around ONE catch-all route to the app topic (empty `DefinitionKey`), and midaz registers no per-definition route override: every event takes that one route. Midaz currently does not pass `WithOutboxRepository` or `WithOutboxWriter`, and does not register an outbox relay; delivery behavior remains lib-streaming's configured policy.

### Event modeling (`pkg/streaming/events`)

One file per event. Use cases NEVER build payload maps inline. Required shape per file:

1. **Definition var** — `<Event>Definition = events.Definition{ResourceType, EventType, SchemaVersion}`.
2. **Payload struct** — wire JSON fields, typed INDEPENDENTLY of `mmodel.*` (mirror nested types explicitly so domain evolution doesn't leak onto the wire).
3. **Constructor** — `New<Event>(domain *mmodel.X) <Event>Payload`. Place for PII redaction, derived fields, contract-locked defaults.
4. **ToEmitRequest method** — `(p <Event>Payload) ToEmitRequest(tenantID string, ts time.Time) (libStreaming.EmitRequest, error)`. Marshals payload + sets `DefinitionKey`, `TenantID`, `Subject`, `Timestamp`. `Source` is NOT on the request (owned by the Builder); `ResourceType`/`EventType`/`SchemaVersion` resolve from the Catalog by `DefinitionKey`. Wrapped `json.Marshal` errors so caller picks Warn (IMPORTANT) vs fail (CRITICAL).

Required unit tests: Definition key lock, minimal-domain mapping, all-optional-fields mapping, ToEmitRequest assembly, JSON shape lock (top-level key set + field count).

### IMPORTANT emission helper pattern

The use-case body MUST NOT inline emission mechanics. Delegate to a private `emit<Event>Event` method on the same UseCase; that method MUST call `pkgStreaming.EmitBrokerBestEffort` for IMPORTANT-posture events:

```go
// in CreateAccount, at the emission anchor:
uc.emitAccountCreatedEvent(ctx, span, logger, acc)

// helper alongside other private UseCase methods:
func (uc *UseCase) emitAccountCreatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, acc *mmodel.Account) {
    pkgStreaming.EmitBrokerBestEffort(ctx, span, logger, uc.Streaming, events.AccountCreatedDefinition.Key(),
        func(tenantID string) (libStreaming.EmitRequest, error) {
            return events.NewAccountCreated(acc).ToEmitRequest(tenantID, acc.CreatedAt)
        })
}
```

`EmitBrokerBestEffort` owns the common IMPORTANT-posture mechanics: nil-emitter guard, tenant resolution, bounded emit context, `libOpentelemetry.HandleSpanError` (build/emit failures are technical, so per T5 they flip the span red — not `HandleSpanBusinessErrorEvent`), Warn logging with `libLog.Err(err)`, and non-propagation of build/emit failures. Use-case helpers remain explicit only about the typed payload constructor, event definition key, subject, and timestamp.

Naming: `emit<Event>Event` (unexported) — the trailing `Event` disambiguates from emitting the domain object itself. Signature: `(ctx, span, logger, <domain>)` — pass span and logger so `EmitBrokerBestEffort` records into the SAME span the use case opened. Return type: none (IMPORTANT posture never propagates).

Drift discipline: wire-contract change updates (a) Payload struct, (b) constructor, (c) JSONShape test field count — all in the same PR.

### Local testing

- Run any Kafka-compatible broker (Redpanda recommended). The local compose stack binds host port `19092` by default; set `REDPANDA_EXTERNAL_PORT` in `components/infra/.env` when another process owns that port. Join `infra-network` so the broker is reachable from both host (`localhost:<external-port>`) and containers (`<container>:9092`).
- Since lib-streaming v4.0.0-beta.4 the producer creates the application's OWN two topics at construction — `lerian.streaming.<app>` and `lerian.streaming.<app>.dlq`, derived from the resolved ce-source — so a local broker needs no pre-provisioning for them. A CreateTopics call that the ACL refuses logs a WARN and startup continues; `STREAMING_TOPIC_AUTO_PROVISION=false` opts out for environments that provision through IaC. Those two topics are all a binary writes, so nothing else needs pre-provisioning. There is no per-event topic list.
- Local debug: `STREAMING_ENABLED=true`, `STREAMING_BROKERS=localhost:<external-port>` (matching `REDPANDA_EXTERNAL_PORT`, default `19092`), `STREAMING_CLOUDEVENTS_SOURCE=<app>` (the application name — `ledger` or `tracer`). If local broker startup is slow, tune `STREAMING_IMPORTANT_EMIT_TIMEOUT_MS`; keep it below the HTTP client timeout.

## Multi-Tenancy

- Enabled via `MULTI_TENANT_ENABLED=true`; auth must also be enabled.
- Tenant ID comes from JWT via auth middleware.
- Tenant DB resolution uses tenant middleware and lib-commons tenant managers.
- Modules `onboarding` and `transaction` have independent PostgreSQL and MongoDB managers.
- Multi-tenant code uses `MT` suffix for names (`NewFooMT`, `runFooMT`, `mtEnabled`, `isMTReady`). Single-tenant code uses no qualifier.
- Under CRM envelope encryption, key material is per-organization; a single shared Vault Transit engine holds all KEKs and tenant isolation lives in the key NAME (`{tenant}_org-{id}`), not in per-tenant mounts. See `## CRM Field Encryption / KMS`.

## CRM Field Encryption / KMS

CRM encrypts holder/instrument PII at rest. The seam is the `FieldEncryptor` interface (`components/ledger/internal/crm/services/encryption`), which the holder/instrument Mongo adapters call to encrypt/decrypt fields and to generate deterministic HMAC search tokens for equality lookups over ciphertext. Deep doc: `docs/architecture/crm-field-encryption.md`.

- Mode is selected by `KMS_VENDOR`: unset/`none` -> legacy (lib-commons symmetric crypto, no KMS); `hashicorp-vault` -> envelope (HashiCorp Vault Transit KEK wrapping per-organization Tink DEKs).
- Shared-engine + tenant-scoped-key-name model: one mode-derived Transit engine (`transit-mt` MT / `transit-st` ST) holds all KEKs; the KEK key name carries scope (`{tenant}_org-{id}` MT, `org-{id}` ST). No per-tenant mounts.
- Envelope-mode routes (midaz namespace, unregistered in legacy mode): `POST .../encryption/provision`, `GET .../encryption/status`, `GET .../protection/audit`.
- Env: `KMS_VENDOR`, `KMS_VAULT_ADDR`, `KMS_VAULT_ROLE_ID`, `KMS_VAULT_SECRET_ID`, `KMS_VAULT_AUTH_METHOD` (`approle`|`token`), `DEPLOYMENT_MODE` (gates the dev root token to `local` only). Legacy AES/HMAC keys: `LCRYPTO_ENCRYPT_SECRET_KEY`, `LCRYPTO_HASH_SECRET_KEY` (live cipher in legacy mode, imported for reads in envelope mode).
- Key rotation is scaffolded, not yet active.

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
- Do not hardcode tenant IDs or call `tmcore.GetTenantIDContext` at streaming emit sites; use `pkgStreaming.EmitBrokerBestEffort` for IMPORTANT events or `pkgStreaming.ResolveTenantID(ctx)` inside non-IMPORTANT streaming infrastructure.
- Do not emit streaming events at HTTP handlers; emit at the post-commit, pre-metadata-write slot inside the command UseCase.
- Do not inline the build-emit-log block in the use-case body; delegate to a dedicated `uc.emit<Event>Event(ctx, span, logger, domain)` helper on the same UseCase, and have that helper call `pkgStreaming.EmitBrokerBestEffort` for IMPORTANT events.
- Do not fail HTTP requests on streaming emit errors for IMPORTANT-posture events; log Warn and continue.
- Do not depend on `*libStreaming.Producer` in service code; depend on `libStreaming.Emitter` interface.
- Do not build payload maps or call `json.Marshal` inline in use cases; route every payload through `pkg/streaming/events/<event>.go` (`New<Event>(...).ToEmitRequest(...)`).
- Do not embed `mmodel.*` types directly in event Payload structs; mirror the shape explicitly so domain evolution does not leak onto the wire.
- Do not import `github.com/LerianStudio/lib-streaming/v4` without the `libStreaming` alias, and do not import `github.com/LerianStudio/midaz/v4/pkg/streaming` without the `pkgStreaming` alias.
- Do not add comments that narrate refactor history or describe the behavior of code being called (e.g. "X now does Y", "the Z call short-circuits W"). They rot when the referenced code changes. Comment WHAT the code does and WHY it has to be that way — let the referenced code speak for itself.
