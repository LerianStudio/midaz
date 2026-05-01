// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
)

// DeleteOperationRouteByID deletes an operation route by ID.
func (uc *UseCase) DeleteOperationRouteByID(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_operation_route_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.operation_route_id", id.String()),
	)

	if err := ctx.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Context canceled before deleting operation route", err)

		return err
	}

	hasLinks, err := uc.OperationRouteRepo.HasTransactionRouteLinks(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to check transaction route links", err)

		logger.Log(ctx, libLog.LevelError, "Failed to check transaction route links",
			libLog.Err(err),
			libLog.String("operation_route_id", id.String()),
		)

		return err
	}

	if hasLinks {
		err := pkg.ValidateBusinessError(constant.ErrOperationRouteLinkedToTransactionRoutes, constant.EntityOperationRoute)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Operation route is linked to transaction routes", err)

		logger.Log(ctx, libLog.LevelWarn, "Operation route is linked to transaction routes",
			libLog.String("operation_route_id", id.String()),
		)

		return err
	}

	if err := uc.OperationRouteRepo.Delete(ctx, organizationID, ledgerID, id); err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, constant.EntityOperationRoute)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Operation route not found", err)

			logger.Log(ctx, libLog.LevelWarn, "Operation route not found",
				libLog.String("operation_route_id", id.String()),
			)

			return err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to delete operation route", err)

		logger.Log(ctx, libLog.LevelError, "Failed to delete operation route",
			libLog.Err(err),
			libLog.String("operation_route_id", id.String()),
		)

		return err
	}

	return nil
}
