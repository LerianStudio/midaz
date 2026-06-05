# F2 — Execution note (decisions, guards, drift)

> Created during execution per `docs/v4/plan/F2.md` §5. Records the F2-T01 DO-NOT-TOUCH
> allowlist (executable guard), the F2-T02 Q7 dual-use audit result + frozen rename list,
> the F2-T03 namespace decision, the F2-T08 entity-constant add, and any HEAD drift.
> Mirrors `docs/monorepo/plan/P2a-EXECUTION-NOTE.md`.
>
> - **Date:** 2026-06-05
> - **Branch:** `feat/monorepo-consolidation`
> - **Module at HEAD:** `github.com/LerianStudio/midaz/v3`

---

## F2-T01 — DO-NOT-TOUCH allowlist (executable guard)

The collision is `mmodel.Alias` (the CRM entity being renamed to `Instrument`) vs `Account.Alias`
(the transaction routing handle, NOT renamed). Every rename in F2 is **entity-scoped**
(`mmodel.Alias` and its surface), never a word-scoped sweep of `alias`. The strings below MUST
survive F2 unchanged. The F2-T18 gate re-runs this exact block.

### Guard block (paste-and-run; all five must pass)

```bash
set -e
cd "$(git rev-parse --show-toplevel)"

# (1) the 6 numeric-coded sentinels — identifiers AND codes stay
grep -nE 'ErrAliasUnavailability|ErrFailedToRetrieveAccountsByAliases|ErrAccountAliasNotFound|ErrAccountAliasInvalid|ErrAccountingAliasValidationFailed|ErrDuplicatedAliasKeyValue' pkg/constant/errors.go
# expect EXACTLY these 6 lines, codes unchanged:
#   34: ErrAliasUnavailability                = errors.New("0020")
#   77: ErrFailedToRetrieveAccountsByAliases  = errors.New("0063")
#   99: ErrAccountAliasNotFound               = errors.New("0085")
#  110: ErrAccountAliasInvalid                = errors.New("0096")
#  132: ErrAccountingAliasValidationFailed    = errors.New("0118")
#  137: ErrDuplicatedAliasKeyValue            = errors.New("0123")

# (2) routing-handle code (fee account resolver) — stays
grep -nE 'resolveByAliases|GetAccountByAlias|target\.Aliases' components/ledger/internal/services/fees/midaz/account_resolver.go
# expect: target.Aliases (:85,:88), resolveByAliases (:88,:116,:117), GetAccountByAlias (:137)

# (3) QueryHeader.Alias *string — dual-use account-listing param, stays (see F2-T02)
#     NOTE: field is gofmt column-aligned; use the relaxed-whitespace pattern, NOT 'Alias \*string'
grep -nE 'Alias +\*string' pkg/net/http/httputils.go
# expect: 70: Alias  *string

# (4) the 7 Mongo collection literals "aliases_" (production data) — byte-for-byte
grep -rn 'aliases_' components/crm/adapters/mongodb/alias/ | grep -v _test | wc -l
# expect: 7
```

Out-of-band DO-NOT-TOUCH (no clean single grep, enforced by reviewer + entity-scoped renames):
- **(5)** `Account.Alias` / `Balance.Alias` struct fields and ALL routing-handle code (the resolver
  above, transaction-DSL `@handle` parsing, streaming `Account.Alias`/`Balance.Alias` payload fields).
- **(6)** CRM-00xx error **code strings** (`"CRM-0008"`, `"CRM-0017"`, `"CRM-0023"`, …) — only the Go
  identifiers rename (F2-T10); the wire code strings stay byte-for-byte.
- **(7)** `components/crm/api/*` — generated swagger; F5 regenerates. Leave stale.
- **(8)** No opportunistic logging-compliance fixes (`fmt.Sprintf`-in-logger, `LevelInfo`) — deferred by §6.

### Baseline (recorded at F2 start, HEAD `2bdbfe556`)

