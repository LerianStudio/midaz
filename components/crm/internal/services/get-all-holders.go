// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"go.opentelemetry.io/otel/attribute"
)

// GetAllHolders retrieves holders that match the query filter.
func (uc *UseCase) GetAllHolders(ctx context.Context, organizationID string, filter http.QueryHeader, includeDeleted bool) ([]*mmodel.Holder, error) {
	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.get_all_holders")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
	)

	logger.Log(ctx, libLog.LevelInfo, "Retrieving holders")

	holders, err := uc.HolderRepo.FindAll(ctx, organizationID, filter, includeDeleted)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get holders", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get holders", libLog.Err(err))

		return nil, err
	}

	return holders, nil
}
