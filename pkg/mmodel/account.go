package mmodel

import (
	"github.com/google/uuid"
	"time"
)

// CreateAccountInput is a struct designed to encapsulate request create payload data.
//
// swagger:model CreateAccountInput
// @Description Request payload for creating a new account within a ledger. Accounts represent individual financial entities such as bank accounts, credit cards, expense categories, or any other financial buckets within a ledger. Accounts are identified by a unique ID, can have aliases for easy reference, and are associated with a specific asset type.
// @example {
//   "name": "Corporate Checking Account",
//   "assetCode": "USD",
//   "portfolioId": "00000000-0000-0000-0000-000000000000",
//   "segmentId": "00000000-0000-0000-0000-000000000000",
//   "status": "ACTIVE",
//   "alias": "@treasury_checking",
//   "type": "deposit",
//   "metadata": {
//     "department": "Treasury", 
//     "purpose": "Operating Expenses", 
//     "region": "Global"
//   }
// }
type CreateAccountInput struct {
	// Human-readable name of the account
	// required: false
	// example: Corporate Checking Account
	// maxLength: 256
	Name string `json:"name" validate:"max=256" example:"Corporate Checking Account" maxLength:"256"`
	
	// ID of the parent account if this is a sub-account (optional)
	// required: false
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	ParentAccountID *string `json:"parentAccountId" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
	
	// Optional external identifier for linking to external systems
	// required: false
	// example: EXT-ACC-12345
	// maxLength: 256
	EntityID *string `json:"entityId" validate:"omitempty,max=256" example:"EXT-ACC-12345" maxLength:"256"`
	
	// Asset code that this account will use for balances and transactions
	// required: true
	// example: USD
	// maxLength: 100
	AssetCode string `json:"assetCode" validate:"required,max=100" example:"USD" maxLength:"100"`
	
	// ID of the portfolio this account belongs to (optional)
	// required: false
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	PortfolioID *string `json:"portfolioId" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
	
	// ID of the segment this account belongs to (optional)
	// required: false
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	SegmentID *string `json:"segmentId" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
	
	// Current operating status of the account
	// required: false
	Status Status `json:"status"`
	
	// Unique alias for the account (optional, must follow alias format rules)
	// required: false
	// example: @treasury_checking
	// maxLength: 100
	Alias *string `json:"alias" validate:"omitempty,max=100,prohibitedexternalaccountprefix" example:"@treasury_checking" maxLength:"100"`
	
	// Type of the account. Valid values are: deposit, savings, loans, marketplace, creditCard, external
	// required: true
	// example: deposit
	// enum: [deposit, savings, loans, marketplace, creditCard, external]
	Type string `json:"type" validate:"required" example:"deposit" enum:"deposit,savings,loans,marketplace,creditCard,external"`
	
	// Custom key-value pairs for extending the account information
	// required: false
	// example: {"department": "Treasury", "purpose": "Operating Expenses", "region": "Global"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name CreateAccountInput

// UpdateAccountInput is a struct designed to encapsulate request update payload data.
//
// swagger:model UpdateAccountInput
// @Description Request payload for updating an existing account. All fields are optional - only specified fields will be updated. Omitted fields will remain unchanged. This allows partial updates to account properties such as name, status, portfolio, segment, and metadata.
// @example {
//   "name": "Primary Corporate Checking Account",
//   "portfolioId": "11111111-1111-1111-1111-111111111111",
//   "segmentId": "22222222-2222-2222-2222-222222222222",
//   "status": "ACTIVE",
//   "metadata": {
//     "department": "Global Treasury", 
//     "purpose": "Primary Operations", 
//     "region": "Global"
//   }
// }
type UpdateAccountInput struct {
	// Updated name of the account
	// required: false
	// example: Primary Corporate Checking Account
	// maxLength: 256
	Name string `json:"name" validate:"max=256" example:"Primary Corporate Checking Account" maxLength:"256"`
	
	// Updated segment ID for the account
	// required: false
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	SegmentID *string `json:"segmentId" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
	
	// Updated portfolio ID for the account
	// required: false
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	PortfolioID *string `json:"portfolioId" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
	
	// Updated status of the account
	// required: false
	Status Status `json:"status"`
	
	// Updated custom key-value pairs for extending the account information
	// required: false
	// example: {"department": "Global Treasury", "purpose": "Primary Operations", "region": "Global"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} // @name UpdateAccountInput

// Account is a struct designed to encapsulate response payload data.
//
// swagger:model Account
// @Description Complete account entity containing all fields including system-generated fields like ID, creation timestamps, and metadata. This is the response format for account operations. Accounts represent individual financial entities (bank accounts, cards, expense categories, etc.) within a ledger and are the primary structures for tracking balances and transactions.
// @example {
//   "id": "a1b2c3d4-e5f6-7890-abcd-1234567890ab",
//   "name": "Corporate Checking Account",
//   "assetCode": "USD",
//   "organizationId": "b2c3d4e5-f6a1-7890-bcde-2345678901cd",
//   "ledgerId": "c3d4e5f6-a1b2-7890-cdef-3456789012de",
//   "portfolioId": "d4e5f6a1-b2c3-7890-defg-4567890123ef",
//   "segmentId": "e5f6a1b2-c3d4-7890-efgh-5678901234fg",
//   "status": "ACTIVE",
//   "alias": "@treasury_checking",
//   "type": "deposit",
//   "createdAt": "2022-04-15T09:30:00Z",
//   "updatedAt": "2022-04-15T09:30:00Z",
//   "metadata": {
//     "department": "Treasury",
//     "purpose": "Operating Expenses",
//     "region": "Global"
//   }
// }
type Account struct {
	// Unique identifier for the account (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	ID string `json:"id" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
	
	// Human-readable name of the account
	// example: Corporate Checking Account
	// maxLength: 256
	Name string `json:"name" example:"Corporate Checking Account" maxLength:"256"`
	
	// ID of the parent account if this is a sub-account (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	ParentAccountID *string `json:"parentAccountId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
	
	// Optional external identifier for linking to external systems
	// example: EXT-ACC-12345
	// maxLength: 256
	EntityID *string `json:"entityId" example:"EXT-ACC-12345" maxLength:"256"`
	
	// Asset code associated with this account (determines currency/asset type)
	// example: USD
	// maxLength: 100
	AssetCode string `json:"assetCode" example:"USD" maxLength:"100"`
	
	// ID of the organization that owns this account (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	OrganizationID string `json:"organizationId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
	
	// ID of the ledger this account belongs to (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	LedgerID string `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
	
	// ID of the portfolio this account belongs to (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	PortfolioID *string `json:"portfolioId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
	
	// ID of the segment this account belongs to (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	SegmentID *string `json:"segmentId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
	
	// Current operating status of the account
	Status Status `json:"status"`
	
	// Unique alias for the account (makes referencing easier)
	// example: @treasury_checking
	// maxLength: 100
	Alias *string `json:"alias" example:"@treasury_checking" maxLength:"100"`
	
	// Type of the account. Valid values are: deposit, savings, loans, marketplace, creditCard, external
	// example: deposit
	Type string `json:"type" example:"deposit"`
	
	// Timestamp when the account was created (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`
	
	// Timestamp when the account was last updated (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`
	
	// Timestamp when the account was soft deleted, null if not deleted (RFC3339 format)
	// example: null
	// format: date-time
	DeletedAt *time.Time `json:"deletedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`
	
	// Custom key-value pairs for extending the account information
	// example: {"department": "Treasury", "purpose": "Operating Expenses", "region": "Global"}
	Metadata map[string]any `json:"metadata,omitempty"`
} // @name Account

