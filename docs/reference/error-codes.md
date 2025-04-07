# Error Codes

**Navigation:** [Home](../../) > [Reference Materials](../README.md) > Error Codes

This document provides a comprehensive reference for error codes used throughout the Midaz APIs.

## Error Response Format

All errors returned by Midaz APIs follow a standardized format:

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

## Common Error Codes

### Authentication and Authorization Errors

| Error Code | Title | Description | HTTP Status |
|------------|-------|-------------|------------|
| `AUTH_INVALID_TOKEN` | Invalid Token | The authentication token is invalid or expired | 401 |
| `AUTH_MISSING_TOKEN` | Missing Token | No authentication token was provided | 401 |
| `AUTH_INSUFFICIENT_PERMISSIONS` | Insufficient Permissions | The authenticated user lacks permission for this operation | 403 |
| `AUTH_ACCOUNT_LOCKED` | Account Locked | The user account is locked | 403 |

### Resource Errors

| Error Code | Title | Description | HTTP Status |
|------------|-------|-------------|------------|
| `RESOURCE_NOT_FOUND` | Resource Not Found | The requested resource does not exist | 404 |
| `RESOURCE_ALREADY_EXISTS` | Resource Already Exists | A resource with the same unique identifier already exists | 409 |
| `RESOURCE_STATE_CONFLICT` | Resource State Conflict | The operation cannot be performed due to the current state of the resource | 409 |
| `RESOURCE_DELETED` | Resource Deleted | The requested resource has been deleted | 410 |

### Validation Errors

| Error Code | Title | Description | HTTP Status |
|------------|-------|-------------|------------|
| `VALIDATION_REQUIRED_FIELD` | Required Field Missing | A required field is missing from the request | 400 |
| `VALIDATION_INVALID_FORMAT` | Invalid Format | A field has an invalid format | 400 |
| `VALIDATION_INVALID_VALUE` | Invalid Value | A field has a value that is not valid for its context | 400 |
| `VALIDATION_VALUE_TOO_LONG` | Value Too Long | A field value exceeds the maximum allowed length | 400 |
| `VALIDATION_VALUE_TOO_SHORT` | Value Too Short | A field value is shorter than the minimum required length | 400 |

### Business Logic Errors

| Error Code | Title | Description | HTTP Status |
|------------|-------|-------------|------------|
| `BUSINESS_INSUFFICIENT_FUNDS` | Insufficient Funds | The account does not have sufficient funds for the operation | 422 |
| `BUSINESS_ACCOUNT_INACTIVE` | Account Inactive | The account is not in an active state | 422 |
| `BUSINESS_ASSET_MISMATCH` | Asset Mismatch | The operation involves mismatched asset types | 422 |
| `BUSINESS_BALANCE_NEGATIVE` | Negative Balance | The operation would result in a negative balance where not allowed | 422 |
| `BUSINESS_TRANSACTION_UNBALANCED` | Unbalanced Transaction | The transaction debits and credits do not balance | 422 |

### Transaction Errors

| Error Code | Title | Description | HTTP Status |
|------------|-------|-------------|------------|
| `TRANSACTION_ALREADY_PROCESSED` | Transaction Already Processed | The transaction has already been processed | 409 |
| `TRANSACTION_EXPIRED` | Transaction Expired | The transaction has expired and cannot be processed | 422 |
| `TRANSACTION_CANCELLED` | Transaction Cancelled | The transaction has been cancelled | 422 |
| `TRANSACTION_DSL_SYNTAX_ERROR` | DSL Syntax Error | The transaction DSL contains syntax errors | 400 |
| `TRANSACTION_VALIDATION_ERROR` | Transaction Validation Error | The transaction failed validation checks | 422 |

### Asset Rate Errors

| Error Code | Title | Description | HTTP Status |
|------------|-------|-------------|------------|
| `ASSET_RATE_NOT_FOUND` | Asset Rate Not Found | No exchange rate found for the specified asset pair | 404 |
| `ASSET_RATE_EXPIRED` | Asset Rate Expired | The exchange rate has expired and needs to be refreshed | 422 |
| `ASSET_RATE_INVALID` | Invalid Asset Rate | The exchange rate value is invalid | 422 |

### System Errors

| Error Code | Title | Description | HTTP Status |
|------------|-------|-------------|------------|
| `SYSTEM_INTERNAL_ERROR` | Internal Server Error | An unexpected error occurred in the system | 500 |
| `SYSTEM_SERVICE_UNAVAILABLE` | Service Unavailable | The service is temporarily unavailable | 503 |
| `SYSTEM_TIMEOUT` | Service Timeout | The operation timed out | 504 |
| `SYSTEM_DATABASE_ERROR` | Database Error | An error occurred while accessing the database | 500 |
| `SYSTEM_DEPENDENCY_ERROR` | Dependency Error | An error occurred in a dependent service | 502 |

### Rate Limiting Errors

| Error Code | Title | Description | HTTP Status |
|------------|-------|-------------|------------|
| `RATE_LIMIT_EXCEEDED` | Rate Limit Exceeded | The client has exceeded the allowed request rate | 429 |
| `RATE_LIMIT_ORG_EXCEEDED` | Organization Rate Limit Exceeded | The organization has exceeded its allowed request rate | 429 |

## Component-Specific Error Codes

### Onboarding Service Error Codes

| Error Code | Title | Description | HTTP Status |
|------------|-------|-------------|------------|
| `ONBOARDING_ORG_LIMIT_EXCEEDED` | Organization Limit Exceeded | The maximum number of organizations has been reached | 422 |
| `ONBOARDING_LEDGER_LIMIT_EXCEEDED` | Ledger Limit Exceeded | The maximum number of ledgers for this organization has been reached | 422 |
| `ONBOARDING_INVALID_PORTFOLIO_SEGMENT` | Invalid Portfolio Segment | The portfolio cannot be assigned to the specified segment | 422 |
| `ONBOARDING_DUPLICATE_ALIAS` | Duplicate Alias | An account with this alias already exists | 409 |

### Transaction Service Error Codes

| Error Code | Title | Description | HTTP Status |
|------------|-------|-------------|------------|
| `TRANSACTION_IDEMPOTENCY_CONFLICT` | Idempotency Conflict | A transaction with this idempotency key already exists but with different parameters | 409 |
| `TRANSACTION_AMOUNT_TOO_LARGE` | Amount Too Large | The transaction amount exceeds the maximum allowed | 422 |
| `TRANSACTION_OPERATIONS_LIMIT` | Operations Limit Exceeded | The transaction contains too many operations | 422 |
| `TRANSACTION_RATE_EXPIRED` | Asset Rate Expired | The asset rate used in the transaction has expired | 422 |

## Error Handling Best Practices

1. **Be specific**: Return the most specific error code that applies to the situation
2. **Include details**: Provide clear error messages that help clients understand what went wrong
3. **Field validation**: For validation errors, specify which fields failed validation and why
4. **Localization**: Error titles and messages should support localization
5. **Logging**: Log errors with correlation IDs for easier troubleshooting

## Example Error Response

```json
{
  "code": "VALIDATION_INVALID_VALUE",
  "title": "Invalid Field Value",
  "message": "One or more field values are invalid",
  "entityType": "Account",
  "fields": {
    "alias": "Account alias must start with '@' and contain only letters, numbers, and underscores",
    "type": "Account type must be one of: deposit, savings, creditCard, loan"
  }
}
```