# API Design Guide

## API Architecture

Midaz provides both **REST** and **gRPC** APIs:
- **REST (GoFiber)**: Primary external API for HTTP clients
- **gRPC**: Inter-service communication for high performance

## REST API Patterns

### URL Structure

```
/v1/organizations/{organizationID}/ledgers/{ledgerID}/accounts
│  │                    │                    │              │
│  │                    │                    │              └─ Resource
│  │                    │                    └────────────────── Parent Resource
│  │                    └─────────────────────────────────────── Parent Resource
│  └──────────────────────────────────────────────────────────── Version
└─────────────────────────────────────────────────────────────── Root
```

**Pattern Rules**:
- Always include version (`/v1/`)
- Use kebab-case for multi-word resources (`/account-balances`)
- Nest resources hierarchically (`/orgs/{id}/ledgers/{id}/accounts`)
- Use plural nouns for collections (`/accounts`, not `/account`)

### HTTP Methods & Status Codes

| Method | Purpose | Success Status | Resource Exists |
|--------|---------|----------------|-----------------|
| `POST` | Create | 201 Created | Location header with new resource URL |
| `GET` | Read | 200 OK | Return resource |
| `PUT` | Replace | 200 OK | Return updated resource |
| `PATCH` | Update | 200 OK | Return updated resource |
| `DELETE` | Remove | 204 No Content | No body |

**Error Status Codes**:
- `400 Bad Request`: Validation error, malformed request
- `401 Unauthorized`: Missing or invalid authentication
- `403 Forbidden`: Authenticated but not authorized
- `404 Not Found`: Resource doesn't exist
- `409 Conflict`: Duplicate resource, constraint violation
- `422 Unprocessable Entity`: Semantic validation error
- `500 Internal Server Error`: Unexpected server error
- `503 Service Unavailable`: Service is down

### Handler Pattern with Swagger

**Location**: `components/onboarding/internal/adapters/http/in/account.go`

```go
type AccountHandler struct {
    Command *command.UseCase
    Query   *query.UseCase
}

// CreateAccount creates a new account
// @Summary Create account
// @Description Creates a new account in the specified ledger
// @Tags Accounts
// @Accept json
// @Produce json
// @Param organizationID path string true "Organization ID" format(uuid)
// @Param ledgerID path string true "Ledger ID" format(uuid)
// @Param account body mmodel.CreateAccountInput true "Account data"
// @Success 201 {object} mmodel.Account
// @Failure 400 {object} http.ErrorResponse
// @Failure 409 {object} http.ErrorResponse
// @Failure 500 {object} http.ErrorResponse
// @Router /v1/organizations/{organizationID}/ledgers/{ledgerID}/accounts [post]
func (h *AccountHandler) CreateAccount(i any, c *fiber.Ctx) error {
    ctx := c.Context()

    // Extract tracking context (logging, tracing, metrics)
    tracking := libCommons.NewTrackingFromContext(ctx)

    // Start OpenTelemetry span
    ctx, span := tracer.Start(ctx, "handler.create_account")
    defer span.End()

    // Extract path parameters (parsed by ParseUUIDPathParameters middleware)
    organizationID := http.LocalUUID(c, "organization_id")
    ledgerID := http.LocalUUID(c, "ledger_id")

    // Extract typed payload (parsed and validated by WithBody middleware)
    payload := http.Payload[*mmodel.CreateAccountInput](c, i)

    // Delegate to command service
    account, err := h.Command.CreateAccount(ctx, organizationID, ledgerID, *payload)
    if err != nil {
        tracking.Logger.Errorf("Failed to create account: %v", err)
        return http.WithError(c, err)
    }

    tracking.Logger.Infof("Account created successfully: %s", account.ID)
    return http.Created(c, account)
}

// GetAccount retrieves an account by ID
// @Summary Get account
// @Description Retrieves an account by its ID
// @Tags Accounts
// @Produce json
// @Param organizationID path string true "Organization ID" format(uuid)
// @Param ledgerID path string true "Ledger ID" format(uuid)
// @Param accountID path string true "Account ID" format(uuid)
// @Success 200 {object} mmodel.Account
// @Failure 404 {object} http.ErrorResponse
// @Failure 500 {object} http.ErrorResponse
// @Router /v1/organizations/{organizationID}/ledgers/{ledgerID}/accounts/{accountID} [get]
func (h *AccountHandler) GetAccount(c *fiber.Ctx) error {
    ctx := c.Context()
    tracking := libCommons.NewTrackingFromContext(ctx)

    ctx, span := tracer.Start(ctx, "handler.get_account")
    defer span.End()

    // Extract path parameters (parsed by ParseUUIDPathParameters middleware)
    organizationID := http.LocalUUID(c, "organization_id")
    ledgerID := http.LocalUUID(c, "ledger_id")
    accountID := http.LocalUUID(c, "account_id")

    account, err := h.Query.GetAccountByID(ctx, organizationID, ledgerID, accountID)
    if err != nil {
        tracking.Logger.Errorf("Failed to get account: %v", err)
        return http.WithError(c, err)
    }

    if account == nil {
        return http.WithError(c, pkg.EntityNotFoundError{
            EntityType: "Account",
            ID:         accountID,
        })
    }

    return http.OK(c, account)
}
```

