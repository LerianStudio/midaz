// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

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

// OperationRouteActionInput represents an operation route with its associated action.
//
// swagger:model OperationRouteActionInput
// @Description OperationRouteActionInput payload for associating an operation route with an action in a transaction route.
type OperationRouteActionInput struct {
	// The action for this operation route association.
	Action string `json:"action" validate:"required,oneof=direct hold commit cancel revert" example:"direct"`
	// The unique identifier of the Operation Route.
	OperationRouteID uuid.UUID `json:"operationRouteId" validate:"required" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a"`
} // @name OperationRouteActionInput

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
	OperationRoutes []OperationRouteActionInput `json:"operationRoutes,omitempty" validate:"required,dive"`
} // @name CreateTransactionRouteInput

// OperationRouteIDs extracts the operation route UUIDs from the input.
func (c *CreateTransactionRouteInput) OperationRouteIDs() []uuid.UUID {
	ids := make([]uuid.UUID, len(c.OperationRoutes))
	for i, route := range c.OperationRoutes {
		ids[i] = route.OperationRouteID
	}

	return ids
}

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
	OperationRoutes *[]OperationRouteActionInput `json:"operationRoutes,omitempty" validate:"omitempty,dive"`
} // @name UpdateTransactionRouteInput

// OperationRouteIDs extracts the operation route UUIDs from the input.
// Returns nil if OperationRoutes is nil.
func (u *UpdateTransactionRouteInput) OperationRouteIDs() []uuid.UUID {
	if u.OperationRoutes == nil {
		return nil
	}

	ids := make([]uuid.UUID, len(*u.OperationRoutes))
	for i, route := range *u.OperationRoutes {
		ids[i] = route.OperationRouteID
	}

	return ids
}

// ActionRouteCache represents cached routes grouped by operation type for a single action.
type ActionRouteCache struct {
	Source        map[string]OperationRouteCache `json:"source,omitempty" msgpack:"source"`
	Destination   map[string]OperationRouteCache `json:"destination,omitempty" msgpack:"destination"`
	Bidirectional map[string]OperationRouteCache `json:"bidirectional,omitempty" msgpack:"bidirectional"`
}

// TransactionRouteCache represents the cache structure for transaction routes in Redis
type TransactionRouteCache struct {
	Actions map[string]ActionRouteCache `json:"actions" msgpack:"actions"`
}

// OperationRouteCache represents the cached data for a single operation route
type OperationRouteCache struct {
	Account       *AccountCache `json:"account,omitempty"`
	OperationType string        `json:"operationType,omitempty"`
}

// AccountCache represents the cached account rule data
type AccountCache struct {
	RuleType string `json:"ruleType"`
	ValidIf  any    `json:"validIf"`
}

// ToCache converts the transaction route into a cache structure for Redis storage.
// Returns a TransactionRouteCache struct with routes grouped by action and categorized by operation type.
func (tr *TransactionRoute) ToCache() TransactionRouteCache {
	cacheData := TransactionRouteCache{
		Actions: make(map[string]ActionRouteCache),
	}

	for _, operationRoute := range tr.OperationRoutes {
		routeData := OperationRouteCache{
			OperationType: operationRoute.OperationType,
		}

		if operationRoute.Account != nil {
			routeData.Account = &AccountCache{
				RuleType: operationRoute.Account.RuleType,
				ValidIf:  operationRoute.Account.ValidIf,
			}
		}

		routeID := operationRoute.ID.String()
		action := operationRoute.Action

		switch operationRoute.OperationType {
		case "source", "destination", "bidirectional":
			actionCache, exists := cacheData.Actions[action]
			if !exists {
				actionCache = ActionRouteCache{
					Source:        make(map[string]OperationRouteCache),
					Destination:   make(map[string]OperationRouteCache),
					Bidirectional: make(map[string]OperationRouteCache),
				}
			}

			switch operationRoute.OperationType {
			case "source":
				actionCache.Source[routeID] = routeData
			case "destination":
				actionCache.Destination[routeID] = routeData
			case "bidirectional":
				actionCache.Bidirectional[routeID] = routeData
			}

			cacheData.Actions[action] = actionCache
		}
	}

	return cacheData
}

// FromMsgpack parses msgpack binary data into TransactionRouteCache.
// After deserialization, inner ActionRouteCache maps are normalized to
// prevent nil pointer dereference when iterating over Source, Destination,
// or Bidirectional fields that were absent in the serialized data.
func (trcd *TransactionRouteCache) FromMsgpack(data []byte) error {
	if err := msgpack.Unmarshal(data, trcd); err != nil {
		return err
	}

	for action, actionCache := range trcd.Actions {
		if actionCache.Source == nil {
			actionCache.Source = make(map[string]OperationRouteCache)
		}

		if actionCache.Destination == nil {
			actionCache.Destination = make(map[string]OperationRouteCache)
		}

		if actionCache.Bidirectional == nil {
			actionCache.Bidirectional = make(map[string]OperationRouteCache)
		}

		trcd.Actions[action] = actionCache
	}

	return nil
}

// ToMsgpack converts TransactionRouteCache to msgpack binary data
func (trcd TransactionRouteCache) ToMsgpack() ([]byte, error) {
	return msgpack.Marshal(trcd)
}
