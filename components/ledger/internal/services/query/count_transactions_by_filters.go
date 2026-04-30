// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/google/uuid"
)

// CountTransactionsByFilters returns the number of transactions matching the given filters.
func (uc *UseCase) CountTransactionsByFilters(ctx context.Context, organizationID, ledgerID uuid.UUID, filter transaction.CountFilter) (int64, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.count_transactions_by_filters")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf(
		"Counting transactions with filters: organizationID=%s, ledgerID=%s, route=%s, status=%s, startDate=%s, endDate=%s",
		organizationID, ledgerID, filter.Route, filter.Status, filter.StartDate, filter.EndDate,
	))

	count, err := uc.TransactionRepo.CountByFilters(ctx, organizationID, ledgerID, filter)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to count transactions by filters", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to count transactions by filters: %v", err))

		return 0, err
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Transaction count result: %d", count))

	return count, nil
}
