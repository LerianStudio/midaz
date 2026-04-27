// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

func (uc *UseCase) UpdateOperationRoute(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID, input *mmodel.UpdateOperationRouteInput) (*mmodel.OperationRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_operation_route")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Trying to update operation route: %v", input))

	operationRoute := &mmodel.OperationRoute{
		Title:                input.Title,
		Description:          input.Description,
		Code:                 input.Code, //nolint:staticcheck // SA1019: backcompat — field still accepted and persisted until clients migrate to rubric codes inside accountingEntries
		Account:              input.Account,
		AccountingEntries:    input.AccountingEntries,
		AccountingEntriesRaw: input.AccountingEntriesRaw,
	}

	operationRouteUpdated, err := uc.OperationRouteRepo.Update(ctx, organizationID, ledgerID, id, operationRoute)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error updating operation route on repo by id: %v", err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update operation route on repo by id", err)

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Error updating operation route on repo by id: %v", err))

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update operation route on repo by id", err)

		return nil, err
	}

	metadataUpdated, err := uc.UpdateTransactionMetadata(ctx, reflect.TypeOf(mmodel.OperationRoute{}).Name(), id.String(), input.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update metadata on repo by id", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error updating metadata on repo by id: %v", err))

		return nil, err
	}

	operationRouteUpdated.Metadata = metadataUpdated

	return operationRouteUpdated, nil
}
