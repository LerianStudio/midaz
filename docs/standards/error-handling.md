# Midaz Error-Handling Standard

**Scope.** This standard governs error production, classification, mapping, and exposure across every Go service in the Midaz monorepo (module `github.com/LerianStudio/midaz/v4`): `components/ledger` (including the embedded CRM and fee packages) and `components/tracer`, plus the shared `pkg/` trees. It is produced from the 2026-06-07 error-handling audit; the audit JSON, its Phase 1 reconciliation appendix, and the normalization plan lived under `docs/plans/2026-06-07-telemetry-error-*` and have since been removed — the audit record survives in git history. The telemetry half of the normalization lives in [`docs/standards/telemetry.md`](./telemetry.md) (rules T1–T13).

The 14 rules below are binding. Rules E7 and E8 cross-reference telemetry rules by number and do not restate them. Each rule names its enforcement mechanism: `forbidigo` / `depguard` (golangci-lint, config in `.golangci.yml`), `custom-lint` (a Midaz-specific analyzer), `contract-test` (a Go test that locks wire behavior), or `review-only` (no automated gate; PR review owns it).

The decision memo (D1–D7) outcomes are baked into the rules below and recorded in the normalization plan's Decision Points table.

---

## E1 — One error platform

**Rule.** Typed error structs (`EntityNotFoundError`, `ValidationError`, `EntityConflictError`, `UnprocessableOperationError`, `UnauthorizedError`, `ForbiddenError`, `InternalServerError`, `ServiceUnavailableError`, and the validation-field carriers), the `ValidateBusinessError` factory and its siblings, and all error sentinels live ONLY in `pkg/errors.go` and `pkg/constant/errors.go`. No slice-private forks. The former forks — `components/ledger/pkg/feeshared/errors.go` and the `components/tracer/pkg` error surface (its `constant` + `net/http` arms) — are deleted; `depguard` blocks their resurrection.

**Rationale.** Parallel definitions of `EntityNotFoundError` mean several places where the type assertion in `WithError` (E2) can silently miss, several sentinel registries that can collide on numeric codes, and several drifting `ValidateBusinessError` implementations. A ledger error that crosses into the fee packages via the shared `pkg/` boundary must satisfy `errors.As` against ONE type, not a structurally-identical clone the assertion does not recognize.

**Canonical example.** `pkg/errors.go` is the single platform — the typed structs live here (`EntityNotFoundError` at `:18`), alongside `ValidateBusinessError` (`:391`) and the `IsBusinessError` class predicate (`:254`). The structurally-identical fork structs that previously lived in `components/ledger/pkg/feeshared/errors.go` no longer exist; any reintroduction of a parallel `EntityNotFoundError`/`ValidateBusinessError` outside the platform is the rejected shape.

**Enforcement.** `depguard` (deny imports of any resurrected fork error surface; the forks are removed, so the rule is terminal).

---

## E2 — One HTTP error boundary

**Rule.** Every HTTP error response flows through `pkg/net/http.WithError`. No handler emits an error envelope by any other path. `WithError` resolves typed errors via `errors.As`, so a wrapped platform error is still classified to its proper status — defensive hardening, not a license to wrap business errors. The convention "business errors are returned unwrapped; only technical errors wrap with `%w` where added context helps" remains normative and is the primary guarantee; the `errors.As` resolution is the belt-and-suspenders layer for the case where that convention is accidentally violated.

**Rationale.** A bare type switch (`switch e := err.(type)`) would match a concrete error but not a wrapped one. As long as business errors are returned unwrapped, the switch would be correct — but `errors.As` removes the sharp edge where a `%w` wrap of a business error degrades to a generic 500, without changing the contract callers rely on.

**Canonical example.** `pkg/net/http/errors.go:29` — `WithError`, the single boundary: it dispatches the `pkg.ResponseError` status-in-code quirk on its own branch (`:33-36`) and delegates everything else to `withProblem`. The `errors.As` cascade over the typed platform structs now lives in `classifyForProblem` (`pkg/net/http/problem.go:57-118`), driven through the `codeOf`/`statusOf` closures in `ProblemDetail` (`problem.go:177-220`); anything that falls through lands on the `constant.ErrInternalServer` fallback passed to `libProblem.MapError` (`problem.go:200`) — a generic 500.

**Enforcement.** `custom-lint` (flag direct `c.Status(...).JSON(...)` and `fiber.Map` error responses in `http/in` handler packages that bypass `WithError`); `contract-test` for the `errors.As` resolution.

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
| Failed precondition | 500 |
| Infrastructure failure | 500 / 503 |

