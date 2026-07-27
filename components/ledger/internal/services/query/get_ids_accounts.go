// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"

	// ListAccountsByIDs get Accounts from the repository by given ids.
	libLog "github.com/LerianStudio/lib-observability/v2/log"
)

func (uc *UseCase) ListAccountsByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Account, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.ListAccountsByIDs")
	defer span.End()

	accounts, err := uc.AccountRepo.ListAccountsByIDs(ctx, organizationID, ledgerID, ids)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Error getting accounts on repo", libLog.Err(err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrIDsNotFoundForAccounts, constant.EntityAccount)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve Accounts by ids", err)

			logger.Log(ctx, libLog.LevelWarn, "No accounts found")

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve Accounts by ids", err)

		return nil, err
	}

	return accounts, nil
}
