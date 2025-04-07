# API Reference

**Navigation:** [Home](../) > API Reference

This section provides comprehensive documentation for the Midaz platform APIs, detailing the endpoints, request/response formats, authentication, and usage patterns.

## API Services

The Midaz platform uses a microservices architecture with two primary API services:

### [Onboarding API](./onboarding-api/)

The Onboarding API handles the creation and management of all financial entities within the Midaz platform. It provides endpoints for:

- **Organizations**: Managing top-level organizational entities
- **Ledgers**: Creating and managing financial ledgers within organizations
- **Assets**: Defining currencies and other financial instruments
- **Segments**: Managing business segments for categorization
- **Portfolios**: Creating collections of related accounts
- **Accounts**: Managing individual financial accounts

[Go to Onboarding API Documentation](./onboarding-api/)

### [Transaction API](./transaction-api/)

The Transaction API handles all financial transactions and balance operations within the Midaz platform. It provides endpoints for:

- **Transactions**: Creating, retrieving, and managing financial transactions
- **Operations**: Managing individual debit and credit operations within transactions
- **Balances**: Tracking and manipulating account balances
- **Asset Rates**: Managing exchange rates between different assets

[Go to Transaction API Documentation](./transaction-api/)

### [Transaction DSL](./transaction-dsl/)

The Transaction DSL (Domain-Specific Language) provides a concise, readable way to define complex financial transactions within the Midaz platform:

- **Grammar**: Specialized syntax for defining transactions
- **Templates**: Reusable transaction patterns with variables
- **Validation**: Built-in validation for transaction integrity

[Go to Transaction DSL Documentation](./transaction-dsl/)

## Common Features

Both APIs share the following common characteristics:

### Authentication

Authentication in Midaz is handled by a separate plugin that can be enabled or disabled through configuration. When enabled (`PLUGIN_AUTH_ENABLED=true`), all API requests require authentication using OAuth 2.0 Bearer tokens. Include the token in the `Authorization` header:

```
Authorization: Bearer YOUR_ACCESS_TOKEN
```

If authentication is disabled (`PLUGIN_AUTH_ENABLED=false`), API endpoints can be accessed without authentication. This configuration is typically used for development and testing environments only.

> **Note:** Detailed authentication documentation, including token acquisition and permissions management, is available in the separate Auth Plugin repository. The Auth Plugin is a modular component that provides centralized authentication and authorization services for Midaz.

### Error Handling

All APIs use a standardized error format:

```json
{
  "code": "ERROR_CODE",
  "title": "Human-readable error title",
  "message": "Detailed error message explaining the issue",
  "entityType": "Optional entity type (e.g., 'Organization')",
  "fields": {
    "fieldName": "Field-specific error message"
  }
}
```

### Content Type

All APIs use JSON (`application/json`) for both request and response payloads.

### Metadata Support

Most resources support a `metadata` field that allows for flexible extension of attributes with custom key-value pairs.

## API Integration Guides

For detailed guides on integrating with the Midaz APIs, refer to:

- [Error Handling Best Practices](../developer-guide/error-handling.md)

## Support

For API support or questions, please reach out through:
- GitHub Issues: [File an issue on GitHub](https://github.com/LerianStudio/midaz/issues)
- GitHub Discussions: [Start a discussion](https://github.com/LerianStudio/midaz/discussions)
- Discord: [Join our community](https://discord.gg/qtKU6Zwq5b)
- Email: contact@lerian.studio