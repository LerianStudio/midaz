# Error Envelope Versioning

The ledger's error body is a function of the **route version prefix**. There is no
configuration: no env var selects it, and no deployment can serve a shape the
published contract does not describe.

| Prefix | Envelope | Media type | OAS schema |
|---|---|---|---|
| `/v1` | midaz v3 `{entityType?, title, message, code, fields?}` | `application/json` | `LegacyError` |
| `>= /v2` | RFC 9457 problem document | `application/problem+json` | `Error` |

A new `/vN` inherits RFC 9457 automatically. `/v1` is the only diverging version,
and it diverges for one reason: it shipped in midaz v3 and clients in production
parse it. A URL that still says `/v1` answers as `/v1` did.

## What changed, and when

midaz v4 initially served RFC 9457 on `/v1` as well. That was a breaking change to
a stable prefix — `message` was renamed to `detail`, `type`/`status`/`instance`
were added, and the media type moved — with nothing in the URL to signal it. This
document describes the restored behavior.

`code` and the HTTP status were never affected and are identical across versions.
A client branching on those needs no changes at all.

## The `/v1` body, per error class

The v3 envelope was never uniform, and the restore reproduces it exactly rather
than flattening it.

| Class | HTTP | Keys |
|---|---|---|
| Not found, conflict, unprocessable, unauthorized, forbidden, 5xx | 404/409/422/401/403/500/503 | exactly `code`, `message`, `title` — always all three, never `entityType` |
| Path/query/body validation | 400 | `title`, `message`, `code` |
| Field validation | 400 | `entityType`, `title`, `message`, `code`, `fields` |

Key order is part of the contract and is locked byte-for-byte by
`envelope_v1_test.go`.

### Mapping from the `/v2` document

| `/v2` (RFC 9457) | `/v1` |
|---|---|
| `detail` | `message` |
| `errors[]` (`location`, `message`) | `fields` object, keyed by field name |
| `code` | `code` — unchanged |
| `title` | `title` — unchanged |
| `type`, `status`, `instance` | absent (`status` is the HTTP status alone) |

## 5xx responses now say what went wrong

This applies to **both** versions, and it is a deliberate reversal.

`>= 500` responses used to have their title and message replaced with
`"Internal Server Error"` / `"internal error"`. What that suppressed was not a raw
cause but static catalog text — and several 5xx codes described a mistake the
CALLER had to fix. A client sending a bad account alias received a storm of
identical opaque 500s with nothing to distinguish its own integration bug from an
outage.

The ledger now publishes the registry text. The tracer keeps the previous
behavior.

Three codes were also re-typed, because restoring the message was not enough — the
status was wrong too:

| Code | Was | Now | Meaning |
|---|---|---|---|
| `0181` | 500 | **404** | account not found; check the alias you sent |
| `0204` | 500 | **400** | malformed decimal; use `.` not `,` |
| `0097` | 500 | **422** | values overflow int64 |

If you were treating any of these as retryable server faults, they are client
errors and retrying will not help.

## Three things deliberately NOT restored

1. **`0094`'s HTTP status.** v3 parsed the HTTP status from the business code, so
   this returned a literal HTTP `94` — not a valid status. v4 pins it to `400` and
   keeps it there.
2. **Panic-recovery bodies.** A recovered panic returns the generic 500 text. The
   panic value never reaches the response in either version.
3. **`entityType` on an empty field-validation error.** A field-validation error
   carrying no field entries is indistinguishable from a plain validation error at
   the point the body is reshaped, so it omits `entityType` where v3 included it.
   Known and accepted; closing it would require a public schema change.

## Where this lives

- Registry and middleware: `components/ledger/internal/adapters/http/in/middleware/envelope.go`
- `/v1` renderer: `.../middleware/envelope_v1.go`
- Standard: `docs/standards/error-handling.md` — E13 (envelope per version), E9
  (the 5xx carve-out and its bound), E3 (status mapping)
