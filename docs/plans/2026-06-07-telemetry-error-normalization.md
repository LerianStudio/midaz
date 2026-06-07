# Telemetry & Error-Handling Normalization Implementation Plan

> **For implementers:** Use ring:executing-plans (rolling wave: implement the
> detailed phase → user checkpoint → detail the next phase → implement → repeat),
> or ring:running-dev-cycle for the full subagent-orchestrated workflow.
> This document is the living source of truth — task elaboration for later
> phases is written back into it during execution.

**Goal:** One telemetry standard and one error-handling platform across the consolidated monorepo (ledger + CRM + fees + tracer + reporter), with the P0 data leaks closed, the three forked error platforms deleted, and CI gates that prevent regression.

**Architecture:** Standards-first, then risk-ordered remediation. Phase 1 closes the audit's coverage gaps and codifies ONE merged rulebook (the two per-dimension rulebooks produced by the audit contradict each other in three places and must be reconciled before any code moves). Phases 2–5 remediate in dependency order: leaks (wire-invisible, do first), platform consolidation (deletes the code the hygiene sweep would otherwise waste effort on), async resilience, then the mechanical sweep. Phase 6 locks the end state with lint gates and contract tests.

**Tech Stack:** Go 1.26, lib-observability v1.0.1 (libLog/libZap/libOpentelemetry/metrics), lib-commons v5, Fiber v2, RabbitMQ, golangci-lint (forbidigo/depguard).

## Phase Overview

| Phase | Milestone | Epics | Status |
|-------|-----------|-------|--------|
| 1 | Audit gaps closed; unified standard authored; decision memo resolved with owner | 1.1, 1.2 | **Complete** |
| 2 | Zero financial values / PII / raw payloads on any telemetry signal or client-surfaced error | 2.1, 2.2, 2.3 | **Complete** |
| 3 | One error platform: forks deleted, canonical boundary hardened, one envelope, status table enforced (D2) | 3.1–3.6 | **Complete** |
| 4 | Async error resilience: transaction consumer can't hot-loop; authorized transactions can't strand (quarantine); panics dispositioned; reporter hardened (D7) | 4.1–4.5 | **Complete** |
| 5 | Hygiene sweep + metrics normalization: structured logging, span topology, level discipline, helper-by-class, one metrics stack (D4), domain metrics (D6) | 5.1–5.7 | **Complete** |
| 6 | Enforcement: lint gates + contract tests in CI; docs synced | 6.1–6.3 | Epic-level |

---

## Ground Truth (audit, 2026-06-07)

Produced by a 32-agent workflow: 6 slices × 2 dimensions surveyed, baseline of the declared standard extracted, per-dimension synthesis, 16 adversarial verifications, completeness critique. Full machine-readable record: `docs/plans/2026-06-07-telemetry-error-audit.json`. Severities below are **post-verification** (4 claims were downgraded, 1 refuted — the original synthesis matrix overstates them; this table is authoritative).

### What is already consistent (do not churn)

- Telemetry acquisition is genuinely unified: `NewTrackingFromContext(ctx)` everywhere, span naming `<layer>.<operation>`, `defer span.End()`, lib-observability helpers (no raw otel). Codify, don't change.
- The error contract *shape* is shared: identical typed structs `{EntityType,Title,Message,Code,Err}`, the `ValidateBusinessError` factory, the `{code,title,message}` envelope. The problem is four **copies** of it, not four designs.
- reporter is the cleanest slice (0 `fmt.Sprintf`-in-logger, classify/backoff/DLQ consumer, `IsBusinessError` predicate); tracer is also clean on logging. Several canonical examples come from the absorbed slices, not from ledger-core.

### Verified findings