```
# (1) 6 numeric sentinels — UNCHANGED
pkg/constant/errors.go:34:  ErrAliasUnavailability                      = errors.New("0020")
pkg/constant/errors.go:77:  ErrFailedToRetrieveAccountsByAliases        = errors.New("0063")
pkg/constant/errors.go:99:  ErrAccountAliasNotFound                     = errors.New("0085")
pkg/constant/errors.go:110: ErrAccountAliasInvalid                      = errors.New("0096")
pkg/constant/errors.go:132: ErrAccountingAliasValidationFailed          = errors.New("0118")
pkg/constant/errors.go:137: ErrDuplicatedAliasKeyValue                  = errors.New("0123")

# (2) routing-handle sites — UNCHANGED
account_resolver.go:85:  case len(target.Aliases) > 0:
account_resolver.go:88:  accounts, err = r.resolveByAliases(ctx, orgID, ledgerID, target.Aliases)
account_resolver.go:116: // resolveByAliases resolves each alias individually via GetAccountByAlias.
account_resolver.go:117: func (r *midazAccountResolver) resolveByAliases(
account_resolver.go:137: account, err := r.resolver.GetAccountByAlias(ctx, orgID, ledgerID, alias)

# (3) QueryHeader.Alias — PRESENT
pkg/net/http/httputils.go:70: Alias  *string

# (4) collection literals — COUNT = 7
alias.mongodb.go:109,189,246,349 + alias_maintenance.mongodb.go:50 + alias_query.mongodb.go:45,211
```

### Drift (F2-T01)

- **The F2.md F2-T01 verification grep `'Alias \*string'` (single space) returns ZERO at HEAD.** The
  field is gofmt column-aligned (`Alias` + tab-padded spaces + `*string`). The guard above uses
  `'Alias +\*string'`. Anyone running the spec's literal pattern would falsely conclude the field was
  already gone. Corrected in the guard block. `QueryHeader` carries **no struct tags** — query-param
  binding is done elsewhere (lib-commons header parser), so this is a Go-identifier-only rename surface
  for F2-T13 (wire param names are independent; see F2-T02 decision).

---

## F2-T02 — Q7 dual-use audit of `QueryHeader.Alias` + frozen rename list

### AUDIT RESULT (measured at HEAD; binding so F2-T13 cannot drift)

**`QueryHeader.Alias` IS dual-use → STAYS (DO-NOT-TOUCH).** It is read by the ledger account-listing
filter path:

```
components/ledger/internal/adapters/postgres/account/account.postgresql.go:313:
    if filter.Alias != nil && *filter.Alias != "" {
account.postgresql.go:314:
    ... = http.EscapeSearchMetacharacters(*filter.Alias)
```

This is the `Account.Alias` routing-handle account-listing `?alias=` param — renaming it would break
account listing. It is NOT an instrument-domain field.

**Frozen instrument-domain rename list (exactly 6 fields, F2-T13 scope):**

| Field (HEAD `httputils.go`) | Line |
|-----------------------------|------|
| `BankingDetailsBranch`      | :52  |
| `BankingDetailsAccount`     | :53  |
| `BankingDetailsIban`        | :54  |
| `RegulatoryFieldsParticipantDocument` | :56 |
| `RelatedPartyDocument`      | :57  |
| `RelatedPartyRole`          | :58  |

### Reader sites F2-T13 must follow (THREE span-flag readers + one CRM filter-construction reader)

1. **CRM** `components/crm/adapters/mongodb/alias/alias_query.mongodb.go`
   - `:64-65` span flags (`has_related_party_filters`, `has_banking_details_filters`).
   - `:143-168` filter construction (reads all 6 via `query.<Field>`). *(Lives in the CRM file F2-T13/T05 already touch.)*
2. **CRM** `components/crm/adapters/http/in/observability.go:38-39` — span flags.
3. **LEDGER** `components/ledger/internal/adapters/http/in/observability.go:207-208` — span flags.
   **This is the third reader the macro plan missed (drift, below). F2-T13 must edit all three.**

`components/crm/adapters/mongodb/alias/alias.go` (`:35,37,38,208,219,229`) carries fields of the SAME
names (`BankingDetailsAccount`, `RegulatoryFieldsParticipantDocument`, `RelatedPartyDocuments`) but
those are the **persistence MongoDBModel's own struct fields**, not `QueryHeader` reads — a distinct
surface owned by F2-T05's model rename, NOT part of the F2-T13 `QueryHeader` field rename. Do not
conflate them.

### DECISION: wire `?param=` names PRESERVED

Only the Go **identifiers** on `QueryHeader` rename in F2-T13. The wire query-param strings clients send
stay current. `QueryHeader` has no struct tags, so the binding mapping lives in the lib-commons header
parser; F5 owns any wire-contract (query-param) change. F2-T13 is a pure Go-identifier rename + reader
follow-through. (F2-T13 itself decides final identifier names; this note freezes the SET of 6 and the
3+1 reader sites, and the dual-use exclusion of `Alias`.)

### Drift (F2-T02)

- **§6 Cross-component sweep said the instrument fields are read "by the CRM alias listing path via
  `observability.go:38-39`" — only the CRM reader.** HEAD has a **THIRD reader**: the ledger-side
  `components/ledger/internal/adapters/http/in/observability.go:207-208` aggregates the same CRM-domain
  fields into ledger span flags. F2-T13 must edit all three reader sites + the CRM filter-construction
  block, not just the CRM observability surface. Confirms Risk 11 was right to mandate this audit. (This
  matches the drift note already pre-written in F2.md F2-T02 Notes — verified true at HEAD.)

