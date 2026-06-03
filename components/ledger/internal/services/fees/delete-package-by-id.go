// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// DeletePackageByID delete a package from the repository
func (uc *UseCase) DeletePackageByID(ctx context.Context, id, organizationID uuid.UUID) error {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.delete_package_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.package_id", id.String()),
	)

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Deleting package by id: %s", id))

	if err := uc.packageRepo.SoftDelete(ctx, id, organizationID); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to delete package on repo by id", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error deleting package on repo by id: %v", err))

		return err
	}

	return nil
}
