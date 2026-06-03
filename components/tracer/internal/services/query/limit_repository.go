// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

//go:generate mockgen -source=limit_repository.go -destination=limit_repository_mock.go -package=query

import (
	"context"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

// LimitRepository defines the interface for limit persistence in queries.
// This interface is intentionally separate from command.LimitRepository per CQRS pattern.
// GetByID exists in both interfaces as it serves different purposes:
// - In queries: direct data retrieval for API responses
// - In commands: reading current state for validation before mutations
type LimitRepository interface {
	// GetByID retrieves a limit by its unique identifier.
	// Returns constant.ErrLimitNotFound if the limit does not exist.
	GetByID(ctx context.Context, limitID uuid.UUID) (*model.Limit, error)

	// List retrieves limits with cursor-based pagination and filtering.
	//
	// Nil filter handling: If filters is nil, the implementation MUST treat it
	// as an empty filter with default values applied:
	//   - Limit defaults to constant.DefaultPaginationLimit (10)
	//   - Limit is capped at constant.MaxPaginationLimit (100)
	//   - SortBy defaults to "created_at"
	//   - SortOrder defaults to "desc"
	//
	// Returns constant.ErrInvalidCursor if the cursor is malformed or expired.
	// Returns constant.ErrInvalidSortColumn if sortBy is not in the allowed list.
	List(ctx context.Context, filters *model.ListLimitsFilter) (*model.ListLimitsResult, error)
}
