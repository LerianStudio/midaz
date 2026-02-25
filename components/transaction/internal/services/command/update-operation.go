// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/google/uuid"
)

// UpdateOperation update an operation from the repository by given id.
func (uc *UseCase) UpdateOperation(ctx context.Context, organizationID, ledgerID, transactionID, operationID uuid.UUID, uoi *operation.UpdateOperationInput) (*operation.Operation, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_operation")
	defer span.End()

	logger.Infof("Trying to update operation: %v", uoi)

	op := &operation.Operation{
		Description: uoi.Description,
	}

	operationUpdated, err := uc.OperationRepo.Update(ctx, organizationID, ledgerID, transactionID, operationID, op)
	if err != nil {
		logger.Errorf("Error updating op on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrOperationIDNotFound, reflect.TypeOf(operation.Operation{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update operation on repo by id", err)

			logger.Warnf("Error updating op on repo by id: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update operation on repo by id", err)

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(operation.Operation{}).Name(), operationID.String(), uoi.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update metadata on repo by id", err)

		return nil, err
	}

	operationUpdated.Metadata = metadataUpdated

	return operationUpdated, nil
}