### Path Parameter Parsing Middleware

`http.ParseUUIDPathParameters()` middleware validates and extracts UUID path parameters:

```go
// Route registration
app.Use("/organizations/:organization_id", http.ParseUUIDPathParameters("organization_id"))

// In handler - values available via LocalUUID
organizationID := http.LocalUUID(c, "organization_id")
```

### Type-Safe Payload Extraction

`http.Payload[T]()` extracts and asserts payload type after WithBody validation:

```go
func (h *AccountHandler) CreateAccount(i any, c *fiber.Ctx) error {
    payload := http.Payload[*mmodel.CreateAccountInput](c, i)
    // payload is now typed as *mmodel.CreateAccountInput
}
```

### Request/Response Patterns

**Create Request**:
```json
POST /v1/organizations/{orgID}/ledgers/{ledgerID}/accounts
Content-Type: application/json

{
  "name": "Checking Account",
  "type": "DEPOSIT",
  "parentAccountID": "uuid-or-null",
  "metadata": {
    "department": "Sales",
    "costCenter": "CC-001"
  }
}
```

**Success Response (201 Created)**:
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "Checking Account",
  "type": "DEPOSIT",
  "organizationID": "org-uuid",
  "ledgerID": "ledger-uuid",
  "parentAccountID": null,
  "balance": 0,
  "status": "ACTIVE",
  "metadata": {
    "department": "Sales",
    "costCenter": "CC-001"
  },
  "createdAt": "2025-12-14T01:30:00Z",
  "updatedAt": "2025-12-14T01:30:00Z"
}
```

**Error Response (400 Bad Request)**:
```json
{
  "error": {
    "code": "INVALID_ACCOUNT_TYPE",
    "message": "Account type must be DEPOSIT, SAVINGS, or INVESTMENT",
    "field": "type",
    "details": {
      "providedValue": "INVALID_TYPE",
      "allowedValues": ["DEPOSIT", "SAVINGS", "INVESTMENT"]
    }
  }
}
```

### Pagination Pattern

Midaz uses **offset-based pagination** with `page` and `limit` parameters.

**Request**:
```
GET /v1/organizations/{orgID}/ledgers/{ledgerID}/accounts?page=2&limit=20&sort=name&order=asc
```

**Response**:
```json
{
  "data": [
    {"id": "...", "name": "Account 1", ...},
    {"id": "...", "name": "Account 2", ...}
  ],
  "pagination": {
    "currentPage": 2,
    "pageSize": 20,
    "totalPages": 5,
    "totalRecords": 98,
    "hasNext": true,
    "hasPrevious": true
  }
}
```

### Filtering & Sorting

```
# Multiple filters
GET /accounts?type=DEPOSIT&status=ACTIVE&minBalance=1000

# Sorting
GET /accounts?sort=balance&order=desc

# Date range
GET /transactions?startDate=2025-01-01&endDate=2025-12-31

# Text search
GET /accounts?search=checking
```

## gRPC Patterns

### Protocol Buffers Definition

**Location**: `pkg/mgrpc/onboarding.proto`

```protobuf
syntax = "proto3";

package onboarding.v1;

option go_package = "github.com/lerianstudio/midaz/pkg/mgrpc/onboarding";

service OnboardingService {
  rpc CreateAccount(CreateAccountRequest) returns (CreateAccountResponse);
  rpc GetAccount(GetAccountRequest) returns (GetAccountResponse);
  rpc ListAccounts(ListAccountsRequest) returns (ListAccountsResponse);
}

message CreateAccountRequest {
  string organization_id = 1;
  string ledger_id = 2;
  string name = 3;
  string type = 4;
  string parent_account_id = 5;
  map<string, string> metadata = 6;
}

message CreateAccountResponse {
  Account account = 1;
}

message Account {
  string id = 1;
  string name = 2;
  string type = 3;
  string organization_id = 4;
  string ledger_id = 5;
  double balance = 6;
  string created_at = 7;
  string updated_at = 8;
}
```

### gRPC Server Implementation

```go
type OnboardingServer struct {
    onboarding.UnimplementedOnboardingServiceServer
    commandUC *command.UseCase
    queryUC   *query.UseCase
}

