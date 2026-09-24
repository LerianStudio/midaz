// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=reserve_admission_ports.go -destination=mocks/reserve_admission_ports_mock.go -package=mocks

// ReserveAdmissionDecisions reads the primary and writes immutable decisions in
// the caller's transaction. Identity collisions must be returned as conflicts.
type ReserveAdmissionDecisions interface {
	Get(context.Context, model.ReserveOperationKey) (*model.ReserveDecision, error)
	GetWithTx(context.Context, pgdb.Tx, model.ReserveOperationKey) (*model.ReserveDecision, error)
	CreateWithTx(context.Context, pgdb.Tx, model.ReserveDecision) error
}

// ReserveAdmissionOperations locks the operation before any capacity or audit.
type ReserveAdmissionOperations interface {
	LockWithTx(context.Context, pgdb.Tx, model.ReserveOperationIdentity) (*model.ReserveOperationState, error)
}

// ReserveAdmissionCapacity must share the legacy account lock namespace.
type ReserveAdmissionCapacity interface {
	AcquireReserveScopeLock(context.Context, pgdb.DB, int64) error
	ReserveForDecisionWithTx(context.Context, pgdb.Tx, uuid.UUID, *model.Reservation, decimal.Decimal, time.Time) error
}

// ReserveAdmissionLimits returns the complete eligible snapshot on the primary.
type ReserveAdmissionLimits interface {
	ListCandidatesWithTx(context.Context, pgdb.Tx, string, []uuid.UUID) ([]model.ContextAccountLimit, error)
}

// ReserveAdmissionPolicies resolves the current binding before compiling/reusing
// its immutable program. Missing policy is never a permissive default.
type ReserveAdmissionPolicies interface {
	ExecuteWithTx(context.Context, pgdb.Tx, string) (*query.PreparedContextPolicy, error)
}

// ReserveAdmissionEvaluator evaluates rules without consuming limit capacity.
type ReserveAdmissionEvaluator interface {
	Execute(context.Context, *query.CompiledContextPolicy, tracercontract.Context, string) (*model.ContextPolicyResult, error)
}
