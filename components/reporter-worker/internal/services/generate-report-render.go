// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"os"
	"strings"

	pkg "github.com/LerianStudio/midaz/v3/pkg/reporter"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/ctxutil"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/pongo"

	"github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"

	// otel/attribute is used for span attribute types (no lib-commons wrapper available)
	"go.opentelemetry.io/otel/attribute"
	// otel/trace is used for trace.Tracer and trace.Span parameter types
	"go.opentelemetry.io/otel/trace"
)

// loadTemplate loads template file from SeaweedFS.
func (uc *UseCase) loadTemplate(ctx context.Context, message GenerateReportMessage, span *trace.Span) ([]byte, error) {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, spanTemplate := uc.Tracer.Start(ctx, "service.report.get_template")
	defer spanTemplate.End()

	spanTemplate.SetAttributes(attribute.String("app.request.request_id", reqId))

	fileBytes, err := uc.TemplateSeaweedFS.Get(ctx, message.TemplateID.String())
	if err != nil {
		if errUpdate := uc.updateReportWithErrors(ctx, message.ReportID, err); errUpdate != nil {
			libOtel.HandleSpanError(*span, "Error to update report status with error.", errUpdate)
			uc.Logger.Log(ctx, log.LevelError, "Error update report status with error", log.Err(errUpdate))

			return nil, errUpdate
		}

		libOtel.HandleSpanError(spanTemplate, "Error getting file from template bucket.", err)
		uc.Logger.Log(ctx, log.LevelError, "Error getting file from template bucket", log.Err(err))

		return nil, err
	}

	uc.Logger.Log(ctx, log.LevelInfo, "Template loaded successfully", log.Int("size_bytes", len(fileBytes)))

	return fileBytes, nil
}

// renderTemplate renders the template with data from external sources.
func (uc *UseCase) renderTemplate(ctx context.Context, templateBytes []byte, result map[string]map[string][]map[string]any, message GenerateReportMessage, span *trace.Span) (string, error) {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, spanRender := uc.Tracer.Start(ctx, "service.report.render_template")
	defer spanRender.End()

	spanRender.SetAttributes(attribute.String("app.request.request_id", reqId))

	renderer := pongo.NewTemplateRenderer()

	out, err := renderer.RenderFromBytes(ctx, templateBytes, result, uc.Logger)
	if err != nil {
		if errUpdate := uc.updateReportWithErrors(ctx, message.ReportID, err); errUpdate != nil {
			libOtel.HandleSpanError(*span, "Error to update report status with error.", errUpdate)
			uc.Logger.Log(ctx, log.LevelError, "Error update report status with error", log.Err(errUpdate))

			return "", errUpdate
		}

		libOtel.HandleSpanError(spanRender, "Error rendering template.", err)
		uc.Logger.Log(ctx, log.LevelError, "Error rendering template", log.Err(err))

		// Template rendering errors are permanent: same template + same data = same failure on retry.
		return "", pkg.FailedPreconditionError{Code: "REP-0081", Title: "Template Rendering Failed", Message: fmt.Sprintf("error rendering template: %s", err.Error()), Err: err}
	}

	return out, nil
}

// convertToPDFIfNeeded converts HTML to PDF if output format is PDF.
func (uc *UseCase) convertToPDFIfNeeded(ctx context.Context, message GenerateReportMessage, htmlOutput string, span *trace.Span) (string, error) {
	if strings.ToLower(message.OutputFormat) != "pdf" {
		return htmlOutput, nil
	}

	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, spanPDF := uc.Tracer.Start(ctx, "service.report.convert_to_pdf")
	defer spanPDF.End()

	spanPDF.SetAttributes(attribute.String("app.request.request_id", reqId))
	uc.Logger.Log(ctx, log.LevelInfo, "Converting HTML to PDF for report",
		log.String("report_id", message.ReportID.String()), log.Int("html_size_bytes", len(htmlOutput)))

	pdfBytes, err := uc.convertHTMLToPDF(ctx, htmlOutput)
	if err != nil {
		if errUpdate := uc.updateReportWithErrors(ctx, message.ReportID, err); errUpdate != nil {
			libOtel.HandleSpanError(*span, "Error to update report status with error.", errUpdate)
			uc.Logger.Log(ctx, log.LevelError, "Error update report status with error", log.Err(errUpdate))

			return "", errUpdate
		}

		libOtel.HandleSpanError(spanPDF, "Error converting HTML to PDF.", err)
		uc.Logger.Log(ctx, log.LevelError, "Error converting HTML to PDF", log.Err(err))

		return "", err
	}

	uc.Logger.Log(ctx, log.LevelInfo, "PDF generated successfully", log.Int("pdf_size_bytes", len(pdfBytes)))

	return string(pdfBytes), nil
}

// convertHTMLToPDF converts HTML content to PDF using Chrome headless via PDF pool.
// Accepts ctx for consistent observability (logging, tracing) throughout the call chain.
func (uc *UseCase) convertHTMLToPDF(ctx context.Context, htmlContent string) ([]byte, error) {
	tmpFile, err := os.CreateTemp("", "pdf-*.pdf")
	if err != nil {
		uc.Logger.Log(ctx, log.LevelError, "Failed to create temporary PDF file", log.Err(err))
		return nil, fmt.Errorf("failed to create temporary PDF file: %w", err)
	}

	tmpFileName := tmpFile.Name()
	if closeErr := tmpFile.Close(); closeErr != nil {
		uc.Logger.Log(ctx, log.LevelWarn, "Failed to close temporary file",
			log.String("file", tmpFileName), log.Err(closeErr))
	}

	defer func() {
		if removeErr := os.Remove(tmpFileName); removeErr != nil {
			uc.Logger.Log(ctx, log.LevelWarn, "Failed to remove temporary PDF file",
				log.String("file", tmpFileName), log.Err(removeErr))
		}
	}()

	err = uc.PdfPool.Submit(htmlContent, tmpFileName)
	if err != nil {
		uc.Logger.Log(ctx, log.LevelError, "Failed to generate PDF from HTML", log.Err(err))
		return nil, fmt.Errorf("failed to generate PDF from HTML: %w", err)
	}

	// Read generated PDF file - tmpFileName is safe as it comes from os.CreateTemp
	// #nosec G304 -- tmpFileName is generated by os.CreateTemp and is safe
	pdfBytes, err := os.ReadFile(tmpFileName)
	if err != nil {
		uc.Logger.Log(ctx, log.LevelError, "Failed to read generated PDF", log.Err(err))
		return nil, fmt.Errorf("failed to read generated PDF: %w", err)
	}

	return pdfBytes, nil
}
