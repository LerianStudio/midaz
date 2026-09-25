// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
)

// FindTransactionsByGroupID returns every live member of a cross-ledger group
// from the tenant-bound repository connection.
func (uc *UseCase) FindTransactionsByGroupID(ctx context.Context, groupID uuid.UUID) ([]*transaction.Transaction, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.find_transactions_by_group_id")
	defer span.End()

	transactions, err := uc.TransactionRepo.FindByGroupID(ctx, groupID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get transactions by group id", err)

		return nil, err
	}

	return transactions, nil
}
