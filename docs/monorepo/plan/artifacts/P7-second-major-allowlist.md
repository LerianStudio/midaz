# P7 Second-Major Allowlist — `go-playground/validator` v9

Records the P7-T07a disposition for the pre-existing second-major case of
`go-playground/validator` that coexists with `validator/v10` in the unified
`go.mod`. T07's single-version assertion treats any unexpected second-major path
as a defect; the documented multi-major allowlist enumerates the legal exceptions.

- Phase: P7 (unified go.mod convergence)
- Branch: `phase/p7-gomod-unification`
- Date recorded: 2026-06-03
- Sign-off: Fred Amaral (owner)

---

## Documented multi-major allowlist (T07)

The unified module resolves exactly ONE version per same-major module path. The
ONLY legal multi-major coexistences in the resolved graph are:

| Module family | Resolved majors | Disposition |
|---|---|---|
| `go.mongodb.org/mongo-driver` | `v1.17.9` + `/v2 v2.6.0` | Path-distinct coexistence (out of scope to collapse — P7-T05). |
| `go-playground/validator` | v9 era + `/v10 v10.30.3` | **This document.** First-party legacy import; KEEP, flag for migration. |

`lib-commons` resolves to exactly one `/v5 v5.4.1` (zero `/v4`); `lib-observability`
to exactly one `v1.0.1`; no `replace` edges anywhere. See T07 evidence.

---

## P7-T07a — `validator/v9` disposition

### `go mod why` evidence (recorded verbatim)

```
$ go mod why github.com/go-playground/validator
# github.com/go-playground/validator
(main module does not need package github.com/go-playground/validator)

$ go mod why gopkg.in/go-playground/validator.v9
# gopkg.in/go-playground/validator.v9
github.com/LerianStudio/midaz/v3/pkg/net/http
gopkg.in/go-playground/validator.v9
```

### Resolved graph (three distinct module paths, each single-version)

```
github.com/go-playground/validator       v9.31.0+incompatible   (indirect)
github.com/go-playground/validator/v10    v10.30.3              (direct)
gopkg.in/go-playground/validator.v9       v9.31.0               (indirect)
```

### Finding — this is FIRST-PARTY, not transitive-only

The v9 era is pulled by **first-party midaz code**, not a transitive dependency.
A single file imports both v9 module paths:

`pkg/net/http/withBody.go`
- line 28: `"gopkg.in/go-playground/validator.v9"` — the validator engine
  (`validator.New()`, `validator.Validate`, `validator.FieldLevel`,
  `validator.ValidationErrors` used throughout the file's custom validators and
  translation wiring).
- line 24: `en2 "github.com/go-playground/validator/translations/en"` — the
  `+incompatible` module's translations subpackage. This is why
  `go mod why github.com/go-playground/validator` reports "main module does not
  need package" for the bare path while the module still appears in the graph:
  the live first-party import is the `/translations/en` subpackage, and the gopkg
  alias path resolves the same v9.31.0 release.

`go mod tidy` does NOT drop either v9 line (verified: tidy idempotent, both lines
survive) precisely because there is a live first-party importer.

### Disposition: KEEP (allowlisted), flag for migration to v10

1. **KEEP / allowlist.** `validator/v9` is added to T07's documented multi-major
   allowlist (table above). It is path-distinct from `validator/v10` and resolves
   to a single version. It is **pre-existing** — it predates every monorepo move
   (it was in the root `go.mod` before P4/P5/P6), so it is NOT a consolidation
   regression introduced by P7. Build, vet, and unit tests are all green with it
   present.

2. **NO `replace` directive.** Forcing a single major via `replace` would be a
   PD-1 shim and is explicitly forbidden by T07a. None introduced.

3. **Flagged for migration (NOT done in P7).** Because the v9 import is
   first-party (not a transitive leaf), it is a real migration candidate:
   `pkg/net/http/withBody.go` should move from `gopkg.in/go-playground/validator.v9`
   + `github.com/go-playground/validator/translations/en` to
   `github.com/go-playground/validator/v10` + `.../v10/translations/en`. This is
   an app-code rewrite (the custom validator registration, translator wiring, and
   `ValidationErrors` handling all need to move to the v10 API surface) and is
   OUT OF SCOPE for P7's go.mod-convergence task. Logged here as the follow-up so
   the second major can eventually be eliminated rather than carried indefinitely.

### Why not "transitive-only" allowlist wording

The orchestrator's default allowlist template assumed v9 would be transitive-only.
It is not — `go mod why` proves a first-party importer. The honest disposition is
"first-party legacy, KEEP-and-flag", not "transitive leaf, acceptable". Recording
the distinction so the migration follow-up is not lost.
