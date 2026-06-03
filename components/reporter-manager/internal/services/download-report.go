// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"

	pkg "github.com/LerianStudio/midaz/v3/pkg/reporter"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/ctxutil"
	pkgHTTP "github.com/LerianStudio/midaz/v3/pkg/reporter/net/http"
	templateUtils "github.com/LerianStudio/midaz/v3/pkg/reporter/templateutils"

	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// DownloadReport retrieves the report file bytes, file name, and content type for a given report ID.
// It validates the report status, fetches the associated template for output format, constructs the
// storage object name, and downloads the file from object storage.
func (uc *UseCase) DownloadReport(ctx context.Context, id uuid.UUID) ([]byte, string, string, error) {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.report.download")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.report_id", id.String()),
	)
	uc.Logger.Log(ctx, log.LevelInfo, "Downloading report", log.String("id", id.String()))

	// Fetch the report
	reportModel, err := uc.GetReportByID(ctx, id)
	if err != nil {
		if pkgHTTP.IsBusinessError(err) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve report on query", err)
		} else {
			libOpentelemetry.HandleSpanError(span, "Failed to retrieve report on query", err)
		}

		uc.Logger.Log(ctx, log.LevelError, "Failed to retrieve report", log.String("id", id.String()), log.Err(err))

		return nil, "", "", err
	}

	// Validate report status
	if reportModel.Status != constant.FinishedStatus {
		errStatus := pkg.ValidateBusinessError(constant.ErrReportStatusNotFinished, "")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Report status is not finished", errStatus)
		uc.Logger.Log(ctx, log.LevelError, "Report is not finished", log.String("id", id.String()))

		return nil, "", "", errStatus
	}

	// Use output format from report snapshot (desnormalized at creation time).
	// Fallback to template lookup for reports created before this change.
	// The fallback uses FindOutputFormatByIDIncludeDeleted so downloads still
	// work after the template has been soft-deleted.
	outputFormat := reportModel.TemplateOutputFormat
	if outputFormat == "" {
		format, err := uc.TemplateRepo.FindOutputFormatByIDIncludeDeleted(ctx, reportModel.TemplateID)
		if err != nil {
			if pkgHTTP.IsBusinessError(err) {
				libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve template output format", err)
			} else {
				libOpentelemetry.HandleSpanError(span, "Failed to retrieve template output format", err)
			}

			uc.Logger.Log(ctx, log.LevelError, "Failed to retrieve template output format", log.String("template_id", reportModel.TemplateID.String()), log.Err(err))

			return nil, "", "", err
		}

		if format != nil {
			outputFormat = *format
		}
	}

	// Construct the storage object name
	objectName := reportModel.TemplateID.String() + "/" + reportModel.ID.String() + "." + outputFormat

	// Download the file from storage
	fileBytes, errFile := uc.ReportSeaweedFS.Get(ctx, objectName)
	if errFile != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to download file from storage", errFile)
		uc.Logger.Log(ctx, log.LevelError, "Failed to download file from storage", log.Err(errFile))

		return nil, "", "", errFile
	}

	uc.Logger.Log(ctx, log.LevelInfo, "Downloaded report file from storage",
		log.String("object_name", objectName),
		log.Int("size_bytes", len(fileBytes)),
	)

	// Determine content type from the output format
	contentType := templateUtils.GetMimeType(outputFormat)

	// Construct proper filename for download
	fileName := reportModel.ID.String() + "." + outputFormat

	return fileBytes, fileName, contentType, nil
}
