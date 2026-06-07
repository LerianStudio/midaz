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
| 1 | Audit gaps closed; unified standard authored; decision memo resolved with owner | 1.1, 1.2 | Detailed |
| 2 | Zero financial values / PII / raw payloads on any telemetry signal or client-surfaced error | 2.1, 2.2, 2.3 | Epic-level |
| 3 | One error platform: forks deleted, canonical boundary hardened, one envelope | 3.1–3.5 | Epic-level |
| 4 | Async error resilience: transaction consumer can't hot-loop; panic inventory dispositioned | 4.1–4.4 | Epic-level |
| 5 | Hygiene sweep: structured logging, correct span topology, level discipline, helper-by-class | 5.1–5.5 | Epic-level |
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

These are wire-visible or ops-owned calls. Each has a recommendation; none is executed before the owner rules.

| # | Decision | Recommendation |
|---|----------|----------------|
| D1 | Fate of wire-visible code families. **FEE-xxxx**: migrate to canonical registry now (CRM precedent `crm_error_contract_test.go`; fees clients just absorbed a breaking route change — one migration window, not two). **TRC-/TPL-/REP-**: relocate into `pkg/constant/errors.go` as documented prefixed namespaces (kill the fork packages) but keep the code strings (operator dashboards grep them; tracer/reporter are separate deploy units with their own clients) | Split: break FEE-, keep-and-relocate TRC-/TPL-/REP- |
| D2 | 400→422/409 re-typing. Forks: fold into the D1 migration (one announcement). Mainline ledger (82 ValidationError sites incl. `ErrTransactionValueMismatch`): **defer** — document as a known deviation; a mainline status-code break needs its own client-comms window | Forks now, mainline documented-deferred |
| D3 | Envelope convergence may alter JSON field casing/presence on some fee/tracer endpoints (fee ValidationError already drifted — no json tags) | Converge; the drift is already a bug-shaped contract |
| D4 | Tracer's bespoke Prometheus metric families (names pinned for Grafana dashboards) vs lib-observability MetricsFactory | Bless tracer's `recorder.go` discipline as the sanctioned model, keep pinned names; mandate MetricsFactory for all NEW metrics |
| D5 | Transaction-consumer DLQ topology + retry budget (infra change to `definitions.json`) | Adopt reporter pattern: maxRetries 3, exponential backoff, `transaction.dlx`/`transaction.dlq` mirroring reporter's TTL/max-len |
| D6 | Domain-metrics scope: mandatory on all endpoints vs middleware coverage sufficient | Middleware sufficient for now; domain metrics opt-in by product priority; no mass label rename (F17 verified labels are bounded) |
| D7 | reporter HMAC soft-fail (D6 legacy) and partial-result posture (D7 legacy) | Keep as documented carve-outs in the standard |

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

- [ ] Done

