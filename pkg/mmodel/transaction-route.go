package mmodel

import (
	"time"

	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"
)

// TransactionRoute is a struct designed to store TransactionRoute data.
//
// swagger:model TransactionRoute
// @Description Complete transaction route entity containing all fields including system-generated fields. Transaction routes combine multiple operation routes to define complete transaction flows, specifying which accounts can be sources (debits) and destinations (credits) for a transaction type.
//
//	@example {
//	  "id": "a1b2c3d4-e5f6-7890-abcd-1234567890ab",
//	  "organizationId": "b2c3d4e5-f6a1-7890-bcde-2345678901cd",
//	  "ledgerId": "c3d4e5f6-a1b2-7890-cdef-3456789012de",
//	  "title": "Charge Settlement",
//	  "description": "Settlement route for service charges",
//	  "operationRoutes": [
//	    {
//	      "id": "d4e5f6a1-b2c3-7890-defg-4567890123ef",
//	      "title": "Source Route",
//	      "operationType": "source"
//	    },
//	    {
//	      "id": "e5f6a1b2-c3d4-7890-efgh-5678901234fg",
//	      "title": "Destination Route",
//	      "operationType": "destination"
//	    }
//	  ],
//	  "createdAt": "2022-04-15T09:30:00Z",
//	  "updatedAt": "2022-04-15T09:30:00Z",
//	  "metadata": {
//	    "category": "Settlement",
//	    "department": "Operations"
//	  }
//	}
type TransactionRoute struct {
	// Unique identifier for the transaction route (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	ID uuid.UUID `json:"id,omitempty" swaggertype:"string" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`

	// ID of the organization that owns this transaction route (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	OrganizationID uuid.UUID `json:"organizationId,omitempty" swaggertype:"string" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`

	// ID of the ledger this transaction route belongs to (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	LedgerID uuid.UUID `json:"ledgerId,omitempty" swaggertype:"string" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`

	// Short text summarizing the purpose of the transaction route
	// example: Charge Settlement
	// maxLength: 50
	Title string `json:"title,omitempty" example:"Charge Settlement" maxLength:"50"`

	// Detailed description of the transaction route
	// example: Settlement route for service charges
	// maxLength: 250
	Description string `json:"description,omitempty" example:"Settlement route for service charges" maxLength:"250"`

	// Custom key-value pairs for extending the transaction route information
	// example: {"category": "Settlement", "department": "Operations"}
	Metadata map[string]any `json:"metadata,omitempty" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`

	// Array of operation routes that define the source and destination accounts for this transaction route
	OperationRoutes []OperationRoute `json:"operationRoutes,omitempty"`

	// Timestamp when the transaction route was created (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the transaction route was last updated (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the transaction route was soft deleted, null if not deleted (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	DeletedAt *time.Time `json:"deletedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`
} // @name TransactionRoute

// CreateTransactionRouteInput is a struct designed to store CreateRouteInput data.
//
// swagger:model CreateTransactionRouteInput
// @Description Request payload for creating a new transaction route. Transaction routes combine multiple operation routes to define complete transaction flows with source and destination accounts.
//
//	@example {
//	  "title": "Charge Settlement",
//	  "description": "Settlement route for service charges",
//	  "operationRoutes": ["d4e5f6a1-b2c3-7890-defg-4567890123ef", "e5f6a1b2-c3d4-7890-efgh-5678901234fg"],
//	  "metadata": {
//	    "category": "Settlement",
//	    "department": "Operations"
//	  }
//	}
type CreateTransactionRouteInput struct {
	// Short text summarizing the purpose of the transaction route
	// required: true
	// example: Charge Settlement
	// maxLength: 50
	Title string `json:"title,omitempty" validate:"required,max=50" example:"Charge Settlement" maxLength:"50"`

	// Detailed description of the transaction route
	// required: false
	// example: Settlement route for service charges
	// maxLength: 250
	Description string `json:"description,omitempty" validate:"max=250" example:"Settlement route for service charges" maxLength:"250"`

	// Custom key-value pairs for extending the transaction route information
	// required: false
	// example: {"category": "Settlement", "department": "Operations"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`

	// Array of operation route IDs that define the source and destination accounts for this transaction route
	// required: true
	// format: uuid
	OperationRoutes []uuid.UUID `json:"operationRoutes,omitempty" validate:"required" swaggertype:"array,string" format:"uuid"`
} // @name CreateTransactionRouteInput

