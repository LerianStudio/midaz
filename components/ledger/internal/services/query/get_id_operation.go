// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"fmt"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operation"
	"github.com/google/uuid"

	// GetOperationByID gets data in the repository.
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
)

func (uc *UseCase) GetOperationByID(ctx context.Context, organizationID, ledgerID, transactionID, operationID uuid.UUID) (*operation.Operation, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_operation_by_id")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, "Trying to get operation")

	o, err := uc.OperationRepo.Find(ctx, organizationID, ledgerID, transactionID, operationID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get operation on repo by id", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error getting operation: %v", err))

		return nil, err
	}

	if o != nil {
		metadata, err := uc.TransactionMetadataRepo.FindByEntity(ctx, reflect.TypeOf(operation.Operation{}).Name(), operationID.String())
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to get metadata on mongodb operation", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error get metadata on mongodb operation: %v", err))

			return nil, err
		}

		if metadata != nil {
			o.Metadata = metadata.Data
		}
	}

	return o, nil
}