---

## F2-T03 — Namespace-target decision (binding unless Fred overrides)

| Dimension | Decision | Rationale |
|-----------|----------|-----------|
| **Namespace** | `midaz` | The ledger's own namespace from the F0 four-namespace inventory. CRM is folded into the ledger binary; it should authorize under the host's namespace, collapsing one of the four into the unified surface. |
| **`holders` resource** | unchanged (`"holders"`) | Holder is not renamed (Q6 / Design call 4); resource name stays. |
| **`aliases` resource** | replaced by `"instruments"` | Tracks the D-1 entity rename. |
| **Related-parties** | UNDER the `instruments` resource (sub-resource maintenance), NOT its own resource | Q8-RESOLVED: the related-parties `DELETE` authorizes with resource `"instruments"`, verb `delete` — one fewer resource in the X1 policy matrix. |
| **`ApplicationName` const** | removed / repointed to the ledger's `midaz` namespace | Done in F2-T12. |

### Implementation shape for F2-T12

CRM `components/crm/adapters/http/in/routes.go` at HEAD:
- `const ApplicationName = "plugin-crm"` (`:21`).
- holders authz `auth.Authorize(ApplicationName, "holders", …)` (`:89-96`, incl. the F1-T13
  `GET /v1/holders/:id/accounts` at `:92`).
- aliases authz `auth.Authorize(ApplicationName, "aliases", …)` — **6 sites at `:99-104`** (drift
  below).
- doc comment `:82` ("plugin-crm via ApplicationName").

The ledger's own namespace is the named constant `midazName = "midaz"`
(`components/ledger/internal/adapters/http/in/routes.go:26`; F0-T07 note). F2-T12 either repoints the
CRM constant to the literal `"midaz"` or imports the ledger namespace constant; either way `plugin-crm`
leaves the codebase.

### X1 implications (load-bearing)

This namespace/resource matrix is exactly what Fred + the plugin-auth team migrate tenant-manager
policies to. Per `docs/auth/RBAC-NAMESPACES.md:8-11`, tenant-manager RBAC policies key on the literal
namespace string; flipping `plugin-crm` → `midaz` **orphans every tenant's `plugin-crm:*` grant** in the
external auth-server. The policy migration is **X1 — Fred-owned, release/deploy gate, NOT an F2 merge
blocker.** Local/dev with auth disabled are unaffected. The post-cut grant matrix tenants migrate TO:

```
plugin-crm:holders:{get,post,patch,delete}    →  midaz:holders:{get,post,patch,delete}     (unchanged resource)
plugin-crm:aliases:{get,post,patch,delete}    →  midaz:instruments:{get,post,patch,delete}  (resource renamed)
   (related-parties DELETE)                    →  midaz:instruments:delete                    (sub-resource, Q8)
```

F2-T17 records this flip in `docs/auth/RBAC-NAMESPACES.md` + the migration guide.

**Citation:** F0 four-namespace inventory — `docs/auth/RBAC-NAMESPACES.md:23` (`plugin-crm` row),
`midaz` row (ledger, `midazName` const); F0-EXECUTION-NOTE.md F0-T07 (named-constant drift).

### Drift (F2-T03)

- **CRM alias route + authz registrations are at `routes.go:99-104` at HEAD, NOT `:93-98` as F2.md
  cites.** The block shifted down because F1-T13 inserted `GET /v1/holders/:id/accounts` at `:92`
  under the SAME `ApplicationName` constant. Consequence for F2-T12: the `ApplicationName` constant now
  carries **7** routes (6 holders/accounts + the constant itself), and repointing the constant moves the
  new accounts route automatically — no extra edit, exactly as the F2-T12 R16-handoff note predicted.
  F2-T11/T12 must target `:99-104` for the alias lines.

---

## F2-T08 — Entity constants added

`pkg/constant/entity.go`: added `EntityInstrument = "Instrument"` (`:17`, between `EntityHolder` and
`EntityLedger`) and `EntityRelatedParty = "RelatedParty"` (`:23`, between `EntityPortfolio` and
`EntitySegment`) — alphabetical, consistent with the existing block ordering.

`EntityHolder = "Holder"` **already existed** (`:16`, added by F1-T04) — NOT re-added. Per F2-T08 Notes,
this task adds only the two instrument/related-party constants and skips the duplicate.