// UpdateTransactionRouteInput is a struct designed to store transaction route update data.
//
// swagger:model UpdateTransactionRouteInput
// @Description Request payload for updating an existing transaction route. All fields are optional - only specified fields will be updated. Omitted fields will remain unchanged.
//
//	@example {
//	  "title": "Updated Charge Settlement",
//	  "description": "Updated settlement route for service charges",
//	  "operationRoutes": ["d4e5f6a1-b2c3-7890-defg-4567890123ef"],
//	  "metadata": {
//	    "category": "Settlement",
//	    "department": "Global Operations",
//	    "updated": true
//	  }
//	}
type UpdateTransactionRouteInput struct {
	// Updated short text summarizing the purpose of the transaction route
	// required: false
	// example: Updated Charge Settlement
	// maxLength: 50
	Title string `json:"title,omitempty" validate:"max=50" example:"Updated Charge Settlement" maxLength:"50"`

	// Updated detailed description of the transaction route
	// required: false
	// example: Updated settlement route for service charges
	// maxLength: 250
	Description string `json:"description,omitempty" validate:"max=250" example:"Updated settlement route for service charges" maxLength:"250"`

	// Updated custom key-value pairs for extending the transaction route information
	// required: false
	// example: {"category": "Settlement", "department": "Global Operations", "updated": true}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`

	// Updated array of operation route IDs for this transaction route
	// required: false
	// format: uuid
	OperationRoutes *[]uuid.UUID `json:"operationRoutes,omitempty" swaggertype:"array,string" format:"uuid"`
} // @name UpdateTransactionRouteInput

// TransactionRouteCache represents the cache structure for transaction routes in Redis.
// This is an internal model not exposed via API.
type TransactionRouteCache struct {
	// Map of source operation routes keyed by route ID
	Source map[string]OperationRouteCache `json:"source"`

	// Map of destination operation routes keyed by route ID
	Destination map[string]OperationRouteCache `json:"destination"`
}

// OperationRouteCache represents the cached data for a single operation route.
// This is an internal model not exposed via API.
type OperationRouteCache struct {
	// Cached account rule configuration
	Account *AccountCache `json:"account,omitempty"`
}

// AccountCache represents the cached account rule data.
// This is an internal model not exposed via API.
type AccountCache struct {
	// The rule type for account selection
	RuleType string `json:"ruleType"`

	// The rule condition for account selection
	ValidIf any `json:"validIf"`
}

// ToCache converts the transaction route into a cache structure for Redis storage.
// Returns a TransactionRouteCache struct with routes pre-categorized by type.
func (tr *TransactionRoute) ToCache() TransactionRouteCache {
	cacheData := TransactionRouteCache{
		Source:      make(map[string]OperationRouteCache),
		Destination: make(map[string]OperationRouteCache),
	}

	for _, operationRoute := range tr.OperationRoutes {
		routeData := OperationRouteCache{}

		if operationRoute.Account != nil {
			routeData.Account = &AccountCache{
				RuleType: operationRoute.Account.RuleType,
				ValidIf:  operationRoute.Account.ValidIf,
			}
		}

		// Categorize by operation type
		routeID := operationRoute.ID.String()

		switch operationRoute.OperationType {
		case "source":
			cacheData.Source[routeID] = routeData
		case "destination":
			cacheData.Destination[routeID] = routeData
		}
	}

	return cacheData
}

// FromMsgpack parses msgpack binary data into TransactionRouteCache
func (trcd *TransactionRouteCache) FromMsgpack(data []byte) error {
	return msgpack.Unmarshal(data, trcd)
}

// ToMsgpack converts TransactionRouteCache to msgpack binary data
func (trcd TransactionRouteCache) ToMsgpack() ([]byte, error) {
	return msgpack.Marshal(trcd)
}
