// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

//go:generate mockgen -source=limit_repository.go -destination=limit_repository_mock.go -package=command

import (
	"context"
	"time"

	"github.com/google/uuid"

	pgdb "tracer/internal/adapters/postgres/db"
	"tracer/pkg/model"
)

// LimitRepository defines the interface for limit persistence in commands.
// This interface is intentionally separate from query.LimitRepository per CQRS pattern.
// GetByID exists in both interfaces as commands need to read current state for validation
// before performing mutations.
type LimitRepository interface {
	// CreateWithTx persists a new limit using the provided database handle. The
	// db parameter accepts either a regular DB connection or a transaction
	// (pgdb.Tx), enabling atomic persistence of the limit together with other
	// changes (for example, its audit event).
	CreateWithTx(ctx context.Context, db pgdb.DB, lmt *model.Limit) error

	// GetByID retrieves a limit by its unique identifier.
	// Returns constant.ErrLimitNotFound if the limit does not exist.
	GetByID(ctx context.Context, limitID uuid.UUID) (*model.Limit, error)

	// UpdateWithTx persists changes to an existing limit using the provided
	// database handle. The db parameter accepts either a regular DB connection
	// or a transaction (pgdb.Tx), enabling atomic persistence of the limit
	// mutation together with other changes (for example, its audit event).
	UpdateWithTx(ctx context.Context, db pgdb.DB, lmt *model.Limit) error

	// UpdateStatus updates only the status field and updatedAt timestamp.
	// More efficient than UpdateWithTx() for lifecycle transitions (activate, deactivate, delete).
	// For DELETED status, also sets deleted_at for soft-delete consistency.
	UpdateStatus(ctx context.Context, limitID uuid.UUID, status model.LimitStatus, updatedAt time.Time) error

	// UpdateStatusWithTx updates the status field using the provided database
	// handle. Semantics mirror UpdateStatus, and the db handle allows callers to
	// run the update inside a transaction together with the audit event insert.
	UpdateStatusWithTx(ctx context.Context, db pgdb.DB, limitID uuid.UUID, status model.LimitStatus, updatedAt time.Time) error
}
