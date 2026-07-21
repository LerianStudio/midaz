# R3 — Money-Path Swap Spec: `WithError` → lib-commons `problem.MapError`

**Read-only design.** No code changed. THIRD RAIL: money path. Goal: every error
`Code` string and its HTTP status survive **byte-for-byte** across the swap to
lib-commons `problem.Detail` (RFC 9457). This doc is the plan's task source: the
verbatim type→status table (§1), the field-by-field swap (§2), the `pkg.HTTPError`
fold (§3), the unified struct + tag policy (§4), and the golden-test design (§5).

Worktree: `.../scratchpad/midaz-consolidation`, module `github.com/LerianStudio/midaz/v4`.

Every current-code reference is cited `file:line`.

---

## 0. The two current status-producing paths (both must be preserved)

The swap has to reproduce **two** dispatchers, not one:

1. **`WithError`** — `pkg/net/http/errors.go:26-111`. The main dispatcher: `errors.As`
   in declaration order, first match wins. Delegates to the `response.go` helpers
   (`pkg/net/http/response.go`) that emit `fiber.Map{code,title,message}` at a fixed
   status per helper.
2. **`CanonicalFiberErrorHandler` + `renderCanonical`** — `pkg/net/http/fiber_error_handler.go:28-75`.
   The Fiber `ErrorHandler` for errors escaping the handler chain. It maps
   `*fiber.Error` classes to canonical errors and — critically — emits **405** and
   **413** at an *explicit* status via `renderCanonical` (`fiber_error_handler.go:44,46,57`),
   statuses the `WithError` table itself never produces.

The `MapError` swap replaces path 1's status table. Path 2's two explicit-status
arms are the only **ambiguous codes** in the whole surface (see §1.3) and must be
carried over as an explicit-status override, not through the code→status table.

Serializer today: `c.Status(n).JSON(fiber.Map{...})` — usually `{code,title,message}`
only; `entityType`/`fields` reach the wire only on the two field-bearing 400 paths
(`response.go:34` `BadRequest` receiving a `ValidationKnownFieldsError`/
`ValidationUnknownFieldsError`) and on the `ResponseError` path (`response.go:123-127`
`JSONResponseError`, which serializes the whole struct).

---

## 1. Type→status table (extracted VERBATIM — the golden fixture)

Source: `WithError` (`pkg/net/http/errors.go:26-111`) joined with the `response.go`
helper each arm calls. Status is a pure function of the **Go error type** chosen in
`ValidateBusinessError` (`pkg/errors.go:391`), never of the numeric code
(`p1a-ledger-errors.md §5.1`). `statusOf(code)` in the new world must therefore first
re-derive "which type does this code map to", then apply this table.

### 1.1 Primary table — `WithError` arms

| # | Typed error (`pkg.*`) | `errors.go` arm | Helper called (`response.go`) | HTTP status |
|---|------------------------|-----------------|-------------------------------|-------------|
| 1 | `EntityNotFoundError` | :27-30 | `NotFound` :69 | **404** |
| 2 | `EntityConflictError` | :32-35 | `Conflict` :78 | **409** |
| 3 | `ValidationError` | :37-45 | `BadRequest` :34 (repacked as `ValidationKnownFieldsError{Fields:nil}`) | **400** |
| 4 | `UnprocessableOperationError` | :47-50 | `UnprocessableEntity` :96 | **422** |
| 5 | `UnauthorizedError` | :52-55 | `Unauthorized` :16 | **401** |
| 6 | `ForbiddenError` | :57-60 | `Forbidden` :25 | **403** |
| 7 | `ValidationKnownFieldsError` | :62-65 | `BadRequest` :34 | **400** (emits `fields`) |
| 8 | `ValidationUnknownFieldsError` | :67-70 | `BadRequest` :34 | **400** (emits `fields`) |
| 9 | `ResponseError` | :72-75 | `JSONResponseError` :123 | **`strconv.Atoi(err.Code)`** — status IS the `Code` field |
| 10 | `InternalServerError` | :95-98 | `InternalServerError` :105 | **500** |
| 11 | `FailedPreconditionError` | :100-103 | `InternalServerError` :105 | **500** (NOT 412) |
| 12 | `ServiceUnavailableError` | :105-108 | `ServiceUnavailable` :114 | **503** |
| 13 | fallthrough (no match) | :110 | `ValidateInternalError(err,"")` → `InternalServerError` type → helper :105 | **500** (code `0046`) |

