# R1 — lib-commons Huma wrapper: adoption mechanics (READ-ONLY recon)

Scope: mechanics of `lib-commons/v5 @ v5.8.0` `commons/net/http/{problem,openapi}` for planning a
swaggo→Huma migration of the midaz server (`components/ledger` + `components/tracer`, both Fiber v2).
Nothing modified. All paths absolute; line numbers cited.

Module cache root (GOMODCACHE = `/Users/fredamaral/go/pkg/mod`):
`.../github.com/!lerian!studio/lib-commons/v5@v5.8.0/` (`!lerian!studio` = case-escaped `LerianStudio`).

---

## TL;DR verdicts

1. **Fiber version: NO forced upgrade.** The wrapper's `openapi.New` calls `humafiber.NewV2WithGroup`,
   defined in `humafiber_v2.go` importing `github.com/gofiber/fiber/v2`. Midaz is already on
   `fiber/v2 v2.52.13` — the *exact same version* lib-commons pins. `fiber/v3 v3.2.0` enters the
   graph only as an **indirect** dep (the `humafiber` package physically contains both a v2 and a v3
   file); v3 code never compiles into the binary. Fiber v2→v3 is NOT required to adopt the wrapper.
2. **Ring skill: EXISTS** — `ring:adopting-lib-commons-huma-wrapper`, present in every cached ring-dev-team
   version 1.81.2 → 1.85.0. It is a full 11-gate orchestrated migration playbook. Summary in §6.
3. **OpenAPI version: 3.1.0** (huma default; the wrapper does not opt into the 3.0.3 downgrade).
4. **Content-Type: `application/problem+json`** — inherited from `huma.ErrorModel.ContentType`, automatic.

---

## 1. `commons/net/http/problem` — the error model

Three source files. All import `github.com/danielgtaylor/huma/v2` ONLY — no Fiber, no transport
adapter (`problem.go:8-9`, `18`). This is the transport-free half of the wrapper.

### 1.1 `problem.Detail` — the body
File: `.../commons/net/http/problem/problem.go:37-40`

```go
const BaseURI = "https://errors.lerian.studio/v1"   // problem.go:26

type Detail struct {
    huma.ErrorModel        // embedded — see §1.2
    Code string `json:"code,omitempty" doc:"Stable, machine-readable domain error code scoped to the emitting service (format: <SERVICE>-NNNN)." example:"ERR-0001"`
}
```

- `*Detail` satisfies `huma.StatusError` by method promotion from the embedded `ErrorModel`
  (`Error`/`GetStatus`/`ContentType`/`Add`). This is what lets it replace `huma.NewError`.
- `Code` is an RFC 9457 **extension member**; `omitempty` drops it for code-less rails.
- `Type` URI convention: `BaseURI + "/" + code` (e.g. `https://errors.lerian.studio/v1/ERR-0001`),
  set only on the `MapError` coded path (`maperror.go:80-81`).

### 1.2 embedded `huma.ErrorModel` — RFC 9457 members + Content-Type
File: `.../danielgtaylor/huma/v2@v2.38.0/error.go:70-92`

```go
type ErrorModel struct {
    Type     string             `json:"type,omitempty" format:"uri" default:"about:blank" ...`
    Title    string             `json:"title,omitempty" ...`
    Status   int                `json:"status,omitempty" ...`
    Detail   string             `json:"detail,omitempty" ...`
    Instance string             `json:"instance,omitempty" format:"uri" ...`
    Errors   []*ErrorDetail     `json:"errors,omitempty" ...`   // per-field list
}
```
`ErrorDetail` = `{message, location, value}` (error.go:17-31), each `omitempty`.

**Content-Type emission** — `error.go:127-135`:
```go
func (e *ErrorModel) ContentType(ct string) string {
    if ct == "application/json" { return "application/problem+json" }
    if ct == "application/cbor" { return "application/problem+cbor" }
    return ct
}
```
So every error body is served as `application/problem+json` automatically — no per-op wiring.

