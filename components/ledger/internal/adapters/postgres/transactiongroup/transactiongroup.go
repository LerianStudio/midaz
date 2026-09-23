// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package transactiongroup persists the intent and lifecycle state of a
// cross-ledger PENDING transaction group.
package transactiongroup

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// TransactionGroup is the durable coordinator record for a cross-ledger hold.
type TransactionGroup struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	LedgerID       uuid.UUID
	Status         string
	AssetCode      string
	Intent         []byte
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Repository owns transaction-group lifecycle persistence.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 --destination=transactiongroup.postgresql_mock.go --package=transactiongroup . Repository
type Repository interface {
	Create(ctx context.Context, group *TransactionGroup) error
	Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*TransactionGroup, error)
	FindByID(ctx context.Context, id uuid.UUID) (*TransactionGroup, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, from, to string) (bool, error)
	// ListByStatusOlderThan returns at most limit groups in status whose
	// created_at is before the cutoff, ordered by id and strictly after afterID.
	// uuid.Nil starts from the first group; callers page by passing the last id
	// they received. It reads the current tenant across ledger scopes.
	ListByStatusOlderThan(ctx context.Context, status string, before time.Time, afterID uuid.UUID, limit int) ([]*TransactionGroup, error)
	Delete(ctx context.Context, id uuid.UUID) error
}
