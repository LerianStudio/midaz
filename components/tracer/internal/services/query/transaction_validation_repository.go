// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

//go:generate mockgen -source=transaction_validation_repository.go -destination=mocks/transaction_validation_repository_mock.go -package=mocks

import (
	"context"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// TransactionValidationRepository defines the interface for transaction validation queries.
// This interface is for read operations only per CQRS pattern.
// NOTE: Transaction validation trail is immutable per SOX/GLBA requirements - no update/delete operations.
// Single-tenant (no organizationId parameter).
type TransactionValidationRepository interface {
	// GetByID retrieves a specific transaction validation record by its unique identifier.
	// Returns constant.ErrTransactionValidationNotFound if the record does not exist.
	GetByID(ctx context.Context, id uuid.UUID) (*model.TransactionValidation, error)

	// FindByRequestID retrieves a transaction validation record by its request ID.
	// Used for idempotency checks to detect duplicate validation requests.
	// Returns (nil, nil) if no record exists with the given request ID (not an error).
	// Returns (validation, nil) if found.
	// Returns (nil, error) for database/infrastructure errors only.
	//
	// NOTE: Unlike GetByID (which returns ErrTransactionValidationNotFound), this method
	// returns (nil, nil) for not-found because absence is the EXPECTED case for new requests.
	// "Get" implies the record should exist; "Find" implies a lookup that may not match.
	FindByRequestID(ctx context.Context, requestID uuid.UUID) (*model.TransactionValidation, error)

	// List retrieves transaction validation records matching the provided filters using cursor-based pagination.
	// If filters is nil, defaults are applied (last 90 days, limit 100).
	// Results are ordered by the specified sortBy field (default: created_at DESC).
	// Returns constant.ErrInvalidTransactionValidationFilters if filter validation fails.
	// Returns constant.ErrInvalidCursor if cursor is malformed.
	List(ctx context.Context, filters *model.TransactionValidationFilters) (*model.ListTransactionValidationsResult, error)

	// Count returns the total number of records matching the filters.
	// Useful for pagination metadata without fetching all records.
	// Returns constant.ErrInvalidTransactionValidationFilters if filter validation fails.
	Count(ctx context.Context, filters *model.TransactionValidationFilters) (int64, error)
}
