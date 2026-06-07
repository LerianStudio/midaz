# Telemetry & Error Audit — Phase 1 Appendix (2026-06-07)

Closes audit coverage gaps G1, G3, G4, G7, G8 from `2026-06-07-telemetry-error-normalization.md`, using the same checklist as the original 32-agent audit (`2026-06-07-telemetry-error-audit.json`). All counts here supersede the original audit's tallies (G7 reconciliation). New findings carry F21+ numbers and are mirrored into the plan's Ground Truth table with a `[Phase 1 addition]` marker.

---

## 1. Streaming surface (G1)

**Scope swept:** `pkg/streaming/**` (emit.go, tenant.go, 32 event-constructor files) + all 29 command files calling `pkgStreaming.EmitImportant` (`rg -l 'pkgStreaming.EmitImportant' components/ --glob '*.go' --glob '!*_test.go'`).

### EmitImportant contract verification (`pkg/streaming/emit.go`)

| Contract element | Status | Evidence |
|---|---|---|
| Nil-emitter guard | ✓ | `emit.go:44-46` |
| Tenant via `ResolveTenantID` | ✓ | `emit.go:48`, `tenant.go:42-52` |
| Bounded emit context (5s default, env override) | ✓ | `emit.go:56-57,65-77` |
| `HandleSpanError` (not BusinessErrorEvent) | ✓ | `emit.go:50,60` |
| Warn + `libLog.Err` | ✓ | `emit.go:51,61` |
| Non-propagation (void return) | ✓ | `emit.go:43-63` |

### Violations found

| Site | Class | Detail |
|---|---|---|
| `send_transaction_events.go:72,83,105,122` | Sprintf-in-logger | legacy RabbitMQ path (pre-lib-streaming), incl. 2 per-request Info lines (`:72,:105`) |
| `send_overdraft_events.go:101` | Per-request Info | "Overdraft events not enabled" fires per request |
| `create_asset.go` (10 sites) | Sprintf-in-logger | non-emit code in an emit-site file |

**Clean:** all 32 event constructors log nothing (no payload leakage); all 29 emit sites delegate to `emit<Event>Event` helpers (zero inlining); zero `SetSpanAttributesFromValue` in the streaming surface; zero emit-failure propagation.

**Counting note:** the emit-site command files belong to the ledger-core slice directory-wise; their violations are counted inside ledger-core's tally (no separate streaming bucket — `pkg/streaming/` itself is at zero on all four classes). The original audit's exclusion of these files understated ledger-core by ~14 Sprintf sites; the §5 tally includes them.

---

## 2. Async loops (G4)

### `bootstrap/redis.consumer.go`

- **Trace posture:** loop contexts derive from bare `context.Background()` (`redis.consumer.go:100`); no trace extraction from queued records.
- **Error posture — NEW FINDING F21 (P1):** malformed/failed records are *skipped but never deleted* from the Redis queue (`:285-293` malformed JSON, `:350` nil Validate, `:454-456` settings fetch, `:521-527` build ops, `:536-543` async write). Next 30-min cycle re-attempts the same poison items forever — unbounded queue growth, no retry counter, no dead-letter, no alert. Redis-path analog of F5.
- **Log discipline:** per-item Info noise at `:406,:454,:524,:541,:546`.

### `bootstrap/balance_sync.worker.go`

- **Trace posture:** bare `context.Background()` (`:162`) but tenant-enriched (`:310`).
- **Error posture:** poison records ARE deleted (`:471`) ✓. **NEW FINDING F23 (P2):** per-tenant PG connection/retrieval failure silently skips that tenant's whole sync cycle (`:313-315,:321-324`) — log line only, no metric, no alert; invisible to readyz (probes common pool only).
- **Log discipline:** correct (Debug per-item, Info lifecycle).

---

## 3. Health/readyz + shutdown ordering (G8)

### Per-slice readyz posture

| Component | Failure log level | Probes traced/metered? | Notes |
|---|---|---|---|
| ledger | Warn | **YES — F22 (P2)** | `unified-server.go` registers `tlMid.WithTelemetry(telemetry)` with no excluded routes before `/health` + `/readyz`; lib supports exclusion via variadic (`lib-observability@v1.0.1 middleware/telemetry.go:86-97`) but ledger doesn't use it. Every probe = span + metrics. |
| tracer | Error | No — filtered | explicit probe-path skip in its otel-fiber wiring |
| reporter-worker | (no logging) | No — bare `net/http` health server | metric name drift: `readyz_check_status` vs ledger/tracer `readyz_check_status_total` |