Verify: `go build ./pkg/constant/` ✓, `go vet ./pkg/constant/` ✓, `go vet -tags=integration ./pkg/constant/` ✓,
`go test ./pkg/constant/` ✓. `grep -n 'EntityInstrument\|EntityRelatedParty\|EntityHolder' pkg/constant/entity.go`
→ `:16` (Holder), `:17` (Instrument), `:23` (RelatedParty).

`EntityInstrument = "Instrument"` makes the externally-visible error-`entity` body value deliberately
flip `"Alias"` → `"Instrument"` (Risk R43) once F2-T09 substitutes the 14 `reflect.TypeOf` sites. The
7 `RelatedParty` sites keep serializing `"RelatedParty"` (value unchanged; only the banned reflect
pattern removed).

---

## F2-T17 — Docs/postman sweep (instruments noun, namespace flip, X1 note)

Edited (8 files, docs/postman only — zero `.go` files touched):

- `docs/api/SCOPING.md` — header-scoping section: `holders / aliases` → `holders / instruments`;
  `POST /v1/holders/{holder_id}/aliases` → `.../instruments`; handler-file ref `alias.go` →
  `instrument.go`.
- `docs/auth/RBAC-NAMESPACES.md` — flipped the matrix: **four namespaces → three** (`plugin-crm`
  collapsed into `midaz`; `holders`+`instruments` resources folded under the `midaz` row; CRM
  `ApplicationName = "midaz"` cited as a second owner of the row). Replaced the "preserved as-is /
  deferred" framing with the executed-flip framing. Added a dedicated **"X1 — policy migration"**
  section with the grant matrix (`plugin-crm:* → midaz:{holders,instruments}:*`), Fred-owned +
  plugin-auth, release/deploy gate NOT merge gate. Kept the policy-key coupling block intact;
  fixed the residual "four"→"three" counts. R9 marked closed for CRM by the flip; `plugin-fees`
  stays distinct by design. Every `plugin-crm` occurrence is now flip/migration-note context.
- `postman/MIDAZ.postman_collection.json` — "Aliases" folder → "Instruments" (lines 16143–16986):
  folder + 5 request names, 6 raw URLs, all `path[]` segments `aliases`→`instruments`, the 4
  url-variable `"alias_id"` keys → `"instrument_id"`, the `{{aliasId}}` template var →
  `{{instrumentId}}`, and the in-folder descriptions. JSON round-trip validated. The two
  routing-handle items ("31. Get Account by Alias" `:2712`, "44. List Balances by Account Alias"
  `:3946`) are DO-NOT-TOUCH and confirmed untouched.
- `AGENTS.md` (`:21,47,48,50`), `STRUCTURE.md` (`:47,49,97,111,130-133,177`), `CLAUDE.md`
  (`:13,14`) — `holders/aliases`→`holders/instruments`, `plugin-crm`→`midaz` namespace prose,
  `mongodb/alias`→`mongodb/instrument` (matches the F2-T05 dir rename), `mmodel.Alias`→`Instrument`
  in the model lists. CLAUDE.md `:14` keeps one `plugin-crm` mention as explicit flip-context with
  an X1 pointer to the RBAC doc.

### llms*.txt judgment call (boundary with F5)

Spec: "LEAVE for F5 (served-surface wording) — only fix lines that name the Go entity directly."
Applied that carve-out narrowly. **Fixed** four lines that name renamed **Go identifiers**:

- `llms.txt:47` and `llms-full.txt:108` — `pkg/mmodel` model list `Alias` → `Instrument` (the Go
  domain model; parallel to the STRUCTURE.md:177 fix).
- `llms-full.txt:372,375,380` — the renamed Go error sentinels `ErrAliasNotFound` →
  `ErrInstrumentNotFound`, `ErrHolderHasAliases` → `ErrHolderHasInstruments`,
  `ErrAliasClosingDateBeforeCreation` → `ErrInstrumentClosingDateBeforeCreation` (F2-T10 renames;
  the `CRM-0008/0017/0023` wire codes stay byte-for-byte — DO-NOT-TOUCH). Updated the coupled
  human description token on those three lines so the line is not self-contradictory.

**LEFT for F5** (served-surface wording, explicitly F5-owned): the `/v1/aliases*` route tables
(`llms-full.txt:308-313`), the `plugin-crm` namespace prose (`llms.txt:5,39`; `llms-full.txt:29`),
the "CRM holders/aliases" descriptions (`llms.txt:3,5,24,38,39,40`; `llms-full.txt:3,41,86,87,293`),
and all Account/Balance `Alias` routing-handle refs (`llms-full.txt:146,147,162,170,178,222,261` —
DO-NOT-TOUCH). Coordinate at F5 to avoid double-editing the served-surface lines.

