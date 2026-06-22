# Telemetry Standard

This is the single telemetry standard for every Go service in the Midaz monorepo: `components/ledger` (including the folded-in CRM and fees code) and `components/tracer`. (The reporter is no longer in this monorepo — it ships from its own standalone repo; the canonical reporter `file:line` examples below are preserved as evidence from the 2026-06-07 audit, when the reporter still lived here.) It governs traces, logs, and metrics — the three telemetry signals. It does **not** govern the lib-streaming event bus, which is a separate data-plane wire contract (see T9). The rules below are derived from the 2026-06-07 telemetry/error audit and are binding; the machine-readable findings are in [`../plans/2026-06-07-telemetry-error-audit.json`](../plans/2026-06-07-telemetry-error-audit.json) and the count/coverage appendix in [`../plans/2026-06-07-telemetry-error-audit-appendix.md`](../plans/2026-06-07-telemetry-error-audit-appendix.md). Each rule carries a normative statement, a one-paragraph rationale, a verified in-repo canonical example (`file:line`), and the enforcement vehicle that gates it (wired in Phase 6).

Two rules — **T5** (span-error helper by class) and **T8** (single-point logging) — are defined **only here** and referenced by `error-handling.md`. They are not restated there; do not duplicate them.

---

## T1 — Telemetry acquisition

**Rule:** Telemetry handles MUST be acquired via `libObservability.NewTrackingFromContext(ctx)` at the top of each instrumented function. MUST NOT inject a tracer or logger into struct fields, and MUST NOT call the raw OpenTelemetry API directly.

**Rationale:** A single acquisition path keeps the logger trace-correlated and the tracer context-bound without per-struct wiring, and prevents an un-instrumented logger or a raw `otel.Tracer(...)` from drifting out of the lib-observability conventions that everything else depends on.

**Canonical example:** [`components/ledger/internal/services/command/create_account.go:35`](../../components/ledger/internal/services/command/create_account.go) — `logger, tracer, requestID, _ := libObservability.NewTrackingFromContext(ctx)`.

**Enforcement:** `custom-lint` — flag `otel.Tracer(`/`otel.GetTracerProvider(` outside bootstrap, and `*libLog.Logger`/`trace.Tracer` struct fields in service/adapter packages.

---

## T2 — Span naming

**Rule:** Span names MUST be `<layer>.<operation>` in dotted snake_case (e.g. `command.create_account`, `mongodb.update_holder`). Child I/O spans MUST extend the parent name with a `.exec` / `.query` / `.find` (etc.) suffix. MUST NOT use bespoke prefixes or free-form names.

**Rationale:** A predictable two-segment scheme makes spans groupable by layer and operation across services and lets dashboards and trace search rely on a stable naming contract instead of per-author conventions.

**Canonical example:** [`components/ledger/internal/adapters/postgres/account/account.postgresql.go:187`](../../components/ledger/internal/adapters/postgres/account/account.postgresql.go) — `_, spanExec := tracer.Start(ctx, "postgres.create.exec")` (child I/O span with `.exec` suffix under the `postgres.create_account` parent).

**Enforcement:** `review-only` — naming intent is not mechanically distinguishable from valid free text; gate at code review.

---

## T3 — Span lifecycle and context binding

**Rule:** `defer span.End()` MUST immediately follow `tracer.Start`. Child I/O spans MUST use the non-rebinding form `_, spanX := tracer.Start(ctx, ...)`. The parent `ctx` MAY be rebound (`ctx, spanX := ...`) ONLY for genuinely sequential nesting, or for a deliberate `context.WithoutCancel` detach — and in either case an intent comment MUST state why.

**Rationale:** Rebinding `ctx` on a leaf I/O span makes sibling spans nest under each other instead of under the shared parent, because `span.End()` does not restore the parent into the Go `ctx`; the audit traced 135+ confirmed leaf sites distorting trace topology this way. Detach via `WithoutCancel` is the one legitimate rebind — it preserves trace and values while severing cancellation — and must be self-documenting.