### 1.3 `Install()` — the process-global override (the crux)
File: `.../problem/install.go:50-105`

```go
func Install()                                                 // install.go:50; sync.Once, idempotent
func newError(status int, msg string, errs ...error) huma.StatusError   // install.go:61
const genericServerErrorDetail = "internal error"              // install.go:15
```

- `Install()` sets `huma.NewError = newError` under a `sync.Once` (install.go:22, 51-53).
- **This is how the error model becomes the OpenAPI error response schema**: `huma.NewError` is a
  package var; every error Huma constructs (domain errors via `MapError`, plus the framework's own
  validation/404/422) becomes a `*Detail`. Huma resolves the error-response schema **at
  operation-registration time**, so the generated OpenAPI reflects `Detail` (including the optional
  `code` property) with zero per-operation registration.
- **ORDERING CONSTRAINT (hard):** `Install()` MUST run *before* any `huma.Register/Get/Post` on
  BOTH the runtime API and every spec-gen entrypoint, or the served bodies and the committed spec
  diverge (install.go:34-36; skill Gate 3).
- **Merge semantics** (install.go:38-46, 61-105):
  - `status >= 500`: body scrubbed to static `"internal error"`, NO `errs` folded. Central info-leak
    guard — even `huma.Error500(rawErr.Error())` cannot leak a cause (install.go:62-70).
  - `status < 500`: `msg` passthrough, `errs` folded into `Errors[]` (skip nil, honor
    `huma.ErrorDetailer`) — native 422 field lists preserved (install.go:72-104).
  - Framework errors keep `Code` empty and `Type` at `about:blank`.

### 1.4 `MapError` — the per-rail flex seam
File: `.../problem/maperror.go:34-65` (+ `newProblem` helper `:70-85`)

```go
func MapError(
    err error,
    codeOf   func(error) (code, msg string, ok bool),
    statusOf func(code string) int,
    fallbackCode string,
) error
```
- Each service injects its own taxonomy: `codeOf` extracts `(code, msg, ok)` from a domain error;
  `statusOf` maps code→HTTP status; `fallbackCode` is the "unknown" code.
- Nil-safe: `err==nil` OR nil callback OR `ok==false` → sanitized 500 carrying `fallbackCode`
  (maperror.go:40-47). A recognized error whose `statusOf` returns `<400` is clamped to 500
  (maperror.go:55-57). `>=500` detail sanitized (maperror.go:60-62).
- Returns a concrete `*Detail` directly — **independent of whether `Install` ran** (maperror.go:33,
  64). So `MapError` + `Detail` can be adopted as a plain response body *without* the Huma transport.
- **This is where midaz's ~394 `0NNN` codes + the `WithError` code→status table get transcribed
  verbatim** (see P1a §3). Money-path: preserve every code value and every code→status decision.

---

## 2. `commons/net/http/openapi` — the Fiber↔Huma adapter

File: `.../commons/net/http/openapi/openapi.go`. Imports (openapi.go:19-31):
`lib-observability/log`, `danielgtaylor/huma/v2`, `danielgtaylor/huma/v2/adapters/humafiber`,
`gofiber/fiber/v2`. **Deliberately does NOT import `problem`** — error policy is the bootstrap's job
(openapi.go:7-16). Applies no error policy.

### 2.1 Operation registration pattern
The wrapper does NOT wrap `huma.Register`; callers use huma directly. Standard huma v2 style:
```go
huma.Register(api, huma.Operation{...}, handler)   // huma.go:711
// func Register[I, O any](api API, op Operation, handler func(context.Context, *I) (*O, error))
```
- Request/response bodies, path/query/header params, and per-op security are declared as **Go struct
  tags** on the `I`/`O` types + fields on `huma.Operation` — NOT swaggo `// @` comments. This is the
  swaggo→Huma delta: every `// @Param`/`// @Success`/`// @Router` annotation becomes typed input/output
  structs (`Body`, `path:"..."`, `query:"..."`, `header:"..."`) and `huma.Operation{Method, Path,
  Tags, Security, ...}` (Operation struct at openapi.go:834).