### 1.2 `libCommons.Response` sub-switch (`errors.go:77-93`)

Lib-commons codes (NOT the `constant/errors.go` set). Resolved at arm position
between #8 and #10, so it precedes the internal/precondition/unavailable arms.

| `commonsResponse.Code` == | Helper | HTTP status |
|---------------------------|--------|-------------|
| `libConstants.ErrInsufficientFunds` OR `ErrAccountIneligibility` (:80) | `UnprocessableEntity` | **422** |
| `libConstants.ErrAssetCodeNotFound` (:82) | `NotFound` | **404** |
| `libConstants.ErrOverFlowInt64` (:84) | `InternalServerError` | **500** |
| default (:86) | `BadRequest` (as `ValidationKnownFieldsError`) | **400** |

### 1.3 Explicit-status arms — `renderCanonical` (`fiber_error_handler.go`)

These bypass the `WithError` table with a hard-coded status. **Ambiguous codes** —
the same code carries a *different* status here than its `ValidateBusinessError` type
would give:

| Trigger | Code | `WithError`-table status (via type) | Actual emitted status | Line |
|---------|------|-------------------------------------|-----------------------|------|
| `fiber.StatusMethodNotAllowed` → `ErrMethodNotAllowed` | `0485` | 400 (`ValidationError`) | **405** | `fiber_error_handler.go:44` |
| `fiber.StatusRequestEntityTooLarge` → `ErrPayloadTooLarge` | `0143` | 400 (`ValidationError`) | **413** | `fiber_error_handler.go:46` |
| `fiber.StatusUnauthorized` → `ErrInvalidToken` | `0042` | 401 (`UnauthorizedError`) — consistent | 401 | `:40` |
| `fiber.StatusNotFound` → `ErrRouteNotFound` | `0484` | 404 (`EntityNotFoundError`) — consistent | 404 | `:42` |

**Distinct HTTP statuses across the whole surface: 9** — `400, 401, 403, 404, 405,
409, 413, 422, 500, 503`. (That is 10 values; **405 and 413 are produced only by the
explicit-status path**, never by the `WithError` table. Counting only what `MapError`
must reproduce from the code→status table: **8** — `400, 401, 403, 404, 409, 422, 500,
503`.)

### 1.4 Status distribution across mapped codes (from the inventories)

- **400**: all `ValidationError` + `ValidationKnownFieldsError` + `ValidationUnknownFieldsError` (largest bucket; ledger ~104 tracer arms + fee/mainline validations).
- **422**: all `UnprocessableOperationError` (money-path heavy: overdraft/reservation/fee-application).
- **404**: all `EntityNotFoundError`. **409**: all `EntityConflictError`. **401**: `UnauthorizedError` (0041,0042,0474). **403**: `ForbiddenError` (0043 only).
- **500**: `InternalServerError` + `FailedPreconditionError` + fallthrough (0046). **503**: `ServiceUnavailableError` (readyz/timeout/reservation-unavailable 0178).
- **status-in-Code**: `ResponseError` only (0094 via `ValidateUnmarshallingError`, `errors.go:320-336`).

---

## 2. The `WithError` → `problem.MapError` swap (field-by-field)

`problem.MapError` signature (`lib-commons/v5@v5.8.0`, `commons/net/http/problem/maperror.go:34`):

```go
func MapError(
    err error,
    codeOf func(error) (code, msg string, ok bool),
    statusOf func(code string) int,
    fallbackCode string,
) error   // returns a concrete *problem.Detail
```

Returns a `*problem.Detail` (`problem/problem.go:37`):

```go
type Detail struct {
    huma.ErrorModel        // Type, Title, Status, Detail, Instance, Errors[]
    Code string `json:"code,omitempty"`
}
```

`huma.ErrorModel` field tags (from `huma/v2/error.go`, confirmed via vendored copy):
`Type json:"type,omitempty"` (default `about:blank`), `Title json:"title,omitempty"`,
`Status json:"status,omitempty"`, `Detail json:"detail,omitempty"`,
`Instance json:"instance,omitempty"`, `Errors []*huma.ErrorDetail json:"errors,omitempty"`.
`huma.ErrorDetail = {Message json:"message"; Location json:"location"; Value json:"value"}`.

