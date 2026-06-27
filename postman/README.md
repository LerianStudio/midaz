# Midaz API Postman Hub

This directory is the self-contained API-documentation hub for Midaz. It holds the
generation tooling, the published OpenAPI specs per service, and the single merged
Postman collection plus environment used to exercise the APIs.

A single command regenerates everything:

```bash
make generate-docs
```

## What gets generated

`make generate-docs` (root) runs `postman/generator/generate-docs.sh`, which:

1. Runs `swag init` for each service to refresh its `components/<service>/api/`
   artifacts (`docs.go`, `swagger.json`, `swagger.yaml`). `docs.go` stays under
   `api/` because Go imports it and the ledger contract test reads
   `components/ledger/api/swagger.json`.
2. Converts each `swagger.json` to `openapi.yaml` with
   `openapitools/openapi-generator-cli:v7.10.0` (Docker required).
3. Copies the `swagger.json` + `swagger.yaml` + `openapi.yaml` triple into
   `postman/specs/<service>/` so the hub is self-describing.
4. Converts each spec to a Postman collection and merges them into one
   `MIDAZ.postman_collection.json`, with a single merged
   `MIDAZ.postman_environment.json`.

### Covered services

| Service | Port | Notes |
|---------|------|-------|
| `ledger` | `:3002` | Unified binary: onboarding + transaction + CRM (holders/instruments) + fees, all on one base URL |
| `tracer` | `:4020` | Real-time transaction validation / fraud prevention |
| `reporter` | `:4005` | Async report management API |

`reporter-worker` is health-only (no REST API) and `crm` is a package tree folded
into the ledger binary (its endpoints are part of the ledger swagger), so neither
is generated separately.

## General-info parity (enforced by `make check-docs`)

The three component specs MUST agree on these `info` fields — keep them in sync when editing any `components/<c>/cmd/app/main.go` header:

- `info.contact` — Discord community / https://discord.gg/DnhqKwkGv3
- `info.license` — Elastic License 2.0 / https://www.elastic.co/licensing/elastic-license
- `info.termsOfService` — https://www.elastic.co/licensing/elastic-license
- `schemes` — `http https`
- `info.version` — `4.0.0` (no `v` prefix)
- `info.title` — must start with `Midaz `

Security schemes intentionally differ and are NOT part of parity: ledger/reporter use `BearerAuth` (Authorization header), tracer uses `ApiKeyAuth` (X-API-Key). Do not normalize them.

## Layout

```
postman/
├── README.md                          # This file
├── WORKFLOW.md                        # Ledger end-to-end workflow definition (DO NOT MODIFY)
├── MIDAZ.postman_collection.json      # Merged collection (ledger + tracer + reporter)
├── MIDAZ.postman_environment.json     # Merged environment
├── specs/                             # Published OpenAPI specs per service
│   ├── ledger/{swagger.json,swagger.yaml,openapi.yaml}
│   ├── tracer/{swagger.json,swagger.yaml,openapi.yaml}
│   └── reporter/{swagger.json,swagger.yaml,openapi.yaml}
├── backups/                           # Timestamped collection/environment backups (gitignored)
├── temp/                              # Scratch space used during a run (gitignored)
└── generator/                         # Generation tooling
    ├── generate-docs.sh               # Top-level orchestrator (called by make generate-docs)
    ├── sync-postman.sh                # OpenAPI -> Postman conversion + merge
    ├── convert-openapi.js             # Per-spec OpenAPI -> Postman converter
    ├── enhance-tests.js               # Adds test scripts to requests
    ├── create-workflow.js             # Builds the ledger workflow folder from WORKFLOW.md
    ├── config/, lib/                  # Workflow generator configuration and helpers
    └── package.json                   # Node.js dependencies
```

## Collection structure

The merged collection is one **MIDAZ** collection. The ledger spec is primary, and
the tracer and reporter specs contribute their own folders (grouped by
OpenAPI tag). Requests route to per-service base URLs:

- Ledger uses `{{onboardingUrl}}` / `{{transactionUrl}}` (both resolve to `:3002`).
- Tracer uses `{{tracerUrl}}` (`:4020`).
- Reporter uses `{{reporterUrl}}` (`:4005`).

Set `host` and `authToken` in the environment and the per-service URLs resolve
automatically.

## Workflow testing

`WORKFLOW.md` defines an end-to-end ledger flow (Organization -> Ledger -> Account
-> Transaction -> balance zero-out). `create-workflow.js` turns it into the
"Complete API Workflow" folder during generation. This workflow chain covers the
**ledger flow only** — tracer and reporter appear in the collection as
plain endpoint folders without a scripted workflow. `WORKFLOW.md` is marked
DO-NOT-MODIFY; treat it as the source of truth for the ledger workflow.

Run the workflow with Newman:

```bash
newman run postman/MIDAZ.postman_collection.json \
  -e postman/MIDAZ.postman_environment.json \
  --folder "Complete API Workflow"
```

## Requirements

- `swag` v1.16.6 (`go install github.com/swaggo/swag/cmd/swag@v1.16.6`)
- Docker (for `openapi-generator-cli:v7.10.0`)
- Node.js (>= 16) and `jq`

`make set-env` / `scripts/setup-deps.sh` install the Go and Node toolchain pins.

## Troubleshooting

- **swag not found**: install the pinned version above, or run `scripts/setup-deps.sh`.
- **Docker errors**: `openapi.yaml` generation needs Docker running.
- **Inspect the collection**:
  ```bash
  jq '.item[].name' postman/MIDAZ.postman_collection.json
  jq '.values[].key' postman/MIDAZ.postman_environment.json
  ```
- **Failed run leftovers**: `postman/temp/` is scratch space and is gitignored;
  it is removed on a successful run.