**Re-typing (D2 outcome).** Business-rule violations that were typed as `400`-`ValidationError` are re-typed to `422`-`UnprocessableOperationError` per the table — e.g. `ErrTransactionValueMismatch` is now an `UnprocessableOperationError` (`pkg/errors.go:873`). The re-typing landed inside the v4 breaking window; new code follows the table from day one.

**Rationale.** A consistent class→status mapping is what lets clients branch on HTTP status without parsing error codes. v4 was the one breaking window — re-typing inside it cost one announcement; re-typing after it ships costs a major version.

**Canonical example.** Mapping authority: `pkg/net/http/errors.go:21-110` (each typed arm selects the status). The re-typed registry entry: `pkg/errors.go:873` (`ErrTransactionValueMismatch` → `UnprocessableOperationError` → 422).

**Enforcement.** `review-only` for new registrations; `contract-test` locks the statuses.

---

## E4 — Sentinels

**Rule.** One numeric sentinel registry in `pkg/constant/errors.go`. Sentinels are `error` values (`errors.New("0073")`-style numeric codes) referenced everywhere by their constant identifier — NEVER by string literal at mapping or classification sites. The prefixed wire-code families (`FEE-`, `TRC-`, `TPL-`, `REP-`) are retired (D1 outcome): every family is folded into the canonical numeric registry, and prefixed code literals are banned. The `CRM-` family is the one surviving prefixed family — 28 live sentinels in `pkg/constant/errors.go`, range `CRM-0006` .. `CRM-0041` (non-contiguous; e.g. `ErrHolderNotFound = errors.New("CRM-0006")`) — pending its own fold-in; it remains a registered, referenced-by-constant set, not an exception to the constant-identifier rule.

**Rationale.** A string literal at a mapping site (`"TRC-0003"`) cannot be found by reference search, cannot be renamed safely, and decouples the wire code from any compile-time check that it exists in the registry. Referencing the constant makes the registry the single source of truth and makes every emission site greppable.

**Canonical example.** Registry: `pkg/constant/errors.go` (e.g. `ErrTransactionValueMismatch = errors.New("0073")` at `:87`). The tracer handlers that previously emitted bare `"TRC-000x"` literals now reference registered constants — e.g. `components/tracer/internal/adapters/http/in/rule_handler.go:81` returns `pkg.ValidationError{Code: constant.ErrInvalidRequestBody.Error(), ...}`. A bare prefixed code literal at a mapping or classification site is the rejected shape.

**Enforcement.** `forbidigo` (forbid string literals matching `/"(FEE|TRC|TPL|REP)-\d+"/` anywhere in non-test code).

---

## E5 — Not-found at the adapter boundary

**Rule.** Adapters map driver-level not-found (`sql.ErrNoRows`, `mongo.ErrNoDocuments`) to a platform sentinel at the repository boundary. The ledger pattern is the **sanctioned alternative**: the adapter returns a generic `services.ErrDatabaseItemNotFound`, and the use-case maps it to the entity-specific 404 (`EntityNotFoundError`) via `errors.Is`. This alternative is permitted **iff every caller guards** the generic sentinel — every ledger call site guards it via `errors.Is(err, services.ErrDatabaseItemNotFound)`. A raw driver not-found error must never reach `WithError`, where it degrades to a generic 500.

**Rationale.** A driver not-found that escapes the adapter is a 404 rendered as a 500 — a correctness bug visible to clients. The two sanctioned shapes (map-at-adapter, or generic-sentinel-mapped-at-use-case) both guarantee the client sees 404; the unguarded raw return does not.

**Canonical example.** `components/ledger/internal/crm/adapters/mongodb/holder/holder.mongodb.go:219` maps driver not-found to the platform sentinel at the adapter boundary — the `if errors.Is(err, mongo.ErrNoDocuments)` branch converts it into `pkg.ValidateBusinessError(constant.ErrHolderNotFound, constant.EntityHolder)` → `EntityNotFoundError` → 404 (`:220-223`). The consuming guard `components/ledger/internal/bootstrap/holder_wiring.go:37` (`Exists`) inspects the result with `errors.As(err, &pkg.EntityNotFoundError{})` plus a code check (`:39-42`) and maps holder-not-found to `(false, nil)` while every other error propagates — so a transient/infrastructure failure can never masquerade as absence, and a missing holder never reaches `WithError` as a raw driver error. A raw driver not-found reaching `WithError` is the rejected shape.

**Enforcement.** `custom-lint` (flag repository methods that can return `sql.ErrNoRows`/`mongo.ErrNoDocuments` without mapping); `contract-test` (assert a missing-entity read renders 404, not 500).

---

## E6 — Typed classification