### 2.1 `codeOf(err) (code, msg string, ok bool)` — pull `Code`+`Message` off the typed errors

One `errors.As` cascade over the exact same 11 typed structs `WithError` walks, in the
**same declaration order** (order matters only for nested-error edge cases; E2 forbids
nesting — `errors.go:20-25`). Each arm returns `(err.Code, err.Message, true)`:

```go
func codeOf(err error) (string, string, bool) {
    var e1 pkg.EntityNotFoundError;          if errors.As(err, &e1) { return e1.Code, e1.Message, true }
    var e2 pkg.EntityConflictError;          if errors.As(err, &e2) { return e2.Code, e2.Message, true }
    var e3 pkg.ValidationError;              if errors.As(err, &e3) { return e3.Code, e3.Message, true }
    var e4 pkg.UnprocessableOperationError;  if errors.As(err, &e4) { return e4.Code, e4.Message, true }
    var e5 pkg.UnauthorizedError;            if errors.As(err, &e5) { return e5.Code, e5.Message, true }
    var e6 pkg.ForbiddenError;               if errors.As(err, &e6) { return e6.Code, e6.Message, true }
    var e7 pkg.ValidationKnownFieldsError;   if errors.As(err, &e7) { return e7.Code, e7.Message, true }
    var e8 pkg.ValidationUnknownFieldsError; if errors.As(err, &e8) { return e8.Code, e8.Message, true }
    var e9 pkg.ResponseError;                if errors.As(err, &e9) { return e9.Code, e9.Message, true }
    // libCommons.Response handled here or before, see 2.2
    var e10 pkg.InternalServerError;         if errors.As(err, &e10) { return e10.Code, e10.Message, true }
    var e11 pkg.FailedPreconditionError;     if errors.As(err, &e11) { return e11.Code, e11.Message, true }
    var e12 pkg.ServiceUnavailableError;     if errors.As(err, &e12) { return e12.Code, e12.Message, true }
    return "", "", false   // -> MapError emits canonical 500 with fallbackCode
}
```

`fallbackCode` = `constant.ErrInternalServer.Error()` (`"0046"`) — matches the current
fallthrough (`errors.go:110` → `ValidateInternalError` → code `0046`).

**`code` is copied verbatim.** `codeOf` reads `err.Code`, which was set to
`constant.ErrXxx.Error()` in the `ValidateBusinessError` map (`pkg/errors.go:391-2823`).
No transform. `allCodesPreserved = true` holds because the code string is never touched.

### 2.2 `statusOf(code string) int` — reproduce §1 table VERBATIM

`MapError` gives `codeOf` the code, then calls `statusOf(code)`. But the current status
is derived from the **Go type**, not the code — so `statusOf` cannot be a code→status
literal map without first re-deriving the type. Two viable shapes; pick the first:

**Option (a) — RECOMMENDED, type-keyed (lazy-correct, no drift risk).** Do the type
classification once inside a combined `codeOf` that returns the status too, or keep a
package-level `statusForType` and have `statusOf` re-run the same `errors.As` cascade
that classified the error. Since `MapError` only hands `statusOf` the *code string*, the
cleanest is to **carry the status out of `codeOf`** via a closure-captured var, OR use a
thin wrapper that classifies once. Concretely, the minimal correct wiring is a local
adapter that does not go through `MapError`'s `statusOf(code)` re-lookup at all — see the
NOTE below. If you keep `MapError`'s signature, `statusOf` becomes a `map[string]int`
built ONCE by iterating `ValidateBusinessError` over every sentinel and reading back the
type→status (the same construction the golden test uses — §5). This makes `statusOf` a
frozen snapshot of §1; the golden test guards it.

```go
// built once at init from the errorMap by classifying each mapped sentinel's type:
var codeStatus = map[string]int{ /* "0007":404, "0001":409, "0004":400, ... */ }

func statusOf(code string) int {
    if s, ok := codeStatus[code]; ok { return s }
    return http.StatusInternalServerError // 500 — matches fallthrough
}
```

