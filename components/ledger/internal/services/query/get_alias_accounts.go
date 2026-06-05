// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	libObs "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/google/uuid"
)

// ListAccountsByAlias gets accounts from the repository by alias.
func (uc *UseCase) ListAccountsByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Account, error) {
	logger, tracer, _, _ := libObs.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.ListAccountsByAlias")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Retrieving accounts by aliases (count=%d)", len(aliases)))

	accounts, err := uc.AccountRepo.ListAccountsByAlias(ctx, organizationID, ledgerID, aliases)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error getting accounts on repo: %v", err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrFailedToRetrieveAccountsByAliases, reflect.TypeOf(mmodel.Account{}).Name())

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("No accounts found for provided aliases (count=%d)", len(aliases)))

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve Accounts by aliases", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve Accounts by aliases", err)

		return nil, err
	}

	return accounts, nil
}
