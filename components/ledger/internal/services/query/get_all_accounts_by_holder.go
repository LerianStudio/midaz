// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// GetAllAccountsByHolder fetches the accounts a holder owns across every ledger of
// the organization. A non-nil ledgerID narrows the listing to a single ledger.
func (uc *UseCase) GetAllAccountsByHolder(ctx context.Context, organizationID, holderID uuid.UUID, ledgerID *uuid.UUID, filter http.QueryHeader, holderPolicy mmodel.HolderPolicy) (_ []*mmodel.Account, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_accounts_by_holder")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "list_accounts_by_holder", start, err)
	}()

	accounts, err := uc.AccountRepo.FindAllByHolder(ctx, organizationID, holderID, ledgerID, filter, holderPolicy)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Error getting accounts by holder on repo", libLog.Err(err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrNoAccountsFound, constant.EntityAccount)

			logger.Log(ctx, libLog.LevelWarn, "No accounts found")

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get accounts by holder on repo", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get accounts by holder on repo", err)

		return nil, err
	}

	return uc.attachAccountMetadata(ctx, span, accounts)
}