### Verification (F2-T17)

- `grep -rn '/v1/holders/{holder_id}/aliases\|/v1/aliases' docs/api/SCOPING.md postman/...json` → ZERO.
- `grep -n 'plugin-crm' docs/auth/RBAC-NAMESPACES.md` → only flip/X1-migration context.
- postman: 6 raw URLs `/instruments`, all `path[]`+`{{instrumentId}}`+`instrument_id` consistent,
  `aliasId`/`alias_id`/`/aliases` count ZERO in-folder, routing-handle items intact (count 8),
  `python3 json.load` VALID.
- DO-NOT-TOUCH re-proven: 6 numeric sentinels unchanged, 7 `aliases_` Mongo literals unchanged,
  zero `.go` files modified by this task.

---

## HEAD-drift summary (for the orchestrator)

1. **F2-T01 guard grep `'Alias \*string'` is wrong** — gofmt column alignment means the literal pattern
   returns zero. Fixed to `'Alias +\*string'` in the guard block. (Affects only the guard's own
   correctness, not the rename.)
2. **F2-T02 third reader confirmed** — ledger `observability.go:207-208` reads the same 6 CRM-domain
   `QueryHeader` fields; F2-T13 has 3 span-flag reader sites + 1 CRM filter-construction site, not 1.
3. **CRM alias routes/authz at `routes.go:99-104`, not `:93-98`** — F1-T13's `/holders/:id/accounts`
   insertion shifted the block. F2-T11/T12 retarget. `ApplicationName` const still `:21`.
4. **`EntityHolder` already present** (`entity.go:16`, F1-T04) — F2-T08 added only Instrument + RelatedParty.

---

## Commit ledger (chronological, on top of the F1 tip `4fd2bb98d`)

F2 landed in four waves on top of `4fd2bb98d` (the F1 execution-note commit, which is also the diff base for every F2 streaming/lint pre-existence claim).

| SHA | Wave | Tasks closed | What it closed |
|-----|------|--------------|----------------|
| `f30ca3317` | W0 — guards/audit/decision/constants | F2-T01, T02, T03, T08 | The DO-NOT-TOUCH allowlist + executable guard block (entity-scoped, not word-scoped); the Q7 dual-use audit freezing the 6-field instrument rename set and excluding `QueryHeader.Alias`; the `midaz` namespace + `instruments` resource + related-parties-as-sub-resource decision; `EntityInstrument` + `EntityRelatedParty` constants (`EntityHolder` already present from F1-T04, not re-added). Recorded in the wave-0 sections above. |
| `0ba9c30da` | W1 — rename + contracts | F2-T04, T05, T06, T07, T09, T10 | `mmodel.Alias`→`Instrument` (file + type + input + `@name` set); Mongo adapter `alias/`→`instrument/` (model, repo, query, maintenance, mock — collection literals preserved); CRM service files + `AliasRepo` field + span attrs; CRM handler `alias.go`→`instrument.go` (type, methods, swag annotation text); the 14 `reflect.TypeOf` sites replaced with the new constants; CRM error sentinels renamed (`ErrHolderHasAliases`→`ErrHolderHasInstruments`, `ErrAliasNotFound`→`ErrInstrumentNotFound`, `ErrAliasClosingDateBeforeCreation`→`ErrInstrumentClosingDateBeforeCreation`; CRM-00xx wire codes byte-for-byte). |
| `68b26daa2` | W2 — routes hard cut + authz flip | F2-T11, T12 | `/v1/aliases*`→`/v1/instruments*` hard cut (no dual-serve); `:alias_id`→`:instrument_id` end-to-end (route + `ParseUUIDPathParameters` Locals write + handler `GetUUIDFromLocals` read); authz flip — local `const ApplicationName = "midaz"` with X1-contract doc comment, `"aliases"` resource literal removed, `plugin-crm` gone from code. |
| `a6e7516ae` | W2 — QueryHeader fields | F2-T13 | The 6 instrument-domain `QueryHeader` filter fields renamed (`Instrument`-prefixed Go identifiers); the 3 span-flag readers + 1 CRM filter-construction reader + 2 test-literal readers followed through. Wire `?param=` strings preserved (no struct tags; F5 owns wire changes). `QueryHeader.Alias` left intact (dual-use, DO-NOT-TOUCH). |
| `1437c1cef` | W3 — tests + docs | F2-T14, T15, T16, T17 | Real router-registration integration tests assert `/instruments*`; fixture-stub + rename-test bookkeeping (`routes_test.go`, `TestApplicationNameConstant`→`midaz`, entity/error tests, the R43 `TestInstrumentEntityFieldContract`); reporter zero-diff confirmation (collection name unchanged); docs/postman sweep (SCOPING, RBAC-NAMESPACES + X1 section, postman Instruments folder, AGENTS/STRUCTURE/CLAUDE, the narrow llms*.txt Go-identifier carve-out). |
| `fcdbb7633` | T18 straggler | F2-T18 (fix) | One genuine entity-reference straggler: `tests/utils/helpers.go:12` doc-comment example `mmodel.Alias{`→`mmodel.Instrument{}` (1 line, 1 file). Caught by grep set 1; outside every DO-NOT-TOUCH category. **= F2 baseline-capture SHA.** |

