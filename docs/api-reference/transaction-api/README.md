# Transaction API

**Navigation:** [Home](../../) > [API Reference](../) > Transaction API

This documentation provides a comprehensive reference for the Midaz Transaction API, which handles all financial transaction processing, including balance management, operations tracking, asset rates, and transaction lifecycle management.

## Overview

The Transaction API is a RESTful API that manages the financial transaction process within the Midaz platform. It implements double-entry bookkeeping principles, ensures data consistency, and handles all aspects of transaction lifecycle.

- **Base URL**: 
  - Production: `https://api.midaz.io/transaction/v1`
  - Development: `http://localhost:3001/transaction/v1`
- **Content Type**: JSON (`application/json`)
- **API Version**: v2.0.0

## Authentication

Authentication in Midaz is handled by a separate plugin that can be enabled or disabled through configuration. When enabled (`PLUGIN_AUTH_ENABLED=true`), all API requests require authentication using OAuth 2.0 Bearer tokens:

```
Authorization: Bearer YOUR_ACCESS_TOKEN
```

If authentication is disabled (`PLUGIN_AUTH_ENABLED=false`), API endpoints can be accessed without authentication. This configuration is typically used for development and testing environments only.

## Common Headers

| Header Name | Required | Description |
|-------------|----------|-------------|
| Authorization | Yes | OAuth 2.0 Bearer token |
| X-Request-Id | No | Unique request identifier for tracing (recommended) |

## Common Response Codes

| Status Code | Description |
|-------------|-------------|
| 200 | Success - The request was processed successfully |
| 201 | Created - The resource was created successfully |
| 204 | No Content - The request was successful, but no content is returned |
| 400 | Bad Request - The request contains invalid parameters or validation errors |
| 401 | Unauthorized - Missing or invalid authentication |
| 403 | Forbidden - Authentication succeeded but the user lacks permissions |
| 404 | Not Found - The requested resource does not exist |
| 409 | Conflict - A conflict occurred with the current state of the resource |
| 500 | Internal Server Error - An error occurred on the server |

## Error Response Structure

All error responses follow a standardized format:

```json
{
  "code": "ERROR_CODE",
  "title": "Human-readable error title",
  "message": "Detailed error message explaining the issue",
  "entityType": "Optional entity type (e.g., 'Transaction')",
  "fields": {
    "fieldName": "Field-specific error message"
  }
}
```

## Resource Categories

The Transaction API is organized into the following resource categories:

