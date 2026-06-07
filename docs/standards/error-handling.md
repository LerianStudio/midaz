# Midaz Error-Handling Standard

**Scope.** This standard governs error production, classification, mapping, and exposure across every Go service in the Midaz monorepo (module `github.com/LerianStudio/midaz/v4`): `components/ledger` (including the embedded CRM and fee packages), `components/tracer`, `components/reporter-manager`, and `components/reporter-worker`, plus the shared `pkg/` trees. It is produced from the 2026-06-07 error-handling audit — ground truth in [`docs/plans/2026-06-07-telemetry-error-audit.json`](../plans/2026-06-07-telemetry-error-audit.json), with Phase 1 reconciliation in [`docs/plans/2026-06-07-telemetry-error-audit-appendix.md`](../plans/2026-06-07-telemetry-error-audit-appendix.md). It is the error-handling half of the normalization plan in [`docs/plans/2026-06-07-telemetry-error-normalization.md`](../plans/2026-06-07-telemetry-error-normalization.md); the telemetry half lives in [`docs/standards/telemetry.md`](./telemetry.md) (rules T1–T13).

The 14 rules below are binding. Rules E7 and E8 cross-reference telemetry rules by number and do not restate them. Each rule names its enforcement mechanism: `forbidigo` / `depguard` (golangci-lint, config in `.golangci.yml`), `custom-lint` (a Midaz-specific analyzer to be built), `contract-test` (a Go test that locks wire behavior), or `review-only` (no automated gate; PR review owns it).

The decision memo (D1–D7) was resolved with the owner on 2026-06-07; outcomes are baked into the rules below and recorded in the normalization plan's Decision Points table.

---

## E1 — One error platform

**Rule.** Typed error structs (`EntityNotFoundError`, `ValidationError`, `EntityConflictError`, `UnprocessableOperationError`, `UnauthorizedError`, `ForbiddenError`, `InternalServerError`, `ServiceUnavailableError`, and the validation-field carriers), the `ValidateBusinessError` factory and its siblings, and all error sentinels live ONLY in `pkg/errors.go` and `pkg/constant/errors.go`. No slice-private forks. The three existing forks — `components/ledger/pkg/feeshared/errors.go`, `pkg/reporter/errors.go`, `components/tracer/pkg` (its `constant` + `net/http` error surface) — are deleted in Phase 3; depguard prevents their resurrection in Phase 6.

**Rationale.** Three parallel definitions of `EntityNotFoundError` mean three places where the type assertion in `WithError` (E2) can silently miss, three sentinel registries that can collide on numeric codes, and three drifting `ValidateBusinessError` implementations. A ledger error that crosses into reporter code via the shared `pkg/` boundary must satisfy `errors.As` against ONE type, not a structurally-identical clone the assertion does not recognize.

**Canonical example.** `pkg/errors.go` is the single platform; the typed structs and factory live here. Counter-examples to delete: `pkg/reporter/errors.go:17` (`EntityNotFoundError`), `:299` (`ValidateBusinessError`), and `components/ledger/pkg/feeshared/errors.go:18` (`EntityNotFoundError`) — structurally-identical forks of the platform types.

