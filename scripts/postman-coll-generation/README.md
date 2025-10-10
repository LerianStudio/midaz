# Midaz API Workflow Generator v2.0 - Simplified Modular Architecture

## 🎯 Overview

This document provides a comprehensive guide to the modernized, modular version of the Midaz API Workflow Generator. It maintains 100% compatibility with the original implementation while offering significant improvements in maintainability, debugging, and extensibility.

### Key Benefits

- **Reduced Code Complexity**: Over 30% reduction in lines of code in the core logic.
- **Modular Architecture**: Each major function (parsing, matching, mapping) is encapsulated in its own class, promoting separation of concerns.
- **Configuration-Driven**: All hardcoded values, such as API patterns and variable names, have been moved to a central configuration file.
- **Enhanced Error Handling**: The system now provides comprehensive validation and detailed error reporting.
- **Preserved Business Logic**: Critical dependency chains and business logic, such as the balance zeroing pattern, are maintained exactly as in the original implementation.

## 🏗️ Architecture

### Core Components

```
scripts/postman-coll-generation/
├── create-workflow.js             # Main entry point
├── config/
│   └── workflow.config.js         # Centralized configuration
├── lib/
│   ├── workflow-processor.js      # Main orchestration logic
│   ├── markdown-parser.js         # Markdown parsing and validation
│   ├── request-matcher.js         # Collection search with alternative path matching
│   ├── path-resolver.js           # URL normalization and corrections
│   ├── variable-mapper.js         # Parameter and variable substitution
│   └── request-body-generator.js  # Transaction body template management
└── enhance-tests.js               # Enhanced test script generation
```

## 🚀 Usage

### Direct Usage

To generate a workflow, run the `create-workflow.js` script with the input collection, workflow Markdown, and output collection file paths:

```bash
node create-workflow.js <input-collection.json> <workflow.md> <output-collection.json>
```

### Environment Variables

- `DEBUG=true`: Enables detailed debug output for troubleshooting.

## 🔧 Configuration

All configuration is centralized in `config/workflow.config.js`.

### API Pattern Management

Define rules for correcting and normalizing API paths to handle API evolution.

```javascript
apiPatterns: {
  pathCorrections: [
    {
      name: "Missing ledger segment",
      detect: /^\/v1\/organizations\/[^/]+\/accounts/,
      correct: (path) => path.replace(/* ... */),
    },
  ];
}
```

### Variable Mapping

Configure how variables are substituted in paths and request bodies.

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

### Transaction Templates

Define standard request bodies for different types of transactions.

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

## 🔍 Critical Preserved Logic

### 1. Dependency Chain Integrity

The exact variable dependency chain from the original implementation is preserved to ensure correct workflow execution:

```
Step 1 → organizationId
Step 5 → ledgerId (uses organizationId)
Step 13 → accountId (uses organizationId, ledgerId)
Step 48 → currentBalanceAmount (reads balance)
Step 49 → Zero Out (uses currentBalanceAmount)
```

### 2. Balance Zeroing Pattern (IMMUTABLE)

Steps 48-49 implement a critical accounting pattern that **must not be modified**:

- **Step 48**: Extracts `Math.abs(balance.available)` and stores it as `currentBalanceAmount`.
- **Step 49**: Creates a reverse transaction using the exact extracted amount to zero out the balance.

### 3. Transaction Type Differentiation

The three distinct transaction patterns from the original implementation are maintained:

- **JSON**: An explicit source and destination are provided.
- **Inflow**: Represents money coming in (no explicit source).
- **Outflow**: Represents money going out (no explicit destination).

## 🐛 Debugging

### Enhanced Logging

The new implementation provides detailed logging to trace the workflow generation process:

```
🔍 Searching for: POST /v1/organizations
   Generated 2 alternative paths:
     1. /v1/organizations/{}/ledgers/{}/accounts/{}
     2. /v1/organizations/{}/balances
✅ Selected: Create Organization
```

### Error Context

The system provides comprehensive error reporting with context for easier debugging:

```javascript
if (error instanceof ValidationError) {
  error.issues.forEach((issue) => {
    console.error(`${issue.type}: ${issue.message}`);
  });
}
```

### Statistics

The workflow processor provides statistics for performance analysis and optimization:

```javascript
const stats = processor.getStatistics();
// stats.matcherStats: success rates, failed requests
// stats.variableMapperStats: mapping coverage
```

## ⚡ Performance

### Optimizations

- **Reduced Complexity**: Cyclomatic complexity has been significantly reduced.
- **Efficient Matching**: Path alternatives are generated once and reused.
- **Memory Management**: Proper object cloning and cleanup are implemented.

## 🛠️ Extending the System

### Adding New Path Corrections

To add a new path correction, update the `pathCorrections` array in `config/workflow.config.js`:

```javascript
// in config/workflow.config.js
pathCorrections: [
  {
    name: "New API Pattern",
    detect: /pattern/,
    correct: (path) => /* transformation */
  }
]
```

### Adding New Transaction Types

To add a new transaction type, define a new template in `config/workflow.config.js` and update the logic in `lib/request-body-generator.js`.

### Adding Custom Validation Rules

To add custom validation rules, extend the `validateCustomRules` function in `lib/markdown-parser.js`.

## 🚨 Critical Warnings

1. **DO NOT MODIFY** the balance zeroing templates without extensive testing.
2. **DO NOT CHANGE** the variable names in the dependency chain.
3. **ALWAYS RUN** regression tests before deploying any changes.
4. **TEST THOROUGHLY** with production-like data before switching to a new version.

## 📞 Support

If you encounter issues, follow these steps:

1. Enable debug output with `DEBUG=true`.
2. Review the logs to identify the point of failure.
3. Verify that the configuration in `config/workflow.config.js` is correct.
4. Check the Markdown file for formatting errors.
