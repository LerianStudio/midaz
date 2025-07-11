package mmodel

import (
	"time"

	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"
)

// TransactionRoute is a struct designed to store TransactionRoute data.
//
// swagger:model TransactionRoute
// @Description TransactionRoute object
type TransactionRoute struct {
	// The unique identifier of the Transaction Route.
	ID uuid.UUID `json:"id,omitempty" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a"`
	// The unique identifier of the Organization.
	OrganizationID uuid.UUID `json:"organizationId,omitempty" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a"`
	// The unique identifier of the Ledger.
	LedgerID uuid.UUID `json:"ledgerId,omitempty" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a"`
	// Short text summarizing the purpose of the transaction. Used as an entry note for identification.
	Title string `json:"title,omitempty" example:"Charge Settlement"`
	// A description for the Transaction Route.
	Description string `json:"description,omitempty" example:"Settlement route for service charges"`
	// Additional metadata stored as JSON
	Metadata map[string]any `json:"metadata,omitempty" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
	// An object containing accounting data of Operation Routes from the Transaction Route.
	OperationRoutes []OperationRoute `json:"operationRoutes,omitempty"`
	// The timestamp when the transaction route was created.
	CreatedAt time.Time `json:"createdAt" example:"2025-01-01T00:00:00Z"`
	// The timestamp when the transaction route was last updated.
	UpdatedAt time.Time `json:"updatedAt" example:"2025-01-01T00:00:00Z"`
	// The timestamp when the transaction route was deleted.
	DeletedAt *time.Time `json:"deletedAt" example:"2025-01-01T00:00:00Z"`
} // @name TransactionRoute

// CreateTransactionRouteInput is a struct designed to store CreateRouteInput data.
//
// swagger:model CreateTransactionRouteInput
// @Description CreateTransactionRouteInput payload
type CreateTransactionRouteInput struct {
	// Short text summarizing the purpose of the transaction. Used as an entry note for identification.
	Title string `json:"title,omitempty" validate:"required,max=50" example:"Charge Settlement"`
	// A description for the Transaction Route.
	Description string `json:"description,omitempty" validate:"max=250" example:"Settlement route for service charges"`
	// Additional metadata stored as JSON
	Metadata map[string]any `json:"metadata,omitempty" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
	// An object containing accounting data of Operation Routes from the Transaction Route.
	OperationRoutes []uuid.UUID `json:"operationRoutes,omitempty" validate:"required"`
} // @name CreateTransactionRouteInput

// UpdateTransactionRouteInput is a struct designed to store transaction route update data.
//
// swagger:model UpdateTransactionRouteInput
// @Description UpdateTransactionRouteInput payload
type UpdateTransactionRouteInput struct {
	// Short text summarizing the purpose of the transaction. Used as an entry note for identification.
	Title string `json:"title,omitempty" validate:"max=50" example:"Charge Settlement"`
	// A description for the Transaction Route.
	Description string `json:"description,omitempty" validate:"max=250" example:"Settlement route for service charges"`
	// Additional metadata stored as JSON
	Metadata map[string]any `json:"metadata,omitempty" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
	// An object containing accounting data of Operation Routes from the Transaction Route.
	OperationRoutes *[]uuid.UUID `json:"operationRoutes,omitempty"`
} // @name UpdateTransactionRouteInput

// TransactionRouteCache represents the cache structure for transaction routes in Redis
type TransactionRouteCache struct {
	Source      map[string]OperationRouteCache `json:"source"`
	Destination map[string]OperationRouteCache `json:"destination"`
}

// OperationRouteCache represents the cached data for a single operation route
type OperationRouteCache struct {
	Account *AccountCache `json:"account,omitempty"`
}

// AccountCache represents the cached account rule data
type AccountCache struct {
	RuleType string `json:"ruleType"`
	ValidIf  any    `json:"validIf"`
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