**`ResponseError` (status-in-Code, code `0094`) is the one arm this breaks** — its status
is `strconv.Atoi(err.Code)` (`response.go:124`), and its `Code` is the HTTP status
integer, not a business code. `statusOf("400")` in the frozen map would miss. Handle
`ResponseError` **before** `MapError`, on its own branch, exactly as `JSONResponseError`
does today (see 2.6). Do NOT route it through the code→status table.

> NOTE (ponytail): `MapError`'s `codeOf`/`statusOf` split assumes status is a function of
> code. Midaz's status is a function of *type*. The honest minimal adapter classifies the
> type once and produces `(code, msg, status)` together, then calls
> `problem.newProblem`-equivalent — but `newProblem` is unexported. So either (i) build the
> frozen `codeStatus` map so `statusOf(code)` is well-defined, or (ii) don't use `MapError`
> and construct `*problem.Detail` directly in a local `withProblem(c, err)` that mirrors
> `WithError`'s cascade. Option (i) reuses the lib (third-rail preference) and the frozen
> map is exactly what the golden test already validates — take (i).

### 2.3 `message` → `detail` rename (no code-string change)

`Message` (registry text, `errors.go`) → `problem.Detail.Detail` (`json:"detail"`). This
is a **field rename in the envelope**, populated by `codeOf`'s `msg` return. The `code`
string is untouched by this; only the JSON key for the human message changes
`message` → `detail`. Money path unaffected (money path is `code`+`status`, not the prose).

