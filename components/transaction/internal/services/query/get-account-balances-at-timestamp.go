// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// GetAccountBalancesAtTimestamp retrieves all balance states for an account at a specific point in time.
// It uses a single optimized query with LEFT JOIN to fetch balance states, avoiding multiple round-trips.
// Balances without operations at the timestamp are returned with zero values (initial state).
func (uc *UseCase) GetAccountBalancesAtTimestamp(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, timestamp time.Time) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_account_balances_at_timestamp")
	defer span.End()

	logger.Infof("Retrieving balances for account %s at timestamp %s", accountID, timestamp.Format(time.RFC3339))

	// Validate timestamp is not in the future (use UTC for consistent comparison)
	if timestamp.After(time.Now().UTC()) {
		err := pkg.ValidateBusinessError(constant.ErrInvalidTimestamp, "Balance", timestamp.Format(time.RFC3339))
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Timestamp is in the future", err)
		logger.Warnf("Timestamp is in the future: %s", timestamp)

		return nil, err
	}

	// Use optimized single query with LEFT JOIN to get all balances at timestamp
	// This replaces the previous approach of 2 separate queries + in-memory merge
	balances, err := uc.BalanceRepo.ListByAccountIDAtTimestamp(ctx, organizationID, ledgerID, accountID, timestamp)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get balances at timestamp", err)
		logger.Errorf("Error getting balances at timestamp: %v", err)

		return nil, err
	}

	logger.Infof("Successfully retrieved %d balances for account %s at timestamp %s", len(balances), accountID, timestamp.Format(time.RFC3339))

	return balances, nil
}