func (s *OnboardingServer) CreateAccount(ctx context.Context, req *onboarding.CreateAccountRequest) (*onboarding.CreateAccountResponse, error) {
    // Convert gRPC request to domain model
    organizationID, _ := uuid.Parse(req.OrganizationId)
    ledgerID, _ := uuid.Parse(req.LedgerId)

    input := mmodel.CreateAccountInput{
        Name:     req.Name,
        Type:     req.Type,
        Metadata: req.Metadata,
    }

    // Call use case
    account, err := s.commandUC.CreateAccount(ctx, organizationID, ledgerID, input)
    if err != nil {
        return nil, status.Errorf(codes.Internal, "failed to create account: %v", err)
    }

    // Convert domain model to gRPC response
    return &onboarding.CreateAccountResponse{
        Account: toProtoAccount(account),
    }, nil
}
```

### gRPC Client Usage

```go
// In transaction service calling onboarding service
type TransactionService struct {
    onboardingClient onboarding.OnboardingServiceClient
}

func (s *TransactionService) ValidateAccount(ctx context.Context, accountID uuid.UUID) error {
    req := &onboarding.GetAccountRequest{
        AccountId: accountID.String(),
    }

    resp, err := s.onboardingClient.GetAccount(ctx, req)
    if err != nil {
        if status.Code(err) == codes.NotFound {
            return pkg.EntityNotFoundError{EntityType: "Account", ID: accountID}
        }
        return fmt.Errorf("calling onboarding service: %w", err)
    }

    if resp.Account.Status != "ACTIVE" {
        return pkg.ValidationError{
            Code:    constant.ErrAccountInactive,
            Message: "Account is not active",
        }
    }

    return nil
}
```

## Swagger/OpenAPI Documentation

### Generating Swagger Docs

```bash
# Generate Swagger documentation
make generate-docs

# Swagger files location
# components/onboarding/docs/swagger.yaml
# components/onboarding/docs/swagger.json
```

### Accessing Swagger UI

```
http://localhost:3000/swagger/index.html
```

### Swagger Annotation Best Practices

```go
// @Summary Short description (appears in list)
// @Description Longer detailed description
// @Tags Accounts          // Group endpoints
// @Accept json            // Request content type
// @Produce json           // Response content type
// @Param name location type required "description" format(type)
// @Success 200 {object} Model  // Success response
// @Failure 400 {object} http.ErrorResponse
// @Router /v1/path [method]
```

## API Versioning

### URL-Based Versioning (Current)

```
/v1/accounts  - Version 1
/v2/accounts  - Version 2 (when breaking changes needed)
```

### Backwards Compatibility

When adding new fields:
```go
// ✅ GOOD - Adding optional fields is backwards compatible
type UpdateAccountInput struct {
    Name        string            `json:"name"`
    Type        string            `json:"type"`
    NewField    *string           `json:"newField,omitempty"`  // Optional
}
```

When changing existing behavior - create new version:
```
/v2/accounts  // New version with breaking changes
```

## Authentication & Authorization

### Plugin-Based Authentication

```go
// Configured via environment variable
PLUGIN_AUTH_ENABLED=true

// Auth middleware applied to routes
func (h *Handler) RegisterRoutes(app *fiber.App) {
    api := app.Group("/v1")

    if config.AuthEnabled {
        api.Use(authMiddleware)
    }

    accounts := api.Group("/organizations/:organization_id/ledgers/:ledger_id/accounts")
    accounts.Use(http.ParseUUIDPathParameters("organization_id", "ledger_id"))
    accounts.Post("/", http.WithBody(new(mmodel.CreateAccountInput), h.CreateAccount))
    accounts.Get("/:account_id", http.ParseUUIDPathParameters("account_id"), h.GetAccount)
}
```

## Rate Limiting

```go
import "github.com/gofiber/fiber/v2/middleware/limiter"

func setupRateLimiting(app *fiber.App) {
    app.Use(limiter.New(limiter.Config{
        Max:        100,                 // 100 requests
        Expiration: 1 * time.Minute,     // per minute
        KeyGenerator: func(c *fiber.Ctx) string {
            return c.IP()  // Rate limit by IP
        },
        LimitReached: func(c *fiber.Ctx) error {
            return c.Status(429).JSON(fiber.Map{
                "error": "Rate limit exceeded",
            })
        },
    }))
}
```

## API Testing

### Newman (Postman) Tests

```bash
# Run API workflow tests
make newman
```

### E2E Tests with Apidog

```bash
# Run E2E tests
make test-e2e
```

## API Design Checklist

✅ **Use consistent URL structure** - `/v1/resources`

✅ **Apply proper HTTP methods** - POST (create), GET (read), PUT/PATCH (update), DELETE (remove)

✅ **Return appropriate status codes** - 2xx success, 4xx client error, 5xx server error

✅ **Include Swagger annotations** - Document all endpoints

✅ **Validate input** at handler level before calling use cases

✅ **Use UUID** for resource identifiers

✅ **Support pagination** for list endpoints

✅ **Provide filtering & sorting** for collections

✅ **Return detailed error messages** with error codes

✅ **Version APIs** to maintain backwards compatibility

✅ **Implement rate limiting** to prevent abuse

## Related Documentation

- Architecture: `docs/agents/architecture.md`
- Error Handling: `docs/agents/error-handling.md`
- Testing: `docs/agents/testing.md`
- Observability: `docs/agents/observability.md`