- [Balances](#balances): Manage account balances
- [Operations](#operations): Track individual financial operations
- [Asset Rates](#asset-rates): Manage exchange rates between assets
- [Transactions](#transactions): Create and manage financial transactions

## Balances

Balances represent the current financial state of an account in a specific asset.

### Get All Balances

Retrieves all balances across accounts.

**Endpoint**: `GET /organizations/{organization_id}/ledgers/{ledger_id}/balances`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier

**Query Parameters**:
- `limit` (integer, optional): Number of items to return (default: 10)
- `start_date` (string, optional): Filter by start date
- `end_date` (string, optional): Filter by end date
- `sort_order` (string, optional): Sort order (asc or desc)
- `cursor` (string, optional): Pagination cursor

**Response**:
```json
{
  "items": [
    {
      "id": "00000000-0000-0000-0000-000000000000",
      "accountId": "00000000-0000-0000-0000-000000000000",
      "organizationId": "00000000-0000-0000-0000-000000000000",
      "ledgerId": "00000000-0000-0000-0000-000000000000",
      "alias": "@person1",
      "accountType": "creditCard",
      "assetCode": "BRL",
      "available": 1500,
      "onHold": 500,
      "scale": 2,
      "allowSending": true,
      "allowReceiving": true,
      "version": 1,
      "metadata": {},
      "createdAt": "2021-01-01T00:00:00Z",
      "updatedAt": "2021-01-01T00:00:00Z"
    }
  ],
  "limit": 10,
  "next_cursor": "MDAwMDAwMDAtMDAwMC0wMDAwLTAwMDAtMDAwMDAwMDAwMDAwMA==",
  "prev_cursor": "MDAwMDAwMDAtMDAwMC0wMDAwLTAwMDAtMDAwMDAwMDAwMDAwMA=="
}
```

### Get Account Balances

Retrieves all balances for a specific account.

**Endpoint**: `GET /organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/balances`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier
- `account_id` (string, required): Account identifier

**Query Parameters**:
- `limit` (integer, optional): Number of items to return (default: 10)
- `start_date` (string, optional): Filter by start date
- `end_date` (string, optional): Filter by end date
- `sort_order` (string, optional): Sort order (asc or desc)
- `cursor` (string, optional): Pagination cursor

**Response**: Same format as Get All Balances

### Get Balance by ID

Retrieves a specific balance by its ID.

**Endpoint**: `GET /organizations/{organization_id}/ledgers/{ledger_id}/balances/{balance_id}`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier
- `balance_id` (string, required): Balance identifier

**Response**:
```json
{
  "id": "00000000-0000-0000-0000-000000000000",
  "accountId": "00000000-0000-0000-0000-000000000000",
  "organizationId": "00000000-0000-0000-0000-000000000000",
  "ledgerId": "00000000-0000-0000-0000-000000000000",
  "alias": "@person1",
  "accountType": "creditCard",
  "assetCode": "BRL",
  "available": 1500,
  "onHold": 500,
  "scale": 2,
  "allowSending": true,
  "allowReceiving": true,
  "version": 1,
  "metadata": {},
  "createdAt": "2021-01-01T00:00:00Z",
  "updatedAt": "2021-01-01T00:00:00Z"
}
```

### Update Balance

Updates a specific balance.

**Endpoint**: `PATCH /organizations/{organization_id}/ledgers/{ledger_id}/balances/{balance_id}`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier
- `balance_id` (string, required): Balance identifier

**Request Body**:
```json
{
  "allowSending": true,
  "allowReceiving": true
}
```

**Response**: Returns the updated balance object

### Delete Balance

Deletes a specific balance.

**Endpoint**: `DELETE /organizations/{organization_id}/ledgers/{ledger_id}/balances/{balance_id}`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier
- `balance_id` (string, required): Balance identifier

**Response**: Returns 204 No Content on success

## Operations

Operations represent individual debit or credit entries that make up a transaction.

### Get All Operations by Account

Retrieves all operations for a specific account.

**Endpoint**: `GET /organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/operations`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier
- `account_id` (string, required): Account identifier

**Query Parameters**:
- `limit` (integer, optional): Number of items to return (default: 10)
- `start_date` (string, optional): Filter by start date
- `end_date` (string, optional): Filter by end date
- `sort_order` (string, optional): Sort order (asc or desc)
- `cursor` (string, optional): Pagination cursor

**Response**:
```json
{
  "items": [
    {
      "id": "00000000-0000-0000-0000-000000000000",
      "transactionId": "00000000-0000-0000-0000-000000000000",
      "accountId": "00000000-0000-0000-0000-000000000000",
      "accountAlias": "@person1",
      "balanceId": "00000000-0000-0000-0000-000000000000",
      "organizationId": "00000000-0000-0000-0000-000000000000",
      "ledgerId": "00000000-0000-0000-0000-000000000000",
      "type": "creditCard",
      "description": "Credit card operation",
      "assetCode": "BRL",
      "chartOfAccounts": "1000",
      "amount": {
        "asset": "BRL",
        "value": 1000,
        "scale": 2,
        "operation": "operation"
      },
      "balance": {
        "available": 1500,
        "onHold": 500,
        "scale": 2
      },
      "balanceAfter": {
        "available": 1500,
        "onHold": 500,
        "scale": 2
      },
      "status": {
        "code": "ACTIVE",
        "description": "Active status"
      },
      "metadata": {},
      "createdAt": "2021-01-01T00:00:00Z",
      "updatedAt": "2021-01-01T00:00:00Z"
    }
  ],
  "limit": 10,
  "next_cursor": "MDAwMDAwMDAtMDAwMC0wMDAwLTAwMDAtMDAwMDAwMDAwMDAwMA==",
  "prev_cursor": "MDAwMDAwMDAtMDAwMC0wMDAwLTAwMDAtMDAwMDAwMDAwMDAwMA=="
}
```

### Get Operation by ID

Retrieves a specific operation by its ID.

**Endpoint**: `GET /organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/operations/{operation_id}`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier
- `account_id` (string, required): Account identifier
- `operation_id` (string, required): Operation identifier

**Response**: Returns a single operation object

### Update Operation

Updates a specific operation.

**Endpoint**: `PATCH /organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/operations/{operation_id}`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier
- `transaction_id` (string, required): Transaction identifier
- `operation_id` (string, required): Operation identifier

**Request Body**:
```json
{
  "description": "Updated operation description",
  "metadata": {
    "key1": "value1"
  }
}
```

**Response**: Returns the updated operation object

## Asset Rates

Asset rates define exchange rates between different assets.

### Create or Update Asset Rate

Creates or updates an asset rate.

**Endpoint**: `PUT /organizations/{organization_id}/ledgers/{ledger_id}/asset-rates`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier

**Request Body**:
```json
{
  "from": "USD",
  "to": "BRL",
  "rate": 100,
  "scale": 2,
  "externalId": "00000000-0000-0000-0000-000000000000",
  "source": "External System",
  "ttl": 3600,
  "metadata": {}
}
```

**Response**:
```json
{
  "id": "00000000-0000-0000-0000-000000000000",
  "organizationId": "00000000-0000-0000-0000-000000000000",
  "ledgerId": "00000000-0000-0000-0000-000000000000",
  "from": "USD",
  "to": "BRL",
  "rate": 100.0,
  "scale": 2.0,
  "externalId": "00000000-0000-0000-0000-000000000000",
  "source": "External System",
  "ttl": 3600,
  "metadata": {},
  "createdAt": "2021-01-01T00:00:00Z",
  "updatedAt": "2021-01-01T00:00:00Z"
}
```

### Get Asset Rates by Asset Code

Retrieves asset rates for a specific asset code.

**Endpoint**: `GET /organizations/{organization_id}/ledgers/{ledger_id}/asset-rates/from/{asset_code}`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier
- `asset_code` (string, required): From asset code

**Query Parameters**:
- `to` (array of strings, optional): Filter by destination asset codes
- `limit` (integer, optional): Number of items to return (default: 10)
- `start_date` (string, optional): Filter by start date
- `end_date` (string, optional): Filter by end date
- `sort_order` (string, optional): Sort order (asc or desc)
- `cursor` (string, optional): Pagination cursor

**Response**:
```json
{
  "items": [
    {
      "id": "00000000-0000-0000-0000-000000000000",
      "organizationId": "00000000-0000-0000-0000-000000000000",
      "ledgerId": "00000000-0000-0000-0000-000000000000",
      "from": "USD",
      "to": "BRL",
      "rate": 100.0,
      "scale": 2.0,
      "externalId": "00000000-0000-0000-0000-000000000000",
      "source": "External System",
      "ttl": 3600,
      "metadata": {},
      "createdAt": "2021-01-01T00:00:00Z",
      "updatedAt": "2021-01-01T00:00:00Z"
    }
  ],
  "limit": 10,
  "next_cursor": "MDAwMDAwMDAtMDAwMC0wMDAwLTAwMDAtMDAwMDAwMDAwMDAwMA==",
  "prev_cursor": "MDAwMDAwMDAtMDAwMC0wMDAwLTAwMDAtMDAwMDAwMDAwMDAwMA=="
}
```

### Get Asset Rate by External ID

Retrieves an asset rate by its external ID.

**Endpoint**: `GET /organizations/{organization_id}/ledgers/{ledger_id}/asset-rates/{external_id}`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier
- `external_id` (string, required): External identifier

**Response**: Returns a single asset rate object

## Transactions

Transactions represent financial movements between accounts.

### Get All Transactions

Retrieves all transactions for a ledger.

**Endpoint**: `GET /organizations/{organization_id}/ledgers/{ledger_id}/transactions`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier

**Query Parameters**:
- `limit` (integer, optional): Number of items to return (default: 10)
- `start_date` (string, optional): Filter by start date
- `end_date` (string, optional): Filter by end date
- `sort_order` (string, optional): Sort order (asc or desc)
- `cursor` (string, optional): Pagination cursor

**Response**:
```json
{
  "items": [
    {
      "id": "00000000-0000-0000-0000-000000000000",
      "organizationId": "00000000-0000-0000-0000-000000000000",
      "ledgerId": "00000000-0000-0000-0000-000000000000",
      "parentTransactionId": "00000000-0000-0000-0000-000000000000",
      "description": "Transaction description",
      "chartOfAccountsGroupName": "Chart of accounts group name",
      "assetCode": "BRL",
      "amount": 1500,
      "amountScale": 2,
      "source": ["@person1"],
      "destination": ["@person2"],
      "template": "Transaction template",
      "status": {
        "code": "ACTIVE",
        "description": "Active status"
      },
      "operations": [
        {
          "id": "00000000-0000-0000-0000-000000000000",
          "transactionId": "00000000-0000-0000-0000-000000000000",
          "accountId": "00000000-0000-0000-0000-000000000000",
          "accountAlias": "@person1",
          "assetCode": "BRL",
          "amount": {
            "asset": "BRL",
            "value": 1000,
            "scale": 2,
            "operation": "operation"
          },
          "type": "creditCard",
          "description": "Credit card operation"
        }
      ],
      "metadata": {},
      "createdAt": "2021-01-01T00:00:00Z",
      "updatedAt": "2021-01-01T00:00:00Z"
    }
  ],
  "limit": 10,
  "next_cursor": "MDAwMDAwMDAtMDAwMC0wMDAwLTAwMDAtMDAwMDAwMDAwMDAwMA==",
  "prev_cursor": "MDAwMDAwMDAtMDAwMC0wMDAwLTAwMDAtMDAwMDAwMDAwMDAwMA=="
}
```

### Get Transaction by ID

Retrieves a transaction by its ID.

**Endpoint**: `GET /organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier
- `transaction_id` (string, required): Transaction identifier

**Response**: Returns a single transaction object with its operations

### Create Transaction using JSON

Creates a new transaction using JSON payload.

**Endpoint**: `POST /organizations/{organization_id}/ledgers/{ledger_id}/transactions/json`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier

**Request Body**:
```json
{
  "description": "Payment transaction",
  "code": "PAY_001",
  "chartOfAccountsGroupName": "PAYMENTS",
  "pending": false,
  "send": {
    "asset": "BRL",
    "value": 1000,
    "scale": 2,
    "source": {
      "from": [
        {
          "account": "@person1",
          "amount": {
            "asset": "BRL",
            "value": 1000,
            "scale": 2
          },
          "chartOfAccounts": "1000",
          "description": "Payment source",
          "isFrom": true
        }
      ]
    },
    "distribute": {
      "to": [
        {
          "account": "@person2",
          "amount": {
            "asset": "BRL",
            "value": 1000,
            "scale": 2
          },
          "chartOfAccounts": "2000",
          "description": "Payment destination"
        }
      ]
    }
  },
  "metadata": {
    "reference": "INV-123"
  }
}
```

**Response**: Returns the created transaction object

### Create Transaction using DSL

Creates a new transaction using a DSL (Domain-Specific Language) file.

**Endpoint**: `POST /organizations/{organization_id}/ledgers/{ledger_id}/transactions/dsl`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier

**Request Body**:
Content-Type: multipart/form-data
Form field: `transaction` (file) - A DSL file describing the transaction

**Response**: Returns the created transaction object

### Create Transaction from Template

Creates a new transaction using a template.

**Endpoint**: `POST /organizations/{organization_id}/ledgers/{ledger_id}/transactions/templates`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier

**Request Body**:
```json
{
  "transactionType": "PAYMENT",
  "transactionTypeCode": "PAY",
  "variables": {
    "fromAccount": "@person1",
    "toAccount": "@person2",
    "amount": 1000
  }
}
```

**Response**: Returns the created transaction template

### Update Transaction

Updates an existing transaction.

**Endpoint**: `PATCH /organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier
- `transaction_id` (string, required): Transaction identifier

**Request Body**:
```json
{
  "description": "Updated transaction description",
  "metadata": {
    "reference": "INV-124"
  }
}
```

**Response**: Returns the updated transaction object

### Revert Transaction

Reverts a transaction, creating a new transaction that reverses the original.

**Endpoint**: `POST /organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/revert`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier
- `transaction_id` (string, required): Transaction identifier

**Response**: Returns the newly created reversal transaction

### Commit Transaction

Commits a previously created transaction.

**Endpoint**: `POST /organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/commit`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier
- `transaction_id` (string, required): Transaction identifier

**Response**: Returns a 201 Created status on success

## Data Models

The Transaction API uses the following primary data models:

### Balance

Represents the available and on-hold funds for an account in a specific asset.

```json
{
  "id": "string",
  "accountId": "string",
  "accountType": "string",
  "organizationId": "string",
  "ledgerId": "string",
  "alias": "string",
  "assetCode": "string",
  "available": "integer",
  "onHold": "integer",
  "scale": "integer",
  "allowSending": "boolean",
  "allowReceiving": "boolean",
  "version": "integer",
  "metadata": "object",
  "createdAt": "string (datetime)",
  "updatedAt": "string (datetime)",
  "deletedAt": "string (datetime)"
}
```

### Operation

Represents an individual debit or credit entry within a transaction.

```json
{
  "id": "string",
  "transactionId": "string",
  "accountId": "string",
  "accountAlias": "string",
  "balanceId": "string",
  "organizationId": "string",
  "ledgerId": "string",
  "type": "string",
  "description": "string",
  "chartOfAccounts": "string",
  "assetCode": "string",
  "amount": {
    "asset": "string",
    "value": "integer",
    "scale": "integer",
    "operation": "string"
  },
  "balance": {
    "available": "integer",
    "onHold": "integer",
    "scale": "integer"
  },
  "balanceAfter": {
    "available": "integer",
    "onHold": "integer",
    "scale": "integer"
  },
  "status": {
    "code": "string",
    "description": "string"
  },
  "metadata": "object",
  "createdAt": "string (datetime)",
  "updatedAt": "string (datetime)",
  "deletedAt": "string (datetime)"
}
```

### AssetRate

Represents an exchange rate between two assets.

```json
{
  "id": "string",
  "organizationId": "string",
  "ledgerId": "string",
  "from": "string",
  "to": "string",
  "rate": "number",
  "scale": "number",
  "externalId": "string",
  "source": "string",
  "ttl": "integer",
  "metadata": "object",
  "createdAt": "string (datetime)",
  "updatedAt": "string (datetime)"
}
```

### Transaction

Represents a financial transaction consisting of one or more operations.

```json
{
  "id": "string",
  "organizationId": "string",
  "ledgerId": "string",
  "parentTransactionId": "string",
  "description": "string",
  "chartOfAccountsGroupName": "string",
  "assetCode": "string",
  "amount": "integer",
  "amountScale": "integer",
  "source": ["string"],
  "destination": ["string"],
  "template": "string",
  "status": {
    "code": "string",
    "description": "string"
  },
  "operations": ["Operation objects"],
  "metadata": "object",
  "createdAt": "string (datetime)",
  "updatedAt": "string (datetime)",
  "deletedAt": "string (datetime)"
}
```

## Transaction DSL

The Transaction API supports a Domain-Specific Language (DSL) for defining transactions. This DSL provides a concise, readable way to express complex transactions.

### Basic DSL Example

```
transaction "Payment" {
  description "Payment for invoice #123"
  code "PAY_001"
  
  send BRL 1000.00 {
    source {
      from "@person1" {
        chart_of_accounts "1000"
        description "Debit from person1's account"
      }
    }
    
    distribute {
      to "@person2" {
        chart_of_accounts "2000"
        description "Credit to person2's account"
      }
    }
  }
}
```

For more information on the Transaction DSL syntax, please refer to the [Transaction DSL Guide](../transaction-dsl/README.md).

## Pagination

List endpoints support cursor-based pagination using the following parameters:

- `limit`: Number of items to return per page (default: 10)
- `cursor`: Cursor for the next page of results

The response includes:
- `prev_cursor`: Cursor for the previous page (null if on first page)
- `next_cursor`: Cursor for the next page (null if on last page)

## Rate Limiting

The Transaction API implements rate limiting to ensure system stability:

- 100 requests per minute per IP address
- 1000 requests per minute per organization

When rate limits are exceeded, the API returns a 429 Too Many Requests status code with a Retry-After header indicating the number of seconds to wait before retrying.

## Webhooks

The Transaction API can notify external systems of events through webhooks. To register webhooks, please contact the Midaz support team.

Available webhook events:
- `transaction.created`: Triggered when a transaction is created
- `transaction.updated`: Triggered when a transaction is updated
- `transaction.committed`: Triggered when a transaction is committed
- `balance.updated`: Triggered when a balance is updated

## Support

For additional support or questions about the Transaction API, please reach out through:
- GitHub Issues: [File an issue on GitHub](https://github.com/LerianStudio/midaz/issues)
- GitHub Discussions: [Start a discussion](https://github.com/LerianStudio/midaz/discussions)
- Discord: [Join our community](https://discord.gg/qtKU6Zwq5b)
- Email: contact@lerian.studio