// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"strings"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"

	"github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"

	// otel/attribute is used for span attribute types (no lib-commons wrapper available)
	"go.opentelemetry.io/otel/attribute"
)

// mimeTypes maps file extensions to their corresponding MIME content types
var mimeTypes = map[string]string{
	"txt":  "text/plain",
	"html": "text/html",
	"json": "application/json",
	"csv":  "text/csv",
	"pdf":  "application/pdf",
}

// saveReport handles saving the generated report file to the report repository and logs any encountered errors.
// It determines the object name, content type, and stores the file using the ReportSeaweedFS interface.
// If ReportTTL is configured, the file will be saved with TTL (Time To Live).
// Returns an error if the file storage operation fails.
func (uc *UseCase) saveReport(ctx context.Context, message GenerateReportMessage, out string) error {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, spanSaveReport := uc.Tracer.Start(ctx, "service.report.save_report")
	defer spanSaveReport.End()

	spanSaveReport.SetAttributes(attribute.String("app.request.request_id", reqId))

	outputFormat := strings.ToLower(message.OutputFormat)
	contentType := getContentType(outputFormat)
	objectName := message.TemplateID.String() + "/" + message.ReportID.String() + "." + outputFormat

	err := uc.ReportSeaweedFS.Put(ctx, objectName, contentType, []byte(out), uc.ReportTTL)
	if err != nil {
		libOtel.HandleSpanError(spanSaveReport, "Error putting report file.", err)
		uc.Logger.Log(ctx, log.LevelError, "Error putting report file", log.Err(err))

		return err
	}

	if uc.ReportTTL != "" {
		uc.Logger.Log(ctx, log.LevelDebug, "Saving report with TTL", log.String("ttl", uc.ReportTTL))
	}

	return nil
}

// getContentType returns the MIME type for a given file extension.
// If the extension is not recognized, it returns "text/plain".
func getContentType(ext string) string {
	if contentType, ok := mimeTypes[ext]; ok {
		return contentType
	}

	return "text/plain"
}
