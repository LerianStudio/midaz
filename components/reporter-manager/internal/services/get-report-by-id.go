// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"

	pkg "github.com/LerianStudio/midaz/v4/pkg"
	cnErr "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb/report"

	"github.com/LerianStudio/lib-observability/log"
	opentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// GetReportByID recover a report by ID
func (uc *UseCase) GetReportByID(ctx context.Context, id uuid.UUID) (*report.Report, error) {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.report.get_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.report_id", id.String()),
	)
	uc.Logger.Log(ctx, log.LevelDebug, "Retrieving report", log.String("id", id.String()))

	reportModel, err := uc.ReportRepo.FindByID(ctx, id)
	if err != nil {
		if nf := (pkg.EntityNotFoundError{}); errors.As(err, &nf) {
			errNotFound := pkg.ValidateBusinessError(cnErr.ErrEntityNotFound, "", constant.MongoCollectionReport)

			opentelemetry.HandleSpanBusinessErrorEvent(span, "Report not found", errNotFound)

			return nil, errNotFound
		}

		opentelemetry.HandleSpanError(span, "Failed to get report on repo by id", err)

		return nil, err
	}

	return reportModel, nil
}
