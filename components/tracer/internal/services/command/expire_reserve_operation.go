// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// ExpireReserveOperationCommand closes an OPEN operation whose reservation TTL
// elapsed as EXPIRED, returns every reservation its decision holds and appends
// one hash-chained audit event, atomically. It does not decide that the TTL
// elapsed: the caller does, and supplies the expiry time. A later confirm or
// release conflicts with the recorded expiry.
type ExpireReserveOperationCommand struct {
	reserveOperationSettlement
	operations ReserveOperationCompleter
	tx         pgdb.TxBeginner
}

func NewExpireReserveOperationCommand(operations ReserveOperationCompleter, decisions ReserveOperationDecisionReader, capacity DecisionCapacitySettler, audit AuditEventRepository, tx pgdb.TxBeginner, config ReserveCompletionConfig) (*ExpireReserveOperationCommand, error) {
	if operations == nil || decisions == nil || capacity == nil || audit == nil || tx == nil {
		return nil, pgdb.ErrNilConnection
	}

	if config.MaxRules <= 0 || config.MaxReservations <= 0 {
		return nil, constant.ErrInvalidRequestBody
	}

	return &ExpireReserveOperationCommand{
		reserveOperationSettlement: reserveOperationSettlement{decisions: decisions, capacity: capacity, audit: audit, config: config},
		operations:                 operations, tx: tx,
	}, nil
}

// Execute returns the number of reservations this call expired. An operation
// that is already terminal, including one a confirm or release completed first,
// is left untouched without another audit event and reports zero. A commit
// failure is never retried here.
func (c *ExpireReserveOperationCommand) Execute(ctx context.Context, key model.ReserveOperationIdentity, at time.Time) (_ int, retErr error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.expire_reserve_operation")
	defer span.End()
	defer func() { recordReserveCompletionError(span, retErr) }()

	if err := key.Validate(); err != nil {
		return 0, err
	}

	if at.IsZero() {
		return 0, constant.ErrInvalidRequestBody
	}

	if !c.config.SingleTenant {
		if tmcore.GetTenantIDContext(ctx) == "" {
			return 0, constant.ErrReservationTenantRequired
		}

		if tmcore.GetPGContext(ctx) == nil {
			return 0, pgdb.ErrNoTenantInContext
		}
	}

	expired := 0

	if err := executeWithTx(ctx, c.tx, func(tx pgdb.Tx) error {
		state, changed, err := c.operations.CompleteWithTx(ctx, tx, key, model.OperationExpired, at.UTC())
		if errors.Is(err, constant.ErrReserveOperationConflict) {
			return nil
		}

		if err != nil {
			return err
		}

		if !changed {
			return nil
		}

		if state == nil || state.Status != model.OperationExpired || state.Validate() != nil {
			return constant.ErrInternalServer
		}

		moved, err := c.settleAndAudit(ctx, tx, key, state, nil)
		if err != nil {
			return err
		}

		expired = moved

		return ctx.Err()
	}); err != nil {
		return 0, err
	}

	if expired > 0 {
		logging.WithTrace(ctx, logger).Log(ctx, libLog.LevelDebug, "Reserve operation expired")
	}

	return expired, nil
}