**Canonical example (sanctioned detach):** [`components/tracer/internal/services/validation_service.go:614`](../../components/tracer/internal/services/validation_service.go) — `context.WithTimeout(context.WithoutCancel(ctx), validationPersistTimeout)` with the preceding intent comment (lines 611–613) explaining the SOX/GLBA audit-trail persistence budget.

**Counter-example (forbidden leaf rebind):** the shape to reject is `ctx, spanExec := tracer.Start(ctx, "postgres.create.exec")` — rebinding `ctx` on a leaf exec/query span with no detach intent, which nests sibling I/O spans under each other instead of the parent. The audit's 135+ leaf rebinds were converted to the non-rebinding `_, spanX :=` form (see the canonical example above); the lint gate keeps them from reappearing.

**Enforcement:** `custom-lint` — flag `ctx, span\w* := .*tracer.Start` on leaf spans; allowlist requires an adjacent `WithoutCancel` or an intent comment (per-file review where ambiguous).

---

## T4 — Span attributes

**Rule:** Inputs (handler args, path/query params, payload-derived values) MUST use the `app.request.*` namespace. Outputs and system observations MUST use `db.*` / `app.response.*`. `libOpentelemetry.SetSpanAttributesFromValue` with a **nil Redactor is BANNED**. Sensitive or large inputs MUST be reduced to boolean presence flags (`has_*`) plus counts, never the value itself.

**Rationale:** `SetSpanAttributesFromValue(span, prefix, value, nil)` flattens every struct field onto the span — the mechanism behind the audit's P0 PII leak (76 of 100 call sites passed a nil Redactor, including plaintext CPF/email/phone before encryption). Namespacing inputs vs. outputs keeps span queries unambiguous, and presence-flags preserve observability of optionality without putting the data on the wire.

**Canonical example (presence-flag idiom):** [`components/ledger/internal/crm/adapters/mongodb/holder/holder.mongodb.go:133`](../../components/ledger/internal/crm/adapters/mongodb/holder/holder.mongodb.go) — `attribute.Bool("app.request.repository_input.has_contact", record.Contact != nil)` and the surrounding `has_*` block (lines 131–136).

**Counter-example (banned nil-Redactor flatten):** the shape to reject is `SetSpanAttributesFromValue(spanUpdate, "app.request.repository_input", holder, nil)`, which flattens the full holder (PII) onto the span. `SetSpanAttributesFromValue` with a nil Redactor now has **zero call sites** repo-wide — the holder paths were rewritten to the `has_*` presence-flag block above; the `forbidigo` gate keeps the flatten from returning.

**Enforcement:** `forbidigo` — pattern `SetSpanAttributesFromValue\(.*,\s*nil\)` rejected in non-test code; `app.request.*` vs `db.*`/`app.response.*` namespace placement is `review-only`.

---

## T5 — Span-error helper by error class

**Rule:** The span-error helper MUST be chosen by **error class**, decided through the shared `IsBusinessError` predicate — never by code origin or call-site convenience. Business / 4xx errors MUST use `libOpentelemetry.HandleSpanBusinessErrorEvent` (span status stays green/UNSET). Technical / 5xx errors MUST use `libOpentelemetry.HandleSpanError` (span flips red, feeding error-rate SLOs).

> This is **the** merged span-error rule. It is referenced by `error-handling.md` **Rule E7** and defined only here — do not restate it there.

**Rationale:** Recording every 4xx via `HandleSpanError` flips spans red on expected business outcomes (validation failures, not-found, conflicts), inflating error-rate SLOs and burying real infrastructure failures. The audit found CRM using `HandleSpanError` exclusively (0 business events) and fees crossing over mid-file. A single predicate keyed on error class makes the choice mechanical and consistent across services.

**Canonical example:** [`pkg/errors.go:254`](../../pkg/errors.go) — `pkg.IsBusinessError(err error) bool`, the `errors.As`-based class predicate that lives beside the typed structs it inspects.

