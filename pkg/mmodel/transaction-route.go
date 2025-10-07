// Package mmodel defines domain models for the Midaz platform.
// This file contains TransactionRoute-related models for accounting rules.
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
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
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
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
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

// ToCache converts the transaction route into a cache-optimized structure for Redis storage.
//
// This method transforms a TransactionRoute into a TransactionRouteCache structure that is
// optimized for fast lookup during transaction processing. The cache structure pre-categorizes
// operation routes by their type (source/destination) and indexes them by ID for O(1) lookup.
//
// The cache structure is used during transaction validation to quickly verify that:
//   - Transactions follow the defined routing rules
//   - Accounts match the expected aliases or account types
//   - Operation types (debit/credit) align with the route configuration
//
// Returns:
//   - TransactionRouteCache: A cache-optimized structure with routes organized by type
//
// Cache Structure:
//   - Source: Map of source operation route IDs to their cached data
//   - Destination: Map of destination operation route IDs to their cached data
//
// Example:
//
//	transactionRoute := &TransactionRoute{
//	    ID: uuid.New(),
//	    OperationRoutes: []OperationRoute{
//	        {ID: uuid1, OperationType: "source", Account: &AccountRule{...}},
//	        {ID: uuid2, OperationType: "destination", Account: &AccountRule{...}},
//	    },
//	}
//	cache := transactionRoute.ToCache()
//	// cache.Source[uuid1.String()] contains the source route data
//	// cache.Destination[uuid2.String()] contains the destination route data
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

// FromMsgpack deserializes msgpack binary data into this TransactionRouteCache.
//
// This method uses the msgpack format for efficient binary serialization when storing
// transaction route cache data in Redis. Msgpack provides better performance and smaller
// payload sizes compared to JSON, which is important for high-throughput transaction processing.
//
// Parameters:
//   - data: Binary msgpack-encoded data to deserialize
//
// Returns:
//   - error: nil on success, error if deserialization fails
//
// Example:
//
//	var cache TransactionRouteCache
//	if err := cache.FromMsgpack(redisData); err != nil {
//	    return fmt.Errorf("failed to deserialize cache: %w", err)
//	}
func (trcd *TransactionRouteCache) FromMsgpack(data []byte) error {
	return msgpack.Unmarshal(data, trcd)
}

// ToMsgpack serializes this TransactionRouteCache to msgpack binary format.
//
// This method converts the cache structure to msgpack binary format for efficient storage
// in Redis. Msgpack provides better performance and smaller payload sizes compared to JSON,
// which is critical for high-throughput transaction processing where cache lookups happen
// frequently.
//
// Returns:
//   - []byte: Msgpack-encoded binary data
//   - error: nil on success, error if serialization fails
//
// Example:
//
//	cache := transactionRoute.ToCache()
//	data, err := cache.ToMsgpack()
//	if err != nil {
//	    return fmt.Errorf("failed to serialize cache: %w", err)
//	}
//	redis.Set(cacheKey, data, ttl)
func (trcd TransactionRouteCache) ToMsgpack() ([]byte, error) {
	return msgpack.Marshal(trcd)
}
