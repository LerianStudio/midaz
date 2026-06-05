// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// shouldSkipProcessing checks if report should be skipped due to idempotency.
func (uc *UseCase) shouldSkipProcessing(ctx context.Context, reportID uuid.UUID) bool {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.report.should_skip_processing")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.report_id", reportID.String()),
	)

	reportStatus, err := uc.checkReportStatus(ctx, reportID)
	if err == nil {
		if reportStatus == constant.FinishedStatus {
			uc.Logger.Log(ctx, log.LevelInfo, "Report is already finished, skipping reprocessing",
				log.String("report_id", reportID.String()))

			return true
		}

		if reportStatus == constant.ErrorStatus {
			uc.Logger.Log(ctx, log.LevelWarn, "Report is in error state, skipping reprocessing",
				log.String("report_id", reportID.String()))

			return true
		}
	}

	return false
}

// checkReportStatus checks the current status of a report to implement idempotency.
func (uc *UseCase) checkReportStatus(ctx context.Context, reportID uuid.UUID) (string, error) {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.report.check_report_status")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.report_id", reportID.String()),
	)

	report, err := uc.ReportDataRepo.FindByID(ctx, reportID)
	if err != nil {
		// Don't record as span error - "not found" on first attempt is expected behavior
		span.SetAttributes(attribute.String("app.report.status_check", "not_found_or_error"))
		uc.Logger.Log(ctx, log.LevelDebug, "Could not check report status (may be first attempt)",
			log.String("report_id", reportID.String()), log.Err(err))

		return "", err
	}

	uc.Logger.Log(ctx, log.LevelDebug, "Report current status",
		log.String("report_id", reportID.String()), log.String("status", report.Status))

	return report.Status, nil
}
