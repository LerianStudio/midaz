# Onboarding API Reference

**Navigation:** [Home](../../) > [Components](../) > [Onboarding](./README.md) > API Reference

This document provides a comprehensive reference for the Onboarding Service API. The API follows RESTful principles and is organized hierarchically around the core financial entities: Organizations, Ledgers, Assets, Portfolios, Segments, and Accounts.

## Table of Contents

- [Base URL](#base-url)
- [Authentication](#authentication)
- [Common Headers](#common-headers)
- [Common Query Parameters](#common-query-parameters)
- [Common Response Structure](#common-response-structure)
- [Resources](#resources)
  - [Organizations](#organizations)
  - [Ledgers](#ledgers)
  - [Assets](#assets)
  - [Portfolios](#portfolios)
  - [Segments](#segments)
  - [Accounts](#accounts)
- [Error Responses](#error-responses)
- [Metadata Support](#metadata-support)
- [API Versioning](#api-versioning)

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
- Resource (e.g., organizations, ledgers)
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

## Common Response Structure

All list endpoints return data in the following format for pagination:

```json
{
  "items": [
    {
      // Resource fields
    }
  ],
  "page": 1,
  "limit": 10,
  "total": 100
}
```

## Resources

### Organizations

Organizations are the top-level entities in the system, representing legal entities like companies or individuals.

#### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/organizations` | Create a new organization |
| `GET` | `/organizations` | List all organizations |
| `GET` | `/organizations/{id}` | Get organization by ID |
| `PATCH` | `/organizations/{id}` | Update organization |
| `DELETE` | `/organizations/{id}` | Delete organization |

#### Request Body (Create/Update)

```json
{
  "legalName": "Lerian Studio",
  "parentOrganizationId": "00000000-0000-0000-0000-000000000000",
  "doingBusinessAs": "Lerian Studio",
  "legalDocument": "00000000000000",
  "address": {
    "line1": "Street 1",
    "line2": "Street 2",
    "zipCode": "00000-000",
    "city": "New York",
    "state": "NY",
    "country": "US"
  },
  "status": {
    "code": "ACTIVE",
    "description": "Active status"
  },
  "metadata": {
    "industry": "Technology",
    "employeeCount": "100-500"
  }
}
```

#### Response (Organization)

```json
{
  "id": "00000000-0000-0000-0000-000000000000",
  "parentOrganizationId": "00000000-0000-0000-0000-000000000000",
  "legalName": "Lerian Studio",
  "doingBusinessAs": "Lerian Studio",
  "legalDocument": "00000000000000",
  "address": {
    "line1": "Street 1",
    "line2": "Street 2",
    "zipCode": "00000-000",
    "city": "New York",
    "state": "NY",
    "country": "US"
  },
  "status": {
    "code": "ACTIVE",
    "description": "Active status"
  },
  "createdAt": "2021-01-01T00:00:00Z",
  "updatedAt": "2021-01-01T00:00:00Z",
  "deletedAt": null,
  "metadata": {
    "industry": "Technology",
    "employeeCount": "100-500"
  }
}
```

### Ledgers

Ledgers represent financial record-keeping systems within organizations, used to track financial data.

#### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/organizations/{organization_id}/ledgers` | Create a ledger |
| `GET` | `/organizations/{organization_id}/ledgers` | List all ledgers for an organization |
| `GET` | `/organizations/{organization_id}/ledgers/{id}` | Get ledger by ID |
| `PATCH` | `/organizations/{organization_id}/ledgers/{id}` | Update ledger |
| `DELETE` | `/organizations/{organization_id}/ledgers/{id}` | Delete ledger |

#### Request Body (Create/Update)

```json
{
  "name": "Main Ledger",
  "status": {
    "code": "ACTIVE",
    "description": "Active status"
  },
  "metadata": {
    "purpose": "General accounting",
    "fiscalYear": "2023"
  }
}
```

#### Response (Ledger)

```json
{
  "id": "00000000-0000-0000-0000-000000000000",
  "name": "Main Ledger",
  "organizationId": "00000000-0000-0000-0000-000000000000",
  "status": {
    "code": "ACTIVE",
    "description": "Active status"
  },
  "createdAt": "2021-01-01T00:00:00Z",
  "updatedAt": "2021-01-01T00:00:00Z",
  "deletedAt": null,
  "metadata": {
    "purpose": "General accounting",
    "fiscalYear": "2023"
  }
}
```

### Assets

Assets represent financial instruments within a ledger, such as currencies or cryptocurrencies.

#### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/organizations/{organization_id}/ledgers/{ledger_id}/assets` | Create an asset |
| `GET` | `/organizations/{organization_id}/ledgers/{ledger_id}/assets` | List all assets |
| `GET` | `/organizations/{organization_id}/ledgers/{ledger_id}/assets/{id}` | Get asset by ID |
| `PATCH` | `/organizations/{organization_id}/ledgers/{ledger_id}/assets/{id}` | Update asset |
| `DELETE` | `/organizations/{organization_id}/ledgers/{ledger_id}/assets/{id}` | Delete asset |

#### Request Body (Create/Update)

```json
{
  "name": "Brazilian Real",
  "type": "currency",
  "code": "BRL",
  "status": {
    "code": "ACTIVE",
    "description": "Active status"
  },
  "metadata": {
    "country": "Brazil",
    "denominations": ["0.05", "0.10", "0.25", "0.50", "1.00", "2.00", "5.00", "10.00", "20.00", "50.00", "100.00", "200.00"]
  }
}
```

#### Response (Asset)

```json
{
  "id": "00000000-0000-0000-0000-000000000000",
  "name": "Brazilian Real",
  "type": "currency",
  "code": "BRL",
  "status": {
    "code": "ACTIVE",
    "description": "Active status"
  },
  "ledgerId": "00000000-0000-0000-0000-000000000000",
  "organizationId": "00000000-0000-0000-0000-000000000000",
  "createdAt": "2021-01-01T00:00:00Z",
  "updatedAt": "2021-01-01T00:00:00Z",
  "deletedAt": null,
  "metadata": {
    "country": "Brazil",
    "denominations": ["0.05", "0.10", "0.25", "0.50", "1.00", "2.00", "5.00", "10.00", "20.00", "50.00", "100.00", "200.00"]
  }
}
```

### Portfolios

Portfolios represent collections of accounts for specific purposes or entities.

#### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/organizations/{organization_id}/ledgers/{ledger_id}/portfolios` | Create a portfolio |
| `GET` | `/organizations/{organization_id}/ledgers/{ledger_id}/portfolios` | List all portfolios |
| `GET` | `/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{id}` | Get portfolio by ID |
| `PATCH` | `/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{id}` | Update portfolio |
| `DELETE` | `/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{id}` | Delete portfolio |

#### Request Body (Create/Update)

```json
{
  "entityId": "00000000-0000-0000-0000-000000000000",
  "name": "Customer Portfolio",
  "status": {
    "code": "ACTIVE",
    "description": "Active status"
  },
  "metadata": {
    "customerType": "Premium",
    "region": "Southeast"
  }
}
```

#### Response (Portfolio)

```json
{
  "id": "00000000-0000-0000-0000-000000000000",
  "name": "Customer Portfolio",
  "entityId": "00000000-0000-0000-0000-000000000000",
  "ledgerId": "00000000-0000-0000-0000-000000000000",
  "organizationId": "00000000-0000-0000-0000-000000000000",
  "status": {
    "code": "ACTIVE",
    "description": "Active status"
  },
  "createdAt": "2021-01-01T00:00:00Z",
  "updatedAt": "2021-01-01T00:00:00Z",
  "deletedAt": null,
  "metadata": {
    "customerType": "Premium",
    "region": "Southeast"
  }
}
```

### Segments

Segments represent logical divisions within a ledger, such as business areas or product lines.

#### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/organizations/{organization_id}/ledgers/{ledger_id}/segments` | Create a segment |
| `GET` | `/organizations/{organization_id}/ledgers/{ledger_id}/segments` | List all segments |
| `GET` | `/organizations/{organization_id}/ledgers/{ledger_id}/segments/{id}` | Get segment by ID |
| `PATCH` | `/organizations/{organization_id}/ledgers/{ledger_id}/segments/{id}` | Update segment |
| `DELETE` | `/organizations/{organization_id}/ledgers/{ledger_id}/segments/{id}` | Delete segment |

#### Request Body (Create/Update)

```json
{
  "name": "Retail Banking",
  "status": {
    "code": "ACTIVE",
    "description": "Active status"
  },
  "metadata": {
    "businessUnit": "Consumer",
    "profitCenter": true
  }
}
```

#### Response (Segment)

```json
{
  "id": "00000000-0000-0000-0000-000000000000",
  "name": "Retail Banking",
  "ledgerId": "00000000-0000-0000-0000-000000000000",
  "organizationId": "00000000-0000-0000-0000-000000000000",
  "status": {
    "code": "ACTIVE",
    "description": "Active status"
  },
  "createdAt": "2021-01-01T00:00:00Z",
  "updatedAt": "2021-01-01T00:00:00Z",
  "deletedAt": null,
  "metadata": {
    "businessUnit": "Consumer",
    "profitCenter": true
  }
}
```

### Accounts

Accounts are the basic units for tracking financial resources within a ledger.

#### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/organizations/{organization_id}/ledgers/{ledger_id}/accounts` | Create an account |
| `GET` | `/organizations/{organization_id}/ledgers/{ledger_id}/accounts` | List all accounts |
| `GET` | `/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id}` | Get account by ID |
| `GET` | `/organizations/{organization_id}/ledgers/{ledger_id}/accounts/alias/{alias}` | Get account by alias |
| `PATCH` | `/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id}` | Update account |
| `DELETE` | `/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id}` | Delete account |

#### Request Body (Create/Update)

```json
{
  "name": "Main Checking Account",
  "parentAccountId": "00000000-0000-0000-0000-000000000000",
  "entityId": "00000000-0000-0000-0000-000000000000",
  "assetCode": "BRL",
  "portfolioId": "00000000-0000-0000-0000-000000000000",
  "segmentId": "00000000-0000-0000-0000-000000000000",
  "status": {
    "code": "ACTIVE",
    "description": "Active status"
  },
  "alias": "@customer1",
  "type": "checking",
  "metadata": {
    "accountNumber": "12345-6",
    "branch": "0001",
    "openingDate": "2021-01-01"
  }
}
```

#### Response (Account)

```json
{
  "id": "00000000-0000-0000-0000-000000000000",
  "name": "Main Checking Account",
  "parentAccountId": "00000000-0000-0000-0000-000000000000",
  "entityId": "00000000-0000-0000-0000-000000000000",
  "assetCode": "BRL",
  "organizationId": "00000000-0000-0000-0000-000000000000",
  "ledgerId": "00000000-0000-0000-0000-000000000000",
  "portfolioId": "00000000-0000-0000-0000-000000000000",
  "segmentId": "00000000-0000-0000-0000-000000000000",
  "status": {
    "code": "ACTIVE",
    "description": "Active status"
  },
  "alias": "@customer1",
  "type": "checking",
  "createdAt": "2021-01-01T00:00:00Z",
  "updatedAt": "2021-01-01T00:00:00Z",
  "deletedAt": null,
  "metadata": {
    "accountNumber": "12345-6",
    "branch": "0001",
    "openingDate": "2021-01-01"
  }
}
```

## Error Responses

The API returns structured error responses with the following format:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Invalid input parameters",
    "details": [
      {
        "field": "name",
        "message": "Name is required"
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
| 409 | Conflict - Resource conflict |
| 500 | Internal Server Error - Server-side error |

## Metadata Support

All resources support flexible metadata as custom key-value pairs:

- Stored in MongoDB for flexibility
- Keys limited to 100 characters
- Values limited to 2000 characters
- Queryable via API with the `metadata` parameter
- No schema constraints for maximum flexibility

Metadata can be used for:

- Storing custom attributes
- Supporting business-specific data
- Extending the data model without schema changes
- Implementing tagging/categorization

## API Versioning

- The API is versioned through the URL path (/v1)
- Breaking changes will result in a new version number
- Backward compatibility is maintained within versions