**5xx sanitization caveat (BEHAVIOR CHANGE — flag to Fred).** `MapError` overwrites the
`detail` to the static `"internal error"` for any `status >= 500` (`maperror.go:60-62`),
and `newProblem` overwrites `Title` to `http.StatusText(status)` (`maperror.go:70-77`).
Today `WithError` passes the registry `Title`+`Message` through unchanged on 500s (e.g.
`0046` "Internal Server Error" / "The server encountered..."; `0181` "Account not found on
Midaz"). Under `MapError`:
- 5xx `detail` → `"internal error"` (was the registry message).
- 5xx `title` → `"Internal Server Error"` (was the registry title).
- 5xx `code` → **still the verbatim registry code** (`maperror.go:64` passes `code` through).

This is a deliberate info-leak fix (closes raw-cause leakage) and the code/status still
survive — so the money-path invariant holds. But the 5xx **title/detail text changes**,
which is a wire change for 500/503 bodies. The golden test asserts code+status only, so it
stays green; a title/message assertion on 5xx codes would (correctly) go red. **Decision
needed:** accept the 5xx text scrub (recommended — it is the whole point of adopting
`problem`) OR pre-empt `MapError`'s 5xx branch to preserve legacy 5xx text. Recommend
accept; note it in the migration PR.

### 2.4 `code` → `problem.Detail.Code` (verbatim, the money-path carrier)

`codeOf` returns `code`; `MapError` sets `Detail.Code = code` when non-empty
(`maperror.go:64,79-80`). Same string, `json:"code"`. **Untouched.**

### 2.5 `type` URI derivation (a view of the code, adds nothing)

`MapError`/`newProblem` sets `Detail.Type = problem.BaseURI + "/" + code`
(`maperror.go:81`, `problem.go:26` → `https://errors.lerian.studio/v1/<code>`). E.g.
`0147` → `https://errors.lerian.studio/v1/0147`; `CRM-0006` →
`.../v1/CRM-0006`. This is a **pure rendering of the existing code** — adds no code,
removes none. New additive field on the wire (`type` was absent before). Empty code →
`type` stays `about:blank` (huma default), which only happens on the unreachable
empty-code branch.

### 2.6 `entityType` preservation (local superset of `problem.Detail`)

`problem.Detail` has no `entityType` member. Preserve it via a local superset in the shared
`pkg/net/http` package (the same extension mechanism lib-commons uses for `Code`):

```go
// pkg/net/http (new, small): the Midaz wire projection = problem.Detail + entityType.
type Detail struct {
    problem.Detail
    EntityType string `json:"entityType,omitempty"`
}
```

Population: after `MapError` returns `*problem.Detail`, wrap it and copy `EntityType`
from the typed error (available in the same `codeOf` cascade — every typed struct has
`EntityType`, `errors.go:19,51,75,...`). `entityType` only ever reached the wire on the
`ResponseError`/field-bearing paths today (`response.go` bare helpers dropped it), so
carrying it as `omitempty` is a strict superset — no regression, matches ledger's
`mmodel.Error.entityType` (`pkg/mmodel/error.go:30`).

### 2.7 `fields` map → `[]huma.ErrorDetail`

Today `fields` (`FieldValidations = map[string]string`, `errors.go:227`; or
`UnknownFields = map[string]any`, `errors.go:248`) ships flat as
`json:"fields"` on the two 400 field-bearing paths. RFC 9457 carries per-field detail in
`errors[]` (`huma.ErrorModel.Errors []*huma.ErrorDetail`). Map each entry:

```go
// FieldValidations{ "name": "is required" } becomes:
Errors = append(Errors, &huma.ErrorDetail{
    Location: "body." + field,   // or bare field; pick one and freeze it
    Message:  msg,
})
```

`Location` = field key (prefix `body.` to match huma's convention if desired; **decide
once, keep stable** — clients parse it). `Message` = the map value. For `UnknownFields`
(`map[string]any`), `Value` = the value, `Message` = a fixed "unexpected field" string
or `fmt.Sprint(v)`. This moves field detail from a flat `fields` object to the RFC
`errors[]` array — a **shape change for the two 400 field paths only**, not a code/status
change. Clients consuming `fields` must migrate to `errors[]`; note in the PR. `fields`
is NOT money-path.

> ponytail: `entityType` and the `fields`→`errors[]` remap are the only two carries
> `problem.Detail` does not do for free. Both are non-money-path envelope shape. The money
> path (`code`,`status`) rides `MapError` unchanged.

### 2.8 Can `problem.Detail` (+ local superset) carry every current field?

**Yes — no information loss.** Mapping:

| Current field (source) | Target | Verbatim? |
|------------------------|--------|-----------|
| `Code` (`errors.go` typed structs) | `Detail.Code` | ✅ verbatim (money-path carrier) |
| `Title` | `Detail.Title` (huma) | ✅ for <500; **overwritten to status text for ≥500** (§2.3) |
| `Message` | `Detail.Detail` | ✅ for <500 (rename); **scrubbed to "internal error" for ≥500** (§2.3) |
| `EntityType` | local `Detail.EntityType` superset | ✅ verbatim (§2.6) |
| `Fields` (2 paths) | `Detail.Errors[]` (`[]huma.ErrorDetail`) | ✅ remapped shape, no data loss (§2.7) |
| status (type-derived) | `Detail.Status` | ✅ via frozen `statusOf` (§2.2) |
| `type` (new) | `Detail.Type` | additive, derived from code (§2.5) |

Only lossy-by-design item: **≥500 title/detail text** (deliberate scrub). Code and status
— the money-path invariants — are fully preserved for every code including 5xx.

---

## 3. Folding `pkg.HTTPError` into the canonical envelope

`pkg.HTTPError` (`pkg/errors.go:140-146`) = `{EntityType, Title, Message, Code, Err error}`.
It is referenced only as a swagger `@Failure` DTO — **10 refs** across two ledger handlers:

- `components/ledger/internal/adapters/http/in/audit.go:86,87` (400, 500)
- `components/ledger/internal/adapters/http/in/encryption.go:36,37,38,39,40` (400,404,409,422,500)
- `components/ledger/internal/adapters/http/in/encryption.go:129,130,131` (400,404,500)

(The generated `components/ledger/api/docs.go` + `api/swagger.json` also carry the schema;
those regenerate.)

**The leak (confirmed).** The generated schema ships the internal `Err error` field as an
**empty-schema property**:

```json
// components/ledger/api/swagger.json  →  definitions."pkg.HTTPError"
{ "type":"object", "properties": {
    "code":{"type":"string"}, "entityType":{"type":"string"},
    "err":{},                      // ← untyped, leaks the internal Err field
    "message":{"type":"string"}, "title":{"type":"string"} } }
```

`err` is never serialized at runtime (the `response.go` helpers emit `fiber.Map`, not the
struct) — so it is a **doc-only leak**: the OpenAPI contract advertises a field no wire
body ever contains.

**Fold plan:**
1. Repoint all 10 `@Failure ... {object} pkg.HTTPError` refs to the unified envelope
   schema (§4). Under Option B that is `{object} mmodel.Error`; under the full-A target it
   is the unified `problem`/`Detail` `@name`. Either way `pkg.HTTPError` stops being a
   swagger DTO.
2. Drop `err` from the OpenAPI contract — automatic once the refs point at the unified
   schema (which has no `err`). The `pkg.HTTPError` **Go type stays** (it is a real typed
   error used by HTTP clients, `errors.go:139`); only its role as a *documented response
   body* ends. The `Err error json:"err,omitempty"` field on the Go struct is unchanged.
3. Regenerate the ledger spec so `definitions."pkg.HTTPError"` disappears and the 10
   endpoints reference the single unified error schema.

No runtime change — `pkg.HTTPError` was never the wire serializer. This is purely removing
a fictional/leaky schema from the contract and pointing the refs at the canonical one.

---

## 4. Unified struct + tag policy (full-identity parity)

Under full-identity parity the tracer and ledger error schemas become **the same schema**
(`@name` unified), so both planes must emit an identical `required[]`. Today they diverge:

- Ledger `mmodel.Error` (`pkg/mmodel/error.go:11-35`): 5 fields, **no `validate` tags**,
  `code`/`title`/`message` are plain (swaggo marks them required by absence of `omitempty`),
  `entityType`/`fields` optional. Ships a **fictional example** `ERR_INVALID_INPUT`
  (`error.go:13,15`) and a garbled `@Description` — real codes are `"0147"`, `"CRM-0006"`.
- Tracer `ErrorResponse` (`components/tracer/api/types.go:9-13`): 3 fields, **`validate:"required"`**
  on `code`/`title`/`message`, example `"0053"`.

### 4.1 Cleaning `mmodel.Error`'s schema (real example, clean description)

Edit `pkg/mmodel/error.go` (doc/tag only, no field changes, no runtime change):
- `code` example `ERR_INVALID_INPUT` → **`"0147"`** (a real registry code; matches the
  tracer's real-code example convention). `error.go:13,15`.
- `@Description` (`error.go:10`): replace the garbled text with a clean one-liner:
  "Standardized RFC-9457-aligned error body: a stable machine-readable `code`, a `title`,
  a human `message`, and optional `entityType`/`fields`."
- Keep the other examples (`Bad Request`, the message) — they are already sane.

### 4.2 Validate-tag policy (decide the `required[]`)

**Policy: `code`, `title`, `message` are `required`; `entityType`, `fields` are optional.**
This matches the tracer's existing 3 required + widens with 2 optionals, and matches what
the runtime guarantees (every `WithError` helper always emits `code`/`title`/`message`;
`entityType`/`fields` are conditional).

Concretely, the unified struct (Option B interim — the canonical `mmodel.Error`, tags added
for parity so both planes generate the same `required[]`):

```go
type Error struct {
    Code       string            `json:"code"        validate:"required" example:"0147"   maxLength:"50"`
    Title      string            `json:"title"       validate:"required" example:"Bad Request" maxLength:"100"`
    Message    string            `json:"message"     validate:"required" example:"..." maxLength:"500"`
    EntityType string            `json:"entityType,omitempty"            example:"Organization" maxLength:"100"`
    Fields     map[string]string `json:"fields,omitempty"`
} // @name Error
```

- Add `validate:"required"` to `code`/`title`/`message` to make the ledger schema's
  `required[]` **byte-identical** to the tracer's (which already has them). Without this the
  two generated schemas differ in `required[]` and parity fails.
- `components/tracer/api/types.go:9` `ErrorResponse` → alias to / replaced by this shape
  (keep `@name` stable so generated client type names don't churn — decide whether the
  unified `@name` is `Error` or `ErrorResponse`; **recommend `Error`** and retire
  `ErrorResponse` `@name`, updating the tracer handler `@Failure ... {object} ErrorResponse`
  refs to the unified name).
- Duplicate mirror `components/tracer/internal/testutil/integration_helpers.go:770` — update
  to match or delete + import the canonical one.

**Under the full-A target** (RFC 9457 `problem.Detail`), the unified `@name` schema is the
Huma-generated `Detail` (via `problem.Install()`), and `required[]` is whatever huma emits
for `ErrorModel` (all `omitempty` → nothing structurally required; the "required" contract
then lives in docs, not the schema). That is a separate phase; for THIS phase the tag policy
above (`code`/`title`/`message` required) is the parity target.

---

## 5. Golden test design (the money-path net, RED-then-GREEN around the swap)

**File:** `pkg/net/http/errors_golden_test.go` (package `http`, next to `errors.go`).

Rationale for location: the existing per-domain contract tests
(`components/ledger/internal/adapters/http/in/{crm,fee,mainline}_error_contract_test.go`
= 2+15+28 = 45 hand-picked cases, and `tracer_error_contract_test.go` = 24 cases) are
**spot-checks**, not a full sweep. The new test lives beside the dispatcher it guards and
sweeps the **entire** `ValidateBusinessError` map plus the helper-path sentinels.

### 5.1 What it asserts

For **every** entry, drive the error through the real dispatcher (`http.WithError` for the
current baseline; the same call site post-swap, since the swap changes `WithError`'s
internals, not its signature) and assert the tuple `(code string, HTTP status)`:

```go
func TestErrorEnvelopeGolden(t *testing.T) {
    cases := allBusinessErrorCases(t)          // full sweep, see 5.2
    for _, tc := range cases {
        t.Run(tc.code, func(t *testing.T) {
            app := fiber.New()
            app.Get("/probe", func(c *fiber.Ctx) error { return http.WithError(c, tc.err) })
            resp, _ := app.Test(httptest.NewRequest("GET", "/probe", nil))
            defer resp.Body.Close()
            assert.Equal(t, tc.wantStatus, resp.StatusCode)      // MONEY-PATH invariant #1
            var body map[string]any
            b, _ := io.ReadAll(resp.Body)
            require.NoError(t, json.Unmarshal(b, &body))
            assert.Equal(t, tc.code, body["code"])               // MONEY-PATH invariant #2 (works for both {message} and {detail} envelopes — key "code" is stable)
        })
    }
}
```

Assert **only `code` + `status`** — the money-path invariants — so the SAME test is green
before the swap (`{code,title,message}` body) and after (`{code,title,detail,type,...}`
body): `body["code"]` and `resp.StatusCode` are identical across both envelopes. (Do NOT
assert `title`/`message` here — those legitimately change on 5xx under `problem`, §2.3.)

### 5.2 Building the full case list (the sweep)

Three sources, unioned:

1. **The whole `ValidateBusinessError` map.** Iterate every mapped sentinel by calling
   `pkg.ValidateBusinessError(sentinel, entityType)` with **no `%v` args** (missing args
   render `%!v(MISSING)` in the message — harmless, since we assert code+status only). The
   map is unexported; expose the sentinel→expected-status pairs one of two ways:
   - (a) Enumerate all 422 `constant.Err*` sentinels (they are exported package vars in
     `pkg/constant/errors.go`), call `ValidateBusinessError` on each, and derive the
     expected status by classifying the returned Go type with the SAME §1 table
     (`errors.As` cascade) — then the test's own classifier IS the frozen `statusOf` the
     production `statusOf` must match. Assert the two agree. This makes the test
     self-generating and drift-proof: add a code, it's swept automatically.
   - (b) A checked-in golden table `{sentinel, code, status}` for all ~585 mapped codes.
     More explicit, higher maintenance. **Recommend (a)** — reuse the type→status classifier
     as the single source, guard it, done. `// ponytail: one classifier, swept over every
     sentinel — the smallest thing that fails if a code or status drifts.`
2. **The ~18 defined-but-unmapped sentinels** (`p1a-ledger-errors.md §3`) reached via the
   three helper paths — assert their code+status explicitly:
   - `ErrInternalServer` (0046) via `pkg.ValidateInternalError(someErr, "")` → **500**.
   - `ErrBadRequest` (0047), `ErrUnexpectedFieldsInTheRequest` (0053),
     `ErrMissingFieldsInRequest` (0009) via `pkg.ValidateBadRequestFieldsError(...)` → **400**
     (each of the three branches: requiredFields→0009, unknownFields→0053, knownInvalid→0047;
     `errors.go:348-380`).
   - `ErrInvalidRequestBody` (0094) via `pkg.ValidateUnmarshallingError(err)` → `ResponseError`,
     status = `strconv.Atoi("0094")` = **94** (the status-in-Code quirk — assert the current
     behavior verbatim; `response.go:124`). This is the one case where status ≠ a normal HTTP
     code; the golden test must lock **94** so the quirk cannot silently change.
   - The remaining unmapped tenant/keyset/registry/audit sentinels (0092, 0146, 0159, 0160,
     0161, CRM-0019..22, CRM-0031..33, CRM-0036, CRM-0040) have no central dispatcher path;
     assert they fall through `WithError` to **500 / code 0046** (the `ValidateInternalError`
     fallthrough, `errors.go:110`) OR document them as not-surfaced. Lock whatever the
     current fallthrough produces.
3. **The two explicit-status arms** (§1.3): drive a `*fiber.Error{Code:405}` and
   `{Code:413}` through `CanonicalFiberErrorHandler` and assert code `0485`→**405**, code
   `0143`→**413**. These are the only status-ambiguous codes; the golden test must pin both
   the table status (400 via `WithError`) AND the explicit-status-path status (405/413) so
   the swap cannot collapse them.

### 5.3 RED-then-GREEN choreography

1. **Land the test FIRST, against current `WithError`.** It must be GREEN on the current
   code (it asserts current behavior). This is the baseline lock — commit it before touching
   the dispatcher.
2. **Swap the dispatcher internals** to `problem.MapError` (§2). Re-run: the code+status
   assertions **stay GREEN** (money path preserved). If any go RED, the swap changed a code
   or status → money-path regression → block.
3. The "RED-then-GREEN AROUND the swap" is: (a) prove the test can fail by temporarily
   perturbing one arm (e.g. flip `0007` to 400) → RED, confirming the net has teeth; (b)
   revert; (c) land the swap → GREEN. Document the deliberate-RED proof in the PR.

### 5.4 What it does NOT assert (and why)

- Not `title`/`message`/`detail` text — those change by design on 5xx (§2.3) and the
  `message`→`detail` rename changes the key. The money path is `code`+`status`.
- Not the `type` URI — additive, derived from `code`; a separate lighter assertion can
  spot-check `type == BaseURI + "/" + code` for a handful of codes if desired.
- Not `fields`→`errors[]` — non-money-path shape; a separate test on the two 400 field
  paths covers it.

---

## Summary (return payload)

- **Distinct HTTP statuses:** **9 total** on the wire (`400, 401, 403, 404, 405, 409, 413,
  422, 500, 503` — that lists 10 values; **405 and 413 come only from the explicit-status
  Fiber-error path**, `fiber_error_handler.go:44,46`). The `MapError` code→status table
  itself reproduces **8**: `400, 401, 403, 404, 409, 422, 500, 503`. Plus the `ResponseError`
  status-in-`Code` quirk (0094 → status **94**), which is not a table entry and is handled
  on its own branch.
- **Ambiguous / risky codes under the swap:**
  1. `0485` `ErrMethodNotAllowed` and `0143` `ErrPayloadTooLarge` — status is **400 via the
     `WithError` table but 405/413 via `renderCanonical`** (`fiber_error_handler.go:44,46`).
     `MapError` must NOT be the sole status source for these; keep the explicit-status path.
  2. `0094` `ErrInvalidRequestBody` (`ResponseError`) — status is `strconv.Atoi(Code)`; must
     be handled on its own branch before `MapError`, never through the code→status map.
  3. **5xx bodies** — `MapError` scrubs `detail`→`"internal error"` and `title`→status text
     for `status>=500` (`maperror.go:60-62`, `newProblem` `maperror.go:70-77`). Code+status
     survive (money path safe), but 500/503 **title/message text changes** — a deliberate
     info-leak fix that is nonetheless a wire change; get Fred's ack.
  4. `FailedPreconditionError` → **500 (not 412)** by current design (`errors.go:100-103`);
     the frozen table must keep 500 or it is a silent status change.
- **Can `problem.Detail` carry every current field without loss?** **Yes**, with two
  non-money-path additions: a local superset `Detail{ problem.Detail; EntityType string }`
  for `entityType`, and a `fields`→`Errors[]huma.ErrorDetail` remap for the two 400 field
  paths. `code` (money-path carrier) and `status` are byte-for-byte preserved for **every**
  code including 5xx. `allCodesPreserved = true`.
