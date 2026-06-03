// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

//go:generate mockgen -source=transaction_validation_repository.go -destination=mocks/transaction_validation_repository_mock.go -package=mocks

import (
	"context"
	"errors"

	pgdb "tracer/internal/adapters/postgres/db"
	"tracer/pkg/model"
)

// ErrDuplicateValidation is returned when a transaction validation record with the same
// request_id already exists. Callers should handle this by fetching the existing record.
var ErrDuplicateValidation = errors.New("duplicate transaction validation")

// TransactionValidationRepository defines the interface for transaction validation persistence.
// This interface is for write operations only per CQRS pattern.
// NOTE: Only INSERT operations allowed - transaction validation trail is immutable per SOX/GLBA requirements.
// API Design v1.3.0: Single-tenant (no organizationId parameter).
type TransactionValidationRepository interface {
	// Insert creates a new transaction validation record (insert-only, no updates allowed).
	// This maintains the immutability requirement for compliance (SOX/GLBA).
	// Implementations MUST return a non-nil error if validation is nil (no panics).
	Insert(ctx context.Context, validation *model.TransactionValidation) error

	// InsertWithTx creates a new transaction validation record using the provided database connection.
	// This allows callers to pass either a regular DB connection or a transaction (*sql.Tx),
	// enabling atomic operations with other database changes.
	// This maintains the immutability requirement for compliance (SOX/GLBA).
	// Implementations MUST return a non-nil error if validation is nil (no panics).
	InsertWithTx(ctx context.Context, db pgdb.DB, validation *model.TransactionValidation) error
}
