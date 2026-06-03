# P7-T12 — `make sec` disposition (unified surface)

Scope: `make sec` (gosec + govulncheck) over `./components/... ./pkg/...` on the
unified module after the P4/P5/P6 folds (fees, tracer, reporter). Toolchain at
scan time: go1.26.4 (see go.mod `toolchain` directive). golangci-lint floor
v2.4.0. govulncheck `v1.3.0`.

## Verdict

`make sec` is GREEN: `sec-gosec` clean and `sec-govulncheck` reports
"No vulnerabilities found." No suppressions or waivers were needed.

## gosec

- Result: `Issues: 0`.
- The merged graph (reporter's aws-sdk-go-v2 / chromedp / mysql / mssql / oracle
  drivers, tracer's CEL, fees) introduced no new gosec findings.
- Pre-existing accepted `#nosec` annotations carried over intact and each has an
  inline justification. Inventory at scan time (17 total):
  - tracer: 7 — `G115` integer-overflow on validated-positive `LIMIT` clauses
    (limit/audit/rule/transaction repos), `G115` deliberate wrap on advisory-lock
    int32 hash, `G101` HTTP-header-name / audit-actor-identifier (not credentials).
  - reporter-worker: 3 — `G118` context cancel returned to caller, `G115`
    pool-size validated positive, `G304` temp filename from `os.CreateTemp`.
  - ledger: 1, crm: 1, pkg: 5 — pre-existing midaz annotations, unchanged.
- No blanket suppression added.

## govulncheck

The merged third-party dep graph (the surface the fold enlarged) scans CLEAN.
The findings the merge surfaced were both Go STANDARD LIBRARY vulns tied to the
build toolchain (go1.26.3), not to any folded third-party dependency.

### Findings on the pre-bump scan (toolchain go1.26.3) and disposition

| Vuln | Package | Severity / reachability | Fixed in | Disposition |
|------|---------|-------------------------|----------|-------------|
| GO-2026-5039 | stdlib `net/textproto` (arbitrary inputs in errors without escaping) | REACHABLE — tracer integration test helper → `io.ReadAll` → `textproto.Reader.ReadMIMEHeader` | go1.26.4 | BUMPED (toolchain go1.26.3 → go1.26.4) |
| GO-2026-5037 | stdlib `crypto/x509` (inefficient candidate hostname parsing) | REACHABLE — `pkg/net/http` cert verification path and ledger balance handler | go1.26.4 | BUMPED (toolchain go1.26.3 → go1.26.4) |
| (1 import-level finding) | imported package, stdlib-path | NOT reachable — govulncheck: "your code doesn't appear to call" | go1.26.4 | Resolved by the same bump |

### Remediation

Added `toolchain go1.26.4` to the unified `go.mod` (kept the `go 1.26.3`
language floor untouched so the P5/P6/P7 module settlement is undisturbed). This
is a same-minor, security-patch-only toolchain bump — the lowest-risk
remediation class. Both stdlib fixes ship in go1.26.4.

Post-bump verification (toolchain go1.26.4):
- `govulncheck ./components/... ./pkg/...` → "No vulnerabilities found." (exit 0)
- `govulncheck -show verbose ...` → "No vulnerabilities found." (no
  unreachable-but-imported residue remains)
- `go build ./...` → green
- `make sec` → green (gosec + govulncheck)

No HIGH/CRITICAL reachable finding remains without remediation. No major-version
bump was required. No finding was suppressed.
