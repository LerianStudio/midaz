// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"

	// UpdateOperation update an operation from the repository by given id.
	libLog "github.com/LerianStudio/lib-observability/log"
)

func (uc *UseCase) UpdateOperation(ctx context.Context, organizationID, ledgerID, transactionID, operationID uuid.UUID, uoi *operation.UpdateOperationInput) (_ *operation.Operation, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_operation")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "update_operation", start, err)
	}()

	op := &operation.Operation{
		Description: uoi.Description,
	}

	operationUpdated, err := uc.OperationRepo.Update(ctx, organizationID, ledgerID, transactionID, operationID, op)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Error updating op on repo by id", libLog.Err(err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrOperationIDNotFound, constant.EntityOperation)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update operation on repo by id", err)

			logger.Log(ctx, libLog.LevelWarn, "Error updating op on repo by id", libLog.Err(err))

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update operation on repo by id", err)

		return nil, err
	}

	metadataUpdated, err := uc.UpdateTransactionMetadata(ctx, constant.EntityOperation, operationID.String(), uoi.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update metadata on repo by id", err)

		return nil, err
	}

	operationUpdated.Metadata = metadataUpdated

	return operationUpdated, nil
}
