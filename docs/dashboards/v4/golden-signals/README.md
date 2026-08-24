# Golden Signals — Midaz Ledger (v4)

**Dashboard UID:** `midaz-v4-golden-signals`
**Audience:** engineering on-call, platform/SRE
**Method:** RED for the HTTP surface, USE for resources

## What this answers

- Is the API serving requests, and how fast?
- Which route regressed?
- Is a dependency down or slow?
- Is the pod resource-starved?
- What was in the logs when it happened?

## SLIs

| SLI | Query basis | Threshold in panel |
|---|---|---|
| Availability | non-5xx share of `http_server_request_duration_seconds_count` | red < 99%, amber < 99.9% |
| Latency p95 | `http_server_request_duration_seconds_bucket` | amber > 500ms, red > 1s |
| 4xx rate | 4xx share of the same counter | amber > 5%, red > 20% |
| Dependency health | `up` share of `readyz_check_status_total` by `checker` | scale 0–1 |

Thresholds are **starting points calibrated to a dev environment**, not agreed SLOs. Do
not copy them to production without a latency budget conversation.

## Reading it correctly

**Health-check routes are excluded from the API panels.** `/readyz`, `/health`, `/version`
and `/` are filtered out. In a low-traffic environment `/readyz` alone can be roughly 80% of all HTTP traffic;
leaving it in makes the throughput panel a flat line of health checks and drags p95 toward
zero. Dependency health is charted separately from `readyz_check_status_total`.

**`vault` sits below 1 on the dependency panel without being broken.** It reports status
`n/a` while the KMS runs in legacy mode.

**A blank legend entry on "Throughput by route" is real traffic**, not a rendering bug —
it is requests that matched no registered route. It is currently the largest single series
on that panel, which is worth understanding rather than hiding.

**Process CPU/memory percentages are not what triggers an OOMKill.** Use "Container
working set" from cAdvisor for that. Both are on the dashboard so they can be compared.

## No alerts attached

No alert rules are wired to these panels. Alerting on a dev environment with this traffic
profile would be noise. The availability and dependency-health panels are the two that
would carry alerts first in staging or production.