**Rule.** Classify errors with `errors.As` / `errors.Is`, never by string matching on `err.Error()`. Mongo duplicate-key detection uses typed inspection (`mongo.IsDuplicateKeyError`) — NEVER `strings.Contains(err.Error(), "E11000")` or equivalent. The idempotency nuance is explicit: `components/ledger/internal/crm/services/create-holder-with-id.go` DEPENDS on duplicate-key classification to deliver idempotent-create semantics (a duplicate `_id` means the deterministic holder already exists → re-fetch and return it as success). The mechanism is typed detection; the contract — idempotent create — is load-bearing and preserved.

**Rationale.** String matching on driver error text is locale- and version-fragile; a driver upgrade that rewords the message silently breaks classification. For `create-holder-with-id.go` this is not cosmetic: a misclassified duplicate-key turns an idempotent retry into a hard failure. The contract (re-fetch-on-duplicate) is load-bearing; the detection mechanism is typed.

**Canonical example.** `components/ledger/internal/crm/services/create-holder-with-id.go:81` — the `if mongo.IsDuplicateKeyError(err) || isDocumentAssociationError(err)` branch driving idempotent create: on a raw `_id` collision (or the holder document-association write error, which is not surfaced as a duplicate-key exception) it re-fetches and returns the existing holder as success. (The repository wraps index collisions as typed business errors, so only the raw `_id` collision satisfies `IsDuplicateKeyError`.) A `strings.Contains(err.Error(), "...")` classification anywhere — matching driver error text instead of inspecting the typed error — is the rejected shape; no such site remains in production code.

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

**Canonical example.** `pkg/net/http/withRecover.go:42` — the panic recovery renders a fixed generic envelope (`code/title/message` at `:69-73`) with no raw error text, the target shape for all unmapped errors. Writing raw error text into a client-readable field is the rejected shape.

**Enforcement.** `custom-lint` (flag `err.Error()` flowing into JSON response fields or persisted client-readable metadata); `contract-test` (assert 500 envelopes carry no raw error text).

---

## E10 — Entity constants

**Rule.** Identify entities for error construction with `constant.Entity*` values only — never `reflect.TypeOf(mmodel.Foo{}).Name()`. The fee packages have Entity constants and use them at fee error sites.

**Rationale.** `reflect.TypeOf(...).Name()` couples the wire-facing entity name to the Go struct name, so a struct rename silently changes the error's entity label. The constant decouples them and keeps entity names stable and centralized.

**Canonical example.** `pkg/constant/entity.go` holds the `Entity*` set, including the fee constants `EntityBillingPackage` (`:17`), `EntityFeeCalculation` (`:20`), and `EntityPackage` (`:29`); `pkg.ValidateBusinessError(constant.ErrX, constant.EntityFoo)` is the call shape.

**Enforcement.** `forbidigo` (forbid `reflect.TypeOf(` in error-construction paths).

---

## E11 — Consumer error posture

**Rule.** Message consumers classify each failure as transient or permanent. Permanent failures → `Nack(requeue=false)` to the dead-letter exchange (DLX). Transient failures → bounded retry with backoff, then DLX on exhaustion. NEVER blanket `Nack(requeue=true)` — it creates an unbounded hot-loop redelivery of poison messages. There are NO soft-fail carve-outs (D7 outcome): a deterministically-failing message is rejected and dead-lettered, never silently skipped or processed partially. The Redis-path analog: poison records must be deleted or dead-lettered with a retry counter, never skipped-in-place (audit appendix F21).

**Rationale.** `Nack(requeue=true)` on a deterministically-failing message redelivers it immediately and forever — CPU burn, log flood, and head-of-line blocking. Classification plus bounded retry plus DLX is the only posture that bounds the work and preserves the poison message for inspection. The Redis skip-in-place variant (F21) is the same failure mode without a queue: poison records re-attempted every cycle, unbounded growth, no counter, no DLQ, no alert.

**Canonical example.** The classify-retry-DLQ posture lives in the shared engine `pkg/rabbitmq/retry.go:67` (`HandleFailure`) — non-retryable → DLQ (`:68-78`), retry exhaustion → DLQ (`:80-91`), transient → backoff + republish (`:93-138`), with `NackToDLQ` (`:143`) doing the `Nack(requeue=false)`. The transaction consumer wraps it: `components/ledger/internal/adapters/rabbitmq/consumer.retry.go:91` (`HandleFailure`) builds the engine with the single-tenant republish hook and the shared `pkgRabbitmq.NewDefaultClassifier()` (`:74`); permanent (business) errors and retry-count exhaustion route to the DLQ, transient errors under budget are republished with incremented headers and the original delivery is Acked (doc comment `:87-90`). A blanket `Nack(requeue=true)` with no classification is the rejected shape; no such site remains.

**Enforcement.** `custom-lint` (flag `Nack(*, true)` outside an explicit, classified retry path); `contract-test` for the classify-retry-DLQ posture; `review-only` for the Redis-path F21 remediation.

