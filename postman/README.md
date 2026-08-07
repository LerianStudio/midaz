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

1. Regenerates each component's native Huma OAS 3.1 dump by running the golden-dump
   tests with `-update`. No `swag`, no Docker — the spec comes straight from the Huma
   router at build time. Ledger emits one dump, `components/ledger/api/openapi.huma.yaml`,
   carrying both its `/v1` and `/v2` paths on self-describing keys; tracer emits one.
2. Copies every dump into `postman/specs/<service>/` so the hub is self-describing.
3. Joins the per-component specs into one consolidated spec
   (`postman/specs/midaz.openapi.{yaml,json}`) with `@redocly/cli join` (ledger
   first, so it acts as the "main" and takes precedence on shared metadata). The
   tracer dump is transformed into a joinable input first: its path keys are
   server-relative (served under `/v1` with unprefixed keys), so they are prefixed
   with `/v1` and its top-level `servers` is set to `/` to match the ledger dump.
   Both inputs then reach the join symmetric — self-describing keys and `servers`
   `/` — so the joined root `servers` is `/` and no path item carries an override.
   This consolidated spec is a published documentation artifact; it is not the
   Postman input.
4. Converts each published per-component spec to its own Postman collection and
   merges them into one `MIDAZ.postman_collection.json`, with a single merged
   `MIDAZ.postman_environment.json`. Converting per component (rather than the
   consolidated spec) is what keeps the per-service base-URL split below.

### Covered services

| Service | Port | Contracts | Notes |
|---------|------|-----------|-------|
| `ledger` | `:3002` | `/v1`, `/v2` | Unified binary: onboarding + transaction + CRM (holders/instruments) + fees, all on one base URL |
| `tracer` | `:4020` | `/v1` | Real-time transaction validation / fraud prevention |

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
├── MIDAZ.postman_collection.json      # Merged collection (every published spec)
├── MIDAZ.postman_environment.json     # Merged environment
├── specs/                             # Published OpenAPI specs
│   ├── ledger/openapi.huma.yaml       # Per-component Huma OAS 3.1 dumps (ledger /v1 + /v2)
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

The merged collection is one **MIDAZ** collection. The ledger spec is primary;
every other published spec contributes its own folders, grouped by OpenAPI tag.
Within the single ledger spec, operations under `/v2` get a folder suffix ` (v2)`
derived from the first path-key segment (`/v1` gets no suffix), so **Transactions**
and **Transactions (v2)** stay distinct even though they share the OpenAPI tag.

Request URLs carry the version prefix from the path key (the spec's `servers` is
`/`), so a generated URL targets the same path the service mounts the operation on,
and `/v1`/`/v2` requests to the same resource stay distinct.

Requests route to per-service base URLs:

- Ledger uses `{{onboardingUrl}}` / `{{transactionUrl}}` (both resolve to `:3002`).
- Tracer uses `{{tracerUrl}}` (`:4020`).

Set `authToken` plus the two host roots — `baseUrl` (ledger) and `host` (tracer) —
in the environment and the per-service URLs resolve automatically.

## Workflow testing

`WORKFLOW.md` defines an end-to-end ledger flow (Organization -> Ledger -> Account
-> Transaction -> balance zero-out). `create-workflow.js` turns it into the
"Complete API Workflow" folder during generation. This workflow chain covers the
**ledger `/v1` flow only** — the ledger `/v2` and tracer endpoints appear in the
collection as plain endpoint folders without a scripted workflow. `WORKFLOW.md` is
marked DO-NOT-MODIFY; treat it as the source of truth for the ledger workflow.

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