- **Struct-tag caveat (from the skill, HIGH severity):** `doc:`/`example:` tags are pure docs (safe).
  But `minLength`/`maxLength`/`minItems`/`maxItems`/`minimum`/`maximum`/`pattern`/`enum` are
  **enforced validation** — Huma rejects a violating request with 422 *before the handler runs*.
  Adding one to "match the runtime limit" silently moves rejection off the handler's coded path. Treat
  as behavioral, not doc-only.

### 2.2 `New` — build the API
File: `openapi.go:75-111`
```go
type Config struct { Title, Version, Description string; Servers []string }   // openapi.go:58-67
func New(app *fiber.App, group fiber.Router, cfg Config) huma.API             // openapi.go:75
```
- Starts from `huma.DefaultConfig`, sets Info, then **nils `Transformers` (strips the
  `$schema`-injecting SchemaLinkTransformer that bypasses custom marshalers), `OnAddOperation`,
  `CreateHooks`, `SchemasPath`** (openapi.go:79-87).
- **Clears `OpenAPIPath` + `DocsPath`** (openapi.go:98-99) to suppress humafiber's un-gated auto-mount
  of `/openapi.json`/`/openapi.yaml`/`/docs` — a latent prod exposure. `api.OpenAPI()` stays populated.
- Binds via `return humafiber.NewV2WithGroup(app, group, humaConfig)` (openapi.go:110) — **the Fiber
  v2 constructor**, see §3.
- `New` registers NO operations; correct order is `New` → `problem.Install()` → register ops.

### 2.3 `ServeSpec` — explicit, gated spec serving
File: `openapi.go:150-192`
```go
func ServeSpec(app *fiber.App, api huma.API, logger libLog.Logger, prefix, title string)
```
- Mounts `{prefix}/openapi.json`, `{prefix}/openapi.yaml`, `{prefix}/docs` (Scalar UI from jsdelivr
  CDN via a relaxed per-route CSP, openapi.go:33-37, 197-202). Spec bytes snapshotted once.
- Normalizes prefix (leading slash, no trailing), HTML-escapes title + spec URL (openapi.go:175, 208-210).
- Nil-safe; render failure logs + skips (openapi.go:151-169). **Caller MUST gate on `Swagger.Enabled`.**
- Serves `api.OpenAPI().YAML()` / `json.Marshal(api.OpenAPI())` with NO downgrade call → **OAS 3.1.0**.

### 2.4 OpenAPI version emitted: **3.1.0**
huma DefaultConfig emits `openapi: "3.1.0"`. The `3.0.3` value exists only inside `downgradeSpec`
(`.../huma/v2@v2.38.0/openapi.go:1593-1596`), reachable only via an explicit `YAMLDowngrade`-style
call. `ServeSpec` uses plain `.YAML()`/marshal, so the wrapper emits **3.1.0**. (swaggo emits Swagger
2.0 today — this is a spec-version bump for any consumer/tooling reading the spec.)

---

## 3. THE FIBER-VERSION BLOCKER — verdict: NOT a blocker

**Adopting the wrapper does NOT force fiber v2→v3.** Evidence:

| Fact | Source | Value |
|---|---|---|
| Wrapper imports fiber | `openapi.go:30` | `github.com/gofiber/fiber/v2` |
| Wrapper's constructor | `openapi.go:110` | `humafiber.NewV2WithGroup(app, group, cfg)` |
| That constructor's file | `.../huma/v2@v2.38.0/adapters/humafiber/humafiber_v2.go:15` | imports `fiberV2 "github.com/gofiber/fiber/v2"` |
| Its signature | `humafiber_v2.go:246` | `func NewV2WithGroup(r *fiberV2.App, g fiberV2.Router, config huma.Config) huma.API` |
| lib-commons go.mod fiber v2 | `.../lib-commons/v5@v5.8.0/go.mod` | `gofiber/fiber/v2 v2.52.13` (direct) |
| lib-commons go.mod fiber v3 | same | `gofiber/fiber/v3 v3.2.0 // indirect` |
| Midaz worktree fiber | `.../midaz-consolidation/go.mod` | `gofiber/fiber/v2 v2.52.13` (**identical version**) |
| Midaz go version | same | `go 1.26.4` (lib-commons needs 1.26.3 — OK) |