Wave boundaries follow the `F2.md` §1 dependency DAG: W0 guards/contracts (allowlist before any sweep) → W1 entity rename + contracts (compile floor) → W2 routes/authz/QueryHeader (the wire/identifier cut) → W3 tests/docs (pins + residue). The T18 straggler is not a new task; it closes a single doc-comment reference the grep gate surfaced.

---

## Baseline (FINAL, captured at `fcdbb7633` with a dedicated `GOCACHE`, `RETRY_ON_FAIL=1` declared)

| Command | Exit | Result |
|---------|------|--------|
| `make test-unit` | 0 | 15,914 tests, 6 skipped |
| `make test-integration` | 0 | 983 tests, 80 skipped (**`RETRY_ON_FAIL=1` declared** — one flake absorbed by the declared retry) |
| `make test-property` | 0 | 70 tests, 7 skipped |
| `make test-reporter-chaos` | 0 | 39 tests, 39 skipped (`CHAOS=1` opt-in by design) |
| `make ci` | 0 | single exit code; all four legs reproduced |

`make test-unit` + `make test-integration` are the macro-Gate-1 mandatory floor; `make ci` is the single-verdict superset. Counts are flat vs the F1 tip (15,914 / 983 / 70 / 39) — F2 is a rename + namespace cut, so test *counts* do not move; what moves is what the same tests assert (`/instruments*` paths, `midaz` namespace, `Instrument` entity value, the new `TestInstrumentEntityFieldContract`). The 6 unit skips and 7 property skips are pre-existing/benign (Balance `DeletedAt` round-trip known-gap, fuzz seeds with non-JSON input).

---

## Environment disclosure (recorded honestly)

The integration leg saw **one flake** during the capture, absorbed by the declared `RETRY_ON_FAIL=1`. Same class as F1: the docker.sock inspect-deadline on macOS Docker Desktop under sustained sequential testcontainers load — the daemon's inspect API wedges at a random position in the matrix while containers start fine; there were zero assertion failures. The declared retry discharged it within the same run, so this baseline did not require the multi-run dance F1 needed. Linux CI runners remain the authoritative environment for this matrix; the declared-retry green plus zero assertion failures is the binding signal. This is the same flake family the F0/F1 notes flagged, discharged the same way.

---

## F2-T18 — phase-gate record (the closing aggregation)

T18 is the cross-cutting greps + full static + unit run. Integration baseline is the main-session job (run above); T18 itself did not run testcontainers and made no commit beyond the straggler fix.

### Grep-zero sets (all 5 clean)

| # | Set | Result |
|---|-----|--------|
| 1 | `mmodel.Alias\b` / `CreateAliasInput` / `UpdateAliasInput` (excl. `crm/api`) | ZERO after the one straggler fix (`tests/utils/helpers.go:12`). `pkg/mmodel/instrument.go` present, `alias.go` gone. |
| 2 | routes `/v1/aliases` / `/v1/holders/:holder_id/aliases` (CRM + ledger, excl. `crm/api`) | ZERO. `plugin-crm` literal (`components/` + `pkg/`, excl. `crm/api`) ZERO. |
| 3 | `@name AliasResponse` / `CreateAliasRequest` / `UpdateAliasRequest` (excl. `crm/api`) | ZERO. |
| 4 | `reflect.TypeOf(mmodel.Alias` / `reflect.TypeOf(mmodel.RelatedParty` (production) | ZERO. `EntityInstrument` + `EntityRelatedParty` production usages = **exactly 14** (7 instrument-mongo/service + 7 related-party). |
| 5 | `plugin-crm` | covered by set 2 — ZERO. |

### DO-NOT-TOUCH survival (all guards pass)