| # | Finding | Sev | Components | Key evidence |
|---|---------|-----|------------|--------------|
| F1 | Financial values logged at Info, unconditionally (fires regardless of tracing config) | **P0** | fees, tracer | `billing-calculate-service.go:182-183,423-424` (gross/net/unitPrice via Sprintf); `tracer/.../validation_service.go:233` (transaction.amount); fees `%#v` full-payload logs |
| F2 | PII / raw payloads on spans via `SetSpanAttributesFromValue(..., nil)` — 48 of 81 call sites pass a nil Redactor; lib flattens every struct field onto the span | **P0** | crm, fees, tracer, ledger-core | `holder.mongodb.go:256` (plaintext CPF/email/phone pre-encryption), `instrument.mongodb.go:252`, `consumer.rabbitmq.go:316` (raw transaction msg.Body), 15+ fees sites |
| F3 | Internal error detail surfaced to clients | P1 | reporter, tracer | `generate-report.go:203-223` (raw `err.Error()` incl. SeaweedFS paths persisted to client-visible `metadata.error_detail`); `rule_handler.go:91` (err.Error() in message field) |
| F4 | Three forked error platforms (typed structs + ValidateBusinessError + sentinels + WithError each) — drift already real: fee `ValidationError` lost its json tags, envelope serializes differently | P1 | fees (`feeshared`), reporter (`pkg/reporter`), tracer (`tracer/pkg`) | `feeshared/errors.go:18-227` + `:50-56` (no json tags), `pkg/reporter/errors.go`, `tracer/pkg/errors.go`; zero imports of canonical pkg in any fork |
| F5 | Ledger transaction consumer: blanket `Nack(requeue=true)`, no classifier, no DLQ in topology — poison message hot-loops forever (async mode) | P1 | ledger-core | `consumer.rabbitmq.go:338,532,636`; `infra/rabbitmq/etc/definitions.json:78-81` (no DLX args) vs reporter queues `:82-108` (full DLX/TTL) |
| F6 | Child I/O spans rebind parent ctx (`ctx, spanX := tracer.Start(ctx, ...)`) — 135 sites (ledger 121, crm 13), sibling spans nest under each other; `.End()` does not restore the parent into Go ctx | P1 | ledger-core, crm, fees | `account.postgresql.go:192`, `holder.mongodb.go:127`, `fees/pack/update.go:52,73` (proven distortion) |
| F7 | Per-request Info noise duplicating spans — inverted level discipline (ledger-http: 192 Info vs 9 Debug; triple-log per create in tracer) | P1 | all six | `account.go:279,290`, `rule_handler.go:97`+`create_rule.go:158`+`rule_repository.go:120`, reporter consumer loop `:218,242` |
| F8 | `fmt.Sprintf` inside logger calls — ~1,450 sites (ledger-core 859, ledger-http 251, fees 82, crm 31; reporter/tracer 0). Mostly benign `: %v` error-wraps; ~a dozen fees sites interpolate amounts (those are F1) | P1 | ledger-core, ledger-http, crm, fees | `account.postgresql.go:202` et al.; correct form already in newer files (`create_account.go`) |
| F9 | Span-error helper tracks code-origin, not error class: CRM uses `HandleSpanError` exclusively (0 business events → every 4xx flips spans red, inflating error-rate SLOs); fees crosses over mid-file (`bizErr` recorded via HandleSpanError) | P1 | crm, fees, ledger-http | `validate-related-party.go:33-60`, `billing-calculate-service.go:351`; the clean mechanism is reporter's `IsBusinessError` (`pkg/reporter/net/http/errors.go:18-57`) |
| F10 | Not-found conventions split; **live 500-on-not-found in reporter** (template update path has zero ErrNoDocuments handling) | P1 | reporter, ledger-core | `update-template-by-id.go:214-217` (live leak); ledger's 10/13-vs-3/13 split is partly intentional (entity-specific 404s via `services.ErrDatabaseItemNotFound`, 73 guarded call sites, no unguarded caller found) |
| F11 | Bare sentinels reach WithError and fall to 500: `GetUUIDFromLocals` returns raw `constant.ErrInvalidPathParameter`, CRM handlers pass bare `cn.ErrInternalServer` | P1 | ledger-http, crm | `pkg/net/http/httputils.go:563-575`, `holder.go:51` |
| F12 | Second envelope shape: `LegacyErrorBoundary` emits `{"error":text}` for raw fiber.Error; tracer emits ≥3 carriers | P1 | ledger-http, tracer | `http/in/errors.go:46-51`, `protected_routes.go:48-58`, `validation_handler.go:100-104` |
| F13 | Production panics: rabbitmq constructor panic (real violation); inventory incomplete — `withBody.go:262`, `ledger.postgresql.go:1011` never classified | P1 | ledger-core, shared | `consumer.rabbitmq.go:116`, `pkg/net/http/withBody.go:262`, `ledger.postgresql.go:1011`, `pongo.go:140` (fail-closed, defensible) |
| F14 | Tracer error machinery is dead/duplicated: central WithError has zero callers, handlers hand-roll mapping with `"TRC-0003"` string literals, two competing ValidateBusinessError in simultaneous use | P1 | tracer | `tracer/pkg/net/http/errors.go:16-65`, `rule_handler.go:548-621`, `activate_rule.go:134` |
| F15 | Canonical `WithError` uses a bare type switch (no `errors.As`) — **latent** trap, not a live bug: convention guarantees business errors arrive unwrapped; all three forks already unwrap. Verification REFUTED the P0 framing | P2 | shared | `pkg/net/http/errors.go:17-67`; forks' errors.As at `feeshared/nethttp/errors.go:62-118`, `pkg/reporter/net/http/errors.go:60-118` |
| F16 | 400-vs-422: business-rule violations typed ValidationError→400 — **pervasive, mainline included** (82 ValidationError vs 31 Unprocessable in pkg/errors.go; `ErrTransactionValueMismatch` is 400 today). Not a fork regression | P2 | all | `feeshared/errors.go:363-572`, `pkg/reporter/errors.go:446`, `pkg/errors.go:548,824` |
| F17 | Metrics ungoverned but labels NOT unbounded (all 29 emit sites checked — org/ledger/tenant bounded by tenant count): real items are two coexisting systems, silent emit-error swallow, zero metrics in fees/crm, cross-signal label naming drift | P2 | ledger, tracer, reporter | `rabbitmq.server.go:376-400` (`_ =`), `metric_state_listener.go:43-51`, `recorder.go:66` (tracer = the disciplined model) |
| F18 | String-matching error classification — narrow (CRM duplicate-key paths only; reporter's string path is supplementary to typed As). NUANCE: `create-holder-with-id.go:62-78` *depends* on the string-match contract for idempotency — breaking it corrupts idempotent-create semantics, not just 409→500 | P2 | crm, reporter | `holder.mongodb.go:144-148`, `instrument.mongodb.go:146-150`, `error_classifier.go:51` |
| F19 | Trace context lost on background work: idempotency/audit goroutines spawned on bare `context.Background()` | P2 | ledger-http | `transaction_create.go:1376` |
| F20 | reflect.TypeOf for entity names (76 ledger sites; crm holder adapter; fees has no Entity constants at all); import-alias drift (`libObs`/`libObservability`, `libOpenTelemetry`/`libOpentelemetry`) | P2 | ledger-core, crm, fees | `get_id_account.go:35`, `holder.mongodb.go:146`, `count_transactions_by_filters.go:54` |
| F21 | Redis consumer poison records skipped-but-never-deleted — re-attempted every 30-min cycle forever; unbounded queue growth, no retry counter/DLQ/alert (Redis-path analog of F5) `[Phase 1 addition]` | P1 | ledger-core | `redis.consumer.go:285-293,350,454-456,521-527,536-543` |
| F22 | Ledger probe traffic generates spans+metrics per k8s probe — `WithTelemetry` route-exclusion exists in lib but unused; tracer filters correctly `[Phase 1 addition]` | P2 | ledger-http | `unified-server.go` middleware block; lib `middleware/telemetry.go:86-97` |
| F23 | balance-sync silently skips a tenant's entire cycle on per-tenant PG failure — log line only, no metric/alert, invisible to readyz `[Phase 1 addition]` | P2 | ledger-core | `balance_sync.worker.go:313-315,321-324` |

**Count supersession:** the appendix (`2026-06-07-telemetry-error-audit-appendix.md` §5) is the authoritative tally. Key corrections: Sprintf ledger-core 780 (not 859); nil-redactor **76 of 100** sites (not 48 of 81) distributed fees 30 / reporter 21 / tracer 18 / crm 4 / ledger-http 2 / ledger-core 1 — Epic 2.2 scope grows ~60%; ctx-rebind raw count 958 with ~325 estimated non-root (original 135 counted only confirmed leaf-I/O sites — Epic 5.2 needs per-file review). Shutdown flush-last claim **validated** for all three services; panic inventory complete (1 class-(a) item).

### Audit coverage gaps (critique — Phase 1 closes these)

| Gap | Why it matters |
|-----|----------------|
| G1: Streaming emit surface unswept (`pkg/streaming/**` + ~28 command emit sites) | Spot-check confirms same violations (`send_transaction_events.go:72-122` Sprintf + Info noise) — uncounted in all tallies |
| G2: PII prohibition collides with the streaming wire contract | `balance_created.go:65-66` carries Available/OnHold **by design** (JSONShape-locked). The rule must be scoped to telemetry signals, with the event bus as a governed carve-out |
| G3: Panic inventory incomplete | Two unclassified prod panics found by critique grep (F13) |
| G4: Redis consumer + balance-sync worker unswept | `bootstrap/redis.consumer.go`, `balance_sync.worker.go` — trace propagation/retry posture unknown |
| G5: Synthesis-vs-verdict contradiction on F15 left unreconciled inside the audit record | Plan treats verdict as authoritative (P2); standard must not say "fixes the P0" |
| G6: Two overlapping rulebooks (one per dimension) with different canonical examples for the same rule | Must merge into ONE rule set before anything is enforced |
| G7: Violation counts disagree between dimensions (859 vs 816 etc.), exclude streaming files | Effort tiers need one authoritative count methodology |
| G8: readyz/health handlers + shutdown flush ordering unverified | Probe traffic in spans/metrics; "flush telemetry last" asserted from reporter, never verified in ledger |
| G9: No enforcement vehicle named per rule | Without lint/contract anchors, normalization decays back |

---

## Decision Points (owner gate at Phase 1 checkpoint)

Resolved with the owner at the Phase 1 checkpoint (2026-06-07). Governing context for several outcomes: **tracer and reporter are greenfield with no effective users** — the v4 window is ONE breaking window, so uniformity beats preservation.

| # | Decision | Recommendation | **Outcome** |
|---|----------|----------------|-------------|
| D1 | Fate of wire-visible code families (FEE-/TRC-/TPL-/REP-) | Split: break FEE-, relocate TRC-/TPL-/REP- | **Break ALL families** — every prefixed family migrates to the canonical numeric registry in Phase 3; prefixed literals banned post-migration |
| D2 | 400→422/409 re-typing of business-rule violations | Forks now, mainline deferred | **Everything now** — forks in the D1 migration AND mainline ledger's ~82 ValidationError sites re-typed in Phase 3 (new Epic 3.6); v4 is the comms window |
| D3 | Envelope convergence | Converge | **Converge** — one `{code,title,message}` envelope everywhere |
| D4 | Tracer bespoke Prometheus families vs MetricsFactory | Bless tracer, Factory for new only | **Migrate tracer to MetricsFactory** — no sanctioned exception; allowlist discipline (`recorder.go:32-49`) survives as the cardinality model; greenfield = no dashboards to preserve (new Epic 5.6) |
| D5 | Transaction-consumer DLQ topology + retry budget | Reporter pattern, maxRetries 3 | **RESOLVED 2026-06-07 (v2, post durability analysis): two-layer design.** (1) RabbitMQ `transaction.dlx`/`transaction.dlq` mirroring reporter (TTL 7d, max-len 10k, `transaction.dlq.key`), classifier + exponential backoff + maxRetries 3 — role is FLOW-CONTROL/DIAGNOSTIC only, never the durable store. (2) The financial safety layer is the Redis backup consumer: poison records in `backup_queue:{transactions}` after N failed cycles → persisted to a Postgres QUARANTINE table + Error log + metrics (backup-queue depth, oldest-entry age); `HDel` only after quarantine persistence confirmed. Rationale: the Redis backup hash is the durable WAL of authorized transactions (seeded ATOMICALLY with authorization inside `balance_atomic_operation.lua`; no TTL; noeviction; AOF everysec; deleted only post-Postgres-persist at `create_balance_transaction_operations_async.go:159→334`) — a RabbitMQ DLQ TTL can never lose the last copy, but silently skipping a poison backup record forever WOULD. F21's remediation is hereby reframed: NEVER delete-only a poison backup record — quarantine-then-delete. |
| D6 | Domain-metrics scope | Middleware sufficient, opt-in | **Mandate domain metrics on all business operations** (new Epic 5.7) |
| D7 | reporter HMAC soft-fail + partial-result posture | Keep as carve-outs | **HMAC hard-fail** (invalid signature → reject + dead-letter; security born enforcing) **+ partial-result explicit** (PARTIAL report status + per-section classified `error_code` per E9) — new Epic 4.5 |

---

## Phase 1: Standards + audit completion

**Milestone:** Coverage gaps G1–G8 closed with the same methodology as the audit; ONE merged standard exists in `docs/standards/`; decision memo resolved with the owner. No production code changes in this phase.

### Epic 1.1: Close audit coverage gaps

**Goal:** Every surface the critique flagged as unswept has findings + counts recorded in the audit appendix, so Phases 2–5 elaborate against complete data.
**Scope:** read-only analysis; writes only to `docs/plans/2026-06-07-telemetry-error-audit-appendix.md`
**Dependencies:** none
**Done when:** appendix exists covering G1, G3, G4, G7, G8 with file:line evidence; effort tiers re-derived from one count methodology.

#### Task 1.1.1: Sweep the streaming emit surface (G1)

- [x] Done

**Context:** The audit's 6 slices excluded `pkg/streaming/**` and the ~28 command-layer emit call sites (`send_transaction_events.go`, `send_overdraft_events.go`, `create_account.go`, etc.). Critique spot-check confirmed `send_transaction_events.go:72,83,105,122` carry `fmt.Sprintf`-in-logger and per-operation Info logs. The `EmitImportant` implementation (`pkg/streaming/emit.go:50-61`) was confirmed to match its CLAUDE.md contract (Warn + `HandleSpanError`, non-propagation) — record that as verified.

**Implementation vision:** Apply the same checklist the audit's telemetry/errors surveyors used (Sprintf-in-logger, Info-noise, span helper choice, leak posture, error propagation) to `pkg/streaming/**` and every command file that calls `EmitImportant` or builds events. List per-file violation counts. Specifically check: does any emit site inline build-emit-log instead of delegating to an `emit<Event>Event` helper; does any event constructor log payload contents. Findings go into the appendix under "Streaming surface".

**Files:**
- Create: `docs/plans/2026-06-07-telemetry-error-audit-appendix.md` (section: Streaming)

**Verification:** appendix section lists every emit call site (grep `pkgStreaming.EmitImportant` + `pkg/streaming/events/` constructors) with counts; cross-check 3 random entries by opening the file.

**Done when:** streaming counts exist and are folded into the Task 1.1.4 master tally.

#### Task 1.1.2: Sweep Redis/balance-sync async loops and readyz/health surface (G4, G8)

- [x] Done

**Context:** `components/ledger/internal/bootstrap/redis.consumer.go` and `balance_sync.worker.go` are live async loops never analyzed. readyz handlers exist in every slice (ledger `bootstrap/readyz.go` + `readyz_checkers.go`, reporter `pkg/reporter/readyz/*` with its own `metrics.go`, tracer `readyz.go`, reporter-worker `health-server.go`). The proposed standard claims "flush telemetry LAST on shutdown" citing reporter — never verified for ledger.

**Implementation vision:** For the two async loops: trace-context posture (bare `context.Background()` vs `WithoutCancel`), error classification/retry posture, log-level discipline in the loop. For readyz across all slices: (a) what level readiness failures log at, (b) whether probe requests generate spans/metrics (cardinality/cost), (c) whether tracer's readyz metric families overlap reporter's. For shutdown: read ledger's `ServerManager` teardown ordering (bootstrap) and record whether telemetry flush is last. Record everything as appendix findings with proposed-rule implications (e.g., probe-exclusion rule).

**Files:**
- Modify: `docs/plans/2026-06-07-telemetry-error-audit-appendix.md` (sections: Async loops, Health/readyz, Shutdown ordering)

**Verification:** appendix answers all three readyz questions per slice + states ledger's actual flush position with a file:line ref.

**Done when:** G4 and G8 have evidence-backed entries; any new P0/P1 found is added to the Ground Truth table above with a `[Phase 1 addition]` marker.

#### Task 1.1.3: Complete the panic inventory (G3)

- [x] Done

**Context:** F13 lists 4 known prod panics; the audit classified only 2. Critique found `pkg/net/http/withBody.go:262` (init-time validator translation failure — shared package, every slice) and `ledger.postgresql.go:1011` (`panic(r)` re-panic after tx rollback) unclassified.

**Implementation vision:** `grep -rn 'panic(' --include='*.go'` over non-test production code in `components/` and `pkg/`. Classify each hit: (a) real violation → convert to error return (goes to Epic 4.3), (b) defensible re-panic-after-cleanup (tracer `tx_helper.go:65-103` is the sanctioned pattern), (c) fail-closed init guard (document, keep — `pongo.go:140` precedent). Table in appendix: path:line, class, disposition.

**Files:**
- Modify: `docs/plans/2026-06-07-telemetry-error-audit-appendix.md` (section: Panic inventory)

**Verification:** appendix table row count equals the grep hit count (excluding `_test.go` and generated files); every row has a disposition.

**Done when:** every production `panic(` is dispositioned a/b/c; class-(a) list feeds Epic 4.3.

#### Task 1.1.4: Authoritative violation counts + effort re-derivation (G7)

- [x] Done

**Context:** The two audit dimensions disagree on Sprintf counts for the same slices (telemetry: ledger-core 816; errors: 402+71; verifier: 859) because each used different grep scopes, and all exclude streaming files (services/command alone has 21 files with violations per the critique's grep).

**Implementation vision:** One methodology, stated in the appendix and reused by Phase 6 lint gates: ripgrep patterns over non-test `.go` files, per-slice directory globs matching the audit's slice definitions plus streaming. Count four violation classes: (1) `fmt.Sprintf` (or `fmt.Sprintln`) as a `logger.Log`/`logger.Info`/etc. argument, (2) `ctx, span\w* := .*tracer.Start` child-span rebinds (excluding root spans — root = first Start in function; count via review of hits, not blind regex), (3) Info-level calls matching `Initiating|Retrieving|Trying to|Successfully|Starting` prefixes, (4) `SetSpanAttributesFromValue(.*, nil)`. Re-derive S/M/L per component from the reconciled numbers; update the Phase 5 epic scopes if a tier changes.

**Files:**
- Modify: `docs/plans/2026-06-07-telemetry-error-audit-appendix.md` (section: Master tally)

**Verification:** tally table has one row per slice × violation class with the exact rg command reproduced; spot-check 2 cells by re-running the command.

**Done when:** one authoritative count table exists; Phase 5 epic scopes reference it.

### Epic 1.2: Author the unified standard + decision memo

**Goal:** ONE merged, enforceable rulebook in-repo; the seven decision points resolved with the owner at the phase checkpoint.
**Scope:** `docs/standards/telemetry.md`, `docs/standards/error-handling.md` (new directory); no code.
**Dependencies:** Epic 1.1 (gap findings feed rules: probe exclusion, streaming carve-out scope, panic classes)
**Done when:** both docs exist; every rule has rationale + canonical in-repo example + enforcement vehicle; zero contradictions between the two docs; decision memo answered.

#### Task 1.2.1: Write `docs/standards/telemetry.md`

- [x] Done

**Context:** The audit produced 13 telemetry rules (`2026-06-07-telemetry-error-audit.json` → `dimensions[telemetry].proposed_standard`) but with critique-flagged defects: the PII rule is unscoped (G2 — would forbid the streaming wire contract), the span-helper rule duplicates/diverges from the errors dimension's version (G6), and no rule names an enforcement vehicle (G9).

**Implementation vision:** Author the telemetry standard from the audit's 13 rules with these resolved decisions baked in:
1. **Acquisition:** `NewTrackingFromContext(ctx)` only; no tracer/logger DI into structs; no raw otel. Canonical: `create_account.go:34`.
2. **Span naming:** `<layer>.<operation>` dotted snake_case; child spans get `.exec/.query/.find` suffixes; no bespoke prefixes.
3. **Span lifecycle:** `defer span.End()` immediately; child I/O spans use `_, spanX :=` — ctx rebind only for genuinely sequential nesting or deliberate `WithoutCancel` detach, with an intent comment. Canonical: `validation_service.go:640` (tracer detach), counter-example `account.postgresql.go:192`.
4. **Span attributes:** `app.request.*` inputs / `db.*`+`app.response.*` outputs; **ban `SetSpanAttributesFromValue` with nil Redactor** (the mechanism behind F2 — the lib flattens every field); sensitive/large inputs become boolean presence flags + counts. Canonical: `observability.go:154`.
5. **Span-error helper — THE merged rule (resolves G5/G6):** choose by error class via a shared `IsBusinessError` predicate (to be lifted to `pkg/` in Phase 5): business/4xx → `HandleSpanBusinessErrorEvent` (span stays green), technical/5xx → `HandleSpanError` (span flips red). Canonical mechanism: `pkg/reporter/net/http/errors.go:18-57`. This rule is referenced (not restated) by the error-handling doc.
6. **Structured logging:** constant message + typed fields (`libLog.Err/String/Int`); `fmt.Sprintf`/`%#v` banned as logger arguments. Canonical: reporter/tracer (0 violations).
7. **Levels:** Debug = per-request/per-message entry+exit, SQL, cache; Info = sparse one-time process milestones (enumerate: boot sequence, config loaded, worker started, leader elected — nothing per-request); Warn = business failure/degraded fallback; Error = infra failure. No `Initiating.../Successfully...` lines.
8. **Single-point logging** (lifted from the errors dimension — absent from the telemetry rulebook, root cause of N-layer triple-logging): an error is logged at exactly ONE layer — the boundary that owns the decision (handler/consumer); inner layers record the span and return.
9. **Sensitive-data prohibition, SCOPED (resolves G2):** applies to telemetry signals — logs, span attributes, metric labels, persisted error metadata. The lib-streaming event bus is a governed wire contract (JSONShape-locked payloads, PII redaction in `New<Event>` constructors) and is explicitly out of this rule's scope.
10. **Trace propagation:** inject/extract on every broker boundary; background goroutines derive from `WithoutCancel`, never bare `context.Background()` (counter-example `transaction_create.go:1376`).
11. **Metrics (per D4/D6):** new metrics via MetricsFactory; snake_case + unit suffix; bounded-cardinality labels (tracer `recorder.go:66` allowlist is the model); emit errors logged at Debug, never `_ =`-swallowed; tracer's pinned Prometheus families are a sanctioned, documented exception; probe traffic exclusion per Task 1.1.2 findings.
12. **Bootstrap/shutdown:** wired once via `NewTelemetry` + `ApplyGlobals`; flush last on shutdown (validated/corrected per Task 1.1.2).
13. **Aliases:** `libObservability`, `libOpentelemetry` (lowercase t), `libLog`, `libZap` — one alias per package repo-wide.

Each rule carries: statement, rationale (one paragraph max), canonical example (file:line, verified to resolve), enforcement vehicle (G9: forbidigo / depguard / custom lint / contract test / review-only — exact mapping decided here, implemented in Phase 6).

**Files:**
- Create: `docs/standards/telemetry.md`

**Verification:** every `file:line` canonical example in the doc resolves (`sed -n` each ref); no rule contradicts `docs/standards/error-handling.md` (cross-read after Task 1.2.2).

**Done when:** doc complete; rules 5 and 8 exist exactly once across the two standards (cross-referenced, not duplicated).

#### Task 1.2.2: Write `docs/standards/error-handling.md`

- [x] Done

**Context:** The audit produced 14 error rules with one P0 framing refuted (F15: WithError unwrap is latent hardening, not a live bug — the rule must say so) and the 400/422 rule colliding with mainline reality (F16).

**Implementation vision:** Author from the audit's 14 rules with corrections:
1. **One platform:** typed structs + `ValidateBusinessError` + sentinels live ONLY in `pkg/errors.go` + `pkg/constant/errors.go`. No slice-private forks. (Phase 3 executes; depguard enforces in Phase 6.)
2. **One boundary:** `pkg/net/http.WithError`, upgraded to resolve via `errors.As` — framed as **defensive hardening (P2)**, not a P0 fix; the convention "business errors return unwrapped" stays normative.
3. **Status mapping:** not-found→404, conflict→409, malformed→400, business-rule→422, auth→401/403, infra→500/503. Mainline's existing 400-typed business rules are a **documented deviation** pending D2; new code follows the table.
4. **Sentinels:** numeric registry + documented prefixed namespaces per D1 outcome; codes referenced by constant identifier, never string literal at mapping sites (tracer F14 counter-example).
5. **Not-found at the adapter boundary** — with the ledger entity-specific-404 deferred-mapping pattern (`services.ErrDatabaseItemNotFound` + use-case `errors.Is`) documented as the sanctioned alternative IF every caller guards (it does today, 73/73); reporter's three-convention mess is the violation to fix (F10 live 500).
6. **Typed classification:** `errors.As/Is`, typed `mongo.WriteException` index inspection — never `strings.Contains(err.Error(), ...)`. Names the F18 idempotency nuance explicitly: the CRM fix must preserve `create-holder-with-id.go:62-78` semantics by switching the *mechanism* (index-name inspection), not the *contract*.
7. **Span helper:** cross-reference telemetry rule 5 (do not restate).
8. **Single-point logging:** cross-reference telemetry rule 8.
9. **No client leakage:** generic 500 body for unmapped errors; worker failure metadata stores classified `error_code`, never raw `err.Error()`. Canonical: `withRecover.go:42-79`.
10. **Entity constants:** `constant.Entity*` only; fees gets Entity constants added (Phase 3).
11. **Consumer posture:** classify transient-vs-permanent; permanent → `Nack(false,false)` to DLX; retryable → bounded backoff+republish; never blanket requeue. Canonical: `retry_manager.go:61-122` + `error_classifier.go:42-163`. Carve-outs per D7.
12. **No panic** in prod paths; recovered-panic-to-wrapped-error for tx helpers (`tx_helper.go:65-103`); class-(c) init guards documented individually per Task 1.1.3 inventory.
13. **One envelope:** `{code,title,message}` (+fields map); no `{"error":text}`, no ad-hoc fiber.Map carriers.
14. **Wire-code families:** per D1 outcome — registry layout, contract-test lock per family (CRM's `crm_error_contract_test.go` as the template).

Same per-rule anatomy as Task 1.2.1 (statement/rationale/canonical/enforcement).

**Files:**
- Create: `docs/standards/error-handling.md`

**Verification:** every canonical ref resolves; grep both standards for `HandleSpanBusinessErrorEvent` and "log at exactly one" — each rule defined exactly once.

**Done when:** doc complete; F15 framed as latent/P2; F16 mainline deviation documented; cross-references to telemetry doc in place.

#### Task 1.2.3: Resolve the decision memo with the owner

- [x] Done — all seven outcomes recorded 2026-06-07 (see Decision Points table)

**Context:** D1–D7 are wire-visible or ops-owned. Phases 3, 4, and 6 elaborate differently depending on the answers (e.g., D1 decides whether Phase 3 renumbers FEE- codes or relocates them; D5 decides Phase 4's infra task).

**Implementation vision:** Present the Decision Points table at the Phase 1 checkpoint using the question tool, one decision at a time where independent, with the recommendation as option 1. Record outcomes in this plan (Decision Points table gains an "Outcome" column) and in the two standards docs (replace `per D<n> outcome` placeholders).

**Files:**
- Modify: `docs/plans/2026-06-07-telemetry-error-normalization.md` (this doc — Outcome column)
- Modify: `docs/standards/telemetry.md`, `docs/standards/error-handling.md` (placeholder resolution)

**Verification:** zero `per D` placeholders remain unresolved in either standard.

**Done when:** all seven decisions have recorded outcomes; later-phase epics whose scope changed are annotated before Phase 2 elaboration.

---

## Phase 2: P0 leak remediation — **Detailed**

**Milestone:** No financial values, PII, or raw payloads on any telemetry signal or client-surfaced error field. Wire-invisible except 2.3's message-text changes (no shape changes). Elaborated 2026-06-07 against ground truth from two fresh sweeps: **14 financial-value log sites** (not 6), **76 nil-redactor sites**, **14 client-leak sites** (not 4).

**Dispatch note:** tasks are organized by epic but EXECUTED by slice (fees / tracer / reporter / crm+ledger) because the same files carry violations from multiple epics (e.g. `fees_package_handler.go` has both `%#v` dumps and nil-redactor sites). One commit for the phase — hunk-splitting per epic across shared files is not worth it.

### Epic 2.1: Financial values out of logs (F1)

**Goal:** fees and tracer log no monetary amounts at any level; `%#v`/`%v` payload dumps in logger calls eliminated.
**Scope:** per Task 2.1.1/2.1.2 site lists.
**Dependencies:** standard T9.
**Done when:** rg for amount-bearing identifiers inside logger calls in fees/tracer returns only non-value matches; tests green.

#### Task 2.1.1: Strip financial values from fees logs + the one span attribute

- [x] Done

**Context:** 13 value-carrying sites: `billing-calculate-service.go:182-183` (totalNetAmount), `:187` (`attribute.String("app.response.total_net_amount", ...)` — explicit span attr), `:423-424` (unitPrice/gross/net), `:429-430` (zero-net context values), `:545-546` (feeAmount/netAmount); `payload_builder.go:37-38` (netAmount), `:227-228` (totalValue/fromSum in an Error message), `:231-232` (totalValue); `billing_calculate_handler.go:103-104` (totalNetAmount); payload dumps `fees_handler.go:74` (`%#v` FeeEstimate), `:95` (`%v` FeeCalculate), `fees_package_handler.go:89,312` (`%#v` package inputs with min/max/fee amounts).

**Implementation vision:** Per T9: keep the log line, drop the value — message becomes constant, fields become IDs (`billing_package_id`, `package_id`), counts (`total_events`, `billable_events`, `account_count`), and presence/zero flags (`net_amount_is_zero` boolean where the zero-branch needs signal). The span attribute at `:187` is deleted (T4: outputs that are financial go nowhere; `app.response.result_count` is the acceptable replacement). `%#v` dumps replaced with ID + presence fields. While editing these exact lines, convert them to structured `libLog` fields (T6) — do NOT sweep the rest of the file (Phase 5 owns that).

**Files:** Modify: `components/ledger/internal/services/fees/billing-calculate-service.go`, `payload_builder.go`, `components/ledger/internal/adapters/http/in/billing_calculate_handler.go`, `fees_handler.go`, `fees_package_handler.go`. Tests: adjust any test asserting old log/attr content.

**Verification:** `rg -n 'logger\.|attribute\.' <those files>` shows no amount-valued field; `go build ./...` + `go test ./components/ledger/internal/services/fees/... ./components/ledger/internal/adapters/http/in/...` green.

**Done when:** zero monetary values in fees logs/span attrs; tests green.

#### Task 2.1.2: Strip transaction.amount from tracer validation log

- [x] Done

**Context:** `components/tracer/internal/services/validation_service.go:233` logs `transaction.amount` as a `libLog.Any` field at Info.

**Implementation vision:** Drop the amount field; keep transaction ID/index fields. If the surrounding line is per-request Info noise, leave the level alone (Phase 5 owns levels) — this task only removes the value.

**Files:** Modify: `components/tracer/internal/services/validation_service.go` (+ test fixture if any asserts the field).

**Verification:** `rg -n 'amount' components/tracer/internal/services/validation_service.go` shows no logged amount value; `go test ./components/tracer/...` green.

**Done when:** no monetary value logged in tracer.

### Epic 2.2: PII and payloads off spans (F2)

**Goal:** zero `SetSpanAttributesFromValue(..., nil)` non-test call sites (76 per the appendix tally: fees 30, reporter 21, tracer 18, crm 4, ledger-http 2, ledger-core 1).
**Scope:** per Task 2.2.1–2.2.4 site lists (appendix §5 per-file breakdown).
**Dependencies:** standard T4.
**Done when:** `rg -U 'SetSpanAttributesFromValue\([^)]*nil\s*\)' --glob '!*_test.go'` returns zero hits repo-wide.

#### Task 2.2.1: fees nil-redactor sites (30)

- [x] Done

**Context:** `mongodb/fees/{pack,billing_package}/{find,create,update,delete}.go` (20), `services/fees/{update-package-by-id,get-all-packages,estimate-fee-calculation,create-package,calculate-fee}.go` (5), `http/in/{fees_package_handler(3),fees_handler(1),billing_calculate_handler(1)}.go` (5). Inputs include package definitions with fee amounts and full calculate payloads.

**Implementation vision:** Default replacement: DELETE the `SetSpanAttributesFromValue` call and ensure the span carries the scoping IDs as explicit `app.request.*` string attributes (org/ledger/package IDs — most spans already set them; add if missing). Where the flattened struct conveyed real signal (e.g. filter shape on find paths), add bounded explicit attributes (`app.request.limit`, `app.request.has_filter_X` booleans). No redactor-based retention — fees data is financial; nothing of the payload goes on the span.

**Files:** Modify the 14 fees files listed in appendix §5.

**Verification:** nil-redactor rg returns zero under the fees globs; `go test ./components/ledger/internal/adapters/mongodb/fees/... ./components/ledger/internal/services/fees/...` green.

**Done when:** fees count = 0.

#### Task 2.2.2: reporter nil-redactor sites (21)

- [x] Done

**Context:** `pkg/reporter/mongodb/{template(6),deadline(6),report(5)}.mongodb.go`, `pkg/reporter/postgres/datasource_filters.go` (2), `reporter-manager http/in/{template,deadline}.go` (2). Templates may embed datasource queries/credentials-adjacent config; reports carry tenant metadata.

**Implementation vision:** Same default: delete the call, keep/add explicit ID attributes (`app.request.template_id`, `report_id`, `organization_id`) and bounded shape attributes (`app.request.field_count` style) where the dump carried signal.

**Files:** the 6 reporter files above.

**Verification:** nil-redactor rg zero under reporter globs; `go test ./pkg/reporter/... ./components/reporter-manager/...` green.

**Done when:** reporter count = 0.

#### Task 2.2.3: tracer nil-redactor sites (18)

- [x] Done

**Context:** `adapters/cel/adapter.go` (5 — CEL program inputs incl. transaction payloads), `services/query/{rule_evaluator(2),list_limits(2),limit_checker(1),get_transaction_validation(1),get_rule(1)}.go`, `services/command/{update_rule(1),create_rule(1)}.go`, `http/in/{rule_handler(2),limit_handler(2)}.go`.

**Implementation vision:** Same default. The CEL adapter sites flatten evaluation inputs (transaction data) — replace with `app.request.rule_id`, `app.request.expression_hash` (or length) and result booleans.

**Files:** the 10 tracer files above.

**Verification:** nil-redactor rg zero under `components/tracer/`; `go test ./components/tracer/...` green.

**Done when:** tracer count = 0.

#### Task 2.2.4: crm + ledger nil-redactor sites (7) and the rabbitmq body dump

- [x] Done

**Context:** `crm/adapters/mongodb/{holder,instrument}.mongodb.go:256/:252` (full entity incl. plaintext CPF/email pre-encryption — THE P0 exemplar), `ledger http/in/{holder,instrument}.go` (1 each), `http/in/metadata.go` (2), `consumer.rabbitmq.go:316` (raw transaction msg.Body flattened onto the consumer span).

**Implementation vision:** CRM update paths: extend the create-path `has_*` idiom (`holder.mongodb.go:132-137` is the canonical block) to the update methods — presence flags + IDs, never the entity. Handlers/metadata: delete the call, keep ID attributes. Consumer: replace body dump with `app.request.message_id`, `app.request.body_size_bytes`, routing key.

**Files:** 6 files above.

**Verification:** nil-redactor rg zero repo-wide (this task closes the last 7); `go test ./components/crm/... ./components/ledger/internal/adapters/rabbitmq/...` green.

**Done when:** repo-wide count = 0.

### Epic 2.3: Client-surfaced internal detail (F3)

**Goal:** no raw `err.Error()` reaches a client-visible field; report metadata carries classified codes only.
**Scope:** 14 sites per fresh sweep.
**Dependencies:** standard E9; minimal fixes only — Phase 3 deletes the tracer/reporter mapping layers these sites live in.
**Done when:** targeted rg for `err.Error()` into response/metadata fields returns zero on the listed sites; tests assert the new shapes.

#### Task 2.3.1: reporter — classified codes only in report metadata + manager fallback

- [x] Done

**Context:** `generate-report.go:203-223` `reportErrorMetadata` persists `metadata["error_detail"] = reportErr.Error()` (`:210`) — flows into Mongo and out through reporter-manager GET report (client-visible `Report.Metadata`). The classified `error_code` channel (`report_generation_failed`, `report_generation_timeout`, ...) already exists and stays. `routes.go:178` (reporter-manager) returns `fiber.Map{"error": err.Error()}` for non-500s. `data-pipeline.go:40` — verify whether raw error text lands in any persisted/client field; fix the same way if so.

**Implementation vision:** Test-first: extend the generate-report tests to assert metadata contains `error_code` and does NOT contain raw error text for an induced failure. Then drop `error_detail` entirely (the operator-facing detail belongs in logs/spans, which already carry it — E9). `routes.go:178` falls back to the canonical envelope shape with a generic message (no raw text).

**Files:** Modify: `components/reporter-worker/internal/services/generate-report.go`, `data-pipeline.go` (if affected), `components/reporter-manager/internal/adapters/http/in/routes.go`. Test: generate-report test file.

**Verification:** new test RED then GREEN; `go test ./components/reporter-worker/... ./components/reporter-manager/...` green.

**Done when:** induced failure persists classified code, zero raw text.

#### Task 2.3.2: tracer — generic messages on the 12 fallback sites

- [x] Done

**Context:** raw `err.Error()` into client `message` fields: `rule_handler.go:91,170,310,552`, `audit_event_handler.go:109,137`, `transaction_validation_handler.go:160,179,329,337`, `limit_handler.go:710` (formatValidationMessage fallback). All are fallback paths of the hand-rolled mappers Phase 3 deletes — fix is message-argument substitution only, no mapping rework.

**Implementation vision:** Replace each `err.Error()` message argument with a constant, code-appropriate generic message ("Validation error", "Invalid timestamp format", "Invalid state transition", ...). The typed-error branches (which carry pre-built client-safe messages) stay untouched.

**Files:** the 4 tracer handler files.

**Verification:** `rg -n 'err\.Error\(\)' components/tracer/internal/adapters/http/in/` shows zero hits flowing into response fields; `go test ./components/tracer/...` green.

**Done when:** zero raw error text in tracer responses.

#### Task 2.3.3: ledger legacy envelope — status text, not error text

- [x] Done

**Context:** `components/ledger/internal/adapters/http/in/errors.go:51` emits `fiber.Map{"error": err.Error()}`; the sibling 500-path at `:47` already uses `stdhttp.StatusText`.

**Implementation vision:** Mirror `:47`: `fiber.Map{"error": stdhttp.StatusText(statusCode)}`. The whole envelope dies in Epic 3.5; this just closes the leak meanwhile.

**Files:** Modify: `components/ledger/internal/adapters/http/in/errors.go` (+ its test).

**Verification:** existing LegacyErrorBoundary tests adjusted and green.

**Done when:** no raw error text in the legacy envelope.

---

## Phase 3: Error platform consolidation — **Detailed**

**Milestone:** `feeshared`, `pkg/reporter` error stack, and `tracer/pkg` error stack are deleted; every handler flows through canonical `pkg/errors.go` + hardened `pkg/net/http.WithError`; one envelope everywhere; all prefixed code families retired to the canonical numeric registry (D1); business-rule violations 422/409 including mainline (D2). Elaborated 2026-06-07 against fresh fork inventories: feeshared 72 FEE- sentinels / 62 importer files; reporter 42 TPL- + 21 REP-; **tracer 73 TRC- sentinels** incl. operator-facing readyz codes, 25+ inline `"TRC-…"` literals, and TWO ValidateBusinessError variants in simultaneous use (fork's + libCommons'). LegacyErrorBoundary confirmed DEAD (zero call sites). Canonical registry tops out at code `0178`.

**Sequencing:** 3.1 → 3.2.1/3.2.2 (registry) → {3.2.3-4, 3.3, 3.4 in parallel — they only consume the registry} → 3.5 → 3.6.

### Epic 3.1: Harden the canonical boundary (F15, F11)

**Goal:** `WithError` resolves via `errors.As` on every arm; `FailedPreconditionError`/`HTTPError` explicitly mapped; `GetUUIDFromLocals` returns typed 400 (today: raw sentinel → 500); bare-sentinel passes eliminated.
**Scope:** `pkg/net/http/{errors,httputils}.go`, 8 ledger CRM-handler sites.
**Dependencies:** none in-phase; lands FIRST.
**Done when:** table-driven test proves wrapped typed errors resolve to their status; path-param failure returns 400/0065 not 500.

#### Task 3.1.1: `WithError` errors.As rewrite + explicit FailedPrecondition/HTTPError arms

- [x] Done

**Context:** `pkg/net/http/errors.go:16-67` — bare type switch; only `pkg.ResponseError` (`:41`) uses errors.As. `FailedPreconditionError` (constructed by `ValidateBusinessError` for `ErrPermissionEnforcement`/`ErrJWKFetch`, `pkg/errors.go:668-673`) and `HTTPError` (`pkg/errors.go:139-150`) hit the default arm → 500. The feeshared/reporter clones (`feeshared/nethttp/errors.go:62-118`, `pkg/reporter/net/http/errors.go:60-118`) already use errors.As on all arms — they are the shape model.

**Implementation vision:** Test-first: table-driven test in `pkg/net/http/errors_test.go` covering every typed arm unwrapped AND wrapped (`fmt.Errorf("ctx: %w", typedErr)`), plus the two unmapped types; capture RED for the wrapped cases. Rewrite the switch as sequential `errors.As` checks in the fork-clone shape. Explicit arms: `FailedPreconditionError` → 500 (current wire behavior, now deliberate — both constructors are infra-class), `HTTPError` → 500. Keep `libCommons.Response` arm semantics intact.

**Files:** Modify `pkg/net/http/errors.go`, `errors_test.go`.

**Verification:** new table test RED→GREEN; `go test ./pkg/net/http/...` green.

**Done when:** wrapped business errors map to typed statuses; default arm reachable only by genuinely unknown errors.

#### Task 3.1.2: `GetUUIDFromLocals` returns a typed 400

- [x] Done

**Context:** `pkg/net/http/httputils.go:563-575` returns raw `constant.ErrInvalidPathParameter` → WithError default → **500 today** for what is a 400-class failure (F11 live wrong-status).

**Implementation vision:** Test-first (handler-level: a route whose local is missing must return 400 with code 0065). Return `pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, <entity from key>)` — check ValidateBusinessError maps 0065 to ValidationError; if the entity arg is awkward at this layer, construct the typed `pkg.ValidationError` directly with code 0065.

**Files:** Modify `pkg/net/http/httputils.go` + test.

**Verification:** RED→GREEN; sweep callers (`rg -l 'GetUUIDFromLocals'`) compile and their tests stay green.

**Done when:** missing/mistyped path-param local renders 400/0065.

#### Task 3.1.3: Eliminate bare-sentinel WithError passes (8 sites)

- [x] Done

**Context:** `components/ledger/internal/adapters/http/in/instrument.go:54,190,199` and `holder.go:51,168,177` pass raw `cn.ErrInternalServer` to WithError (works by accident via default arm; violates E2/E4 — sentinel, not typed).

**Implementation vision:** Replace with `pkg.ValidateInternalError(err, <entity>)` (the canonical internal-error constructor WithError maps deliberately). No wire change (still 500).

**Files:** `holder.go`, `instrument.go`.

**Verification:** `rg -n 'WithError\(c, cn\.Err|WithError\(c, constant\.Err'` zero hits; http/in tests green.

**Done when:** zero bare-sentinel WithError args in ledger/crm handlers.

### Epic 3.2: Canonical registry unification + feeshared deletion (F4, D1, D2, D3)

**Goal:** ONE mapping exercise retires all four prefixed families into the numeric registry (new codes from `0179`); then feeshared's error platform is deleted and fee surfaces flow through canonical.
**Scope:** `pkg/constant/{errors,entity}.go`, `pkg/errors.go`, `docs/plans/` mapping artifact, `components/ledger/pkg/feeshared/**`, 62 fee importer files.
**Dependencies:** Epic 3.1.
**Done when:** registry uniqueness test green; zero feeshared error imports; fee contract test locks the new codes.

#### Task 3.2.1: Author the four-family mapping table

- [x] Done

**Context:** D1 = break ALL. FEE-0001..0072 (gaps; semantic overlaps with canonical: ErrInvalidPathParameter≈0065, ErrInvalidQueryParameter, ErrEntityNotFound, ErrInternalServer, pagination/date codes...), TPL-0001..0062 + REP-0060..0080, TRC-0001..0378 (73 codes, categorized ranges incl. readyz TRC-0328..0342 and HTTP-code consts `constant.CodeBadRequest="TRC-0003"` etc.). Canonical highest = 0178. D2: each mapped code gets a status class — business-rule violations → 422 (`UnprocessableOperationError`), conflicts → 409, syntactic → 400, not-found → 404.

**Implementation vision:** Produce `docs/plans/2026-06-07-error-code-migration.md`: one table per family with columns `old code | old sentinel name | disposition (reuse <canonical code> / new <0179+>) | status class | entity`. Reuse an existing canonical code ONLY on exact semantic match (same meaning, same class); otherwise allocate sequentially. Allocate contiguous blocks per family (fees from 0179, reporter next, tracer next) so the registry stays readable. Readyz/operator codes (TRC-0328..0342, TRC-0281...) map to new numeric codes — greenfield, no compatibility shim. Entity constants: list new `Entity*` for fees (Package, BillingPackage, FeeCalculation...), tracer (Rule, Limit, Reservation, AuditEvent, TransactionValidation...), reporter (Template, Report, Deadline, DataSource...).

**Files:** Create `docs/plans/2026-06-07-error-code-migration.md`.

**Verification:** every old code appears exactly once; no new code collides (script-check in the doc); spot-review 10 rows.

**Done when:** table reviewed and committed; 3.2.2 implements it verbatim.

#### Task 3.2.2: Implement the unified registry

- [x] Done

**Context:** mapping table from 3.2.1. `pkg/errors.go` `ValidateBusinessError` errorMap must gain an arm per new code with the table's status class; `pkg/constant/entity.go` gains the new entities.

**Implementation vision:** Add all new sentinels to `pkg/constant/errors.go` (grouped, commented per family origin); errorMap arms typed per class (422 = UnprocessableOperationError etc.); entity constants. Registry uniqueness contract test (E14): a test in `pkg/constant` asserting no duplicate code strings across the whole file and no `(FEE|TRC|TPL|REP)-` prefixed codes remain. Migrations in 3.2.3/3.3/3.4 must NOT touch these shared files again (conflict-free parallelism).

**Files:** Modify `pkg/constant/errors.go`, `pkg/constant/entity.go`, `pkg/errors.go`; create uniqueness test.

**Verification:** uniqueness test green; `go build ./...`.

**Done when:** registry complete; shared-file freeze for parallel migrations declared.

#### Task 3.2.3: Migrate fee surfaces and delete the feeshared error platform

- [x] Done

**Context:** 62 non-test importers. Error surface to delete: `feeshared/errors.go`, `feeshared/constant/errors.go`, `feeshared/nethttp/**` (WithError clone + httputils/body_parser/body_validator clones — handlers migrate to `pkg/net/http` equivalents). SURVIVES: `feeshared/model/**`, `bsondecimal`, `resolver`, non-error constants (mongo collections etc.). Fee `ValidationError` json-tag drift dies with the fork (D3 — envelope fix is automatic).

**Implementation vision:** Mechanical per the mapping table: `feeshared.ValidateBusinessError(fsconstant.ErrX, ...)` → `pkg.ValidateBusinessError(constant.ErrY, constant.EntityZ)`; `feesharednethttp.WithError` → `pkg/net/http.WithError`; typed-struct references → pkg types. Delete the dead files; fix imports. `pkg/fee` engine error returns re-pointed the same way.

**Files:** ~62 fee files + deletions under `components/ledger/pkg/feeshared/`.

**Verification:** `rg -l 'feeshared/nethttp|feeshared/constant/errors|feeshared\.Validate|feeshared\.EntityNotFound|feeshared\.ValidationError'` → zero non-test hits (model/bsondecimal imports remain fine); `go build ./...`; fee unit+handler tests green.

**Done when:** fork error platform deleted; build green.

#### Task 3.2.4: Fee error contract test + artifact regen

- [x] Done

**Context:** CRM template `components/ledger/internal/adapters/http/in/crm_error_contract_test.go` (`TestErrorContract_CanonicalCodes:56`). Fee routes' swagger/postman embed FEE- codes in examples; envelope shape changed (json tags restored).

**Implementation vision:** Table-driven contract test locking fee code→status→title→message for the migrated codes (sample the high-traffic ones: duplicate package, package range, calculate-fee failures, billing codes). Regenerate swagger + postman artifacts atomically (same PR discipline as the fees route break — `TestContractSpecMatchesRoutes` must stay green).

**Files:** create fee contract test; regenerate `api/` artifacts + postman.

**Verification:** contract test green; `TestContractSpecMatchesRoutes` green.

**Done when:** fee wire contract locked.

### Epic 3.3: Delete the reporter fork; fix the live not-found 500 (F4, F10)

**Goal:** reporter on canonical platform; `IsBusinessError` relocated to `pkg/errors.go` as `pkg.IsBusinessError`; mongo not-found conventions unified; live 500 fixed.
**Scope:** `pkg/reporter/{errors.go,constant/errors.go,net/http/**}`, reporter-manager/worker services+handlers, `error_classifier.go`.
**Dependencies:** Task 3.2.2 (registry).
**Done when:** zero fork imports; missing-template update returns 404 (test); classifier on canonical types.

#### Task 3.3.1: Migrate reporter to canonical + relocate IsBusinessError

- [x] Done

**Context:** ~50 importer files (manager 20+, worker 15+). `pkg/reporter/net/http/errors.go:18-57` `IsBusinessError` is the canonical predicate model — it MUST survive as `pkg.IsBusinessError` (in `pkg/errors.go`, beside the types it inspects; Phase 5 Epic 5.4 consumes it). TPL-/REP- per mapping table.

**Implementation vision:** Add `IsBusinessError` to `pkg/errors.go` (errors.As over the 8 business types) + unit test FIRST (it's new shared production code). Then mechanical migration: reporter ValidateBusinessError/typed refs/WithError → canonical; delete `pkg/reporter/errors.go`, `pkg/reporter/constant/errors.go` (keep non-error constants — split file if mixed), `pkg/reporter/net/http` error writers (keep Respond/http-utils if non-error). Re-point `error_classifier.go:42-163` typed checks at pkg types; the `strings.Contains(err.Error(), "TPL-")` supplementary path (E6 violation) re-keyed to the new numeric codes via typed inspection — or deleted if the typed path now covers it.

**Files:** ~50 reporter files + deletions.

**Verification:** `rg -l 'pkg/reporter/constant"|reporter\.ValidateBusinessError|pkgReporter\.Validate'` zero non-test; build + reporter test trees green; IsBusinessError unit test RED→GREEN.

**Done when:** fork gone; predicate lives in pkg.

#### Task 3.3.2: Unify mongo not-found conventions; fix the live 500

- [x] Done

**Context:** Three coexisting conventions in `pkg/reporter/mongodb`: raw `mongo.ErrNoDocuments` return (`template.mongodb.go:132-133` FindByID), mapped `ValidateBusinessError(ErrEntityNotFound...)`, bare sentinel return (`report.mongodb.go` update). Live bug: `update-template-by-id.go:213-218` propagates raw ErrNoDocuments → 500.

**Implementation vision:** Test-first: manager-level test asserting PATCH on missing template → 404 (capture RED = current 500). Standardize per E5: adapters map driver not-found → typed `pkg.EntityNotFoundError` (via ValidateBusinessError with the right entity) at the repository boundary — all three collections (template, report, deadline), all read/update/delete paths. Callers that branched on raw ErrNoDocuments re-pointed to `errors.As`/`Is` on the typed error.

**Files:** `pkg/reporter/mongodb/{template,report,deadline}/*.mongodb.go`, `update-template-by-id.go`, affected services.

**Verification:** RED→GREEN on the 404 test; `rg -n 'ErrNoDocuments' pkg/reporter components/reporter-*` shows only adapter-boundary mappings.

**Done when:** one convention; 404 on missing template update.

### Epic 3.4: Delete the tracer fork (F4, F14)

**Goal:** tracer on canonical platform: one ValidateBusinessError, canonical WithError + envelope, numeric codes per mapping table, zero `"TRC-` literals, dead mapper + response-helper clones deleted.
**Scope:** `components/tracer/pkg/{errors.go,constant/errors.go,net/http/**}`, all 6 handler files, command/query services, readyz.
**Dependencies:** Task 3.2.2 (registry).
**Done when:** zero fork imports; `rg '"TRC-'` zero non-test hits; tracer contract test locks the new codes.

#### Task 3.4.1: Migrate tracer services + collapse ValidateBusinessError variants

- [x] Done

**Context:** Fork variant at `tracer/pkg/errors.go:294` (used by `with_body.go:361-530`); libCommons variant used at 18 command sites (`activate_limit.go:158` et al.). Canonical target: `pkg.ValidateBusinessError` only.

**Implementation vision:** Per mapping table, re-point every sentinel reference to `constant.Err*` and both ValidateBusinessError call shapes to `pkg.ValidateBusinessError(constant.ErrX, constant.EntityY)`. The fork's typed structs → pkg types.

**Files:** tracer services/command+query, pkg/model validators, with_body users.

**Verification:** `rg -n 'libCommons\.ValidateBusinessError|tracerpkg\.Validate' components/tracer` zero; build green.

**Done when:** one factory repo-wide.

#### Task 3.4.2: Migrate tracer handlers to the canonical boundary + envelope

- [x] Done

**Context:** Handlers hand-roll mapping via `pkgHTTP.BadRequestWithMessage/NotFound/Conflict/...` (`tracer/pkg/net/http/response.go`) with 25+ inline `"TRC-` literals; `validation_handler.go:178-199` has a 17-entry error→response map; dead central mapper at `tracer/pkg/net/http/errors.go` (zero callers). Canonical envelope comes free with `pkg/net/http.WithError`.

**Implementation vision:** Handlers return typed canonical errors and delegate rendering to `http.WithError` (the Phase-2 generic messages survive as the typed errors' messages). The validation_handler timeout/cancel special statuses (504/503) map via typed errors WithError already understands — if it lacks 504-class arms, extend WithError deliberately (one arm, tested) rather than keeping a side-renderer. Delete `tracer/pkg/net/http/{errors.go,response.go}` once handlers no longer import them; with_body/httputils clones migrate to `pkg/net/http` equivalents. Readyz error strings switch to the new numeric codes (`readyz.go:243-398`).

**Files:** 6 handler files, readyz.go, deletions under tracer/pkg/net/http.

**Verification:** `rg -n '"TRC-' components/tracer --glob '!*_test.go'` zero; handler + readyz tests green (update expected codes).

**Done when:** one envelope, no literals, dead code gone.

#### Task 3.4.3: Tracer error contract test

- [x] Done

**Context:** CRM template as in 3.2.4; tracer's wire surface = rule/limit/validation/audit/reservation endpoints + readyz codes.

**Implementation vision:** Table-driven lock on the migrated high-traffic codes (validation input errors, rule lifecycle, limit CRUD, readyz codes) — code→status→title.

**Files:** create tracer contract test under `components/tracer/internal/adapters/http/in/`.

**Verification:** contract test green; deliberate drift (edit one expected code) fails it.

**Done when:** tracer wire contract locked.

### Epic 3.5: One envelope + typed classification (F12, F18)

**Goal:** dead LegacyErrorBoundary deleted; fiber-level errors render the canonical envelope; CRM duplicate-key classification typed (index-name inspection) preserving idempotent-create.
**Scope:** `components/ledger/internal/adapters/http/in/errors.go` (delete), the unified server fiber ErrorHandler, `components/crm/adapters/mongodb/{holder,instrument}/`.
**Dependencies:** Epics 3.1–3.2.
**Done when:** fiber.Error sources (auth 401s, router 404/405) render `{code,title,message}`; CRM idempotent-create tests green on the typed mechanism.

#### Task 3.5.1: Delete LegacyErrorBoundary; canonical envelope for fiber.Errors

- [x] Done

**Context:** `LegacyErrorBoundary`/`legacyFiberErrorHandler` (`http/in/errors.go`) has ZERO call sites — dead code (Phase 2 patched its leak; now it dies). Real fiber.Error producers: `MarkTrustedAuthAssertion` (`pkg/net/http/protected_routes.go:48-58`, three 401s) and Fiber's own router/body-limit errors — today rendered by Fiber's default handler (`{"message":...}` shape), NOT the canonical envelope.

**Implementation vision:** Delete the dead boundary + its test. Wire a fiber `ErrorHandler` on the unified server app config translating `*fiber.Error` → canonical envelope (code = a registry code per class: 401→unauthorized code, 404→route-not-found, 405, 413; default 500) — test via hitting a nonexistent route and asserting envelope keys. Check tracer/reporter-manager fiber apps for the same gap; wire identically.

**Files:** delete `http/in/errors.go` (+test), modify unified-server fiber config (+ tracer/reporter-manager configs), add envelope test.

**Verification:** RED (current `{"message":...}`) → GREEN (`{code,title,message}`) on a 404 route hit; auth 401 envelope asserted.

**Done when:** no surface can emit a non-canonical envelope.

#### Task 3.5.2: CRM duplicate-key via WriteException index-name inspection

- [x] Done

**Context:** `holder.mongodb.go:144-148` (`strings.Contains(err.Error(), "document")` → ErrDocumentAssociationError; unique index on `search.document`), `instrument.mongodb.go:146-150` (`"account_id"` → ErrAccountAlreadyAssociated; unique index `account_id`; also `ledger_id+account_id` and `_id+holder_id` uniques). Idempotency contract: `create-holder-with-id.go:62-78` relies on raw `_id` collisions remaining classifiable AFTER the named-index branches.

**Implementation vision:** Test-first against the idempotency contract (existing integration tests + a unit test on the classifier helper). Extract a small helper (in the crm mongodb package) `classifyDuplicateKey(err) (indexName string, ok bool)` using `errors.As` → `mongo.WriteException`/`mongo.BulkWriteException`, reading `WriteError.Message`?? NO — read the index name from the write error's `Details`/message via the driver's structured fields: prefer `we.WriteErrors[i].Details` lookup; if driver version only exposes the index in the message, isolate that parsing in ONE helper with a unit test pinning driver behavior (documented as the single sanctioned string-touch). Branch on index name: `search.document*` → ErrDocumentAssociationError; `account_id*`/`ledger_id_account_id*` → ErrAccountAlreadyAssociated; `_id_` → raw duplicate (idempotency path preserved).

**Files:** `holder.mongodb.go`, `instrument.mongodb.go`, new classifier helper + unit test.

**Verification:** classifier unit test (synthetic WriteExceptions per index) green; `create-holder-with-id` tests green; `rg 'strings\.Contains\(err\.Error' components/crm` zero.

**Done when:** typed classification; idempotency intact.

### Epic 3.6: Mainline status re-typing (D2 outcome) `[added at Phase 1 checkpoint]`

**Goal:** mainline business-rule violations re-typed per E3 — semantic rule violations leave 400 for 422/409; syntactic stays 400.
**Scope:** `pkg/errors.go` errorMap arms, contract tests, swagger regen, migration notes.
**Dependencies:** Epic 3.1; AFTER 3.2-3.5 land (same shared files).
**Done when:** every re-typed code locked by contract test; migration notes list each change.

#### Task 3.6.1: Classify the 82 ValidationError registrations

- [x] Done

**Context:** `pkg/errors.go` errorMap: 82 codes typed ValidationError-400 vs 31 Unprocessable-422. E3 table: semantic business-rule → 422, conflict/duplicate → 409, malformed/syntactic input → 400.

**Implementation vision:** Produce a classification table (appendix to the 3.2.1 migration doc): per code — keep-400 (syntactic: bad format, missing field, invalid UUID/date/pagination) / move-422 (business rule: value mismatch, balance rules, route validation, state rules) / move-409 (duplicates living as ValidationError, if any). Borderlines get one-line rationale. Review gate: I sanity-check the table before 3.6.2 executes.

**Files:** extend `docs/plans/2026-06-07-error-code-migration.md`.

**Verification:** 82 rows, each dispositioned with rationale.

**Done when:** table complete and reviewed.

#### Task 3.6.2: Execute the re-typing + lock + migration notes

- [x] Done

**Context:** table from 3.6.1.

**Implementation vision:** Flip errorMap arms per table; update every test asserting the old statuses (expect a large but mechanical test sweep — handler tests assert 400 on business failures today); extend the mainline error contract test to lock the new statuses; regenerate swagger; write v4 migration notes listing every code whose status changed (old→new).

**Files:** `pkg/errors.go`, affected handler/service tests, contract tests, `api/` artifacts, migration notes doc.

**Verification:** full unit suite green; contract test locks; migration notes row count == flipped codes count.

**Done when:** wire statuses match E3; documented.

---

## Phase 4: Async resilience — **Detailed**

**Milestone:** A poison message cannot pin a ledger worker; an authorized transaction can never strand silently (D5-v2 quarantine); every production panic is dispositioned; reporter posture hardened (D7). Elaborated 2026-06-07 against post-Phase-3 ground truth.

**Dispatch note:** 4.3's rabbitmq constructor panic folds into the 4.2 agent (same file). Parallel groups: {4.1→4.2}, {4.4}, {4.5}, {4.3-withBody}.

### Epic 4.1: Shared consumer retry machinery in `pkg/rabbitmq`

**Goal:** reporter's classify/backoff/DLQ machinery generalized into `pkg/rabbitmq` (new), consumable by any consumer; reporter-worker re-pointed with behavior-identical tests.
**Scope:** new `pkg/rabbitmq/`; `components/reporter-worker/internal/adapters/rabbitmq/` + `pkg/reporter/rabbitmq/` callers.
**Dependencies:** Phase 3 complete (canonical types).
**Done when:** reporter-worker runs on the shared package; its retry behavior tests green unchanged.

#### Task 4.1.1: Extract the generic retry core

- [x] Done

**Context:** `components/reporter-worker/internal/adapters/rabbitmq/retry_manager.go` `HandleFailure:61-122` (classify → maxRetries → backoff → BuildRetryHeaders → republish ST/MT → Ack) and `pkg/reporter/rabbitmq/` (`error_classifier.go`, retry-header helpers, `TenantIDFromHeaders`). Generic vs reporter-specific split mapped in the Phase-4 exploration: headers/backoff/republish/DLQ-nack are generic; the classifier's domain code set is reporter-specific.

**Implementation vision:** Create `pkg/rabbitmq/` with: `ErrorClassifier` interface (`IsRetryable(err) bool`, `ClassifyFailureReason(err) string`), `RetryManager` (the HandleFailure engine, config: maxRetries, backoff, sleepFunc, republish hooks ST/MT), retry-header builders, `DefaultClassifier` baseline (pkg.IsBusinessError → permanent; context.DeadlineExceeded/connection errors → transient). Reporter keeps a thin domain classifier (its permanent-code set {0287,0289} + SchemaAmbiguity) implementing the interface. Move generic tests with the code; reporter behavior tests stay in place and must pass unchanged.

**Files:** Create `pkg/rabbitmq/{retry_manager,classifier,headers}.go` (+tests). Modify reporter-worker rabbitmq adapter + `pkg/reporter/rabbitmq` to re-point.

**Verification:** `go test ./pkg/rabbitmq/... ./components/reporter-worker/... ./pkg/reporter/...` green; reporter retry behavior tests unchanged.

**Done when:** one retry engine; reporter is a consumer of it.

### Epic 4.2: Transaction consumer DLQ + classification (F5, D5-v2 layer 1)

**Goal:** ledger transaction consumer classifies permanent-vs-transient via `pkg/rabbitmq`; permanents dead-letter to `transaction.dlq`; transients bounded-retry (maxRetries 3, exponential backoff); the three blanket `Nack(requeue=true)` sites gone; constructor panic → error return (4.3 class-a item).
**Scope:** `components/ledger/internal/adapters/rabbitmq/consumer.rabbitmq.go`, `components/ledger/internal/bootstrap/rabbitmq.server.go` (constructor error handling), `components/infra/rabbitmq/etc/definitions.json`.
**Dependencies:** Epic 4.1.
**Done when:** poison message lands in DLQ within retry budget (test); no hot loop; bootstrap surfaces connection failure as error.

#### Task 4.2.1: Provision `transaction.dlx`/`transaction.dlq` topology

- [x] Done

**Context:** `definitions.json:77-81` — transaction queue has no DLX args; reporter pattern at `:82-108` (DLX direct exchange, DLQ with `x-message-ttl: 604800000`, `x-max-length: 10000`, binding via `.dlq.key`). D5-v2: TTL acceptable — the queue is never the last copy (Redis backup hash is).

**Implementation vision:** Mirror reporter exactly: `transaction.dlx` direct exchange; `transaction.dlq` queue (TTL 7d, max-len 10k); binding `transaction.dlq.key`; `transaction.transaction_balance_operation.queue` gains `x-dead-letter-exchange`/`x-dead-letter-routing-key` args. NOTE: changing queue args on an existing queue requires queue re-declaration — definitions.json is provisioning config (local/dev); add a comment in the JSON? No — JSON forbids comments; record the redeploy caveat in the migration notes doc instead.

**Files:** Modify `components/infra/rabbitmq/etc/definitions.json`; append a deployment caveat to `docs/plans/2026-06-07-v4-error-status-migration-notes.md`.

**Verification:** JSON valid (`python3 -m json.tool`); local stack boot (if feasible) or schema-shape diff against reporter entries.

**Done when:** topology present and consistent with reporter convention.

#### Task 4.2.2: Rework consumer error paths onto the retry engine + constructor error

- [x] Done

**Context:** `consumer.rabbitmq.go:339,533,637` blanket `Nack(false,true)`; `:116` constructor panic (`NewConsumerRoutes` on `conn.GetNewConnect()` failure); bulk path invariants (`acknowledgeByResults` multiple=false comment). Handlers route to `CreateBalanceTransactionOperationsAsync` which is idempotent via PG unique constraints — safe for redelivery.

**Implementation vision:** Wire `pkg/rabbitmq.RetryManager` (maxRetries 3, exponential backoff) into the consumer loop: handler error → classify (DefaultClassifier; json/msgpack unmarshal failures and nil-payload = permanent → DLQ nack; infra errors = transient → bounded republish). Preserve the bulk-ack invariant. Constructor: return `(*ConsumerRoutes, error)`; bootstrap (`rabbitmq.server.go`) propagates — find how sibling constructors report fatal init errors there and match. Tests: unit test classification branches with fake delivery; RED first for "permanent error → Nack(false,false)" (current code requeues).

**Files:** Modify `consumer.rabbitmq.go`, `rabbitmq.server.go` (+tests).

**Verification:** RED→GREEN on the permanent-path test; `rg -n 'Nack\(false, true\)' components/ledger` → zero; `ALLOW_INSECURE_TLS=true go test ./components/ledger/...` green.

**Done when:** classified posture live; panic gone.

### Epic 4.3: Panic disposition (F13) — withBody remainder

**Goal:** `newValidator` panic (`pkg/net/http/withBody.go:262`) converted to error return (runs per-request via ValidateStruct — not an init guard; reclassified (a)).
**Scope:** `pkg/net/http/withBody.go` only (the rabbitmq item ships in 4.2.2).
**Dependencies:** none.
**Done when:** zero class-(a) panics remain repo-wide.

#### Task 4.3.1: newValidator returns error

- [x] Done

**Context:** `withBody.go:253-262` — `newValidator()` panics if `RegisterDefaultTranslations` fails; called from `ValidateStruct` (`:185`) per request; single internal call site; ValidateStruct already returns error.

**Implementation vision:** Signature → `(*validator.Validate, ut.Translator, error)`; ValidateStruct wraps with `%w`. Test: not feasibly inducible (translator registration failing needs a broken locale) — cover via signature-level unit test that ValidateStruct still validates correctly; the conversion is mechanical.

**Files:** `pkg/net/http/withBody.go` (+ test touch if any).

**Verification:** `rg -n 'panic\(' pkg/net/http/withBody.go` → zero; `go test ./pkg/net/http/...` green.

**Done when:** appendix panic inventory class-(a)+borderline list empty.

### Epic 4.4: Redis backup-consumer quarantine + balance-sync posture (G4, F21 reframed, D5-v2 layer 2) — *(epic header above; tasks below)*

#### Task 4.4.1: Quarantine table + repository

- [x] Done

**Context:** Migration pattern: `components/ledger/migrations/transaction/0000NN_*.up/.down.sql`. The poison record payload is the `mmodel.TransactionRedisQueue` JSON held in the backup hash; key shape `transaction:{orgId}:{ledgerId}:{txId}`.

**Implementation vision:** Migration `create_transaction_backup_quarantine`: columns id (uuid pk), organization_id, ledger_id, transaction_id, redis_key text, payload jsonb (raw record — the financial copy), failure_reason text, attempts int, first_failed_at, quarantined_at timestamptz. Repository in `components/ledger/internal/adapters/postgres/transactionquarantine/` (squirrel, integration-test-friendly): `Insert(ctx, rec)` + `ExistsByKey` (idempotent re-quarantine guard). Follow the postgres adapter idioms (T-standard spans, no value logging — payload column is data-plane, never logged).

**Files:** Create migration pair + adapter package (+ unit test on SQL construction or integration test per repo pattern).

**Verification:** `go build`; migration applies on local stack if feasible (else SQL lint by inspection); adapter tests green.

**Done when:** durable quarantine surface exists.

#### Task 4.4.2: Poison-record retry counter + quarantine flow in the consumer

- [x] Done

**Context:** Five skip paths (`redis.consumer.go:286,291,354,452,520` post-drift). Counter must survive pod restarts → Redis parallel hash `backup_queue:{transactions}:attempts` keyed by the same field key (HIncrBy); cycle = 30min.

**Implementation vision:** On each poison classification (unmarshal failure, nil Validate, settings fetch failure after N, build failure): HIncrBy attempts; if attempts >= 3 → Insert into quarantine repo; on success → HDel record + HDel attempts (order: quarantine insert MUST succeed before any HDel — never delete-only). TTL-fresh skips (`:291`) are NOT poison (by design) — untouched. Route-cache miss (`:520`) is non-terminal — untouched. Error log + `HandleSpanError` on quarantine events. Successful processing also clears the attempts field. Trace posture: derive cycle ctx via `context.WithoutCancel` from the boot ctx per T10 if currently bare Background (verify; minimal change).

**Files:** Modify `redis.consumer.go` (+ tests with fake repo: poison → 3 cycles → quarantined+deleted; success → attempts cleared).

**Verification:** RED→GREEN unit tests; `ALLOW_INSECURE_TLS=true go test ./components/ledger/internal/bootstrap/...` green.

**Done when:** induced poison record quarantined within 3 cycles; nothing silently stranded.

#### Task 4.4.3: Backup-queue + balance-sync observability

- [x] Done

**Context:** No metrics today; MetricsFactory reachable via bootstrap wiring (`rabbitmq.server.go:189` precedent); F23 silent tenant skip at `balance_sync.worker.go:313-324`.

**Implementation vision:** Metrics per T11 (MetricsFactory, snake_case, bounded labels): `redis_backup_queue_depth` (gauge, set each cycle), `redis_backup_queue_oldest_age_seconds` (gauge), `redis_backup_quarantine_total` (counter), `balance_sync_tenant_skip_total` (counter, label tenant_id — bounded by tenant count). Emit-errors logged Debug.

**Files:** `redis.consumer.go`, `balance_sync.worker.go`, bootstrap wiring for factory injection.

**Verification:** unit-test metric emission via factory fake/registry inspection where the repo has a pattern; else assert wiring compiles + cycle code paths call the recorders (test with fake).

**Done when:** divergence is alertable.

### Epic 4.5: Reporter posture hardening (D7 outcome) `[added at Phase 1 checkpoint]` — *(tasks)*

#### Task 4.5.1: HMAC hard-fail

- [x] Done

**Context:** `components/reporter-worker/internal/services/data-pipeline.go:73-104` `auditHMAC` — mismatch logs Warn and processing continues; empty-signature and no-key paths exist. Consumer chain has retry-manager access (notification consumer). D7: invalid signature → reject + dead-letter; security born enforcing.

**Implementation vision:** RED: test asserting a mismatched HMAC returns a permanent error (and the pipeline does not process). Then: `auditHMAC` → `verifyHMACOrReject` returning error on mismatch; mismatch → typed permanent error (canonical sentinel — check the migration table for the HMAC/auth-adjacent reporter code; else `pkg.UnauthorizedError` shape) that the classifier marks non-retryable → DLQ. Decide empty-signature/no-key postures explicitly: no-key-configured = skip (deployment without HMAC stays functional — D7 targets INVALID signatures, not absent config); empty signature WITH key configured = reject (a producer that should sign didn't). Document both in the code.

**Files:** `data-pipeline.go`, `data-hmac.go` (+tests), classifier permanent-set addition if code-based.

**Verification:** RED→GREEN; reporter-worker tests green.

**Done when:** invalid signature can never produce a report.

#### Task 4.5.2: Explicit PARTIAL report status + per-section error codes

- [x] Done

**Context:** Reports with failed sections deliver partial today with no loud marker. Status enum + section metadata shape: locate in `pkg/reporter/mongodb/report/report.go` (Status field) and the worker's section-failure path (generate-report.go section loop). E9: classified `error_code` per section, never raw text.

**Implementation vision:** RED: test asserting a one-failed-section report persists status `PARTIAL` and section metadata carries a classified code. Add `PARTIAL` to the status values (find the enum/consts; update any status-validation); worker sets PARTIAL when ≥1 section failed and ≥1 succeeded (all-failed stays FAILED; all-ok stays DONE/finished value); per-section `error_code` from the canonical registry (the section-failure classification mirrors the worker's existing error_code mapping). GET surface already returns metadata — assert shape in test.

**Files:** report model/status consts, `generate-report.go` (+tests), any status-transition validation.

**Verification:** RED→GREEN; reporter trees green.

**Done when:** partiality is impossible to miss.

## Phase 5: Hygiene sweep (mechanical, high-volume) — **Detailed**

**Execution model (elaborated 2026-06-07):** Epics 5.1–5.5 execute as ONE pass per file-territory (six parallel territories: ledger-services+streaming, ledger-adapters, ledger-http+bootstrap+pkg, crm+fees, tracer, reporter), each applying all of T3/T5/T6/T7/T13/E10 to its territory — concern-sliced passes would re-edit the same files repeatedly. Epic 5.6 (tracer→MetricsFactory) follows, then 5.7 (domain metrics catalog + emission). Task checkboxes below map to the original epics; the territory pass ticks them collectively when its tallies reach zero.

**Milestone:** The four bulk violation classes are at zero per the Task 1.1.4 master tally. Ordered AFTER Phase 3 so the sweep never touches code the consolidation deletes. Batches are per-slice, each independently verifiable (build + tests + tally re-run), making this phase highly parallelizable across subagents.

### Epic 5.1: `fmt.Sprintf`-in-logger → structured fields (F8)

**Goal:** ~1,450+ sites (per reconciled tally, streaming included) converted to constant-message + `libLog.Err/String/Int` fields.
**Scope:** ledger-core (859+), ledger-http (251), fees (82), crm (31), streaming command files; reporter/tracer already clean.
**Dependencies:** Phase 3 (fees fork gone); Phase 1 rule 6.
**Done when:** tally methodology returns zero; `make test-unit` green per batch.

### Epic 5.2: Child-span ctx-rebind flips (F6, F19)

**Goal:** 135 `ctx, spanX :=` leaf-I/O sites flipped to `_, spanX :=`; legitimate sequential/detach rebinds annotated; `transaction_create.go:1376` background goroutines re-seeded with `WithoutCancel`.
**Scope:** ledger postgres/mongo adapters, crm mongo adapters, fees mongo adapters.
**Dependencies:** Phase 1 rule 3.
**Done when:** tally returns zero unannotated rebinds; a trace-topology assertion test (parent-child) on one representative adapter passes.

### Epic 5.3: Log-level discipline + single-point logging (F7)

**Goal:** per-request `Initiating/Retrieving/Successfully` Info lines demoted to Debug or deleted; N-layer duplicate logging collapsed to the owning boundary; Info reserved for the enumerated milestone list.
**Scope:** all six slices (192 Info sites in ledger-http alone; reporter consumer loop; tracer triple-log).
**Dependencies:** Phase 1 rules 7–8.
**Done when:** tally pattern (3) returns zero at Info level; sampled trace shows one log line per induced failure.

### Epic 5.4: Span-helper by class via shared predicate (F9)

**Goal:** `IsBusinessError` lives in `pkg/` (home defined by Epic 3.3); CRM's 31+18 `HandleSpanError`-on-business sites and fees' crossovers gated on it; ledger-http onboarding rebalanced.
**Scope:** crm services+handlers, fees services, ledger-http onboarding handlers.
**Dependencies:** Epic 3.3 (predicate relocated); Phase 1 rule 5.
**Done when:** induced validation failure leaves the span green (status UNSET) with a business event; induced infra failure flips it red — asserted by test on one CRM and one fees path.

### Epic 5.5: Aliases + entity constants (F20)

**Goal:** one alias per lib-observability package repo-wide; 76 ledger `reflect.TypeOf` sites + crm holder adapter on `constant.Entity*`; stale tracer component CLAUDE.md fixed.
**Scope:** mechanical, repo-wide.
**Dependencies:** none within the phase.
**Done when:** rg for forbidden aliases and `reflect.TypeOf(mmodel` returns zero.

### Epic 5.6: Tracer metrics migration to MetricsFactory (D4 outcome) `[added at Phase 1 checkpoint]`

**Goal:** tracer's bespoke Prometheus families rebuilt on lib-observability MetricsFactory; the `recorder.go:32-49` bounded-label allowlist discipline preserved as the model; old families removed (greenfield — no dashboard compatibility required).
**Scope:** `components/tracer/internal/observability/`, tracer bootstrap wiring, any local Grafana provisioning under `components/infra`.
**Dependencies:** Phase 1 rule T11; independent of 5.1–5.5.
**Done when:** zero direct prometheus client usage in tracer outside the factory path; label cardinality still bounded (test); T11's sanctioned-exception clause already removed from the standard.

### Epic 5.7: Domain metrics on all business operations (D6 outcome) `[added at Phase 1 checkpoint]`

**Goal:** every business operation (commands + key queries) across ledger/CRM/fees/tracer/reporter emits domain metrics via MetricsFactory per T11 — fees and crm go from zero to covered.
**Scope:** all six slices' service layers; metric naming per T11 (snake_case + unit suffix, bounded labels).
**Dependencies:** Epic 5.6 (one stack first); elaboration defines the per-operation metric catalog before any code.
**Done when:** a documented metric catalog exists and every cataloged operation emits; spot-verified via local Prometheus scrape.

---

## Phase 6: Enforcement + docs sync

**Milestone:** Every standard rule with a mechanical enforcement vehicle has one wired in CI; declared docs match the new reality. Locks the end state — landing this earlier would fail CI mid-migration.

### Epic 6.1: Lint gates (G9)

**Goal:** golangci-lint gains: forbidigo for `fmt.Sprintf` as logger argument and `SetSpanAttributesFromValue(..., nil)`; depguard for fork-package imports (feeshared/reporter-pkg/tracer-pkg error paths — now-deleted, gate prevents resurrection) and alias enforcement where lint supports it; review-only rules flagged as such in the standards docs.
**Scope:** `.golangci.yml` (or repo equivalent), CI workflow.
**Dependencies:** Phase 5 complete (gates must pass on day one).
**Done when:** `make lint` green on the final tree and red on a deliberate violation of each gated rule.

### Epic 6.2: Contract tests

**Goal:** envelope shape locked (canonical `{code,title,message}` — extend the CRM contract-test pattern to fee/tracer/reporter surfaces per D1/D3 outcomes); sentinel-registry uniqueness test (no duplicate codes across families); streaming JSONShape tests confirmed untouched.
**Scope:** `pkg/net/http`, `pkg/constant`, per-family contract tests.
**Dependencies:** Phase 3 outcomes.
**Done when:** contract tests fail on a simulated envelope/code drift.

### Epic 6.3: Docs sync

**Goal:** CLAUDE.md Observability/Logging/Errors sections point at `docs/standards/`; the span-helper contradiction (PROJECT_RULES `HandleSpanBusinessErrorEvent`-for-all vs streaming `HandleSpanError`) resolved in favor of the merged rule 5; `llms-full.txt` and `AGENTS.md` updated; dangling "log level matrix" cross-reference fixed.
**Scope:** `CLAUDE.md`, `docs/PROJECT_RULES.md` (targeted edits only — do not overwrite), `llms-full.txt`, `AGENTS.md`, stale tracer component CLAUDE.md.
**Dependencies:** all prior phases (docs describe the end state).
**Done when:** grep finds no doc instance of the old contradiction; CLAUDE.md references resolve.

---

## Execution Notes

- **2026-06-07 Phase 1 executed and checkpointed.** Epic 1.1: four parallel sweep agents closed G1/G3/G4/G7/G8 into the appendix; spot-check corrections applied (nil-redactor 76/100 with anchored pattern — agent's unanchored 87 rejected; probe-traffic finding downgraded P1→P2 after confirming `WithTelemetry` exclusion is opt-in, one-arg fix). Epic 1.2: both standards authored with all canonical refs verified; agents corrected 10+ suggested line refs against reality. Decision memo: D1/D2/D4/D6 resolved AGAINST the original recommendations — owner's governing frame is "tracer/reporter greenfield, v4 = one breaking window"; plan scope updated (Epics 3.6, 4.5, 5.6, 5.7 added; D5 deferred to Phase 4 elaboration). D7 re-framed at the checkpoint (greenfield flips the HMAC asymmetry: enforce now is free, enforce later is breaking).
- Commits: `65b793010` (plan + audit JSON + appendix), `9e48d3ef4` (standards), `fd39b6d6c` (Phase 1 close).
- **2026-06-07 Phase 2 executed.** Dispatched by slice (4 parallel agents), one commit (shared files across epics). Nil-redactor true count was **99 of 100** (appendix corrected — anchored regex missed multi-line map-literal calls; tracer held 41, not 18). Extra F1-class financial leaks found and fixed during execution beyond the elaborated list: tracer `validation_handler.go` (`amount` in a span map), `create_limit.go`/`limit_checker.go` (`max_amount` on spans), fees `fees_package_handler.go:137` (`packOut` `%v` dump with Min/MaxAmount), `payload_builder.go:225` (amount-carrying string recorded onto span via HandleSpanBusinessErrorEvent). Reporter `error_detail` dropped entirely (E9) with RED→GREEN test evidence; `routes_test.go:410` was asserting a path leak. Working tree carries an unrelated pre-existing format pass (~100 files import-reorder) + 4 substantive non-plan changes (`tracer/bootstrap/config*.go`, `tracer/pkg/model/limit.go`, `pkg/reporter/pdf/pool.go`, workflow version bumps, `AGENTS.md`) — deliberately NOT staged.

- **2026-06-07 Phase 3 executed** (commits 7ded3c7af, dd3ad2cb4, b3fa4e298, 72ea1bb85, 99bcda86a, 0410dc95b, 2aa805a24). Corrections vs elaboration: tracer fork held **172** TRC- sentinels (not 73 — second tracer undercount); feeshared/nethttp is NOT a pure clone (fee-specific QueryHeader/WithBodyTracing infra survives with canonical internals — deleting it would have re-typed the ledger-wide QueryHeader); LegacyErrorBoundary was dead code (deleted, replaced by CanonicalFiberErrorHandler for fiber.Error producers, new sentinels 0484/0485); reporter E5 went full adapter-boundary (the two extra raw-ErrNoDocuments gaps proved the service-layer convention fails the every-caller-guards test); codes 0046/0047/0053/0094 are deliberately factory-served (NOT ValidateBusinessError arms) — convention now followed by all migrations; mainline re-typing shipped 23x 400→422 + 1x 409 + 2 reverse fixes (0017, 0096), locked by mainline_error_contract_test. Tracer commit absorbed previously uncommitted limit-validation/bootstrap-clock work entangled in shared files — flagged for owner awareness.

- **2026-06-07 Phase 4 executed** (commits 9adb0365a, 45c322866). pkg/rabbitmq retry engine extracted with RepublishFunc seam (reporter MT coupling stays reporter-side); two load-bearing classifier divergences preserved deliberately (reporter treats ctx-cancel and FailedPrecondition as permanent; generic engine does not). Quarantine invariant (insert-before-delete) guarded by mock-ordering tests; threshold 3 cycles; attempts counter survives restarts in a parallel Redis hash. HMAC hard-fail uses new sentinel 0310 (reporter headroom) typed UnauthorizedError -> non-retryable on both classifier paths; reconciler recovery path enforced too. PARTIAL status required converting the report data loop from fail-fast to per-database accumulation (no section concept existed; database = section unit). Transaction queue x-arguments are immutable -> deployment caveat recorded (drain+recreate). Staging slip: 9adb0365a missed test files + PARTIAL constant; landed in 45c322866.

- **2026-06-07 Phase 5 executed** (commits a79f31adc, domain-metrics commit). Six parallel territory sweeps: Sprintf-in-logger ~830 sites -> 0; per-request Info narration ~400 -> 0 (Info = sanctioned milestone list only); leaf ctx-rebinds ~150 flipped (one live mis-nesting bug found in reporter find_list cursor spans); helper-by-class ~135 conversions (CRM 0->24 business events + span-status contract test); duplicate inner-layer error logs ~330 dropped; reflect.TypeOf -> Entity* completed; aliases normalized. **Epic 5.6 premise was stale**: tracer was already on MetricsFactory via the OTel-Prometheus bridge (prometheus_factory.go is the sanctioned exposition wiring, not a bespoke stack) - D4's migration had effectively already happened; only verification + the allowlist model confirmation were real work. Epic 5.7: 100 operations instrumented across 5 components with bounded {component,operation,result} labels; catalog in telemetry.md T11. T8 applied conservatively in ledger-services (canonical create_account keeps error logs; stricter pass deferred to review). Known stale doc refs for Phase 6.3: telemetry.md T5 canonical example still points at deleted pkg/reporter/net/http/errors.go; tracer CLAUDE.md prescribes the old per-layer logging; recorder.go transitional docstring.

## Out of Scope

- Mass metric-label renames outside the D4 tracer migration (F17 verified labels are bounded).
- lib-observability / lib-commons upstream changes (e.g., making `SetSpanAttributesFromValue` require a redactor) — file upstream tickets if Phase 2 wants them, but do not block on them.
- Streaming event payload schemas (governed wire contract — G2 carve-out).