**Enforcement:** `contract-test` — a per-path test asserting that an induced validation failure leaves span status UNSET with a business event, and an induced infra failure sets status Error. (`review-only` for the predicate-selection wiring at each call site.)

---

## T6 — Structured logging

**Rule:** Log calls MUST use a constant string message plus typed fields (`libLog.Err` / `libLog.String` / `libLog.Int` / …). `fmt.Sprintf`, `fmt.Sprintln`, and `%#v` MUST NOT appear as arguments to a logger call. Printf-style logger methods (`Infof`, `Errorf`, etc.) MUST NOT be used in new code.

**Rationale:** Interpolating into the message destroys field-level queryability and is the single largest violation class in the codebase (~1,450 sites). Constant messages with typed fields keep logs aggregatable and parseable, and stop accidental interpolation of sensitive values into the message string (see T9).

**Canonical example:** [`pkg/rabbitmq/retry.go:81`](../../pkg/rabbitmq/retry.go) — constant message `"Max retries exceeded, sending to DLQ"` with `log.Int(...)`, `log.String(...)`, `log.Err(err)` fields (lines 81–86), in the shared retry engine that reporter-worker delegates to. reporter and tracer have zero `fmt.Sprintf`-in-logger violations.

**Enforcement:** `custom-lint` — flag `fmt.Sprintf`/`fmt.Sprintln` as an argument to `logger.Log`/`logger.Info`/etc., and any `.Infof(`/`.Errorf(`/`.Warnf(`/`.Debugf(` logger method.

---

## T7 — Log levels

**Rule:** Log levels MUST follow this matrix:
- **Debug** — per-request / per-message entry-exit, assembled SQL, cache detail, batch stats.
- **Info** — sparse, one-time process milestones ONLY. The closed list: boot-sequence steps, config loaded, server/worker started, leader elected, shutdown begun. Nothing per-request.
- **Warn** — business/validation failure, or degraded-but-recoverable fallback.
- **Error** — infrastructure/system failure requiring operator attention.

Per-request `Initiating...` / `Retrieving...` / `Successfully...` lines MUST NOT be logged at Info.

**Rationale:** The audit found inverted discipline (ledger-http: 192 Info vs 9 Debug) where per-request narration drowns the rare milestones that Info is meant to surface and duplicates information already captured by spans. A closed Info list keeps production logs scannable and makes per-request noise a mechanical violation.

**Canonical example:** [`components/reporter/internal/worker/bootstrap/service.go:166`](../../components/reporter/internal/worker/bootstrap/service.go) — `app.Info("Flushing telemetry...")` is a genuine one-time shutdown milestone (the only sanctioned Info shape).

**Enforcement:** `custom-lint` — flag Info-level calls whose message matches `^(Initiating|Retrieving|Trying to|Successfully|Starting)\b` outside bootstrap packages.

---

## T8 — Single-point logging

**Rule:** An error MUST be logged at exactly **one** layer — the boundary that owns the handling decision (HTTP handler or consumer loop). Inner layers (use cases, repositories, adapters) MUST record the error onto the span and return it; they MUST NOT log it.

> This rule is referenced by `error-handling.md` **Rule E8** and defined only here — do not restate it there.

**Rationale:** Logging the same error at every layer it passes through produces N-layer duplicate log lines (the audit found triple-logs per create in tracer), inflating log volume and making it impossible to count distinct failures. The span carries the error detail for trace correlation; the owning boundary logs it once with the level decision (T7).

**Canonical example:** [`components/ledger/internal/services/command/create_account.go:54`](../../components/ledger/internal/services/command/create_account.go) — the use case records onto the span via `HandleSpanBusinessErrorEvent` and returns; the logging-vs-recording decision is owned upward at the boundary. (Inner adapters such as `account.postgresql.go` are the violation pattern: they both `HandleSpanError` and `logger.Log` the same error.)

**Enforcement:** `review-only` — "which layer owns the decision" is not mechanically inferable; gate at code review against the boundary-owns-logging convention.

---

## T9 — Sensitive-data prohibition (scoped to telemetry)

