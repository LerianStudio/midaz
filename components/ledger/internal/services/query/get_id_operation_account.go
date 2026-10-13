// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func (uc *UseCase) GetOperationByAccount(ctx context.Context, organizationID, ledgerID, accountID, operationID uuid.UUID) (*operation.Operation, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_operation_by_account")
	defer span.End()

	op, err := uc.OperationRepo.FindByAccount(ctx, organizationID, ledgerID, accountID, operationID)
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrNoOperationsFound, constant.EntityOperation)
		}

		recordQueryError(ctx, span, logger, "Failed to get operation on repo by account", err)

		return nil, err
	}

	if op != nil {
		metadata, err := uc.TransactionMetadataRepo.FindByEntity(ctx, constant.EntityOperation, operationID.String())
		if err != nil {
			recordQueryError(ctx, span, logger, "Failed to get metadata on mongodb operation", err)

			return nil, err
		}

		if metadata != nil {
			op.Metadata = metadata.Data
		}
	}

	return op, nil
}
