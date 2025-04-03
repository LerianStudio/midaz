# Midaz Go SDK Documentation

This directory contains documentation for the Midaz Go SDK, including API mappings and usage guides.

## Documentation Generation

The Midaz Go SDK provides built-in documentation generation through the Makefile. You can generate documentation in two ways:

### Interactive Documentation Server

To start an interactive documentation server:

```bash
make godoc
```

This will start a local server at http://localhost:6060/pkg/github.com/LerianStudio/midaz/sdks/go-sdk/pkg/ where you can browse the documentation interactively.

### Static Documentation

To generate static documentation files:

```bash
make godoc-static
```

This will generate text-based documentation files in the `artifacts/godoc/` directory.

## API Mappings

The `mapping` directory contains detailed mappings of the Midaz Go SDK APIs:

- [External APIs](./mapping/external_apis.md) - Public-facing APIs that are intended for direct use by SDK users
- [Internal APIs](./mapping/internal_apis.md) - Internal implementation details for SDK maintainers

### External APIs

The [External APIs](./mapping/external_apis.md) document provides a comprehensive overview of all public methods available in the Midaz Go SDK, organized by purpose and package. It covers:

- High-Level APIs (top-level package)
- Resource Services (organizations, ledgers, accounts, etc.)
- Builder APIs (fluent interfaces for resource creation and updates)
- Error Handling utilities

### Internal APIs

The [Internal APIs](./mapping/internal_apis.md) document provides details about the internal implementation of the SDK, which is useful for SDK maintainers and contributors. It covers:

- Client Package (HTTP client, API client)
- Resource Clients (organization, ledger, account, etc.)
- Builder Implementations (internal structures behind the builder interfaces)
- Error Handling (error types, creation, checking)
- Models Package (data structures)
- Utility Functions (HTTP, validation, conversion, time utilities)

## Best Practices

When working with the Midaz Go SDK:

1. Use the builder pattern for creating and updating resources
2. Handle errors appropriately using the provided error checking functions
3. Use the fluent API for a more readable and maintainable codebase
4. Refer to the documentation when unsure about method parameters or return values

## Contributing to Documentation

When enhancing or modifying the SDK, please ensure that:

1. All exported types, functions, and methods are properly documented
2. Documentation comments follow Go's standard format (godoc)
3. Examples are provided for complex operations
4. The API mapping documents are updated to reflect any changes