**Rule:** No financial values (amounts, balances, prices), PII, secrets, raw payloads, or SQL args MAY appear on ANY telemetry signal: log lines, span attributes, metric labels, or persisted error metadata.

**Carve-out:** The lib-streaming **event bus is OUT of this rule's scope.** It is a governed wire contract — JSONShape-locked payloads with PII redaction in the `New<Event>` constructors — and intentionally carries domain data (e.g. balance Available/OnHold) by design. This rule governs telemetry, not data-plane events.

**Rationale:** Telemetry is sampled, fanned out to third-party backends, and retained on different terms than the data plane; financial values and PII on a span or log line are an uncontrolled disclosure. The event bus is the opposite: a deliberate, versioned, redaction-reviewed contract that consumers depend on — applying the telemetry prohibition to it would break legitimate downstream consumers.

**Canonical example (carve-out, by design):** [`pkg/streaming/events/balance_created.go:65`](../../pkg/streaming/events/balance_created.go) — `Available decimal.Decimal` and `OnHold decimal.Decimal` (lines 65–66) carried on the wire intentionally, JSONShape-locked.

**Counter-example (telemetry violation):** the eliminated pattern was a full holder PII flatten onto a span via `SetSpanAttributesFromValue(..., nil)` in the CRM holder adapter (also a T4 violation). That call site was rewritten to the `has_*` presence-flag block (see T4); the flatten has zero call sites repo-wide.

**Enforcement:** `review-only` — value sensitivity is semantic, not syntactic; the mechanical proxy is the T4 `forbidigo` nil-Redactor gate plus the T6 `fmt.Sprintf`-in-logger gate. Field-name sensitivity is gated at review.

---

## T10 — Trace propagation

**Rule:** Trace context MUST be injected and extracted on every broker boundary (e.g. RabbitMQ message headers). Background goroutines MUST derive their context via `context.WithoutCancel(ctx)` — never bare `context.Background()`.

**Rationale:** A bare `context.Background()` severs the goroutine's work from the request trace, making async side-effects (idempotency writes, audit emits) invisible in the parent trace and unattributable when they fail. `WithoutCancel` preserves trace and values while correctly detaching the request's cancellation.

**Counter-example (forbidden bare Background):** [`components/ledger/internal/adapters/http/in/transaction_create.go:1371`](../../components/ledger/internal/adapters/http/in/transaction_create.go) — `context.Background()` (wrapped only with the tenant ID) seeds the idempotency and audit goroutines on lines 1373–1375, dropping the trace.

**Enforcement:** `custom-lint` — flag `context.Background()` used as the seed for a `go` statement's context; broker inject/extract presence is `review-only`.

---

## T11 — Metrics

