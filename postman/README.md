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

1. Regenerates each component's native Huma OAS 3.1 dump
   (`components/<service>/api/openapi.huma.yaml`) by running its golden-dump test
   `TestOpenAPISpecDump` with `-update`. No `swag`, no Docker — the spec comes
   straight from the Huma router at build time.
2. Copies each `openapi.huma.yaml` into `postman/specs/<service>/` so the hub is
   self-describing.
3. Joins the per-component specs into one consolidated spec
   (`postman/specs/midaz.openapi.{yaml,json}`) with `@redocly/cli join` (ledger
   first, so it acts as the "main" and takes precedence on shared metadata).
4. Converts the consolidated spec to a Postman collection and merges into one
   `MIDAZ.postman_collection.json`, with a single merged
   `MIDAZ.postman_environment.json`.

### Covered services

| Service | Port | Notes |
|---------|------|-------|
| `ledger` | `:3002` | Unified binary: onboarding + transaction + CRM (holders/instruments) + fees, all on one base URL |
| `tracer` | `:4020` | Real-time transaction validation / fraud prevention |

Only `ledger` and `tracer` are generated. `crm` is a package tree folded into the
ledger binary (its endpoints are part of the ledger spec) and `reporter-worker` is
health-only (no REST API), so neither is generated separately.

## General-info parity (enforced by `make check-docs`)

The Huma dumps emit only `info.title` and `info.version`; the swaggo-era
`info.contact` / `info.license` / `info.termsOfService` / `schemes` fields are
honestly dropped (OAS 3.1 has no `.schemes`). `make check-docs` enforces:

- `info.version` — byte-identical across ledger and tracer, matching `^4.0.0$`
  (no `v` prefix). Set it in each `components/<c>/cmd/app/main.go` header.
- `info.title` — must start with `Midaz ` (each plane names itself, e.g.
  "Midaz Ledger API" vs "Midaz Tracer API"); title is NOT byte-parity metadata.

With `CHECK_DOCS_REGEN=1`, `make check-docs` also regenerates the docs and asserts
the committed artifacts still reproduce (drift check).

Security schemes intentionally differ and are NOT part of parity: ledger uses
`BearerAuth` (Authorization header), tracer uses `ApiKeyAuth` (X-API-Key). Do not
normalize them.

## Layout

```
postman/
├── README.md                          # This file
├── WORKFLOW.md                        # Ledger end-to-end workflow definition (DO NOT MODIFY)
├── MIDAZ.postman_collection.json      # Merged collection (ledger + tracer + reporter)
├── MIDAZ.postman_environment.json     # Merged environment
├── specs/                             # Published OpenAPI specs
│   ├── ledger/openapi.huma.yaml       # Per-component Huma OAS 3.1 dumps
│   ├── tracer/openapi.huma.yaml
│   └── midaz.openapi.{yaml,json}      # Consolidated (redocly join) spec
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

- Go toolchain (the Huma dumps are produced by `go test ... -update`; no `swag`,
  no Docker)
- Node.js (>= 16) and `jq`

`make set-env` / `scripts/setup-deps.sh` install the Go and Node toolchain pins.
`@redocly/cli` and the Postman converters are bundled under
`postman/generator/node_modules` (installed on first run).

## Troubleshooting

- **redocly not found**: run `make generate-docs` once to install the bundled
  Node dependencies under `postman/generator/node_modules`.
- **Inspect the collection**:
  ```bash
  jq '.item[].name' postman/MIDAZ.postman_collection.json
  jq '.values[].key' postman/MIDAZ.postman_environment.json
  ```
- **Failed run leftovers**: `postman/temp/` is scratch space and is gitignored;
  it is removed on a successful run.
