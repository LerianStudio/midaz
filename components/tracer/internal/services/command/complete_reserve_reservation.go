// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// CompleteReserveReservationCommand resolves an immutable reservation address
// and completes its whole operation through the same atomic coordinator. The
// preliminary primary read moves no capacity; the coordinator owns all locks,
// tenant transactions, audit and conflict detection. No partial outcome exists.
type CompleteReserveReservationCommand struct {
	locator      ReservationOperationLocator
	reporter     ReserveOperationReporter
	singleTenant bool
}

func NewCompleteReserveReservationCommand(locator ReservationOperationLocator, reporter ReserveOperationReporter, singleTenant bool) (*CompleteReserveReservationCommand, error) {
	if locator == nil || reporter == nil {
		return nil, pgdb.ErrNilConnection
	}

	return &CompleteReserveReservationCommand{locator: locator, reporter: reporter, singleTenant: singleTenant}, nil
}

func (c *CompleteReserveReservationCommand) Execute(ctx context.Context, reservationID uuid.UUID, outcome model.ReserveOperationStatus) (_ *tracercontract.ReservationCompletionResult, retErr error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.complete_reserve_reservation")
	defer span.End()
	defer func() { recordReserveCompletionError(span, retErr) }()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	identity, ok := contextutil.GetIntegrationIdentity(ctx)
	if !ok {
		return nil, constant.ErrInsufficientPrivileges
	}

	if !c.singleTenant {
		if tmcore.GetTenantIDContext(ctx) == "" {
			return nil, constant.ErrReservationTenantRequired
		}

		if tmcore.GetPGContext(ctx) == nil {
			return nil, pgdb.ErrNoTenantInContext
		}
	}

	if reservationID == uuid.Nil || (outcome != model.OperationConfirmed && outcome != model.OperationReleased) {
		return nil, constant.ErrInvalidRequestBody
	}

	owner, err := c.locator.GetReservationOwner(ctx, identity.ID, reservationID)
	if err != nil {
		return nil, err
	}

	if owner == nil {
		return nil, constant.ErrReservationNotFound
	}

	if owner.Validate() != nil || owner.ReservationID != reservationID || owner.Operation.IntegrationID != identity.ID {
		return nil, constant.ErrInternalServer
	}

	report, err := c.reporter.ExecuteReport(ctx, owner.Operation.TransactionID, outcome)
	if err != nil {
		return nil, err
	}

	if err := validateReservationCompletionReport(owner, report, outcome); err != nil {
		return nil, err
	}

	evaluation := *report.EvaluationID
	result := &tracercontract.ReservationCompletionResult{ContractRevision: report.ContractRevision, TransactionID: report.TransactionID, ReservationID: reservationID, Status: report.Status, EvaluationID: &evaluation}

	logging.WithTrace(ctx, logger).Log(ctx, libLog.LevelDebug, "Reservation address completed its coordinated operation")

	return result, nil
}

func validateReservationCompletionReport(owner *model.ReserveReservationOwner, report *tracercontract.TransactionCompletionResult, outcome model.ReserveOperationStatus) error {
	if report == nil || report.Validate() != nil || report.TransactionID != owner.Operation.TransactionID || report.Status != string(outcome) || report.EvaluationID == nil || *report.EvaluationID != owner.EvaluationID {
		return constant.ErrInternalServer
	}

	return nil
}
