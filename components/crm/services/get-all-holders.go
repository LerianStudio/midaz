// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"go.opentelemetry.io/otel/attribute"
)

// GetAllHolders retrieves holders that match the query filter.
func (uc *UseCase) GetAllHolders(ctx context.Context, organizationID string, filter http.QueryHeader, includeDeleted bool) ([]*mmodel.Holder, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.get_all_holders")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
	)

	holders, err := uc.HolderRepo.FindAll(ctx, organizationID, filter, includeDeleted)
	if err != nil {
		recordSpanError(span, "Failed to get holders", err)

		return nil, err
	}

	return holders, nil
}
