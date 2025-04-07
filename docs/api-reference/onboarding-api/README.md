# Onboarding API

**Navigation:** [Home](../../) > [API Reference](../) > Onboarding API

This documentation provides a comprehensive reference for the Midaz Onboarding API, which handles the creation and management of all financial entities in the platform, including organizations, ledgers, assets, segments, portfolios, and accounts.

## Overview

The Onboarding API is a RESTful API that manages the financial entity hierarchy within the Midaz platform. It follows a structured entity model where organizations contain ledgers, which then contain various financial entities like assets, segments, portfolios, and accounts.

- **Base URL**: 
  - Production: `https://api.midaz.io/onboarding/v1`
  - Development: `http://localhost:3000/onboarding/v1`
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
| 422 | Unprocessable Entity - The request was well-formed but cannot be processed |
| 500 | Internal Server Error - An error occurred on the server |

## Error Response Structure

All error responses follow a standardized format:

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

## Resource Categories

The Onboarding API is organized into the following hierarchical resource categories:

- [Organizations](#organizations): Top-level entities that group ledgers and other resources
- [Ledgers](#ledgers): Financial ledgers within organizations that contain financial entities
- [Assets](#assets): Currencies or financial instruments used within ledgers
- [Segments](#segments): Business segments used to categorize accounts
- [Portfolios](#portfolios): Collections of accounts for financial grouping
- [Accounts](#accounts): Individual financial accounts that can hold balances

## Entity Hierarchy

The Midaz entity hierarchy follows this structure:

```
Organization
├── Ledger
│   ├── Asset
│   ├── Segment
│   ├── Portfolio
│   └── Account
```

Each entity can have metadata associated with it for flexible extension of attributes.

## Organizations

Organizations are the top-level entities in the Midaz platform hierarchy.

### List Organizations

Retrieves a paginated list of organizations.

**Endpoint**: `GET /organizations`

**Query Parameters**:
- `metadata` (string, optional): JSON string to filter organizations by metadata fields
- `limit` (integer, optional): Maximum number of records per page (default: 10, max: 100)
- `page` (integer, optional): Page number for pagination (default: 1)
- `start_date` (string, optional): Filter by creation date (format: YYYY-MM-DD)
- `end_date` (string, optional): Filter by creation date (format: YYYY-MM-DD)

**Response**:
```json
{
  "items": [
    {
      "id": "00000000-0000-0000-0000-000000000000",
      "name": "Example Organization",
      "code": "EXO",
      "description": "This is an example organization",
      "metadata": {
        "industry": "Financial Services",
        "region": "North America"
      },
      "created_at": "2021-01-01T00:00:00Z",
      "updated_at": "2021-01-01T00:00:00Z"
    }
  ],
  "total": 1,
  "page": 1,
  "limit": 10,
  "pages": 1
}
```

### Create Organization

Creates a new organization.

**Endpoint**: `POST /organizations`

**Request Body**:
```json
{
  "name": "Example Organization",
  "code": "EXO",
  "description": "This is an example organization",
  "metadata": {
    "industry": "Financial Services",
    "region": "North America"
  }
}
```

**Response**:
```json
{
  "id": "00000000-0000-0000-0000-000000000000",
  "name": "Example Organization",
  "code": "EXO",
  "description": "This is an example organization",
  "metadata": {
    "industry": "Financial Services",
    "region": "North America"
  },
  "created_at": "2021-01-01T00:00:00Z",
  "updated_at": "2021-01-01T00:00:00Z"
}
```

### Get Organization

Retrieves a specific organization by ID.

**Endpoint**: `GET /organizations/{organization_id}`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier

**Response**: Returns a single organization object

### Update Organization

Updates an existing organization.

**Endpoint**: `PATCH /organizations/{organization_id}`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier

**Request Body**:
```json
{
  "name": "Updated Organization Name",
  "description": "Updated organization description",
  "metadata": {
    "industry": "Financial Services",
    "region": "Europe"
  }
}
```

**Response**: Returns the updated organization object

### Delete Organization

Deletes an organization.

**Endpoint**: `DELETE /organizations/{organization_id}`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier

**Response**: Returns 204 No Content on success

## Ledgers

Ledgers are financial books within organizations, containing various financial entities.

### List Ledgers

Retrieves a paginated list of ledgers for an organization.

**Endpoint**: `GET /organizations/{organization_id}/ledgers`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier

**Query Parameters**:
- `metadata` (string, optional): JSON string to filter ledgers by metadata fields
- `limit` (integer, optional): Maximum number of records per page (default: 10, max: 100)
- `page` (integer, optional): Page number for pagination (default: 1)
- `start_date` (string, optional): Filter by creation date (format: YYYY-MM-DD)
- `end_date` (string, optional): Filter by creation date (format: YYYY-MM-DD)

**Response**:
```json
{
  "items": [
    {
      "id": "00000000-0000-0000-0000-000000000000",
      "organization_id": "00000000-0000-0000-0000-000000000000",
      "name": "Main Ledger",
      "code": "MAIN",
      "description": "Main financial ledger",
      "metadata": {
        "type": "General",
        "fiscal_year": "2025"
      },
      "created_at": "2021-01-01T00:00:00Z",
      "updated_at": "2021-01-01T00:00:00Z"
    }
  ],
  "total": 1,
  "page": 1,
  "limit": 10,
  "pages": 1
}
```

### Create Ledger

Creates a new ledger within an organization.

**Endpoint**: `POST /organizations/{organization_id}/ledgers`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier

**Request Body**:
```json
{
  "name": "Main Ledger",
  "code": "MAIN",
  "description": "Main financial ledger",
  "metadata": {
    "type": "General",
    "fiscal_year": "2025"
  }
}
```

**Response**: Returns the created ledger object

### Get Ledger

Retrieves a specific ledger by ID.

**Endpoint**: `GET /organizations/{organization_id}/ledgers/{ledger_id}`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier

**Response**: Returns a single ledger object

### Update Ledger

Updates an existing ledger.

**Endpoint**: `PATCH /organizations/{organization_id}/ledgers/{ledger_id}`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier

**Request Body**:
```json
{
  "name": "Updated Ledger Name",
  "description": "Updated ledger description",
  "metadata": {
    "type": "General",
    "fiscal_year": "2026"
  }
}
```

**Response**: Returns the updated ledger object

### Delete Ledger

Deletes a ledger.

**Endpoint**: `DELETE /organizations/{organization_id}/ledgers/{ledger_id}`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier

**Response**: Returns 204 No Content on success

## Assets

Assets represent currencies or financial instruments used within ledgers.

### List Assets

Retrieves a paginated list of assets for a ledger.

**Endpoint**: `GET /organizations/{organization_id}/ledgers/{ledger_id}/assets`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier

**Query Parameters**:
- `metadata` (string, optional): JSON string to filter assets by metadata fields
- `limit` (integer, optional): Maximum number of records per page (default: 10, max: 100)
- `page` (integer, optional): Page number for pagination (default: 1)
- `start_date` (string, optional): Filter by creation date (format: YYYY-MM-DD)
- `end_date` (string, optional): Filter by creation date (format: YYYY-MM-DD)

**Response**:
```json
{
  "items": [
    {
      "id": "00000000-0000-0000-0000-000000000000",
      "organization_id": "00000000-0000-0000-0000-000000000000",
      "ledger_id": "00000000-0000-0000-0000-000000000000",
      "code": "USD",
      "name": "US Dollar",
      "symbol": "$",
      "decimals": 2,
      "description": "United States Dollar",
      "metadata": {
        "country": "United States",
        "type": "fiat"
      },
      "created_at": "2021-01-01T00:00:00Z",
      "updated_at": "2021-01-01T00:00:00Z"
    }
  ],
  "total": 1,
  "page": 1,
  "limit": 10,
  "pages": 1
}
```

### Create Asset

Creates a new asset within a ledger.

**Endpoint**: `POST /organizations/{organization_id}/ledgers/{ledger_id}/assets`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier

**Request Body**:
```json
{
  "code": "USD",
  "name": "US Dollar",
  "symbol": "$",
  "decimals": 2,
  "description": "United States Dollar",
  "metadata": {
    "country": "United States",
    "type": "fiat"
  }
}
```

**Response**: Returns the created asset object

### Get Asset

Retrieves a specific asset by ID.

**Endpoint**: `GET /organizations/{organization_id}/ledgers/{ledger_id}/assets/{asset_id}`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier
- `asset_id` (string, required): Asset identifier

**Response**: Returns a single asset object

### Update Asset

Updates an existing asset.

**Endpoint**: `PATCH /organizations/{organization_id}/ledgers/{ledger_id}/assets/{asset_id}`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier
- `asset_id` (string, required): Asset identifier

**Request Body**:
```json
{
  "name": "Updated Asset Name",
  "description": "Updated asset description",
  "metadata": {
    "country": "United States",
    "type": "fiat",
    "status": "active"
  }
}
```

**Response**: Returns the updated asset object

### Delete Asset

Deletes an asset.

**Endpoint**: `DELETE /organizations/{organization_id}/ledgers/{ledger_id}/assets/{asset_id}`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier
- `asset_id` (string, required): Asset identifier

**Response**: Returns 204 No Content on success

## Segments

Segments represent business segments used to categorize accounts.

### List Segments

Retrieves a paginated list of segments for a ledger.

**Endpoint**: `GET /organizations/{organization_id}/ledgers/{ledger_id}/segments`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier

**Query Parameters**: Similar to other list endpoints

**Response**:
```json
{
  "items": [
    {
      "id": "00000000-0000-0000-0000-000000000000",
      "organization_id": "00000000-0000-0000-0000-000000000000",
      "ledger_id": "00000000-0000-0000-0000-000000000000",
      "name": "Retail Banking",
      "code": "RETAIL",
      "description": "Retail banking segment",
      "metadata": {
        "risk_level": "low",
        "customer_type": "individual"
      },
      "created_at": "2021-01-01T00:00:00Z",
      "updated_at": "2021-01-01T00:00:00Z"
    }
  ],
  "total": 1,
  "page": 1,
  "limit": 10,
  "pages": 1
}
```

### Create Segment

Creates a new segment within a ledger.

**Endpoint**: `POST /organizations/{organization_id}/ledgers/{ledger_id}/segments`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier

**Request Body**:
```json
{
  "name": "Retail Banking",
  "code": "RETAIL",
  "description": "Retail banking segment",
  "metadata": {
    "risk_level": "low",
    "customer_type": "individual"
  }
}
```

**Response**: Returns the created segment object

### Get Segment

Retrieves a specific segment by ID.

**Endpoint**: `GET /organizations/{organization_id}/ledgers/{ledger_id}/segments/{segment_id}`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier
- `segment_id` (string, required): Segment identifier

**Response**: Returns a single segment object

### Update Segment

Updates an existing segment.

**Endpoint**: `PATCH /organizations/{organization_id}/ledgers/{ledger_id}/segments/{segment_id}`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier
- `segment_id` (string, required): Segment identifier

**Request Body**:
```json
{
  "name": "Updated Segment Name",
  "description": "Updated segment description",
  "metadata": {
    "risk_level": "medium",
    "customer_type": "individual"
  }
}
```

**Response**: Returns the updated segment object

### Delete Segment

Deletes a segment.

**Endpoint**: `DELETE /organizations/{organization_id}/ledgers/{ledger_id}/segments/{segment_id}`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier
- `segment_id` (string, required): Segment identifier

**Response**: Returns 204 No Content on success

## Portfolios

Portfolios represent collections of accounts for financial grouping.

### List Portfolios

Retrieves a paginated list of portfolios for a ledger.

**Endpoint**: `GET /organizations/{organization_id}/ledgers/{ledger_id}/portfolios`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier

**Query Parameters**: Similar to other list endpoints

**Response**:
```json
{
  "items": [
    {
      "id": "00000000-0000-0000-0000-000000000000",
      "organization_id": "00000000-0000-0000-0000-000000000000",
      "ledger_id": "00000000-0000-0000-0000-000000000000",
      "segment_id": "00000000-0000-0000-0000-000000000000",
      "name": "High-Value Customers",
      "code": "HVC",
      "description": "Portfolio for high-value customers",
      "metadata": {
        "investment_strategy": "balanced",
        "min_value": 100000
      },
      "created_at": "2021-01-01T00:00:00Z",
      "updated_at": "2021-01-01T00:00:00Z"
    }
  ],
  "total": 1,
  "page": 1,
  "limit": 10,
  "pages": 1
}
```

### Create Portfolio

Creates a new portfolio within a ledger.

**Endpoint**: `POST /organizations/{organization_id}/ledgers/{ledger_id}/portfolios`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier

**Request Body**:
```json
{
  "segment_id": "00000000-0000-0000-0000-000000000000",
  "name": "High-Value Customers",
  "code": "HVC",
  "description": "Portfolio for high-value customers",
  "metadata": {
    "investment_strategy": "balanced",
    "min_value": 100000
  }
}
```

**Response**: Returns the created portfolio object

### Get Portfolio

Retrieves a specific portfolio by ID.

**Endpoint**: `GET /organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{portfolio_id}`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier
- `portfolio_id` (string, required): Portfolio identifier

**Response**: Returns a single portfolio object

### Update Portfolio

Updates an existing portfolio.

**Endpoint**: `PATCH /organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{portfolio_id}`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier
- `portfolio_id` (string, required): Portfolio identifier

**Request Body**:
```json
{
  "name": "Updated Portfolio Name",
  "description": "Updated portfolio description",
  "metadata": {
    "investment_strategy": "aggressive",
    "min_value": 150000
  }
}
```

**Response**: Returns the updated portfolio object

### Delete Portfolio

Deletes a portfolio.

**Endpoint**: `DELETE /organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{portfolio_id}`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier
- `portfolio_id` (string, required): Portfolio identifier

**Response**: Returns 204 No Content on success

## Accounts

Accounts represent individual financial accounts that can hold balances.

### List Accounts

Retrieves a paginated list of accounts for a ledger.

**Endpoint**: `GET /organizations/{organization_id}/ledgers/{ledger_id}/accounts`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier

**Query Parameters**:
- `metadata` (string, optional): JSON string to filter accounts by metadata fields
- `portfolio_id` (string, optional): Filter by portfolio ID
- `segment_id` (string, optional): Filter by segment ID
- `limit` (integer, optional): Maximum number of records per page (default: 10, max: 100)
- `page` (integer, optional): Page number for pagination (default: 1)
- `start_date` (string, optional): Filter by creation date (format: YYYY-MM-DD)
- `end_date` (string, optional): Filter by creation date (format: YYYY-MM-DD)

**Response**:
```json
{
  "items": [
    {
      "id": "00000000-0000-0000-0000-000000000000",
      "organization_id": "00000000-0000-0000-0000-000000000000",
      "ledger_id": "00000000-0000-0000-0000-000000000000",
      "portfolio_id": "00000000-0000-0000-0000-000000000000",
      "segment_id": "00000000-0000-0000-0000-000000000000",
      "name": "John Doe Checking",
      "number": "1234567890",
      "alias": "@johndoe",
      "type": "checking",
      "status": "active",
      "description": "Primary checking account for John Doe",
      "metadata": {
        "customer_id": "CUST-123",
        "risk_score": 85
      },
      "created_at": "2021-01-01T00:00:00Z",
      "updated_at": "2021-01-01T00:00:00Z"
    }
  ],
  "total": 1,
  "page": 1,
  "limit": 10,
  "pages": 1
}
```

### Create Account

Creates a new account within a ledger.

**Endpoint**: `POST /organizations/{organization_id}/ledgers/{ledger_id}/accounts`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier

**Request Body**:
```json
{
  "portfolio_id": "00000000-0000-0000-0000-000000000000",
  "segment_id": "00000000-0000-0000-0000-000000000000",
  "name": "John Doe Checking",
  "number": "1234567890",
  "alias": "@johndoe",
  "type": "checking",
  "status": "active",
  "description": "Primary checking account for John Doe",
  "metadata": {
    "customer_id": "CUST-123",
    "risk_score": 85
  }
}
```

**Response**: Returns the created account object

### Get Account

Retrieves a specific account by ID.

**Endpoint**: `GET /organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier
- `account_id` (string, required): Account identifier

**Response**: Returns a single account object

### Get Account by Alias

Retrieves a specific account by its alias.

**Endpoint**: `GET /organizations/{organization_id}/ledgers/{ledger_id}/accounts/alias/{alias}`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier
- `alias` (string, required): Account alias (e.g., "@johndoe")

**Response**: Returns a single account object

### Update Account

Updates an existing account.

**Endpoint**: `PATCH /organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier
- `account_id` (string, required): Account identifier

**Request Body**:
```json
{
  "name": "Updated Account Name",
  "status": "inactive",
  "description": "Updated account description",
  "metadata": {
    "customer_id": "CUST-123",
    "risk_score": 90,
    "notes": "Account pending verification"
  }
}
```

**Response**: Returns the updated account object

### Delete Account

Deletes an account.

**Endpoint**: `DELETE /organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- `ledger_id` (string, required): Ledger identifier
- `account_id` (string, required): Account identifier

**Response**: Returns 204 No Content on success

## Metadata Management

All entities in the Onboarding API support metadata fields which allow for flexible extension of attributes. Metadata is stored as a JSON object and can be updated independently from the main entity attributes.

### Update Entity Metadata

Updates the metadata for any entity type.

**Endpoint**: `PATCH /organizations/{organization_id}/[entity_type]/{entity_id}/metadata`

Where `[entity_type]` can be:
- For organization: (no additional path segment needed)
- For ledger: `ledgers`
- For asset: `ledgers/{ledger_id}/assets`
- For segment: `ledgers/{ledger_id}/segments`
- For portfolio: `ledgers/{ledger_id}/portfolios`
- For account: `ledgers/{ledger_id}/accounts`

**Path Parameters**:
- `organization_id` (string, required): Organization identifier
- Additional path parameters based on entity type

**Request Body**:
```json
{
  "metadata": {
    "key1": "value1",
    "key2": "value2",
    "nested": {
      "subkey": "subvalue"
    }
  }
}
```

**Response**: Returns the updated entity with the new metadata

## Pagination

List endpoints support page-based pagination using the following parameters:

- `limit`: Maximum number of items to return per page (default: 10, max: 100)
- `page`: Page number (starting from 1)

The response includes:
- `total`: Total number of items matching the query
- `pages`: Total number of pages available
- `page`: Current page number
- `limit`: Items per page

## Rate Limiting

The Onboarding API implements rate limiting to ensure system stability:

- 100 requests per minute per IP address
- 1000 requests per minute per organization

When rate limits are exceeded, the API returns a 429 Too Many Requests status code with a Retry-After header indicating the number of seconds to wait before retrying.

## Webhooks

The Onboarding API can notify external systems of events through webhooks. To register webhooks, please contact the Midaz support team.

Available webhook events include:
- `organization.created`: Triggered when an organization is created
- `organization.updated`: Triggered when an organization is updated
- `ledger.created`: Triggered when a ledger is created
- `ledger.updated`: Triggered when a ledger is updated
- `account.created`: Triggered when an account is created
- `account.updated`: Triggered when an account is updated

## Support

For additional support or questions about the Onboarding API, please reach out through:
- GitHub Issues: [File an issue on GitHub](https://github.com/LerianStudio/midaz/issues)
- GitHub Discussions: [Start a discussion](https://github.com/LerianStudio/midaz/discussions)
- Discord: [Join our community](https://discord.gg/qtKU6Zwq5b)
- Email: contact@lerian.studio