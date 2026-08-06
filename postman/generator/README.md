# Postman Generator

Tooling for the Midaz API documentation hub. This directory converts each
service's OpenAPI spec into a single merged Postman collection and environment,
and builds the ledger end-to-end workflow folder.

Normal usage is via the root target:

```bash
make generate-docs
```

which runs `generate-docs.sh` here. The pieces below document the tooling for
maintainers.

## Components

```
postman/generator/
├── generate-docs.sh               # Orchestrator: Huma dumps -> publish specs -> join -> postman
├── sync-postman.sh                # Converts published specs and merges the collection
├── convert-openapi.js             # Per-spec OpenAPI -> Postman converter (+ env template)
├── enhance-tests.js               # Adds test scripts to requests
├── create-workflow.js             # Main entry point for the workflow folder (npm run workflow)
├── config/
│   └── workflow.config.js         # Centralized workflow configuration
└── lib/
    ├── workflow-processor.js      # Main orchestration
    ├── markdown-parser.js         # Markdown parsing & validation
    ├── request-matcher.js         # Collection search with alternatives
    ├── path-resolver.js           # URL normalization & corrections
    ├── variable-mapper.js         # Parameter substitution
    └── request-body-generator.js  # Transaction body templates
```

## Scope

`sync-postman.sh` converts one Postman collection per published spec and merges
them. Its `SPEC_SOURCES` list names the specs, each entry carrying three fields —
`component|spec-file|requirement`: ledger publishes `openapi.huma.yaml` (carrying
both its `/v1` and `/v2` paths) and tracer publishes `openapi.huma.yaml`. Each
component contributes exactly one consolidated spec, so there is no version-tag
field and `convert_spec` takes no version argument.

Each source is declared `required` or `optional`. A required source is fatal both
when its spec fails to convert and when the spec file is absent, because either way
that surface's folders vanish from the merged collection and the run would otherwise
report success. An optional source tolerates an absent spec and contributes nothing.
Both current sources are `required`: `generate-docs.sh` produces every one of
them and the results are committed under `postman/specs`, so a missing file means the
pipeline is broken rather than trimmed.

Version suffixes on folder names are derived per operation from the path-key version
prefix (`/v2/...` -> a ` (v2)` folder suffix) inside `convert-openapi.js`, not from a
spec version tag. One consolidated document serves both API versions over the same
OpenAPI tags, so without this per-operation split the `/v1` and `/v2` operations would
merge into folders with identical names. Unversioned path keys (e.g. the tracer's) get
no suffix.

The **workflow generator** (`create-workflow.js`, `config/`, `lib/`) is
**ledger-`/v1`-only by design**: it consumes `postman/WORKFLOW.md` (the ledger
end-to-end flow) and produces the "Complete API Workflow" folder. Everything else
is documented as plain endpoint folders without a scripted workflow.

## Usage

### Workflow folder generation

```bash
node create-workflow.js input.json WORKFLOW.md output.json
# or
npm run workflow input.json WORKFLOW.md output.json
```

### Environment variables

- `DEBUG=true/false` — enable debug output

## Configuration

All workflow configuration is centralized in `config/workflow.config.js`.

### API pattern management

```javascript
apiPatterns: {
  pathCorrections: [
    {
      name: "Missing ledger segment",
      detect: /^\/v1\/organizations\/[^/]+\/accounts/,
      correct: (path) => path.replace(/* ... */)
    }
  ]
}
```

### Variable mapping

```javascript
variables: {
  mapping: {
    direct: {
      "{organizationId}": "{{organizationId}}",
      "{ledgerId}": "{{ledgerId}}"
    },
    contextual: {
      "{id}": [
        {
          pattern: /\/organizations\/\{id\}/,
          replacement: "{{organizationId}}"
        }
      ]
    }
  }
}
```

### Transaction templates

```javascript
transactions: {
  templates: {
    json: { /* full transaction body */ },
    inflow: { /* inflow transaction */ },
    outflow: { /* outflow transaction */ },
    zeroOut: { /* CRITICAL - balance zeroing logic */ }
  }
}
```

## Testing

### Run the generated collection

```bash
npm run test:collection
```

This runs `newman run` directly against the generated collection
(`postman/MIDAZ.postman_collection.json`) with the configured environment,
scoped to the `Complete API Workflow` folder. `newman` is declared in
`devDependencies`; install it with `npm install` in this directory first. For
verbose output of just the workflow folder:

```bash
npm run test:collection:verbose
```

### Validate collection/environment JSON

```bash
npm run validate:all
```

## Critical preserved logic (workflow generator)

### 1. Dependency chain integrity

The ledger workflow's variable dependency chain is preserved:

```
organizationId -> ledgerId -> accountId -> currentBalanceAmount -> Zero Out
```

### 2. Balance zeroing pattern (IMMUTABLE)

The balance zero-out steps implement a critical accounting pattern that must not
be modified:

- Extract `Math.abs(balance.available)` -> `currentBalanceAmount`
- Create a reverse transaction using the exact extracted amount

### 3. Transaction type differentiation

Three distinct transaction patterns are maintained:

- **JSON**: explicit source and destination
- **Inflow**: money coming in (no source)
- **Outflow**: money going out (no destination)

## Debugging

### Enhanced logging

```
Searching for: POST /v1/organizations
   Generated 2 alternative paths:
     1. /v1/organizations/{}/ledgers/{}/accounts/{}
     2. /v1/organizations/{}/balances
Selected: Create Organization
```

### Error context

```javascript
if (error instanceof ValidationError) {
  error.issues.forEach(issue => {
    console.error(`${issue.type}: ${issue.message}`);
  });
}
```

## Extending the system

### Adding new path corrections

```javascript
// In config/workflow.config.js
pathCorrections: [
  {
    name: "New API Pattern",
    detect: /pattern/,
    correct: (path) => /* transformation */
  }
]
```

### Adding transaction types

```javascript
// In config/workflow.config.js
transactions: {
  templates: {
    newType: { /* template */ }
  }
}

// In lib/request-body-generator.js
if (step.path.includes('/transactions/newtype')) {
  return this.config.transactions.templates.newType;
}
```

## Critical warnings

1. **DO NOT MODIFY** the balance zeroing templates without extensive testing
2. **DO NOT CHANGE** variable names in the dependency chain
3. **ALWAYS RUN** the generated collection (`npm run test:collection`) before relying on it
4. `postman/WORKFLOW.md` is DO-NOT-MODIFY — it is the source of truth for the ledger workflow