---

## E12 — No panic in production paths

**Rule.** Production code paths return errors; they do not `panic`. Sanctioned exceptions, individually documented, are the only permitted panics:

- **(b) Re-panic after rollback** — a recovered panic is re-raised only after transactional cleanup runs. Example: `components/ledger/internal/adapters/postgres/ledger/ledger.postgresql.go:933` (rollback-then-`panic(r)`).
- **Recover-to-wrapped-error helpers** — a deferred `recover()` converts a panic into a returned error and never re-raises. Example: `components/tracer/internal/services/command/tx_helper.go` (the `*WithTx` recover converts the panic into a wrapped error at `:66-81`, explicitly not re-raising).

The former class-(a) violations are converted to returned errors: the `consumer.rabbitmq.go` constructor panic on connection failure and the `pkg/net/http/withBody.go` validator-construction `panic(err)` no longer exist.

**Rationale.** A panic in a request or message path crashes the goroutine and, without a recover boundary, the process — turning a recoverable error into an availability incident. The sanctioned exceptions are narrow: they either guarantee cleanup-then-controlled-failure, convert to errors, or fail closed at boot where running on would be unsafe.

**Canonical example.** Sanctioned: `ledger.postgresql.go:933` (re-panic after rollback) and `tx_helper.go:66-81` (recover-to-error). A `panic(` in a request or message path that does none of the above is the rejected shape.

**Enforcement.** `custom-lint` (forbid `panic(` in production packages with an allowlist for the documented exceptions); `review-only` for adding any new entry to the allowlist.

---

## E13 — One client error envelope

**Rule.** The client-facing error shape is the **RFC 9457 `application/problem+json`** body served by `WithError` — a `problem.Detail` superset carrying `type`, `title`, `status`, `detail`, and `instance`, plus the Midaz `code` and (omitempty) `entityType`, and an `errors[]` array for field-level validation errors. The single-field `{"error": "<text>"}` shape and ad-hoc `fiber.Map` error carriers are banned. The `(code, HTTP status)` money-path tuple is preserved byte-for-byte from the retired legacy `{code, title, message}` envelope — only the envelope SHAPE changed (`message` → `detail`, plus `type` and `errors[]`), so the E3 status mapping table stays correct. For `>=500` the `title`/`detail` are deliberately scrubbed to generic text (E9); below 500 the registry title is preserved.

**Rationale.** A single, standards-based envelope lets every client deserialize errors with one media type (`application/problem+json`) and branch on `code`/`status`. The legacy `{"error": text}` shape carried no machine-branchable code and, when it inlined `err.Error()`, also violated E9. RFC 9457 gives the same guarantees on a wire contract clients and tooling already understand.

**Canonical example.** `pkg/net/http/problem.go` — `withProblem` (`:152`) writes the body as `application/problem+json` (`problemContentType`, `:23`), and `ProblemDetail` (`:177`) is the single source of the `Detail` envelope (`:30`, embedding `libProblem.Detail` and adding `EntityType`). Both the Fiber path and the Huma handlers serialize the identical `Detail`. A single-field `{"error": text}` envelope or a raw `fiber.Map` error carrier is the rejected shape.

**KNOWN GAP (code inconsistency, not fixed here).** The panic-recovery path `pkg/net/http/withRecover.go:69-73` still emits the legacy `fiber.Map{code, title, message}` shape rather than problem+json, so the panic path and the `WithError` path currently emit different envelopes. This is a known divergence between the two error-response producers; it is flagged here, not remediated in this doc.

**Enforcement.** `custom-lint` (forbid `fiber.Map{"error": ...}` and any error response not matching the problem+json envelope); `contract-test` (lock the problem+json envelope key set and the `code`/`status` tuple).

---

## E14 — Wire-code contract locks

**Rule.** Every API surface's error codes are locked by a per-surface contract test asserting the code→status→title→message mapping against drift. With the prefixed families retired (D1 outcome, see E4), the locks cover the canonical numeric registry per surface: fees, tracer, CRM, and mainline ledger each carry a lock.

**Rationale.** Wire codes are an external API surface; a silent change to a code's status or message breaks client error handling. A contract test makes any such change a failing test rather than a production surprise, the same way the streaming `JSONShape` tests lock event wire contracts.

**Canonical example / template.** `components/ledger/internal/adapters/http/in/crm_error_contract_test.go` — `TestErrorContract_CanonicalCodes` (`:56`) locks the post-shim CRM error codes via table-driven cases, and `TestErrorContract_SurvivingDomainCodeUnchanged` (`:166`) asserts a surviving domain code is unchanged after the namespace flip. This is the template each surface's lock follows.

**Enforcement.** `contract-test` (one per surface, modeled on `crm_error_contract_test.go`).