Logging middleware already excludes `/health`, `/readyz`, `/metrics` by default (lib `middleware/logging.go: defaultLogExcludedRoutes`) — the gap is spans/metrics only, and only in ledger.

**Rule implication (telemetry standard rule 11/12):** probe-path exclusion must be explicit — pass excluded routes to `WithTelemetry`; tracer is the canonical example.

### Shutdown / flush ordering

| Component | Order | Flush last? |
|---|---|---|
| ledger | HTTP drain → Telemetry → Logger (ServerManager) | ✓ (caveat: background worker tasks may log after telemetry close — benign race, note in standard) |
| tracer | drain → grace → HTTP → workers → PG → ServerManager telemetry | ✓ (implicit via ServerManager) |
| reporter-worker | health → PDF pool → EventListener → MT cleanup → RabbitMQ → MongoDB → **Telemetry last** | ✓ (explicit; cleanest) |

The standard's "flush telemetry last" claim is **validated** for all three.

---

## 4. Panic inventory (G3)

**Command:** `rg -n 'panic\(' components/ pkg/ --glob '*.go' --glob '!*_test.go' --glob '!*mock*' --glob '!*_mock.go'` → **7 hits, 7 rows** (match ✓).

| Path:line | Class | Disposition |
|---|---|---|
| `components/ledger/internal/adapters/rabbitmq/consumer.rabbitmq.go:116` | (a) real violation — constructor panic on connection failure | **convert-to-error** (Epic 4.3) |
| `components/ledger/internal/adapters/postgres/ledger/ledger.postgresql.go:1011` | (b) re-panic after tx rollback | keep — sanctioned defer-recover-cleanup-repanic |
| `pkg/net/http/withBody.go:262` | (c) init guard — validator translation registration | keep-document; **borderline**: reachable on first request, not at boot — Epic 4.3 re-reviews whether to convert |
| `pkg/reporter/pongo/pongo.go:140` | (c) fail-closed security guard (SSTI tag ban) | keep — sanctioned |
| `components/tracer/internal/testhelper/time_of_day.go:18` | (d) test helper | out-of-scope |
| `components/tracer/internal/testutil/uuid_helpers.go:66,109` | (d) test helpers (2 hits) | out-of-scope |

- Class (a) list for Epic 4.3: **1 item** (`consumer.rabbitmq.go:116`).
- `tracer tx_helper.go` (`internal/services/command/`) recovers and converts to wrapped error — it does NOT re-panic, hence absent from the grep; confirmed as the sanctioned recover-to-error pattern.
- `log.Fatal*`/`os.Exit`: zero hits outside `main.go`/bootstrap.

---

## 5. Master tally (G7) — authoritative counts

Single methodology, non-test `.go`, excluding mocks/generated. Slice globs:

- **ledger-core**: `components/ledger/internal/` minus `adapters/http/`, `services/fees/`, `adapters/mongodb/fees/`
- **ledger-http**: `components/ledger/internal/adapters/http/` + `pkg/net/http/`
- **fees**: `components/ledger/pkg/fee{,shared}/`, `internal/services/fees/`, `internal/adapters/mongodb/fees/`, fee handlers in `adapters/http/in/` (ownership: counted under fees for nil-redactor)
- **crm**: `components/crm/` + crm handlers in `adapters/http/in/`
- **tracer**: `components/tracer/` · **reporter**: `components/reporter-{manager,worker}/` + `pkg/reporter/` · **pkg-shared**: `pkg/` minus `net/http`, `reporter`, `streaming`

### Commands