- **6 numeric sentinels** unchanged at `errors.go:34,77,99,110,132,137`.
- **Routing-handle code** unchanged: `resolveByAliases` / `GetAccountByAlias` / `target.Aliases` in `account_resolver.go:88,116,117,137`.
- **`QueryHeader.Alias *string`** present at `httputils.go:70` (verified via `grep -n Alias`; the literal `'Alias \*string'` pattern false-alarms on gofmt tab-alignment — see drift).
- **7 `aliases_` collection literals** byte-for-byte in the renamed `instrument/` dir (`instrument.mongodb.go:108,188,245,348` + `instrument_query.mongodb.go:45,211` + `instrument_maintenance.mongodb.go:48`).

### Static + suite

- `go build ./...` → exit 0.
- `go vet -tags=integration ./components/... ./pkg/... ./tests/...` → exit 0, no output.
- `go vet -tags=chaos ./components/...` → exit 0, no output.
- `make test-unit` → exit 0 (15,914 / 6 skip).
- **Streaming zero-diff:** `git diff 4fd2bb98d..HEAD --stat -- pkg/streaming` → **empty** (re-verified at the F2 tip for this note); all `*_JSONShape` tests green in the unit run.

### Lint — one pre-existing finding (exit 1, NOT an F2 regression)

`make lint` → exit 1 with **exactly one** issue: `gocyclo` on `FindOrListAllWithOperations` at `components/ledger/internal/adapters/postgres/transaction/transaction.postgresql.go:1406`, complexity **19 > 18**.

