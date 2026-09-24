// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// GetReservationOwner reads only immutable decision-owned addressing data from
// the resolved tenant primary. Legacy and foreign-producer IDs are indistinguishable
// from missing IDs. It acquires no capacity locks and performs no state transition.
func (r *ReserveDecisionRepository) GetReservationOwner(ctx context.Context, integration string, reservationID uuid.UUID) (_ *model.ReserveReservationOwner, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.get_reservation_owner")
	defer span.End()
	defer func() { recordReserveDecisionRepositoryError(span, retErr) }()

	if err := (model.ReserveOperationIdentity{IntegrationID: integration, TransactionID: reservationID}).Validate(); err != nil {
		return nil, err
	}

	database, err := r.primaryDatabase(ctx)
	if err != nil {
		return nil, err
	}

	owner := &model.ReserveReservationOwner{ReservationID: reservationID}

	err = database.QueryRowContext(ctx, `SELECT d.integration_id, d.transaction_id, d.evaluation_id FROM usage_reservations AS r JOIN reserve_decisions AS d ON d.evaluation_id = r.decision_id AND d.transaction_id = r.transaction_id WHERE r.id = $1 AND d.integration_id = $2`, reservationID, integration).Scan(&owner.Operation.IntegrationID, &owner.Operation.TransactionID, &owner.EvaluationID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("read reservation owner: %w", err)
	}

	if owner.Validate() != nil || owner.Operation.IntegrationID != integration {
		return nil, constant.ErrInternalServer
	}

	logging.WithTrace(ctx, logger).Log(ctx, libLog.LevelDebug, "Resolved immutable reservation owner on primary")

	return owner, nil
}

func (r *ReserveDecisionRepository) primaryDatabase(ctx context.Context) (pgdb.DB, error) {
	database, err := r.conn.GetDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve decision database: %w", err)
	}

	if primary, ok := database.(interface{ ReadWrite() *sql.DB }); ok {
		writer := primary.ReadWrite()
		if writer == nil {
			return nil, pgdb.ErrNilConnection
		}

		database = writer
	}

	if database == nil {
		return nil, pgdb.ErrNilConnection
	}

	return database, nil
}
