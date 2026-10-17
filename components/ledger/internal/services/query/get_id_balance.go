// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// GetBalanceByID returns one balance with its current state.
func (uc *UseCase) GetBalanceByID(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID) (*mmodel.Balance, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_balance_by_id")
	defer span.End()

	balance, err := uc.BalanceRepo.Find(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		recordQueryError(ctx, span, logger, "Failed to get balance on repo by id", err)

		return nil, err
	}

	if balance == nil {
		err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Balance not found", err)

		return nil, err
	}

	if err := uc.applyBalanceState(ctx, span, organizationID, ledgerID, []*mmodel.Balance{balance}); err != nil {
		return nil, err
	}

	return balance, nil
}
