// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	mongodb "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"

	// CreateOperationRoute creates a new operation route.
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
)

func (uc *UseCase) CreateOperationRoute(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *mmodel.CreateOperationRouteInput) (*mmodel.OperationRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_operation_route")
	defer span.End()

	now := time.Now()

	operationRoute := &mmodel.OperationRoute{
		ID:                uuid.Must(libCommons.GenerateUUIDv7()),
		OrganizationID:    organizationID,
		LedgerID:          ledgerID,
		Title:             payload.Title,
		Description:       payload.Description,
		Code:              payload.Code, //nolint:staticcheck // SA1019: backcompat — field still accepted and persisted until clients migrate to rubric codes inside accountingEntries
		OperationType:     payload.OperationType,
		Account:           payload.Account,
		AccountingEntries: payload.AccountingEntries,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	createdOperationRoute, err := uc.OperationRouteRepo.Create(ctx, organizationID, ledgerID, operationRoute)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create operation route", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to create operation route: %v", err))

		return nil, err
	}

	if payload.Metadata != nil {
		meta := mongodb.Metadata{
			EntityID:   createdOperationRoute.ID.String(),
			EntityName: reflect.TypeOf(mmodel.OperationRoute{}).Name(),
			Data:       payload.Metadata,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		if err := uc.TransactionMetadataRepo.Create(ctx, reflect.TypeOf(mmodel.OperationRoute{}).Name(), &meta); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create operation route metadata", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to create operation route metadata: %v", err))

			return nil, err
		}

		createdOperationRoute.Metadata = payload.Metadata
	}

	return createdOperationRoute, nil
}