**Context:** D1–D7 are wire-visible or ops-owned. Phases 3, 4, and 6 elaborate differently depending on the answers (e.g., D1 decides whether Phase 3 renumbers FEE- codes or relocates them; D5 decides Phase 4's infra task).

**Implementation vision:** Present the Decision Points table at the Phase 1 checkpoint using the question tool, one decision at a time where independent, with the recommendation as option 1. Record outcomes in this plan (Decision Points table gains an "Outcome" column) and in the two standards docs (replace `per D<n> outcome` placeholders).

**Files:**
- Modify: `docs/plans/2026-06-07-telemetry-error-normalization.md` (this doc — Outcome column)
- Modify: `docs/standards/telemetry.md`, `docs/standards/error-handling.md` (placeholder resolution)

**Verification:** zero `per D` placeholders remain unresolved in either standard.

**Done when:** all seven decisions have recorded outcomes; later-phase epics whose scope changed are annotated before Phase 2 elaboration.

---

## Phase 2: P0 leak remediation

**Milestone:** No financial values, PII, or raw payloads on any telemetry signal or client-surfaced error field. Wire-invisible (telemetry-only), so safe to ship first. Verified by grep sweeps + targeted tests.

### Epic 2.1: Financial values out of logs (F1)

**Goal:** fees and tracer log no monetary amounts at any level; `%#v` payload logging eliminated repo-wide.
**Scope:** `components/ledger/internal/services/fees/` (billing-calculate, volume/maintenance builders), `components/tracer/internal/services/validation_service.go`, fees handlers with `%#v`.
**Dependencies:** Phase 1 (standard rule 9 defines what replaces them: IDs + presence flags + counts).
**Done when:** rg for amount-bearing log fields (`gross|net|unitPrice|feeAmount|totalNetAmount|amount`) inside logger calls in those slices returns only non-value matches; existing tests still green.

### Epic 2.2: PII and payloads off spans (F2)

**Goal:** zero `SetSpanAttributesFromValue(..., nil)` call sites; CRM update-path spans carry presence flags, not entities; rabbitmq consumer span carries message IDs, not the body.
**Scope:** all 48 nil-redactor sites (tracer 28, fees 9+, crm 2, reporter 2, rabbitmq 1, + remainder per Task 1.1.4 tally); `consumer.rabbitmq.go:316`; `holder.mongodb.go:256`, `instrument.mongodb.go:252`.
**Dependencies:** Phase 1 (rule 4 decides presence-flag shape; D4 unaffected).
**Done when:** `rg 'SetSpanAttributesFromValue\(.*nil\)'` returns zero non-test hits; crm create-path `has_*` idiom extended to update paths.

### Epic 2.3: Client-surfaced internal detail (F3)

**Goal:** reporter persists a classified `error_code` (not raw `err.Error()`) into report metadata; tracer never sets client message fields from unmapped `err.Error()`.
**Scope:** `components/reporter-worker/internal/services/generate-report.go:203-223`, `data-pipeline.go:40`, `components/tracer/internal/adapters/http/in/rule_handler.go:91,170,310`.
**Dependencies:** Phase 1 (rule 9); independent of Epics 2.1/2.2.
**Done when:** GET report metadata shows classified codes for induced failures; tracer unmapped errors render generic messages; tests cover both.

---

## Phase 3: Error platform consolidation

**Milestone:** `feeshared`, `pkg/reporter` error stack, and `tracer/pkg` error stack are deleted; every handler in the binary + both services flows through canonical `pkg/errors.go` + hardened `pkg/net/http.WithError`; one envelope everywhere. Shaped by D1–D3 outcomes.

### Epic 3.1: Harden the canonical boundary (F15, F11)

**Goal:** `WithError` resolves via `errors.As`; `GetUUIDFromLocals` returns a typed pkg error; `FailedPreconditionError`/`HTTPError` explicitly mapped or removed; bare-sentinel passes (CRM `cn.ErrInternalServer`) eliminated.
**Scope:** `pkg/net/http/errors.go`, `pkg/net/http/httputils.go:563-575`, CRM handler bare-sentinel sites.
**Dependencies:** Phase 1 standard. Must land FIRST in this phase — fork deletion re-points onto it.
**Done when:** table-driven test proves wrapped business errors map to their typed status; existing error-contract tests green.

### Epic 3.2: Delete the feeshared fork (F4, F16-fork-scope)

**Goal:** `feeshared/errors.go`, `feeshared/constant/errors.go`, `feeshared/nethttp` gone; fee handlers single-import canonical `http.WithError`; FEE codes migrated/relocated per D1; business-rule errors re-typed 422/409 per D2; fees Entity constants added.
**Scope:** `components/ledger/pkg/feeshared/**`, fee/billing handlers, `pkg/constant/{errors,entity}.go`; contract test per D1 (CRM template).
**Dependencies:** Epic 3.1; D1/D2 outcomes.
**Done when:** zero imports of feeshared error packages; fee error contract locked by test; swagger/postman regenerated if envelope changed (D3).

### Epic 3.3: Delete the reporter fork; fix the live not-found 500 (F4, F10)

**Goal:** `pkg/reporter/errors.go` + `constant/errors.go` + `net/http` writers deleted onto canonical; the three mongo not-found conventions unified at the adapter boundary; `update-template-by-id.go` 404s correctly; `IsBusinessError` predicate preserved (it moves to `pkg/` — Phase 5 consumers depend on it, define the new home here).
**Scope:** `pkg/reporter/**`, reporter-manager/worker handlers and services; TPL-/REP- relocation per D1.
**Dependencies:** Epic 3.1; D1 outcome.
**Done when:** zero fork imports; not-found template update returns 404 (test); classifier (`error_classifier.go`) re-pointed at canonical types.

### Epic 3.4: Delete the tracer fork (F4, F14)

**Goal:** tracer's `pkg/errors.go`, `pkg/constant/errors.go`, dead `pkg/net/http` mapper deleted; handlers route through canonical WithError; TRC- codes relocated per D1, referenced by constant identifier (no `"TRC-0003"` literals); dual ValidateBusinessError resolved to one; envelope carriers collapsed per D3.
**Scope:** `components/tracer/pkg/**`, `components/tracer/internal/adapters/http/in/**`.
**Dependencies:** Epic 3.1; D1/D3 outcomes.
**Done when:** zero fork imports; readyz TRC- codes still emitted (operator contract per D1); handler mapping is table-driven through the canonical boundary.

### Epic 3.5: One envelope + typed classification (F12, F18)

**Goal:** `LegacyErrorBoundary`'s `{"error":text}` retired (raw `fiber.Error` sources converted to typed errors, incl. `MarkTrustedAuthAssertion`); CRM duplicate-key disambiguation switches to typed `mongo.WriteException` index-name inspection **preserving** the `create-holder-with-id.go:62-78` idempotency contract.
**Scope:** `components/ledger/internal/adapters/http/in/errors.go`, `pkg/net/http/protected_routes.go`, `components/crm/adapters/mongodb/{holder,instrument}/`.
**Dependencies:** Epics 3.1–3.2 (envelope decisions settled).
**Done when:** no route can emit `{"error":text}` (test via panic/raw-fiber-error injection); CRM idempotent-create integration tests green with the typed mechanism.

---

## Phase 4: Async resilience

**Milestone:** A poison message cannot pin a ledger worker; every production panic is dispositioned; Redis-path posture matches the standard. Shaped by D5 and Task 1.1.2/1.1.3 findings.

### Epic 4.1: Shared consumer error classifier

**Goal:** reporter's classify/backoff/DLQ machinery (`retry_manager.go`, `error_classifier.go`) generalized into a shared `pkg/` package consumable by any consumer, re-pointed at canonical error types (depends on Epic 3.3 having landed).
**Scope:** new `pkg/` package; `pkg/reporter/rabbitmq/` callers migrate.
**Dependencies:** Phase 3 complete.
**Done when:** reporter-worker runs on the shared package with behavior-identical tests.

### Epic 4.2: Transaction consumer DLQ + classification (F5)

**Goal:** ledger transaction consumer classifies permanent-vs-transient, dead-letters permanents, bounded-retries transients; blanket `Nack(requeue=true)` gone from all three sites; DLX/DLQ provisioned in infra topology per D5.
**Scope:** `components/ledger/internal/adapters/rabbitmq/consumer.rabbitmq.go`, `components/infra/rabbitmq/etc/definitions.json`.
**Dependencies:** Epic 4.1; D5 outcome (retry budget, topology names).
**Done when:** integration test feeds a poison message (nil-Transaction payload) and observes DLQ delivery within the retry budget — no hot loop.

### Epic 4.3: Panic disposition (F13)

**Goal:** every class-(a) panic from the Task 1.1.3 inventory converted to error returns (known: `consumer.rabbitmq.go:116` constructor); class-(b)/(c) documented in the standard's exception list.
**Scope:** per inventory.
**Dependencies:** Task 1.1.3 inventory; independent of 4.1/4.2.
**Done when:** class-(a) list empty; constructor failure path returns error and bootstrap handles it.

### Epic 4.4: Redis/balance-sync posture (G4)

**Goal:** the Redis consumer and balance-sync worker conform to the trace-propagation and error-posture rules; whether the DLQ standard extends to the Redis path is decided from Task 1.1.2 findings.
**Scope:** `components/ledger/internal/bootstrap/redis.consumer.go`, `balance_sync.worker.go`.
**Dependencies:** Task 1.1.2 findings; Phase 1 rules 10–11.
**Done when:** both loops carry trace context per the standard; error posture documented or remediated per findings.

---

## Phase 5: Hygiene sweep (mechanical, high-volume)

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

*(filled during execution — deviations, learnings, count corrections)*

## Out of Scope

- Mainline ledger 400→422 re-typing (D2 defers; documented deviation).
- Renaming tracer's pinned Prometheus metric families (D4 recommendation keeps them).
- Mass metric-label renames (F17 verified labels are bounded; cross-signal naming documented, not migrated).
- lib-observability / lib-commons upstream changes (e.g., making `SetSpanAttributesFromValue` require a redactor) — file upstream tickets if Phase 2 wants them, but do not block on them.
- Streaming event payload schemas (governed wire contract — G2 carve-out).
- reporter HMAC soft-fail / partial-result postures (D7 carve-outs).
