# Swagger API Documentation

The Midaz platform provides interactive API documentation through Swagger UI for all services.

## Accessing Swagger UI

### Onboarding Service
- **URL**: http://localhost:3000/swagger/
- **Description**: APIs for managing organizations, ledgers, assets, portfolios, segments, and accounts

### Transaction Service  
- **URL**: http://localhost:3001/swagger/
- **Description**: APIs for managing transactions, operations, balances, and asset rates

## Features

- **Interactive Documentation**: Try out API endpoints directly from the browser
- **Schema Definitions**: View request/response schemas with examples
- **Authentication**: Test authenticated endpoints (when configured)
- **Multiple Formats**: Access documentation in OpenAPI 3.0 and Swagger 2.0 formats

## Raw API Specifications

### Onboarding Service
- OpenAPI 3.0: `/api/openapi.yaml`
- Swagger 2.0 JSON: `/api/swagger.json`
- Swagger 2.0 YAML: `/api/swagger.yaml`

### Transaction Service
- OpenAPI 3.0: `/api/openapi.yaml`
- Swagger 2.0 JSON: `/api/swagger.json`
- Swagger 2.0 YAML: `/api/swagger.yaml`

## Environment Configuration

You can customize the Swagger UI by setting these environment variables:

```bash
# Common settings
SWAGGER_TITLE="Your API Title"
SWAGGER_DESCRIPTION="Your API Description"
SWAGGER_VERSION="1.0.0"
SWAGGER_HOST="localhost:3000"
SWAGGER_BASE_PATH="/api"
SWAGGER_SCHEMES="http,https"

# Template delimiters (for customization)
SWAGGER_LEFT_DELIM="{{"
SWAGGER_RIGHT_DELIM="}}"
```

## Development Usage

1. Start the services:
```bash
make up-dev
```

2. Access Swagger UI:
- Onboarding: http://localhost:3000/swagger/
- Transaction: http://localhost:3001/swagger/

3. Use the "Try it out" button on any endpoint to test the API

## Updating API Documentation

The API documentation is auto-generated from code annotations. To update:

1. Modify the code annotations in your handlers
2. Run the swagger generation command:
```bash
cd components/onboarding && make generate-docs
cd components/transaction && make generate-docs
```

## Security Notes

- In production, consider restricting access to Swagger UI
- Use authentication middleware to protect sensitive endpoints
- Set appropriate CORS headers for cross-origin requests