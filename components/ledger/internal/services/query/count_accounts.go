// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/google/uuid"

	// CountAccounts returns the number of accounts for the specified organization, ledger and optional portfolio.
	libLog "github.com/LerianStudio/lib-observability/log"
)

func (uc *UseCase) CountAccounts(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.count_accounts")
	defer span.End()

	count, err := uc.AccountRepo.Count(ctx, organizationID, ledgerID)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Error counting accounts on repo", libLog.Err(err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrNoAccountsFound, constant.EntityAccount)

			logger.Log(ctx, libLog.LevelWarn, "No accounts found for organization")

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to count accounts on repo", err)

			return 0, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to count accounts on repo", err)

		return 0, err
	}

	return count, nil
}
