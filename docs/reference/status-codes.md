# Status Codes

**Navigation:** [Home](../../) > [Reference Materials](../README.md) > Status Codes

This document provides a comprehensive reference for status codes used throughout the Midaz platform.

## Entity Status Codes

These status codes are used for organizations, ledgers, assets, portfolios, segments, accounts, and other entities in the system.

| Status Code | Description | Usage |
|-------------|-------------|-------|
| `ACTIVE` | Entity is active and available for use | Default status for most entities after creation |
| `INACTIVE` | Entity exists but is not currently active | Temporarily disabled entities |
| `PENDING` | Entity is waiting for activation or approval | Newly created entities that require verification |
| `SUSPENDED` | Entity is temporarily blocked from use | Entities with compliance or security issues |
| `DELETED` | Entity has been soft-deleted | Entities that have been removed but retained for record-keeping |
| `BLOCKED` | Entity is permanently blocked from use | Entities with permanent restrictions |
| `ARCHIVED` | Entity is no longer active but preserved for historical purposes | Historical records that should not be used for new transactions |

## Transaction Status Codes

These status codes represent the state of transactions in the system.

| Status Code | Description | Usage |
|-------------|-------------|-------|
| `PENDING` | Transaction is created but not yet processed | Initial state for transactions |
| `PROCESSING` | Transaction is currently being processed | Transactions in the execution phase |
| `COMPLETED` | Transaction has been successfully completed | Successful transactions |
| `FAILED` | Transaction processing has failed | Transactions that encountered errors |
| `REVERSED` | Transaction has been reversed | Transactions that have been reversed by a subsequent transaction |
| `CANCELLED` | Transaction was cancelled before processing | Transactions cancelled by user or system |
| `ON_HOLD` | Transaction is on hold awaiting further action | Transactions requiring additional verification |

## Operation Status Codes

These status codes represent the state of individual operations within transactions.

| Status Code | Description | Usage |
|-------------|-------------|-------|
| `PENDING` | Operation is created but not yet processed | Initial state for operations |
| `COMPLETED` | Operation has been successfully completed | Successful operations |
| `FAILED` | Operation processing has failed | Operations that encountered errors |
| `REVERSED` | Operation has been reversed | Operations that have been reversed by a subsequent operation |

## Balance Status Codes

These status codes represent the state of account balances.

| Status Code | Description | Usage |
|-------------|-------------|-------|
| `ACTIVE` | Balance is active and can be used for transactions | Normal functioning balances |
| `FROZEN` | Balance is temporarily frozen and cannot be used | Balances with temporary restrictions |
| `CLOSED` | Balance is closed and no longer accessible | Balances that have been permanently closed |
| `NEGATIVE` | Balance is in a negative state | Balances that have gone below zero (where permitted) |

## HTTP Status Codes

These HTTP status codes are returned by the Midaz APIs.

### Success Codes

| Status Code | Description | Usage |
|-------------|-------------|-------|
| `200 OK` | The request was successful | Successful GET, PATCH, PUT, DELETE operations |
| `201 Created` | The resource was successfully created | Successful POST operations |
| `204 No Content` | The request was successful but no content is returned | DELETE operations with no response body |

### Client Error Codes

| Status Code | Description | Usage |
|-------------|-------------|-------|
| `400 Bad Request` | The request contained invalid parameters | Validation errors, malformed requests |
| `401 Unauthorized` | Authentication is required | Missing or invalid authentication |
| `403 Forbidden` | The client lacks permissions for the requested action | Permission denied for authenticated users |
| `404 Not Found` | The requested resource does not exist | Resource not found |
| `409 Conflict` | The request conflicts with the current state | Duplicate entity creation, conflicting updates |
| `422 Unprocessable Entity` | The request was well-formed but contains semantic errors | Business rule violations |
| `429 Too Many Requests` | Rate limit exceeded | Too many requests in a given time period |

### Server Error Codes

| Status Code | Description | Usage |
|-------------|-------------|-------|
| `500 Internal Server Error` | An error occurred on the server | Unexpected server errors |
| `502 Bad Gateway` | The server received an invalid response from an upstream server | Errors from dependent services |
| `503 Service Unavailable` | The server is temporarily unavailable | Maintenance or overload |
| `504 Gateway Timeout` | The server timed out waiting for a response from an upstream server | Timeouts from dependent services |

## Usage Guidelines

1. **Consistency**: Always use the predefined status codes rather than creating new ones
2. **Transitions**: Follow allowed status transitions (e.g., `ACTIVE` can transition to `INACTIVE` but not directly to `DELETED`)
3. **Documentation**: Include status code in API responses with both code and description
4. **Internationalization**: Status descriptions can be translated, but status codes remain consistent

## Example Status Usage

```json
{
  "status": {
    "code": "ACTIVE",
    "description": "The account is active and available for transactions"
  }
}
```