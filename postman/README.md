# Midaz API Postman Collection

This directory contains the Postman collection and environment files for the Midaz API. The collection is generated from the OpenAPI/Swagger documentation in the `components/onboarding/api` and `components/transaction/api` directories.

## Collection Structure

The collection is organized into the following sections:

- **Organizations**: API endpoints for managing organizations
- **Ledgers**: API endpoints for managing ledgers
- **Assets**: API endpoints for managing assets
- **Accounts**: API endpoints for managing accounts
- **Portfolios**: API endpoints for managing portfolios
- **Segments**: API endpoints for managing segments
- **Transactions**: API endpoints for managing transactions
- **Balances**: API endpoints for managing balances
- **Operations**: API endpoints for managing operations
- **E2E Flow**: A sequential flow of API calls that demonstrate a complete end-to-end workflow

## Running Tests

You can run the Postman tests in several ways:

### Using Make Commands

Run all tests in the collection:
```
make test-postman
```

Run only the E2E flow tests:
```
make test-postman-e2e
```

### Using NPM Scripts

Run all tests in the collection:
```
cd scripts && npm run test:postman
```

Run only the E2E flow tests:
```
cd scripts && npm run test:postman:e2e
```

### Using Newman Directly

Run all tests in the collection:
```
npx newman run ./postman/MIDAZ.postman_collection.json -e ./postman/MIDAZ.postman_environment.json
```

Run only the E2E flow tests:
```
npx newman run ./postman/MIDAZ.postman_collection.json -e ./postman/MIDAZ.postman_environment.json --folder "E2E Flow"
```

## Maintaining the Collection

The Postman collection is automatically generated from the OpenAPI/Swagger documentation. To update the collection after making changes to the API:

1. Generate the Swagger documentation:
   ```
   make generate-docs
   ```

2. Sync the Postman collection:
   ```
   make sync-postman
   ```

These commands will update both the OpenAPI documentation and the Postman collection.

## Recent Fixes

- Fixed test script syntax for POST endpoints to properly check for 200 or 201 status codes
- Corrected the URL path for balance endpoints (Get Balance by ID, Update Balance, Delete Balance)
- Enhanced JSON transaction payload swagger annotations to correctly document the expected structure
- Added dedicated testing scripts and Makefile targets for running Postman tests