// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"

	// GetTransactionRouteByID retrieves a transaction route by its ID.
	// It returns the transaction route if found, otherwise it returns an error.
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
)

func (uc *UseCase) GetTransactionRouteByID(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID) (*mmodel.TransactionRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_transaction_route_by_id")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Retrieving transaction route for id: %s", id))

	transactionRoute, err := uc.TransactionRouteRepo.FindByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error getting transaction route on repo by id: %v", err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrTransactionRouteNotFound, reflect.TypeOf(mmodel.TransactionRoute{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get transaction route", err)

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Error getting transaction route on repo by id: %v", err))

			return nil, err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to get transaction route", err)

		return nil, err
	}

	if transactionRoute != nil {
		metadata, err := uc.TransactionMetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.TransactionRoute{}).Name(), transactionRoute.ID.String())
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to get metadata on mongodb transaction route", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error get metadata on mongodb transaction route: %v", err))

			return nil, err
		}

		if metadata != nil {
			transactionRoute.Metadata = metadata.Data
		}
	}

	return transactionRoute, nil
}
