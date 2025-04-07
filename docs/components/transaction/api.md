# Transaction API Reference

**Navigation:** [Home](../../) > [Components](../) > [Transaction](./README.md) > API Reference

This document provides a comprehensive reference for the Transaction Service API. The API follows RESTful principles and is organized around the core financial processing entities: Transactions, Operations, Asset Rates, and Balances.

## Table of Contents

- [Base URL](#base-url)
- [Authentication](#authentication)
- [Common Headers](#common-headers)
- [Common Query Parameters](#common-query-parameters)
- [Resources](#resources)
  - [Transactions](#transactions)
  - [Operations](#operations)
  - [Asset Rates](#asset-rates)
  - [Balances](#balances)
- [Error Responses](#error-responses)
- [Metadata Support](#metadata-support)
- [Transaction Domain-Specific Language (DSL)](#transaction-domain-specific-language-dsl)

## Base URL

The base URL for all API endpoints is:

```
/v1
```

## Authentication

All API endpoints require authentication using a Bearer token:

```
Authorization: Bearer {token}
```

Permissions are managed through a resource-based authorization model, where access is controlled at the level of:
- Resource (e.g., transactions, operations, balances)
- Action (e.g., post, get, patch, delete)

## Common Headers

| Header | Description | Required |
|--------|-------------|----------|
| `Authorization` | Bearer token for authentication | Yes |
| `X-Request-Id` | Request ID for tracing | No |
| `Content-Type` | Set to `application/json` for requests with body | Yes (for POST/PATCH) |

## Common Query Parameters

The following query parameters are available for list endpoints:

| Parameter | Description | Default | Example |
|-----------|-------------|---------|---------|
| `limit` | Maximum number of records to return | 10 | `?limit=25` |
| `page` | Page number for pagination | 1 | `?page=2` |
| `metadata` | JSON string for filtering by metadata fields | N/A | `?metadata={"customField":"value"}` |
| `start_date` | Filter by creation date (format: YYYY-MM-DD) | N/A | `?start_date=2023-01-01` |
| `end_date` | Filter by creation date (format: YYYY-MM-DD) | N/A | `?end_date=2023-12-31` |
| `sort_order` | Sort direction (asc/desc) | asc | `?sort_order=desc` |

## Resources

### Transactions

Transactions are the core financial entries representing the movement of assets between accounts within a ledger.

#### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/organizations/{organization_id}/ledgers/{ledger_id}/transactions/dsl` | Create a transaction using DSL format |
| `POST` | `/organizations/{organization_id}/ledgers/{ledger_id}/transactions/json` | Create a transaction using JSON format |
| `POST` | `/organizations/{organization_id}/ledgers/{ledger_id}/transactions/templates` | Create a transaction from a template |
| `POST` | `/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/commit` | Commit a pending transaction |
| `POST` | `/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/revert` | Revert a transaction |
| `PATCH` | `/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}` | Update a transaction |
| `GET` | `/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}` | Get a specific transaction |
| `GET` | `/organizations/{organization_id}/ledgers/{ledger_id}/transactions` | List all transactions |

#### Create Transaction (JSON)

**Request Body:**

```json
{
  "chartOfAccountsGroupName": "default",
  "description": "Payment for services",
  "code": "PAY-001",
  "pending": false,
  "metadata": {
    "invoiceNumber": "INV-123",
    "department": "Engineering"
  },
  "send": {
    "amount": 100,
    "scale": 2,
    "assetCode": "BRL",
    "source": {
      "from": [
        {
          "alias": "@customer1",
          "amount": 100,
          "scale": 2
        }
      ]
    },
    "distribute": {
      "to": [
        {
          "alias": "@merchant1",
          "amount": 100,
          "scale": 2
        }
      ]
    }
  }
}
```

**Response:**

```json
{
  "id": "00000000-0000-0000-0000-000000000000",
  "description": "Payment for services",
  "template": "simple_transfer",
  "status": {
    "code": "COMMITTED",
    "description": "Transaction successfully processed"
  },
  "amount": 10000,
  "amountScale": 2,
  "assetCode": "BRL",
  "chartOfAccountsGroupName": "default",
  "source": ["@customer1"],
  "destination": ["@merchant1"],
  "ledgerId": "00000000-0000-0000-0000-000000000000",
  "organizationId": "00000000-0000-0000-0000-000000000000",
  "createdAt": "2023-05-01T14:30:00Z",
  "updatedAt": "2023-05-01T14:30:00Z",
  "metadata": {
    "invoiceNumber": "INV-123",
    "department": "Engineering"
  },
  "operations": [
    {
      "id": "00000000-0000-0000-0000-000000000000",
      "type": "DEBIT",
      "accountId": "00000000-0000-0000-0000-000000000000",
      "amount": 10000,
      "amountScale": 2,
      "assetCode": "BRL",
      "status": {
        "code": "COMPLETED",
        "description": "Operation completed successfully"
      }
    },
    {
      "id": "11111111-1111-1111-1111-111111111111",
      "type": "CREDIT",
      "accountId": "11111111-1111-1111-1111-111111111111",
      "amount": 10000,
      "amountScale": 2,
      "assetCode": "BRL",
      "status": {
        "code": "COMPLETED",
        "description": "Operation completed successfully"
      }
    }
  ]
}
```

#### Create Transaction (DSL)

**Request Body (multipart/form-data):**

File upload with DSL content:

```
TRANSACTION
  CHART_OF_ACCOUNTS <chart_id>
  DESCRIPTION "Payment for services"
  CODE "PAY-001"
  SEND BRL 10000 2
    FROM @customer1 AMOUNT BRL 10000 2
    DISTRIBUTE TO @merchant1 AMOUNT BRL 10000 2
```

**Response:**
Same as JSON create transaction response.

#### Create Transaction (Template)

**Request Body:**

```json
{
  "transactionType": "00000000-0000-0000-0000-000000000000",
  "transactionTypeCode": "PAYMENT",
  "variables": {
    "sourceAccount": "@customer1",
    "destinationAccount": "@merchant1",
    "amount": 100.00,
    "currency": "BRL"
  }
}
```

**Response:**
Same as JSON create transaction response.

#### Update Transaction

**Request Body:**

```json
{
  "description": "Updated payment description",
  "metadata": {
    "invoiceNumber": "INV-123-UPDATED",
    "department": "Engineering"
  }
}
```

**Response:**
Updated transaction object.

### Operations

Operations represent individual debit or credit entries that make up a transaction, affecting account balances.

#### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/operations` | List operations for an account |
| `GET` | `/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/operations/{operation_id}` | Get a specific operation |
| `PATCH` | `/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/operations/{operation_id}` | Update an operation |

#### List Account Operations

**Response:**

```json
{
  "items": [
    {
      "id": "00000000-0000-0000-0000-000000000000",
      "transactionId": "00000000-0000-0000-0000-000000000000",
      "type": "DEBIT",
      "accountId": "00000000-0000-0000-0000-000000000000",
      "amount": 10000,
      "amountScale": 2,
      "assetCode": "BRL",
      "description": "Payment from customer",
      "balanceBefore": {
        "id": "00000000-0000-0000-0000-000000000000",
        "accountId": "00000000-0000-0000-0000-000000000000",
        "available": 50000,
        "onHold": 0,
        "availableScale": 2,
        "onHoldScale": 2,
        "assetCode": "BRL"
      },
      "balanceAfter": {
        "id": "00000000-0000-0000-0000-000000000000",
        "accountId": "00000000-0000-0000-0000-000000000000",
        "available": 40000,
        "onHold": 0,
        "availableScale": 2,
        "onHoldScale": 2,
        "assetCode": "BRL"
      },
      "status": {
        "code": "COMPLETED",
        "description": "Operation completed successfully"
      },
      "createdAt": "2023-05-01T14:30:00Z",
      "updatedAt": "2023-05-01T14:30:00Z",
      "metadata": {
        "category": "payment"
      }
    }
  ],
  "page": 1,
  "limit": 10
}
```

#### Update Operation

**Request Body:**

```json
{
  "description": "Updated operation description",
  "metadata": {
    "category": "subscription_payment"
  }
}
```

**Response:**
Updated operation object.

### Asset Rates

Asset Rates represent exchange rates between different assets, allowing for multi-currency transactions.

#### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `PUT` | `/organizations/{organization_id}/ledgers/{ledger_id}/asset-rates` | Create or update an asset rate |
| `GET` | `/organizations/{organization_id}/ledgers/{ledger_id}/asset-rates/{external_id}` | Get an asset rate by external ID |
| `GET` | `/organizations/{organization_id}/ledgers/{ledger_id}/asset-rates/from/{asset_code}` | List asset rates for a source asset |

#### Create/Update Asset Rate

**Request Body:**

```json
{
  "externalId": "USD-BRL-20230501",
  "from": "USD",
  "to": "BRL",
  "rate": 495,
  "rateScale": 2,
  "ttl": 3600,
  "metadata": {
    "source": "central_bank",
    "market": "commercial"
  }
}
```

**Response:**

```json
{
  "id": "00000000-0000-0000-0000-000000000000",
  "externalId": "USD-BRL-20230501",
  "from": "USD",
  "to": "BRL",
  "rate": 495,
  "rateScale": 2,
  "ttl": 3600,
  "organizationId": "00000000-0000-0000-0000-000000000000",
  "ledgerId": "00000000-0000-0000-0000-000000000000",
  "createdAt": "2023-05-01T14:30:00Z",
  "updatedAt": "2023-05-01T14:30:00Z",
  "expiresAt": "2023-05-01T15:30:00Z",
  "metadata": {
    "source": "central_bank",
    "market": "commercial"
  }
}
```

#### List Asset Rates by Source Asset

**Query Parameters:**
- `to`: Filter by target asset code (optional)

**Response:**

```json
{
  "items": [
    {
      "id": "00000000-0000-0000-0000-000000000000",
      "externalId": "USD-BRL-20230501",
      "from": "USD",
      "to": "BRL",
      "rate": 495,
      "rateScale": 2,
      "ttl": 3600,
      "organizationId": "00000000-0000-0000-0000-000000000000",
      "ledgerId": "00000000-0000-0000-0000-000000000000",
      "createdAt": "2023-05-01T14:30:00Z",
      "updatedAt": "2023-05-01T14:30:00Z",
      "expiresAt": "2023-05-01T15:30:00Z",
      "metadata": {
        "source": "central_bank",
        "market": "commercial"
      }
    }
  ],
  "page": 1,
  "limit": 10
}
```

### Balances

Balances represent the current financial position of an account for a specific asset.

#### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/organizations/{organization_id}/ledgers/{ledger_id}/balances` | List all balances |
| `GET` | `/organizations/{organization_id}/ledgers/{ledger_id}/balances/{balance_id}` | Get a specific balance |
| `GET` | `/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/balances` | List balances for an account |
| `PATCH` | `/organizations/{organization_id}/ledgers/{ledger_id}/balances/{balance_id}` | Update a balance |
| `DELETE` | `/organizations/{organization_id}/ledgers/{ledger_id}/balances/{balance_id}` | Delete a balance |

#### List Account Balances

**Response:**

```json
{
  "items": [
    {
      "id": "00000000-0000-0000-0000-000000000000",
      "accountId": "00000000-0000-0000-0000-000000000000",
      "available": 40000,
      "onHold": 0,
      "availableScale": 2,
      "onHoldScale": 2,
      "assetCode": "BRL",
      "allowSending": true,
      "allowReceiving": true,
      "version": 5,
      "organizationId": "00000000-0000-0000-0000-000000000000",
      "ledgerId": "00000000-0000-0000-0000-000000000000",
      "createdAt": "2023-05-01T14:30:00Z",
      "updatedAt": "2023-05-01T14:30:00Z",
      "metadata": {
        "accountType": "checking"
      }
    }
  ],
  "page": 1,
  "limit": 10
}
```

#### Update Balance

**Request Body:**

```json
{
  "allowSending": true,
  "allowReceiving": true,
  "metadata": {
    "accountType": "savings"
  }
}
```

**Response:**
Updated balance object.

## Error Responses

The API returns structured error responses with the following format:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Invalid input parameters",
    "details": [
      {
        "field": "send.amount",
        "message": "Amount must be greater than zero"
      }
    ]
  }
}
```

### Common Status Codes

| Status Code | Description |
|-------------|-------------|
| 201 | Resource successfully created |
| 200 | Request successful |
| 204 | Resource successfully deleted |
| 400 | Bad Request - Invalid input or validation errors |
| 401 | Unauthorized - Authentication failed |
| 403 | Forbidden - User lacks permissions |
| 404 | Not Found - Resource not found |
| 409 | Conflict - Resource conflict or optimistic concurrency violation |
| 422 | Unprocessable Entity - Business rule violation |
| 500 | Internal Server Error - Server-side error |

## Metadata Support

All resources support flexible metadata as custom key-value pairs:

- Stored in MongoDB for flexibility
- Keys limited to 100 characters
- Values limited to 2000 characters
- Queryable via API with the `metadata` parameter
- No nested objects for query performance

## Transaction Domain-Specific Language (DSL)

The Transaction API supports a Domain-Specific Language (DSL) for defining transactions, making it easier to express financial movements in a readable format. The DSL follows this general structure:

```
TRANSACTION
  CHART_OF_ACCOUNTS <chart_id>
  DESCRIPTION <description>
  CODE <code>
  SEND <asset_code> <amount> <scale>
    FROM <account_alias> AMOUNT <asset_code> <amount> <scale>
    DISTRIBUTE TO <account_alias> AMOUNT <asset_code> <amount> <scale>
```

### DSL Example

```
TRANSACTION
  CHART_OF_ACCOUNTS default
  DESCRIPTION "Payment for services"
  CODE "PAY-001"
  SEND BRL 10000 2
    FROM @customer1 AMOUNT BRL 10000 2
    DISTRIBUTE TO @merchant1 AMOUNT BRL 10000 2
```

The DSL enforces double-entry accounting rules, ensuring that debits (FROM) always equal credits (TO) within a transaction.