**Enforcement.** `depguard` (deny imports of the fork packages' error surfaces from outside themselves during the deprecation window; full removal in Phase 3 makes the rule terminal).

---

## E2 — One HTTP error boundary

**Rule.** Every HTTP error response flows through `pkg/net/http.WithError`. No handler emits an error envelope by any other path. `WithError` will be upgraded to resolve typed errors via `errors.As` (so a wrapped platform error is still classified) — this is **defensive hardening (P2)**, NOT a live-bug fix. The convention "business errors are returned unwrapped; only technical errors wrap with `%w` where added context helps" remains normative and is the primary guarantee. The `errors.As` upgrade is a belt-and-suspenders layer for the case where that convention is accidentally violated; it does not license wrapping business errors.

**Rationale.** `WithError` today dispatches on a bare type switch (`switch e := err.(type)`), which matches the concrete type but not a wrapped one. As long as business errors are returned unwrapped, the switch is correct. The hardening removes a sharp edge — a future `%w` wrap of a business error degrading to a generic 500 — without changing the contract callers rely on.

**Canonical example.** `pkg/net/http/errors.go:17-67` — `WithError`, the single boundary. The `pkg.ResponseError` arm at `:38-43` already uses `errors.As`; the upgrade generalizes that pattern across the typed arms.

**Enforcement.** `custom-lint` (flag direct `c.Status(...).JSON(...)` and `fiber.Map` error responses in `http/in` handler packages that bypass `WithError`); `contract-test` for the post-upgrade `errors.As` resolution.

---

## E3 — Status mapping table

**Rule.** HTTP status derives from error class, not from local handler choice:

| Class | Status |
|---|---|
| Not found | 404 |
| Conflict / duplicate | 409 |
| Malformed input (syntactic) | 400 |
| Business-rule violation (semantic) | 422 |
| Authentication failure | 401 |
| Authorization failure | 403 |
| Infrastructure failure | 500 / 503 |

**Transitional state (D2 outcome: re-type everything).** Mainline ledger currently types many business-rule violations as `400`-`ValidationError` rather than `422`-`UnprocessableOperationError` — e.g. `ErrTransactionValueMismatch` at `pkg/errors.go:824-826`. Per the D2 outcome these are re-typed to the table in Phase 3 (Epic 3.6) inside the v4 breaking window, with each re-typed code listed in the migration notes. Until that epic lands, the mismatch is a known transitional state, not a sanctioned pattern. New code follows the table from day one.

**Rationale.** A consistent class→status mapping is what lets clients branch on HTTP status without parsing error codes. v4 is the one breaking window — re-typing inside it costs one announcement; re-typing after it ships costs a major version.

**Canonical example.** Mapping authority: `pkg/net/http/errors.go:19-63` (each typed arm selects the status). Deviation: `pkg/errors.go:824-826` (`ErrTransactionValueMismatch` → `ValidationError` → 400).

**Enforcement.** `review-only` for new registrations; `contract-test` locks the post-Epic-3.6 statuses.

---

## E4 — Sentinels

**Rule.** One numeric sentinel registry in `pkg/constant/errors.go`. Sentinels are `error` values (`errors.New("0073")`-style numeric codes) referenced everywhere by their constant identifier — NEVER by string literal at mapping or classification sites. Prefixed wire-code families (`FEE-`, `TRC-`, `TPL-`, `REP-`) are RETIRED (D1 outcome): every family migrates to the canonical numeric registry in Phase 3, and prefixed code literals are banned thereafter.

**Rationale.** A string literal at a mapping site (`"TRC-0003"`) cannot be found by reference search, cannot be renamed safely, and decouples the wire code from any compile-time check that it exists in the registry. Referencing the constant makes the registry the single source of truth and makes every emission site greppable.

**Canonical example.** Registry: `pkg/constant/errors.go` (e.g. `ErrTransactionValueMismatch = errors.New("0073")` at `:87`). Counter-example: `components/tracer/internal/adapters/http/in/rule_handler.go:80` and `:159` emit the bare literal `"TRC-0003"` (and the file repeats `"TRC-0001"`, `"TRC-0007"`, etc. across its handlers) instead of referencing a registered constant.

**Enforcement.** `forbidigo` (forbid string literals matching `/"(FEE|TRC|TPL|REP)-\d+"/` anywhere in non-test code once the Phase 3 migration lands).

---

## E5 — Not-found at the adapter boundary

**Rule.** Adapters map driver-level not-found (`sql.ErrNoRows`, `mongo.ErrNoDocuments`) to a platform sentinel at the repository boundary. The ledger pattern is the **sanctioned alternative**: the adapter returns a generic `services.ErrDatabaseItemNotFound`, and the use-case maps it to the entity-specific 404 (`EntityNotFoundError`) via `errors.Is`. This alternative is permitted **iff every caller guards** the generic sentinel — verified across the ledger at 73/73 call sites today. A raw driver not-found error must never reach `WithError`, where it degrades to a generic 500.

**Rationale.** A driver not-found that escapes the adapter is a 404 rendered as a 500 — a correctness bug visible to clients. The two sanctioned shapes (map-at-adapter, or generic-sentinel-mapped-at-use-case) both guarantee the client sees 404; the unguarded raw return does not.

**Canonical example / violation to fix.** `pkg/reporter/mongodb/template/template.mongodb.go:95` (`FindByID`) returns raw `mongo.ErrNoDocuments` (`:132-133`). The caller `components/reporter-manager/internal/services/update-template-by-id.go:213-218` (`getTemplateStateForUpdate`) propagates that raw error with zero `ErrNoDocuments` handling → **live 500-on-not-found** for a missing template. The fix maps it to the platform not-found sentinel at the adapter or use-case boundary.

**Enforcement.** `custom-lint` (flag repository methods that can return `sql.ErrNoRows`/`mongo.ErrNoDocuments` without mapping); `contract-test` (assert a missing-entity read renders 404, not 500).

---

## E6 — Typed classification

**Rule.** Classify errors with `errors.As` / `errors.Is`, never by string matching on `err.Error()`. Mongo duplicate-key detection uses typed inspection of `mongo.WriteException` index entries — NEVER `strings.Contains(err.Error(), "E11000")` or equivalent. The idempotency nuance is explicit: `components/crm/services/create-holder-with-id.go:62-78` DEPENDS on duplicate-key classification to deliver idempotent-create semantics (a duplicate `_id` means the deterministic holder already exists → re-fetch and return it as success). The fix must switch the **mechanism** to index-name inspection while **preserving that contract** — idempotent create must keep working.

**Rationale.** String matching on driver error text is locale- and version-fragile; a driver upgrade that rewords the message silently breaks classification. For `create-holder-with-id.go` this is not cosmetic: a misclassified duplicate-key turns an idempotent retry into a hard failure. The contract (re-fetch-on-duplicate) is load-bearing; only the detection mechanism changes.

**Canonical example.** `components/crm/services/create-holder-with-id.go:62-78` — the duplicate-key branch driving idempotent create. The reporter classifier at `pkg/reporter/rabbitmq/error_classifier.go:51` shows the anti-pattern to eliminate elsewhere too: `strings.Contains(err.Error(), "TPL-")`.

**Enforcement.** `forbidigo` (forbid `strings.Contains(*.Error(), ...)` for error classification); `contract-test` (lock idempotent-create returns the existing holder on duplicate).

---

## E7 — Span helper

**Rule.** Record errors onto spans via the class-appropriate helper as defined in telemetry rule **T5**. See [`docs/standards/telemetry.md`](./telemetry.md#t5--span-error-helper-by-error-class).

**Enforcement.** See T5.

---

## E8 — Single-point logging

**Rule.** Log each error once, at the single point defined in telemetry rule **T8** — do not log-and-return the same error up the stack. See [`docs/standards/telemetry.md`](./telemetry.md#t8--single-point-logging).

**Enforcement.** See T8.

---

## E9 — No client leakage

**Rule.** Unmapped or internal errors render a generic 500 envelope with a fixed message — never the raw `err.Error()` in any client-visible field. For async/worker failures, persisted failure metadata stores a classified `error_code`, never a raw error string in a client-readable field.

**Rationale.** Raw error strings leak internal structure (table names, file paths, driver internals, tenant identifiers) to clients and downstream metadata readers. A classified code is stable, safe to expose, and machine-branchable.

**Canonical example.** `pkg/net/http/withRecover.go:42-79` — the panic recovery renders a fixed generic envelope (`code/title/message` at `:69-73`) with no raw error text, which is the target shape for all unmapped errors. Counter-example: `components/reporter-worker/internal/services/generate-report.go:203-223` (`reportErrorMetadata`) persists `metadata["error_detail"] = reportErr.Error()` (`:210`) — raw error text written into report metadata. The `error_code` field (`:206`, `:216`, `:219`) is the correct, classified channel; the raw `error_detail` must be dropped from any client-visible surface.

**Enforcement.** `custom-lint` (flag `err.Error()` flowing into JSON response fields or persisted client-readable metadata); `contract-test` (assert 500 envelopes carry no raw error text).

---

## E10 — Entity constants

**Rule.** Identify entities for error construction with `constant.Entity*` values only — never `reflect.TypeOf(mmodel.Foo{}).Name()`. The fee packages have no Entity constants today; they are added in Phase 3 and used at fee error sites thereafter.

**Rationale.** `reflect.TypeOf(...).Name()` couples the wire-facing entity name to the Go struct name, so a struct rename silently changes the error's entity label. The constant decouples them and keeps entity names stable and centralized.

**Canonical example.** `pkg/constant/entity.go` holds the `Entity*` set; `pkg.ValidateBusinessError(constant.ErrX, constant.EntityFoo)` is the call shape. Fee error sites currently lack a constant and must gain one in Phase 3.

**Enforcement.** `forbidigo` (forbid `reflect.TypeOf(` in error-construction paths).

---

## E11 — Consumer error posture

**Rule.** Message consumers classify each failure as transient or permanent. Permanent failures → `Nack(requeue=false)` to the dead-letter exchange (DLX). Transient failures → bounded retry with backoff, then DLX on exhaustion. NEVER blanket `Nack(requeue=true)` — it creates an unbounded hot-loop redelivery of poison messages. There are NO soft-fail carve-outs (D7 outcome): invalid HMAC signatures → reject + dead-letter, never process; partial-result reports carry an explicit `PARTIAL` status with per-section classified `error_code` (E9), never silent partiality. The Redis-path analog: poison records must be deleted or dead-lettered with a retry counter, never skipped-in-place (audit appendix F21).

**Rationale.** `Nack(requeue=true)` on a deterministically-failing message redelivers it immediately and forever — CPU burn, log flood, and head-of-line blocking. Classification plus bounded retry plus DLX is the only posture that bounds the work and preserves the poison message for inspection. The Redis skip-in-place variant (F21) is the same failure mode without a queue: poison records re-attempted every cycle, unbounded growth, no counter, no DLQ, no alert.

**Canonical example.** Correct posture: `components/reporter-worker/internal/adapters/rabbitmq/retry_manager.go:61-122` (`HandleFailure`) — non-retryable → DLQ (`:62-72`), retry exhaustion → DLQ (`:74-85`), transient → backoff + republish (`:87-121`); driven by the classifier at `pkg/reporter/rabbitmq/error_classifier.go:42-163` (`IsRetryable`, `IsPermanentTenantError`, `ClassifyFailureReason`, `isNonRetryableDomainError`). Counter-example: `components/ledger/internal/adapters/rabbitmq/consumer.rabbitmq.go:338` — blanket `_ = msg.Nack(false, true)` with no classification, requeueing every failure unconditionally.

**Enforcement.** `custom-lint` (flag `Nack(*, true)` outside an explicit, classified retry path); `contract-test` for the HMAC hard-fail and PARTIAL-status postures (Epic 4.5); `review-only` for the Redis-path F21 remediation.

---

## E12 — No panic in production paths

**Rule.** Production code paths return errors; they do not `panic`. Sanctioned exceptions, individually documented, are the only permitted panics:

- **(b) Re-panic after rollback** — a recovered panic is re-raised only after transactional cleanup runs. Example: `components/ledger/internal/adapters/postgres/ledger/ledger.postgresql.go:1011` (rollback-then-`panic(r)`).
- **Recover-to-wrapped-error helpers** — a deferred `recover()` converts a panic into a returned error and never re-raises. Example: `components/tracer/internal/services/command/tx_helper.go` (the `*WithTx` recover converts the panic into a wrapped error at `:66-79`, explicitly not re-raising).
- **(c) Fail-closed init guards** — a panic at construction/initialization where continuing would be unsafe; documented individually. Examples: `pkg/reporter/pongo/pongo.go:140` (`panic` if a security tag ban fails) and `pkg/net/http/withBody.go:262` (`panic(err)`), both pending Epic 4.3 review.

The one class-(a) real violation in scope — `components/ledger/internal/adapters/rabbitmq/consumer.rabbitmq.go:116`, a constructor panic on connection failure — is converted to a returned error in Epic 4.3 (audit appendix §4).

**Rationale.** A panic in a request or message path crashes the goroutine and, without a recover boundary, the process — turning a recoverable error into an availability incident. The sanctioned exceptions are narrow: they either guarantee cleanup-then-controlled-failure, convert to errors, or fail closed at boot where running on would be unsafe.

**Canonical example.** Sanctioned: `ledger.postgresql.go:1011` (re-panic after rollback) and `tx_helper.go:66-79` (recover-to-error). Violation: `consumer.rabbitmq.go:116` (convert to error, Epic 4.3).

**Enforcement.** `custom-lint` (forbid `panic(` in production packages with an allowlist for the documented exceptions); `review-only` for adding any new entry to the allowlist.

---

## E13 — One client error envelope

**Rule.** The ONLY client-facing error shape is `{code, title, message}`, plus an optional `fields` map for field-level validation errors. The `{"error": "<text>"}` shape and ad-hoc `fiber.Map` error carriers are banned.

**Rationale.** A single envelope lets every client deserialize errors with one type and branch on `code`. The legacy `{"error": text}` shape carries no machine-branchable code and, when it inlines `err.Error()`, also violates E9.

**Canonical example.** Target envelope: `pkg/net/http/withRecover.go:69-73` (`{code, title, message}`). Counter-example: `components/ledger/internal/adapters/http/in/errors.go:46-51` — `LegacyErrorBoundary` / `legacyFiberErrorHandler` emits `fiber.Map{"error": stdhttp.StatusText(statusCode)}` (`:47`) and `fiber.Map{"error": err.Error()}` (`:51`), the banned single-field shape (the latter also leaking raw error text per E9).

**Enforcement.** `custom-lint` (forbid `fiber.Map{"error": ...}` and any error response not matching the canonical envelope keys); `contract-test` (lock envelope key set).

---

## E14 — Wire-code contract locks

**Rule.** Every API surface's error codes are locked by a per-surface contract test asserting the code→status→title→message mapping against drift. With the prefixed families retired (D1 outcome, see E4), the locks cover the canonical numeric registry per surface: fees, tracer, reporter, CRM, and mainline ledger each get (or already have) a lock; the Phase 3 migrations land WITH their locks in the same PR.

**Rationale.** Wire codes are an external API surface; a silent change to a code's status or message breaks client error handling. A contract test makes any such change a failing test rather than a production surprise, the same way the streaming `JSONShape` tests lock event wire contracts.

**Canonical example / template.** `components/ledger/internal/adapters/http/in/crm_error_contract_test.go` — `TestErrorContract_CanonicalCodes` (`:56`) locks the post-shim CRM error codes via table-driven cases, and `TestErrorContract_SurvivingDomainCodeUnchanged` (`:166`) asserts a surviving domain code is unchanged after the namespace flip. This is the template each surface's lock follows.

**Enforcement.** `contract-test` (one per surface, modeled on `crm_error_contract_test.go`).
