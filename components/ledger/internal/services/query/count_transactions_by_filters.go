// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/google/uuid"
)

// CountTransactionsByFilters returns the number of transactions matching the given filters.
func (uc *UseCase) CountTransactionsByFilters(ctx context.Context, organizationID, ledgerID uuid.UUID, filter transaction.CountFilter) (int64, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.count_transactions_by_filters")
	defer span.End()

	count, err := uc.TransactionRepo.CountByFilters(ctx, organizationID, ledgerID, filter)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to count transactions by filters", err)

		logger.Log(ctx, libLog.LevelError, "Failed to count transactions by filters", libLog.Err(err))

		return 0, err
	}

	return count, nil
}