// IDtoUUID converts the account's string ID to a UUID object
//
// Returns the UUID representation of the account's ID
func (a *Account) IDtoUUID() uuid.UUID {
	return uuid.MustParse(a.ID)
}

// Accounts struct to return a paginated list of accounts.
//
// swagger:model Accounts
// @Description Paginated list of accounts with metadata about the current page, limit, and the account items themselves. Used for list operations.
// @example {
//   "items": [
//     {
//       "id": "a1b2c3d4-e5f6-7890-abcd-1234567890ab",
//       "name": "Corporate Checking Account",
//       "assetCode": "USD",
//       "ledgerId": "c3d4e5f6-a1b2-7890-cdef-3456789012de",
//       "status": "ACTIVE",
//       "alias": "@treasury_checking",
//       "type": "deposit",
//       "createdAt": "2022-04-15T09:30:00Z",
//       "updatedAt": "2022-04-15T09:30:00Z"
//     },
//     {
//       "id": "f6a1b2c3-d4e5-7890-fghi-6789012345gh",
//       "name": "Operating Expenses",
//       "assetCode": "USD",
//       "ledgerId": "c3d4e5f6-a1b2-7890-cdef-3456789012de",
//       "status": "ACTIVE",
//       "alias": "@operating_expenses",
//       "type": "expense",
//       "createdAt": "2022-04-16T10:15:00Z",
//       "updatedAt": "2022-04-16T10:15:00Z"
//     }
//   ],
//   "page": 1,
//   "limit": 10
// }
type Accounts struct {
	// Array of account records returned in this page
	// example: [{"id":"00000000-0000-0000-0000-000000000000","name":"Corporate Checking Account","assetCode":"USD","status":"ACTIVE"}]
	Items []Account `json:"items"`
	
	// Current page number in the pagination
	// example: 1
	// minimum: 1
	Page int `json:"page" example:"1" minimum:"1"`
	
	// Maximum number of items per page
	// example: 10
	// minimum: 1
	// maximum: 100
	Limit int `json:"limit" example:"10" minimum:"1" maximum:"100"`
} // @name Accounts

// AccountResponse represents a success response containing a single account.
//
// swagger:response AccountResponse
// @Description Successful response containing a single account entity.
type AccountResponse struct {
	// in: body
	Body Account
}

// AccountsResponse represents a success response containing a paginated list of accounts.
//
// swagger:response AccountsResponse
// @Description Successful response containing a paginated list of accounts.
type AccountsResponse struct {
	// in: body
	Body Accounts
}

// AccountErrorResponse represents an error response for account operations.
//
// swagger:response AccountErrorResponse
// @Description Error response for account operations with error code and message.
// @example {
//   "code": 400001,
//   "message": "Invalid input: field 'assetCode' is required",
//   "details": {
//     "field": "assetCode",
//     "violation": "required"
//   }
// }
type AccountErrorResponse struct {
	// in: body
	Body struct {
		// Error code identifying the specific error
		// example: 400001
		Code int `json:"code"`
		
		// Human-readable error message
		// example: Invalid input: field 'assetCode' is required
		Message string `json:"message"`
		
		// Additional error details if available
		// example: {"field": "assetCode", "violation": "required"}
		Details map[string]interface{} `json:"details,omitempty"`
	}
}