1. Sprintf-in-logger: `rg 'logger\.(Log|Info|Infof|Warn|Warnf|Error|Errorf|Debug|Debugf|Fatal)[^\n]*fmt\.Sprint' <slice> --glob '*.go' --glob '!*_test.go' --glob '!*mock*' -c`
2. ctx-rebinds (raw): `rg -n 'ctx, span\w* :?= .*[tT]racer\.Start' <slice> ...` — raw count includes legitimate root spans; non-root share estimated by span-name depth sampling (~30-37% per slice)
3. Info noise: `rg '(Info|LevelInfo)[^\n]*"(Initiating|Retrieving|Trying to|Successfully|Starting|Sending|Getting|Creating|Updating|Deleting|Fetching)' <slice> ...`
4. Nil-redactor: count ALL `SetSpanAttributesFromValue(` call sites (`rg -Un 'SetSpanAttributesFromValue\(' ...`) and inspect each final argument. **Correction (Phase 2 execution):** the anchored single-line pattern (`[^)]*nil\s*\)`) undercounts — it misses multi-line calls whose map-literal args contain inner parens. True count was **99 of 100** sites passing nil (tracer had 41, not 18; the 23 extra were multi-line map-literal calls). The unanchored count of 87 was also wrong (different miss profile). Lesson recorded: for call-shape counts, enumerate call sites and inspect; don't anchor on argument position.

### Tally

| Slice | Sprintf-in-logger | ctx-rebinds raw (~non-root) | Info noise | Nil-redactor | Tier |
|---|---|---|---|---|---|
| ledger-core | 780¹ | 453 (~166) | 88 | 1 (`consumer.rabbitmq.go:316`) | **L** |
| ledger-http | 251 | 108 (~36) | 162 | 2 (`metadata.go`) | **L** |
| fees | 88 | 50 (~17) | 14 | **30** (mongodb 20 + services 5 + handlers 5) | **M** |
| crm | 31 | 39 (~13) | 5 | 4 (2 mongodb + 2 handlers) | **S** |
| tracer | 0 | 115 (~35) | 44 | 18 | **M** |
| reporter | 0 | 193 (~58) | 87 | 21 | **M/L** |
| pkg-shared + pkg/streaming | 0 | 0 | 0 | 0 | — |
| **Total** | **1,150** | **958 (~325)** | **400** | **76 of 100 call sites** | |

¹ Spot-check re-run produced 784 (0.5% delta from glob-exclusion differences); 780 recorded as the slice-glob-exact figure.

### Corrections vs original audit

| Claim (original) | Authoritative | Impact |
|---|---|---|
| ledger-core Sprintf 859 / 816 / 402 | **780** | tier unchanged (L) |
| nil-redactor "48 of 81", tracer 28 | **99 of 100** (corrected during Phase 2; see §5 command-4 note); fees 30, tracer 41, reporter 21, crm 4, ledger-http 2, ledger-core 1 | Epic 2.2 scope doubled vs original; tracer is the largest holder |
| ctx-rebinds "135 (ledger 121, crm 13)" | raw 958; ~325 estimated non-root — original counted only *confirmed leaf-I/O* rebinds | Epic 5.2 elaboration must do per-file review, not blind regex; estimate is name-depth heuristic (medium confidence) |
| Info noise (uncounted) | 400 | Epic 5.3 scope quantified |

---

## 6. New findings (mirrored into Ground Truth)

| # | Finding | Sev | Evidence |
|---|---|---|---|
| F21 | Redis consumer poison records skipped-not-deleted — re-attempted every cycle forever, no retry counter/alert. **Remediation reframed by D5-v2 (2026-06-07): these records are the ONLY durable copy of authorized transactions (the backup hash is the async WAL) — delete-only would destroy the financial record. Correct fix: after N failed cycles, persist to a Postgres quarantine table + Error log + alert metrics; HDel only after quarantine confirmed.** | **P1** | `redis.consumer.go:285-293,350,454-456,521-527,536-543`; durability chain: `balance_atomic_operation.lua` (~:220 atomic seed), `create_balance_transaction_operations_async.go:159,334` (post-persist delete), Valkey AOF everysec + noeviction (`infra/docker-compose.yml:55-56`) |
| F22 | Ledger probe traffic generates spans+metrics on every k8s probe (`WithTelemetry` exclusion unused); tracer filters correctly | P2 | `unified-server.go` middleware block; lib `middleware/telemetry.go:86-97` |
| F23 | balance-sync silently skips a tenant's full cycle on per-tenant PG failure — no metric/alert, invisible to readyz | P2 | `balance_sync.worker.go:313-315,321-324` |
| — | reporter-worker readyz metric name drift (`readyz_check_status` vs `_total`) | P3 | reporter-worker health server vs ledger/tracer readyz metrics |
