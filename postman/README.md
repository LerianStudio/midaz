# Midaz Postman Collections

This directory contains Postman collections and environment configurations for testing and interacting with the Midaz API.

## Files

- **MIDAZ.postman_collection.json** - Main Postman collection containing all API endpoints from both the Onboarding and Transaction services.
- **MIDAZ.postman_environment.json** - Environment template with all required variables for the API.
- **MIDAZ_Test_Workflow.postman_collection.json** - Collection containing example workflows for common API usage patterns.

## Features

The Postman collection has been enhanced with:

1. **Pre-request Scripts**:
   - Authentication handling with environment variables
   - Variable validation for dependencies
   - Request ID generation

2. **Test Scripts**:
   - Response status code validation
   - Variable extraction from responses
   - Response schema validation

3. **Environment Variables**:
   - Centralized environment configuration
   - Variables for all entity IDs (organizations, ledgers, accounts, etc.)
   - Authentication token management

4. **Dependencies**:
   - Variable substitution in request URLs
   - Proper handling of path parameters
   - Chaining of requests in workflows

## Usage

1. Import the collections and environment into Postman
2. Set the `baseUrl` variable to your Midaz API endpoint
3. Set the `authToken` variable with your authentication token
4. Follow the API workflows, starting with organization creation

## Variables

| Variable | Description |
|----------|-------------|
| `baseUrl` | Base URL for the API (default: http://localhost:3000) |
| `authToken` | Authentication token (JWT format) |
| `organizationId` | ID of the active organization |
| `ledgerId` | ID of the active ledger |
| `assetId` | ID of the active asset |
| `accountId` | ID of the active account |
| `transactionId` | ID of the active transaction |
| `operationId` | ID of the active operation |

## Workflow Recommendations

For the best experience, follow these API call sequences:

1. Create an organization
2. Create a ledger in that organization
3. Create assets in the ledger
4. Create accounts in the ledger
5. Create transactions between accounts

The test scripts will automatically extract IDs from responses and store them in environment variables for use in subsequent requests.

## Maintenance

These collections are automatically generated from the OpenAPI specifications in the Midaz codebase. To update them, run:

```bash
./scripts/sync-postman.sh
```

This script will:
1. Generate new Postman collections from the latest OpenAPI specs
2. Create an updated environment template
3. Merge the collections into a single, comprehensive collection
4. Back up the previous collection and environment files