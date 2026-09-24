// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	"github.com/google/uuid"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

//go:generate mockgen -source=reserve_completion_repository.go -destination=mocks/reserve_completion_repository_mock.go -package=mocks

// ReserveOperationCompleter locks and records known completion in the caller's
// transaction. changed=false requires an identical previously committed outcome.
type ReserveOperationCompleter interface {
	CompleteWithTx(context.Context, pgdb.Tx, model.ReserveOperationIdentity, model.ReserveOperationStatus, time.Time) (*model.ReserveOperationState, bool, error)
}

// ReserveOperationDecisionReader reads the immutable decision for the verified
// integration and transaction without needing the original request identifier.
type ReserveOperationDecisionReader interface {
	GetByOperationWithTx(context.Context, pgdb.Tx, model.ReserveOperationIdentity) (*model.ReserveDecision, error)
}

// DecisionCapacitySettler returns the pre-transition snapshots of all moved
// reservations. The command validates ownership before recording their audit.
type DecisionCapacitySettler interface {
	SettleDecisionWithTx(context.Context, pgdb.Tx, uuid.UUID, model.ReservationStatus) ([]*model.Reservation, error)
}