**Rule:** ALL metrics MUST be created via the lib-observability `MetricsFactory` — there are no sanctioned bespoke stacks (D4 outcome: tracer's direct-Prometheus families migrate to the factory; greenfield, no dashboard compatibility owed). Every business operation (commands and key queries) MUST emit domain metrics (D6 outcome; rolled out in Phase 5). Names MUST be snake_case with a unit suffix (e.g. `_ms`, `_total`). Labels MUST be bounded-cardinality only. Metric emit errors MUST be logged at Debug — never swallowed via `_ =`. HTTP telemetry middleware MUST exclude the probe paths `/health`, `/readyz`, `/metrics` by passing them as `excludedRoutes` to `WithTelemetry`.

**Rationale:** Unbounded label cardinality is the classic way to blow up a Prometheus backend; bounding labels at the emission point and routing all new metrics through one factory keeps the metric surface governable. Probe traffic generates a span and metric per k8s probe — high-volume, zero-information — so excluding probe routes removes pure noise and cost. Swallowed emit errors hide a broken metrics pipeline.

**Canonical example (bounded-label allowlist — the model):** [`components/tracer/internal/observability/recorder.go:38`](../../components/tracer/internal/observability/recorder.go) — `allowedDeps` / `allowedStatuses` bounded sets (lines 37–49) drop any out-of-set label value at the emission point. The allowlist discipline survives tracer's migration to MetricsFactory; the emission mechanism does not.

**Probe-exclusion support:** `github.com/LerianStudio/lib-observability@v1.0.1/middleware/telemetry.go:86` — `WithTelemetry(tl, excludedRoutes ...string)` with the `isRouteExcludedFromList` check (lines 86–97). Tracer passes excluded routes; ledger currently does not (audit appendix F22) and MUST be fixed.

**Enforcement:** `custom-lint` for `_ =` on a metrics emit return; `review-only` for `MetricsFactory` usage, naming, label cardinality, and the probe-exclusion argument.

---

## Domain metric catalog (D6)

Every public use-case entrypoint (commands + flagship queries) emits two metric families via `pkg/utils.RecordDomainOperation`, called once at the exit boundary (deferred, named error):

- `domain_operations_total` — counter. Labels: `component`, `operation`, `result` (`success` | `business_error` | `technical_error`; derived from `pkg.IsBusinessError`).
- `domain_operation_duration_ms` — histogram (ms). Labels: `component`, `operation`.

`operation` is a fixed compile-time set per component (T11): no caller-derived values. One table per component.

### reporter

`component = "reporter"`. Shared by both the reporter-manager (HTTP use cases) and reporter-worker (RabbitMQ-consumer use cases); the families aggregate across both binaries. Operation-name constants live in `components/reporter/internal/manager/services/metrics.go` and `components/reporter/internal/worker/services/metrics.go`.

| operation | binary | use-case entrypoint |
| --- | --- | --- |
| `create_template` | manager | `(*UseCase).CreateTemplate` |
| `update_template` | manager | `(*UseCase).UpdateTemplateByID` |
| `delete_template` | manager | `(*UseCase).DeleteTemplateByID` |
| `get_template` | manager | `(*UseCase).GetTemplateByID` |
| `list_templates` | manager | `(*UseCase).GetAllTemplates` |
| `create_report` | manager | `(*UseCase).CreateReport` |
| `get_report` | manager | `(*UseCase).GetReportByID` |
| `list_reports` | manager | `(*UseCase).GetAllReports` |
| `download_report` | manager | `(*UseCase).DownloadReport` |
| `send_report_queue` | manager | `(*UseCase).SendReportQueueReports` |
| `create_deadline` | manager | `(*UseCase).CreateDeadline` |
| `update_deadline` | manager | `(*UseCase).UpdateDeadlineByID` |
| `delete_deadline` | manager | `(*UseCase).DeleteDeadlineByID` |
| `deliver_deadline` | manager | `(*UseCase).DeliverDeadline` |
| `list_deadlines` | manager | `(*UseCase).GetAllDeadlines` |
| `get_datasource_details` | manager | `(*UseCase).GetDataSourceDetailsByID` |
| `validate_schema` | manager | `(*UseCase).ValidateSchemaViaProvider` |
| `generate_report` | worker | `(*UseCase).GenerateReport` (whole pipeline) |
| `process_notification` | worker | `(*UseCase).ProcessFetcherNotification` |

The `MetricsFactory` is wired at each bootstrap from `telemetry.MetricsFactory` (manager: `initHandlers`; worker: `initWorkerDependencies`). A nil factory (telemetry disabled / single-tenant without OTel) makes emission a no-op.

### ledger

`component = "ledger"`. Covers the onboarding + transaction command use cases (`components/ledger/internal/services/command`) and the flagship read use cases (`components/ledger/internal/services/query`). The `MetricsFactory` is set on both `command.UseCase` and `query.UseCase` at bootstrap (`InitServers` in `components/ledger/internal/bootstrap/config.go`).

| operation | trigger method |
| --- | --- |
| `create_account` | `(command.UseCase).CreateAccount` |
| `update_account` | `(command.UseCase).UpdateAccount` |
| `delete_account` | `(command.UseCase).DeleteAccountByID` |
| `create_account_type` | `(command.UseCase).CreateAccountType` |
| `update_account_type` | `(command.UseCase).UpdateAccountType` |
| `delete_account_type` | `(command.UseCase).DeleteAccountTypeByID` |
| `create_asset` | `(command.UseCase).CreateAsset` |
| `update_asset` | `(command.UseCase).UpdateAssetByID` |
| `delete_asset` | `(command.UseCase).DeleteAssetByID` |
| `create_asset_rate` | `(command.UseCase).CreateOrUpdateAssetRate` |
| `create_ledger` | `(command.UseCase).CreateLedger` |
| `update_ledger` | `(command.UseCase).UpdateLedgerByID` |
| `delete_ledger` | `(command.UseCase).DeleteLedgerByID` |
| `update_ledger_settings` | `(command.UseCase).UpdateLedgerSettings` |
| `create_organization` | `(command.UseCase).CreateOrganization` |
| `update_organization` | `(command.UseCase).UpdateOrganizationByID` |
| `delete_organization` | `(command.UseCase).DeleteOrganizationByID` |
| `create_portfolio` | `(command.UseCase).CreatePortfolio` |
| `update_portfolio` | `(command.UseCase).UpdatePortfolioByID` |
| `delete_portfolio` | `(command.UseCase).DeletePortfolioByID` |
| `create_segment` | `(command.UseCase).CreateSegment` |
| `update_segment` | `(command.UseCase).UpdateSegmentByID` |
| `delete_segment` | `(command.UseCase).DeleteSegmentByID` |
| `create_operation_route` | `(command.UseCase).CreateOperationRoute` |
| `update_operation_route` | `(command.UseCase).UpdateOperationRoute` |
| `delete_operation_route` | `(command.UseCase).DeleteOperationRouteByID` |
| `create_transaction_route` | `(command.UseCase).CreateTransactionRoute` |
| `update_transaction_route` | `(command.UseCase).UpdateTransactionRoute` |
| `delete_transaction_route` | `(command.UseCase).DeleteTransactionRouteByID` |
| `create_balance` | `(command.UseCase).CreateAdditionalBalance` |
| `update_balance` | `(command.UseCase).Update` |
| `delete_balance` | `(command.UseCase).DeleteBalance` |
| `delete_all_balances` | `(command.UseCase).DeleteAllBalancesByAccountID` |
| `update_operation` | `(command.UseCase).UpdateOperation` |
| `create_transaction` | `(command.UseCase).WriteTransaction` |
| `update_transaction` | `(command.UseCase).UpdateTransaction` |
| `update_transaction_status` | `(command.UseCase).UpdateTransactionStatus` |
| `get_account` | `(query.UseCase).GetAccountByID` |
| `list_accounts` | `(query.UseCase).GetAllAccount` |
| `get_ledger` | `(query.UseCase).GetLedgerByID` |
| `list_ledgers` | `(query.UseCase).GetAllLedgers` |
| `get_organization` | `(query.UseCase).GetOrganizationByID` |
| `list_organizations` | `(query.UseCase).GetAllOrganizations` |
| `get_transaction` | `(query.UseCase).GetTransactionByID` |
| `list_transactions` | `(query.UseCase).GetAllTransactions` |

### crm

`component = "crm"`. Covers the CRM holder/instrument use cases (`components/ledger/internal/crm/services`). The `MetricsFactory` is set on the shared `crmservices.UseCase` instance at bootstrap (`crmMgo.holderHandler.Service.MetricsFactory` in `config.go`).

| operation | trigger method |
| --- | --- |
| `create_holder` | `(services.UseCase).CreateHolder` |
| `create_holder_with_id` | `(services.UseCase).CreateHolderWithID` |
| `update_holder` | `(services.UseCase).UpdateHolderByID` |
| `delete_holder` | `(services.UseCase).DeleteHolderByID` |
| `create_instrument` | `(services.UseCase).CreateInstrument` |
| `update_instrument` | `(services.UseCase).UpdateInstrumentByID` |
| `delete_instrument` | `(services.UseCase).DeleteInstrumentByID` |
| `delete_related_party` | `(services.UseCase).DeleteRelatedPartyByID` |
| `get_holder` | `(services.UseCase).GetHolderByID` |
| `list_holders` | `(services.UseCase).GetAllHolders` |

### fees

`component = "fees"`. Covers the fee/billing use cases (`components/ledger/internal/services/fees`). The `MetricsFactory` is set on `fees.useCase`, `fees.billingPackageService`, and `fees.billingCalculateService` at bootstrap (`config.go`, after `initFees`).

| operation | trigger method |
| --- | --- |
| `create_package` | `(fees.UseCase).CreatePackage` |
| `update_package` | `(fees.UseCase).UpdatePackageByID` |
| `delete_package` | `(fees.UseCase).DeletePackageByID` |
| `calculate_fee` | `(fees.UseCase).CalculateFee` |
| `estimate_fee` | `(fees.UseCase).EstimateFeeCalculation` |
| `get_package` | `(fees.UseCase).GetPackageByID` |
| `list_packages` | `(fees.UseCase).GetAllPackages` |
| `create_billing_package` | `(BillingPackageService).CreateBillingPackage` |
| `update_billing_package` | `(BillingPackageService).UpdateBillingPackage` |
| `delete_billing_package` | `(BillingPackageService).DeleteBillingPackage` |
| `calculate_billing` | `(BillingCalculateService).Calculate` |

---

## T12 — Bootstrap and shutdown

**Rule:** Telemetry MUST be wired exactly once at bootstrap via `libOpentelemetry.NewTelemetry(...)` followed by `ApplyGlobals()`. Telemetry flush/shutdown MUST happen **last** in the teardown sequence, after all other components have stopped.

**Rationale:** Wiring once at the composition root keeps a single global tracer/meter provider; flushing last guarantees that spans and metrics emitted during the shutdown of every other component are captured before the exporter closes. The audit validated flush-last for all three services; reporter-worker is the cleanest reference.

**Canonical example (wiring):** [`components/ledger/internal/bootstrap/config.go:415`](../../components/ledger/internal/bootstrap/config.go) — `libOpentelemetry.NewTelemetry(...)` then `telemetry.ApplyGlobals()` at line 430.

**Canonical example (flush last):** [`components/reporter/internal/worker/bootstrap/service.go:164`](../../components/reporter/internal/worker/bootstrap/service.go) — `// Flush telemetry (must be last to capture shutdown spans)` followed by `ShutdownTelemetry()` (lines 164–168).

**Enforcement:** `review-only` — teardown ordering is sequence-dependent and not mechanically gateable; verify at review of bootstrap changes.

---

## T13 — Import aliases

**Rule:** lib-observability and lib-commons packages MUST use exactly these aliases, one alias per package repo-wide: `libObservability`, `libOpentelemetry` (lowercase `t`), `libLog`, `libZap`, `libCommons`. Variants such as `libObs` or `libOpenTelemetry` (capital `T`) MUST NOT be used.

**Rationale:** One alias per package keeps imports greppable and diffs clean, and lets depguard/import-rule lints reason about a single canonical name. The audit found drift (`libObs`/`libObservability`, `libOpenTelemetry`/`libOpentelemetry`) that defeats mechanical import enforcement and makes cross-file search unreliable.

**Canonical example (correct):** [`components/ledger/internal/services/command/create_account.go:14`](../../components/ledger/internal/services/command/create_account.go) — `libObservability "..."`, `libLog "..."`, `libOpentelemetry "..."` (lowercase `t`), lines 13–16.

**Counter-example (drift):** the eliminated pattern was importing the tracing package as `libOpenTelemetry` (capital `T`) and lib-observability as `libObs` instead of `libObservability` (observed in the CRM holder adapter pre-sweep). Both variants have zero occurrences repo-wide now; the `depguard` alias rules keep them out.

**Enforcement:** `depguard` — import-alias rules pinning each lib-observability/lib-commons package to its canonical alias; reject all variants.