**Why v3 appears at all (and why it's benign):** the `humafiber` package ships two files —
`humafiber.go` (imports `fiber/v3`, `humafiber.go:15`) and `humafiber_v2.go` (imports `fiber/v2`).
Go compiles the whole package, so importing it for the v2 funcs pulls `fiber/v3` into the module
graph as indirect. Only the v2 code path compiles into the binary; v3 is unreachable at runtime.
The Ring skill states this explicitly (SKILL.md:56): "`humafiber` drags `gofiber/fiber/v3` as an
indirect dep — this is EXPECTED, not a bug. … `govulncheck` stays quiet (v3 unreachable);
Trivy/SBOM may flag *future* fiber/v3 CVEs — a waivable false positive, NOT a reason to avoid the
wrapper." Midaz is already on the exact fiber v2 version lib-commons uses, so there is zero fiber
churn on the runtime path.

Residual risk (MEDIUM, not a blocker): SBOM/Trivy noise from the indirect fiber/v3; waive it.

---

## 4. Security schemes (Bearer JWT + API key)

### 4.1 Bearer JWT — helper provided
File: `openapi.go:119-135`
```go
func DeclareBearerAuth(api huma.API)   // registers components.securitySchemes["BearerAuth"]
```
emits exactly:
```go
&huma.SecurityScheme{ Type: "http", Scheme: "bearer", BearerFormat: "JWT",
                      Description: "JWT bearer token issued by the identity provider." }
```
(openapi.go:129-134). Idempotent, nil-safe. **`type: http, scheme: bearer` — the proper shape both
planes emit identically by calling this one helper.**

**Critical gotcha (skill HIGH, SKILL.md:35-39, 140-145):** `DeclareBearerAuth` registers the scheme
*component* only — it advertises ZERO secured operations by itself. You MUST also attach a requirement:
- Global default (one line, drift-safe): `api.OpenAPI().Security = []map[string][]string{{"BearerAuth": {}}}`
- Or per-op `Security` on each `huma.Operation`.
- Genuinely-public ops must override with `Security: []map[string][]string{}` (non-nil empty →
  Huma renders `security: []`; a nil `omitempty` would silently re-inherit the global default).

### 4.2 API key — NO helper; declare inline
There is no `DeclareApiKeyAuth` in the wrapper. Declare it the same way `DeclareBearerAuth` does,
using the huma `SecurityScheme` struct (`.../huma/v2@v2.38.0/openapi.go:1201+`, fields `Type`,
`Name`, `In`, `Scheme`, `Description`):
```go
api.OpenAPI().Components.SecuritySchemes["ApiKeyAuth"] =
    &huma.SecurityScheme{ Type: "apiKey", In: "header", Name: "X-API-Key",
                          Description: "..." }
```
Both planes emit identical `securitySchemes` by sharing this declaration (a small local helper, or
copy the 6-line `DeclareBearerAuth` shape). Same requirement-attachment rules apply.

Note vs prior memory: the SDK-v4 remodel memo says tracer auth is Bearer, not X-API-Key — so the
API-key scheme may only be needed on the ledger plane, if at all. Confirm per plane during planning.

---

## 5. Highest-risk mechanics of the migration (3–5)

1. **`problem.Install()` on BOTH paths, before every `Register`** (CRITICAL). Wiring it on only the
   runtime path (not the spec-gen builder) or after operation registration silently diverges served
   bodies from the committed spec. It's a process-global var swap resolved at registration time.
2. **Money-path code table transcription** (CRITICAL, third-rail). The ~394 `0NNN` codes and the
   `WithError` code→status arms (incl. the `libCommons.Response` 422/404/500 money-path arm) are
   money-path semantics. `MapError`'s `codeOf`/`statusOf` must reproduce them byte-for-byte. Guard
   with a golden-file test over the code→status map before/after. This is a rewire, never a redesign.
3. **swaggo→Huma re-annotation is the real budget line** (HIGH effort). Every `// @Param/@Success/
   @Router` becomes typed input/output structs + `huma.Operation`. Landmine: schema-constraint tags
   (`minLength`/`maxLength`/`enum`/`pattern`…) are *enforced validation* (422 before handler), not
   docs — adding them changes the error contract. Only `doc:`/`example:` are doc-only.
4. **Bearer/API-key: scheme ≠ requirement** (HIGH). `DeclareBearerAuth` alone makes the spec advertise
   every op as public while runtime enforces JWT — the spec lies. Attach a global/per-op requirement
   AND give genuinely-public routes an explicit empty `Security: []map[string][]string{}` override.
5. **OAS version + client contract break** (HIGH, cross-repo). Spec jumps Swagger 2.0 → OpenAPI 3.1.0,
   and error bodies change from midaz's `{entityType,title,message,code}` to RFC 9457
   `{type,title,status,detail,code,instance?,errors?}` (Content-Type `application/problem+json`).
   Breaking for the Go SDK (this repo's parent) and any consumer parsing `{message}`/`entityType`.
   Needs SDK-side alignment + version bump. `MapError`+`Detail` can be adopted as a plain body first
   (no Huma transport, since `MapError` returns a concrete `*Detail`) to stage the shape change
   ahead of the swaggo→Huma transport swap.

---

## 6. Ring skill `ring:adopting-lib-commons-huma-wrapper` — EXISTS

Located at (every cached ring-dev-team version 1.81.2–1.85.0):
`/Users/fredamaral/.claude/plugins/cache/ring/ring-dev-team/1.85.0/skills/adopting-lib-commons-huma-wrapper/SKILL.md`
(202 lines). It is an orchestrated, gated migration playbook. Key prescriptions:

- **Two-package architecture**, minimum lib-commons **v5.6.0** (we have v5.8.0 — OK). huma version
  MUST match the lib build (`v2.38.0` — matches our cache). fiber/v3 indirect is expected (§3).
- **Orchestration model:** the skill orchestrates; all Go edits go through `Task(ring:backend-go)`;
  TDD (RED→GREEN→REFACTOR) mandatory; orchestrator owns git and never commits without go-ahead.
- **11 gates** (SKILL.md:71-89): 0 stack-detect+audit → 1 codebase analysis (`ring:codebase-explorer`)
  → 1.5 preview (`ring:visualizing`) → 2 version/huma alignment → 3 `New`+`Install` on BOTH paths →
  4 per-rail `MapError` → 5 `ServeSpec` gated + bearer (scheme+requirement+public overrides) → 5.5
  spec richness (`doc:`/`example:`, contact/license/tags) → 6 delete local wrapper (migration mode) →
  7 regenerate spec + prove rename-only → 8 tests (build + `-race` + spec-drift gate) → 9 code review
  → 10 user validation → 11 release/pin discipline if wrapper unreleased.
- **Mode detection:** migration mode (local wrapper/hand-rolled `huma.NewError` exists → Gate 6 runs)
  vs greenfield (swaggo-only or no Huma → Gate 6 skipped). Midaz = **migration-from-swaggo / greenfield-Huma**
  (swaggo present, no existing Huma, no local huma wrapper → Gate 6 skipped, Gate 3 builds fresh).
- **Test via `app.Test`, not `humatest`** (humatest diverges on `Unwrap`, SKILL.md:57).
- **Highest-severity defects** it enforces (SKILL.md:189-192) map 1:1 to §5 above: `Install` on one
  path only, surviving hand-rolled override, 5xx cause leak, unwired `MapError`, scheme-without-
  requirement, doc-gesture constraint tags.

Directly relevant to planning: it prescribes exactly the staged shape this recon recommends and names
the money-path preservation ("preserve the service's existing per-rail code taxonomy verbatim").