**Proven pre-existing:** the file has **zero commits in the F2 range** (`4fd2bb98d..HEAD`); the function is **byte-identical at the F2 base `4fd2bb98d:1406`**; it was last touched by `da3ee2583`, which predates F2. This is a *different* finding than the spec-anticipated ANTLR `pkg/gold/parser` unreachable-code noise, but the same posture: not introduced by F2. Candidate fix lands in **F3** (which owns that file's transaction-read seam) or **F5** (harness/quality debt). Flagged explicitly because `make lint` returns exit 1, not 0 — the F2 gate treats it as PASS-equivalent only because pre-existence is proven.

---

## HEAD-drift log (distilled from the wave agents; all confirmed at HEAD, none changed F2 semantics)

The wave-0 drift notes (guard grep, third reader, route-block shift, `EntityHolder` already present) are recorded in the sections above. The execution-time drift the rename surfaced:

1. **T05 fan-out wider than the spec's `From/ToEntity` touchpoint.** The spec named only `FromEntity`/`ToEntity` for the `mmodel.Alias`→`Instrument` adapter mapping, but `mmodel.Alias` **ceased to exist after T04**, so HEAD-compile forced flipping **all** `mmodel.Alias` refs in the adapter (interface, `Create`/`Find`/`Update`/`FindAll`, mock, integration tests). HEAD-compile reality wins; collection literals stayed untouched.
2. **`tests/utils/mongodb/fixtures.go` outside the spec inventory.** It references `mmodel.Alias` and carries alias fixture helpers (`AliasParams`/`CreateTestAlias*`) consumed only by the moved integration test. Flipped `mmodel.Alias`→`Instrument` (compile-required) and renamed the fixture helper surface + its single caller for internal consistency.
3. **Residual `Alias`-named test symbols deliberately KEPT.** Test function names (`TestUpdateAliasByID`, `TestIntegration_AliasRepo_*`, `TestGetAllAliases`), the unexported `buildAliasFilter` helper, and local vars (`createdAlias`/`alias`) were **not** renamed. They sit outside all 5 grep-zero gate sets, and the F2 rename is **entity-scoped, not word-scoped** — renaming them would be a word-sweep the allowlist explicitly forbids. They compile and pass.
4. **`crm/api/` generated swagger left stale by design.** `docs.go` still says `Aliases`/`AliasResponse`; F5 regenerates (the `make generate-docs` CRM leg is broken at HEAD, so F2 cannot regenerate it). Excluded from every grep set per R10.
5. **The `cn.` import-alias false alarm on the 14-site count.** An initial `constant.EntityInstrument` grep returned 1, not 14 — the CRM code imports `pkg/constant` under the `cn.` alias, so most sites read `cn.EntityInstrument`/`cn.EntityRelatedParty`. The broader `EntityInstrument|EntityRelatedParty` grep confirms exactly 14. Gate 4 satisfied.
6. **Wire `?param=` names preserved.** `QueryHeader` carries no struct tags, so query-param binding lives in the lib-commons header parser. F2-T13 is a pure Go-identifier rename + reader follow-through; the wire query-param strings clients send stay current. **F5 owns any wire-contract change.**

The full per-task decision/drift log lives in the workflow result; the above are the items a later phase must not relitigate.

---

## X1 register — policy migration (Fred-owned, release/deploy gate, NOT a merge gate)

The in-code namespace flip (`plugin-crm`→`midaz`, `aliases`→`instruments`) **orphans every tenant's `plugin-crm:*` grant** in the external auth-server, because tenant-manager RBAC policies key on the literal namespace string (`docs/auth/RBAC-NAMESPACES.md:8-11`). The flip is merged in F2; the **policy migration is X1 — owned by Fred + the plugin-auth team, gated at v4 release/deploy, not at F2 merge.** Local/dev with auth disabled are unaffected.

Target grant matrix tenants migrate TO:

```
plugin-crm:holders:{get,post,patch,delete}   →  midaz:holders:{get,post,patch,delete}      (resource unchanged)
plugin-crm:aliases:{get,post,patch,delete}   →  midaz:instruments:{get,post,patch,delete}  (resource renamed)
   (related-parties DELETE)                   →  midaz:instruments:delete                    (sub-resource, Q8)
```

Namespace `midaz`; resources `holders` (unchanged) + `instruments` (replaces `aliases`); related-parties authorize **under** `instruments` (verb `delete`), not as their own resource — one fewer resource in the matrix. `docs/auth/RBAC-NAMESPACES.md` carries the migration note (the dedicated "X1 — policy migration" section added by F2-T17), and R9 is marked closed for CRM by the flip (`plugin-fees` stays distinct by design).

---

## Gate-closure walk (`F2.md` §3, all 10 exit gates)

| Gate (§6) | Closing task(s) | Where the proof lives |
|-----------|-----------------|-----------------------|
| **1. Go grep-zero** (`mmodel.Alias*` zero; file renamed) | T04 (rename), T05/T06/T07 (fan-out), T18 (grep) | Grep set 1 ZERO after the `fcdbb7633` straggler fix; `pkg/mmodel/instrument.go` present, `alias.go` gone. |
| **2. Route + authz grep-zero** (`/aliases`, `plugin-crm`, `"aliases"` zero) | T11 (routes), T12 (authz), T18 (grep) | Grep set 2 ZERO; `const ApplicationName = "midaz"` in CRM `routes.go`; routes block at `:104-109` post-cut. |
| **3. `@name` grep-zero** | T04 (`@name` tags), T07 (handler annotations), T18 (grep) | Grep set 3 ZERO (excl. `crm/api`). |
| **4. Entity-name contract** (14 reflect sites + 2 constants) | T08 (constants), T09 (substitution) | Grep set 4: `reflect.TypeOf(mmodel.Alias/RelatedParty` production ZERO; `EntityInstrument`+`EntityRelatedParty` used at exactly 14 production sites; constants at `entity.go:17,23`. |
| **5. Mongo accessibility incl. MT** | T05 (rename + literal preservation; integration/tenant suites) | 7 `aliases_` collection literals byte-for-byte in `instrument/`; CRM rename integration suite green inside the 983 (`TestInstrumentHandler_*`, `TestIntegration_*Repo`). |
| **6. Streaming unchanged** | (no rename task by design; T18 backstop) | `git diff 4fd2bb98d..HEAD -- pkg/streaming` empty; all `*_JSONShape` tests green. Only `Account.Alias`/`Balance.Alias` (DO-NOT-TOUCH) live in streaming. |
| **7. Reporter unbroken (zero diff)** | T16 | `get-data-source-details-by-id.go` `"aliases"` switch sites byte-identical at F1 base and HEAD; file diff empty (spec mislabeled the `"holders"` case as a third aliases site — drift, code unaffected). |
| **8. Route/integration tests green** (real pins vs fixtures) | T14 (real router pins), T15 (fixture stubs + entity tests) | Integration router-pin tests assert `/instruments*`; `routes_test.go` fixture `/v1/instruments`; `TestApplicationNameConstant`→`midaz`; `TestInstrumentEntityFieldContract` locks the R43 entity flip at the typed `pkg.ValidateBusinessError(...Instrument...)` layer (the CRM error envelope strips `entityType` off the wire — HEAD won over the spec's body-assertion premise). |
| **9. Full suite + lint** | T18 (also T13's `go build ./...` backstop) | Baseline table above (unit/integration/property/chaos + `make ci`, all exit 0); lint exit 1 from the single pre-existing gocyclo finding, pre-existence proven. |
| **10. Old-name residue confined** | T17 (docs/postman), T18 (grep-zero excl. migration-guide/CHANGELOG/`crm/api`) | SCOPING/RBAC/postman/AGENTS/STRUCTURE/CLAUDE swept to instruments + `midaz`; X1 section added to RBAC doc; narrow llms*.txt Go-identifier carve-out applied, served-surface wording left to F5. |

No gate is left without a closing task and a located proof. Gate 6 has no rename task by design; T18's full suite is its backstop.
