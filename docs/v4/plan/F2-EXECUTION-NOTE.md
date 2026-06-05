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
