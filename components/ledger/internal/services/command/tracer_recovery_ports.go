// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	"github.com/google/uuid"

	traceradapter "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/tracerreservation"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=tracer_recovery_ports.go -destination=mock_tracer_recovery_ports_test.go -package=command

// TracerObligationStore commits coordination before Reserve and fences dispatch
// independently of the accounting engine. Outcome setters require known evidence.
type TracerObligationStore interface {
	Find(context.Context, tracerreservation.Key) (*tracerreservation.Pending, error)
	Prepare(context.Context, tracerreservation.Intent) (*tracerreservation.Record, error)
	BeginExecution(context.Context, tracerreservation.Key, time.Time) error
	BeginExecutions(context.Context, []tracerreservation.Key, time.Time) error
	SetOutcome(context.Context, tracerreservation.Key, tracerreservation.State, time.Time) error
	ExpirePrepared(context.Context, tracerreservation.Key, time.Time) (bool, error)
	MarkDelivered(context.Context, tracerreservation.Key, tracerreservation.State, time.Time) error
	ClaimDue(context.Context, time.Time, time.Time, int) ([]tracerreservation.Pending, error)
	ScheduleRetry(context.Context, tracerreservation.Pending, time.Time, bool) error
}

// ContextTracerReserver exposes only the complete shared contract. Recovery
// addresses outcomes by transaction even when admission returned no holds or its
// response was lost. Neither recovery nor the client can execute accounting.
type ContextTracerReserver interface {
	Reserve(context.Context, tracercontract.ReserveRequest) (*tracercontract.ReserveResult, error)
	ConfirmByTransaction(context.Context, uuid.UUID) (*tracercontract.TransactionCompletionResult, error)
	ReleaseByTransaction(context.Context, uuid.UUID) (*tracercontract.TransactionCompletionResult, error)
}

// TracerAccountingEvidence reads the durable projection on the tenant primary.
// Missing/PENDING/unknown status is not evidence of abort. Retrying this read is
// safe; rerunning the accounting engine to obtain a result is forbidden.
type TracerAccountingEvidence interface {
	ReadAccountingStatus(context.Context, tracerreservation.Key) (string, error)
}

// TracerFactsLoader reads official records only after participation/skip gates.
type TracerFactsLoader interface {
	EvaluationContext(context.Context, uuid.UUID, uuid.UUID, []traceradapter.PreparedEntry) (tracercontract.Context, error)
}
