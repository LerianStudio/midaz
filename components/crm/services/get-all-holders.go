// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"go.opentelemetry.io/otel/attribute"
)

// GetAllHolders retrieves holders that match the query filter.
func (uc *UseCase) GetAllHolders(ctx context.Context, organizationID string, filter http.QueryHeader, includeDeleted bool) (_ []*mmodel.Holder, err error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.get_all_holders")
	defer span.End()

	start := time.Now()
	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "crm", "list_holders", start, err)
	}()

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
