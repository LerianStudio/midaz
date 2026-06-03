// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

//go:generate mockgen -source=repository.go -destination=repository_mock.go -package=command

import (
	"context"
	"time"

	"github.com/google/uuid"

	pgdb "github.com/LerianStudio/midaz/v3/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

// RuleRepository defines the interface for rule persistence.
type RuleRepository interface {
	// CreateWithTx persists a new rule using the provided database handle. The
	// db parameter accepts either a regular DB connection or a transaction
	// (pgdb.Tx), enabling atomic persistence of the rule together with other
	// changes (for example, its audit event).
	CreateWithTx(ctx context.Context, db pgdb.DB, rule *model.Rule) (*model.Rule, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.Rule, error)
	GetByName(ctx context.Context, name string) (*model.Rule, error)
	ListByStatus(ctx context.Context, status *model.RuleStatus) ([]*model.Rule, error)
	// UpdateWithTx persists changes to an existing rule using the provided
	// database handle. The db parameter accepts either a regular DB connection
	// or a transaction (pgdb.Tx), enabling atomic persistence of the rule
	// mutation together with other changes (for example, its audit event).
	UpdateWithTx(ctx context.Context, db pgdb.DB, rule *model.Rule) error
	// DeleteWithTx soft-deletes a rule using the provided database handle.
	// The db handle allows callers to run the update inside a transaction
	// together with the audit event insert.
	DeleteWithTx(ctx context.Context, db pgdb.DB, id uuid.UUID) error
	ListActiveByScopes(ctx context.Context, scopes []model.Scope) ([]*model.Rule, error)
	// UpdateStatus updates the status field and related timestamps.
	// activatedAt is set when transitioning to ACTIVE, deactivatedAt when transitioning to INACTIVE.
	UpdateStatus(ctx context.Context, id uuid.UUID, status model.RuleStatus, updatedAt time.Time, activatedAt *time.Time, deactivatedAt *time.Time) error
}
