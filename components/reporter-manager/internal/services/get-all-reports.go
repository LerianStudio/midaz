// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"

	"github.com/LerianStudio/reporter/pkg/ctxutil"
	"github.com/LerianStudio/reporter/pkg/mongodb/report"
	"github.com/LerianStudio/reporter/pkg/net/http"

	"github.com/LerianStudio/lib-observability/log"
	opentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"go.opentelemetry.io/otel/attribute"
)

// GetAllReports fetch all Reports from the repository
func (uc *UseCase) GetAllReports(ctx context.Context, filters http.QueryHeader) ([]*report.Report, error) {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.report.get_all")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.Int("app.request.page", filters.Page),
		attribute.Int("app.request.limit", filters.Limit),
		attribute.Bool("app.request.has_metadata", filters.Metadata != nil),
	)

	uc.Logger.Log(ctx, log.LevelInfo, "Retrieving reports",
		log.Int("page", filters.Page),
		log.Int("limit", filters.Limit),
		log.Bool("has_metadata", filters.Metadata != nil),
	)

	reports, err := uc.ReportRepo.FindList(ctx, filters)
	if err != nil {
		opentelemetry.HandleSpanError(span, "Failed to get all reports on repo", err)

		return nil, err
	}

	// Return empty slice if no reports found instead of error (consistent with templates)
	if reports == nil {
		return []*report.Report{}, nil
	}

	return reports, nil
}
