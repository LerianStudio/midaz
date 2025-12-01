package main

import (
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/bootstrap"
)

// @title						Midaz Onboarding API
// @version					v1.48.0
// @description				Financial infrastructure API for managing organizations, ledgers, assets, accounts, and portfolios.
// @description
// @description				## Overview
// @description				The Onboarding API provides endpoints for setting up the core entities in the Midaz ledger system:
// @description				- **Organizations**: Top-level entities representing legal entities or business units
// @description				- **Ledgers**: Accounting ledgers within organizations for tracking financial transactions
// @description				- **Assets**: Currencies, commodities, or custom value units tracked in ledgers
// @description				- **Accounts**: Individual accounts that hold balances of specific assets
// @description				- **Portfolios**: Collections of accounts grouped for business purposes
// @description				- **Segments**: Logical groupings for organizing portfolios
// @description
// @description				## Authentication
// @description				All endpoints require Bearer token authentication. Include your token in the Authorization header:
// @description				```
// @description				Authorization: Bearer <your-token-here>
// @description				```
// @description				Obtain tokens via your organization's authentication service or API key management console.
// @description
// @description				## Rate Limiting
// @description				Rate limiting is enforced at the infrastructure level (API Gateway/Load Balancer).
// @description				Default limits: 1000 requests per minute per organization.
// @description				Contact your administrator for custom rate limit configuration.
// @description				When enabled, rate limit headers are included in responses:
// @description				- `X-RateLimit-Limit`: Maximum requests per minute
// @description				- `X-RateLimit-Remaining`: Remaining requests in current window
// @description				- `X-RateLimit-Reset`: Timestamp when the limit resets
// @description
// @description				## Pagination
// @description				List endpoints support pagination via query parameters:
// @description				- `limit`: Maximum records per page (default: 10, max: 100)
// @description				- `page`: Page number (default: 1, min: 1)
// @description				- Responses include pagination metadata with total count and page info
// @description
// @description				## Error Handling
// @description				Errors follow a structured format with:
// @description				- `code`: Machine-readable error code
// @description				- `title`: Human-readable error title
// @description				- `message`: Detailed error message
// @description				- `fields`: Field-level validation errors (for 400 responses)
// @description
// @description				Common HTTP status codes:
// @description				- `400 Bad Request`: Invalid input or validation errors
// @description				- `401 Unauthorized`: Missing or invalid authentication token
// @description				- `403 Forbidden`: Authenticated but lacking required permissions
// @description				- `404 Not Found`: Requested resource does not exist
// @description				- `409 Conflict`: Resource with same identifier already exists
// @description				- `500 Internal Server Error`: Unexpected server error
// @description
// @description				## Idempotency
// @description				Use the `X-Request-Id` header to ensure idempotent operations for POST/PATCH/DELETE requests.
// @description				Requests with the same `X-Request-Id` within a 24-hour window will return the same response.
// @description
// @description				## Multi-Tenancy
// @description				All resources are scoped to organizations. Organization ID is included in most resource paths
// @description				to ensure proper tenant isolation via Row Level Security (RLS).
// @termsOfService				http://swagger.io/terms/
// @contact.name				Discord Community
// @contact.url				https://discord.gg/DnhqKwkGv3
// @license.name				Apache 2.0
// @license.url				http://www.apache.org/licenses/LICENSE-2.0.html
// @host						localhost:3000
// @BasePath					/
// @securityDefinitions.apikey	BearerAuth
// @in							header
// @name						Authorization
// @description				Bearer token authentication. Format: Bearer {token}
func main() {
	libCommons.InitLocalEnvConfig()
	bootstrap.InitServers().Run()
}
