// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/datasource"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/fetcher"

	"github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// handleReconcileCompleted processes a stale mapping that Fetcher reports as completed.
// It triggers the completion flow: updates mapping status, then resumes report generation.
func (r *Reconciler) handleReconcileCompleted(
	ctx context.Context,
	mapping *datasource.ExtractionMapping,
	jobResp *fetcher.ExtractionJobResponse,
) {
	ctx, span := r.tracer.Start(ctx, "service.reconciler.handle_completed")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.job_id", mapping.JobID),
		attribute.String("app.request.report_id", mapping.ReportID),
	)

	now := time.Now()

	if err := r.extractionRepo.UpdateStatus(ctx, mapping.JobID, constant.ExtractionStatusCompleted, &now); err != nil {
		libOtel.HandleSpanError(span, "Failed to update extraction mapping status to completed", err)
		r.logger.Log(ctx, log.LevelError, "Failed to update extraction mapping status",
			log.String("job_id", mapping.JobID), log.Err(err))

		return
	}

	r.logger.Log(ctx, log.LevelInfo, "Reconciler: extraction mapping updated to completed",
		log.String("job_id", mapping.JobID),
		log.String("report_id", mapping.ReportID))

	// Resume report generation via the UseCase pipeline
	reportID, err := uuid.Parse(mapping.ReportID)
	if err != nil {
		libOtel.HandleSpanError(span, "Invalid report ID in stale mapping", err)
		r.logger.Log(ctx, log.LevelError, "Invalid report ID in extraction mapping",
			log.String("report_id", mapping.ReportID), log.Err(err))

		return
	}

	templateID, err := uuid.Parse(mapping.TemplateID)
	if err != nil {
		libOtel.HandleSpanError(span, "Invalid template ID in stale mapping", err)
		r.logger.Log(ctx, log.LevelError, "Invalid template ID in extraction mapping",
			log.String("template_id", mapping.TemplateID), log.Err(err))

		return
	}

	// Build a GenerateReportMessage to reuse existing pipeline helpers.
	// Restore the original output format from the extraction mapping.
	outputFormat := mapping.OutputFormat
	if outputFormat == "" {
		outputFormat = "html"
	}

	message := GenerateReportMessage{
		TemplateID:   templateID,
		ReportID:     reportID,
		OutputFormat: outputFormat,
	}

	templateBytes, err := r.useCase.loadTemplate(ctx, message, &span)
	if err != nil {
		r.logger.Log(ctx, log.LevelError, "Reconciler: failed to load template",
			log.String("job_id", mapping.JobID), log.Err(err))

		return
	}

	// Download, decrypt, verify, and parse the extracted data from storage
	dataPath := ""
	dataHMAC := ""

	if jobResp.Result != nil {
		dataPath = jobResp.Result.Path
		dataHMAC = jobResp.Result.HMAC
	}

	if dataPath == "" {
		r.logger.Log(ctx, log.LevelError, "Reconciler: completed job has no data path",
			log.String("job_id", mapping.JobID))

		return
	}

	rawData, err := r.useCase.downloadExtractedData(ctx, dataPath)
	if err != nil {
		r.logger.Log(ctx, log.LevelError, "Reconciler: failed to download extracted data",
			log.String("job_id", mapping.JobID), log.Err(err))

		return
	}

	decryptedData, err := r.useCase.decryptExtractedData(ctx, rawData)
	if err != nil {
		r.logger.Log(ctx, log.LevelError, "Reconciler: failed to decrypt extracted data",
			log.String("job_id", mapping.JobID), log.Err(err))

		return
	}

	r.useCase.auditHMAC(ctx, decryptedData, dataHMAC)

	result, err := parseExtractedData(decryptedData)
	if err != nil {
		r.logger.Log(ctx, log.LevelError, "Reconciler: failed to parse extracted data",
			log.String("job_id", mapping.JobID), log.Err(err))

		return
	}

	result = convertResultSchemaNotation(result)

	renderedOutput, err := r.useCase.renderTemplate(ctx, templateBytes, result, message, &span)
	if err != nil {
		r.logger.Log(ctx, log.LevelError, "Reconciler: failed to render template",
			log.String("job_id", mapping.JobID), log.Err(err))

		return
	}

	finalOutput, err := r.useCase.convertToPDFIfNeeded(ctx, message, renderedOutput, &span)
	if err != nil {
		r.logger.Log(ctx, log.LevelError, "Reconciler: failed to convert to PDF",
			log.String("job_id", mapping.JobID), log.Err(err))

		return
	}

	if err := r.useCase.saveReport(ctx, message, finalOutput); err != nil {
		if updateErr := r.useCase.handleErrorWithUpdate(ctx, reportID, &span, "Error saving report after reconciliation", err); updateErr != nil {
			r.logger.Log(ctx, log.LevelError, "Reconciler: failed to save report and update status",
				log.String("job_id", mapping.JobID), log.Err(updateErr))
		}

		return
	}

	if err := r.useCase.markReportAsFinished(ctx, reportID, &span); err != nil {
		r.logger.Log(ctx, log.LevelError, "Reconciler: failed to mark report as finished",
			log.String("job_id", mapping.JobID), log.Err(err))
	}
}

// handleReconcileFailed processes a stale mapping that Fetcher reports as failed.
// It marks the extraction mapping as failed and updates the report status to Error.
func (r *Reconciler) handleReconcileFailed(
	ctx context.Context,
	mapping *datasource.ExtractionMapping,
	jobResp *fetcher.ExtractionJobResponse,
) {
	ctx, span := r.tracer.Start(ctx, "service.reconciler.handle_failed")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.job_id", mapping.JobID),
		attribute.String("app.request.report_id", mapping.ReportID),
	)

	now := time.Now()

	if err := r.extractionRepo.UpdateStatus(ctx, mapping.JobID, constant.ExtractionStatusFailed, &now); err != nil {
		libOtel.HandleSpanError(span, "Failed to update extraction mapping status to failed", err)
		r.logger.Log(ctx, log.LevelError, "Failed to update extraction mapping status",
			log.String("job_id", mapping.JobID), log.Err(err))

		return
	}

	reportID, err := uuid.Parse(mapping.ReportID)
	if err != nil {
		libOtel.HandleSpanError(span, "Invalid report ID in stale mapping", err)
		r.logger.Log(ctx, log.LevelError, "Invalid report ID in extraction mapping",
			log.String("report_id", mapping.ReportID), log.Err(err))

		return
	}

	extractionErr := fmt.Errorf("fetcher extraction failed (reconciled): %s", jobResp.Error)
	if errUpdate := r.useCase.updateReportWithErrors(ctx, reportID, extractionErr); errUpdate != nil {
		libOtel.HandleSpanError(span, "Failed to update report status to Error", errUpdate)
		r.logger.Log(ctx, log.LevelError, "Failed to update report status to Error",
			log.String("job_id", mapping.JobID), log.Err(errUpdate))

		return
	}

	r.logger.Log(ctx, log.LevelInfo, "Reconciler: report marked as failed due to extraction failure",
		log.String("job_id", mapping.JobID),
		log.String("report_id", mapping.ReportID),
		log.String("error", jobResp.Error))
}
