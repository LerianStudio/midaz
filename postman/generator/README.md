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
├── generate-docs.sh               # Orchestrator: swag -> openapi -> publish specs -> postman
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

`generate-docs.sh` and `sync-postman.sh` cover **ledger, tracer and
reporter-manager**. The ledger spec is primary; tracer and reporter-manager
contribute their own folders to the merged collection.

The **workflow generator** (`create-workflow.js`, `config/`, `lib/`) is
**ledger-only by design**: it consumes `postman/WORKFLOW.md` (the ledger
end-to-end flow) and produces the "Complete API Workflow" folder. Tracer and
reporter-manager are documented as plain endpoint folders without a scripted
workflow.

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

This delegates to `make newman` at the repo root, which runs the generated
collection (`postman/MIDAZ.postman_collection.json`) against the configured
environment. For verbose output of just the workflow folder:

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
