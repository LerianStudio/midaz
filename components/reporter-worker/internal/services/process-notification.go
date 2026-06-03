// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	pkg "github.com/LerianStudio/reporter/pkg"
	"github.com/LerianStudio/reporter/pkg/constant"
	"github.com/LerianStudio/reporter/pkg/ctxutil"
	"github.com/LerianStudio/reporter/pkg/datasource"
	"github.com/LerianStudio/reporter/pkg/fetcher"

	"github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// ProcessFetcherNotification handles a Fetcher completion notification message.
// It looks up the ExtractionMapping by jobID, checks idempotency (skips if
// already processed), then handles the notification status:
//   - completed: updates mapping, resumes report generation with extracted data
//   - failed: updates mapping and report status to FAILED
func (uc *UseCase) ProcessFetcherNotification(ctx context.Context, body []byte) error {
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.notification.process_fetcher_notification")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.request_id", reqID))

	notification, err := parseNotificationMessage(body)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to parse notification message", err)
		uc.Logger.Log(ctx, log.LevelError, "Failed to parse notification message", log.Err(err))

		return pkg.ValidationError{
			Code:    "NOTIF-0002",
			Title:   "Invalid Notification Message",
			Message: fmt.Sprintf("parse notification: %s", err.Error()),
		}
	}

	span.SetAttributes(
		attribute.String("app.request.fetcher_job_id", notification.JobID),
		attribute.String("app.request.fetcher_status", notification.Status),
	)

	uc.Logger.Log(ctx, log.LevelInfo, "Processing Fetcher notification",
		log.String("job_id", notification.JobID),
		log.String("status", notification.Status))

	// Look up ExtractionMapping by jobID
	mapping, err := uc.ExtractionMappingRepo.FindByJobID(ctx, notification.JobID)
	if err != nil {
		// "no documents in result" means the job was not created by Reporter (e.g., direct
		// Fetcher API call). This is a permanent error — retrying will never succeed.
		if strings.Contains(err.Error(), "no documents") {
			uc.Logger.Log(ctx, log.LevelWarn, "No extraction mapping for job — job may have been created outside Reporter",
				log.String("job_id", notification.JobID))

			return pkg.EntityNotFoundError{
				EntityType: "extraction_mapping",
				Code:       "NOTIF-0001",
				Title:      "Extraction Mapping Not Found",
				Message:    fmt.Sprintf("no extraction mapping for fetcher job %s", notification.JobID),
			}
		}

		libOtel.HandleSpanError(span, "Failed to lookup extraction mapping", err)
		uc.Logger.Log(ctx, log.LevelError, "Failed to lookup extraction mapping",
			log.String("job_id", notification.JobID), log.Err(err))

		return fmt.Errorf("lookup extraction mapping for job %s: %w", notification.JobID, err)
	}

	if mapping == nil {
		return pkg.EntityNotFoundError{
			EntityType: "extraction_mapping",
			Code:       "NOTIF-0001",
			Title:      "Extraction Mapping Not Found",
			Message:    fmt.Sprintf("no extraction mapping found for job %s", notification.JobID),
		}
	}

	span.SetAttributes(
		attribute.String("app.request.report_id", mapping.ReportID),
		attribute.String("app.request.template_id", mapping.TemplateID),
	)

	// Gap 14 (P17): Atomic idempotency via findOneAndUpdate.
	// AtomicClaimPending atomically transitions status from "pending" to "processing".
	// If another worker already claimed this job, claimed=false and we skip.
	claimed, claimErr := uc.ExtractionMappingRepo.AtomicClaimPending(ctx, notification.JobID)
	if claimErr != nil {
		libOtel.HandleSpanError(span, "Failed to atomically claim extraction mapping", claimErr)
		uc.Logger.Log(ctx, log.LevelError, "Failed to atomically claim extraction mapping",
			log.String("job_id", notification.JobID), log.Err(claimErr))

		return fmt.Errorf("atomic claim for job %s: %w", notification.JobID, claimErr)
	}

	if !claimed {
		uc.Logger.Log(ctx, log.LevelInfo, "Notification already claimed by another worker, skipping (idempotent)",
			log.String("job_id", notification.JobID),
			log.String("current_status", mapping.Status))

		return nil
	}

	switch notification.Status {
	case constant.FetcherStatusCompleted:
		return uc.handleCompletedNotification(ctx, notification, mapping)
	case constant.FetcherStatusFailed:
		return uc.handleFailedNotification(ctx, notification, mapping)
	default:
		// This should not happen due to parseNotificationMessage validation,
		// but handle defensively.
		err := fmt.Errorf("unexpected notification status: %s", notification.Status)
		libOtel.HandleSpanError(span, "Unexpected notification status", err)

		return err
	}
}

// handleCompletedNotification processes a successful extraction notification.
// It updates the extraction mapping status, downloads and decrypts extracted data,
// verifies HMAC integrity, converts schema notation, and resumes report generation.
func (uc *UseCase) handleCompletedNotification(
	ctx context.Context,
	notification fetcher.FetcherNotification,
	mapping *datasource.ExtractionMapping,
) error {
	ctx, span := uc.Tracer.Start(ctx, "service.notification.handle_completed")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.fetcher_job_id", notification.JobID),
		attribute.String("app.request.report_id", mapping.ReportID),
		attribute.String("app.request.data_path", notification.GetDataPath()),
	)

	now := time.Now()

	// Update extraction mapping status to completed
	if err := uc.ExtractionMappingRepo.UpdateStatus(ctx, notification.JobID, constant.ExtractionStatusCompleted, &now); err != nil {
		libOtel.HandleSpanError(span, "Failed to update extraction mapping status", err)
		uc.Logger.Log(ctx, log.LevelError, "Failed to update extraction mapping status",
			log.String("job_id", notification.JobID), log.Err(err))

		return fmt.Errorf("update extraction mapping status for job %s: %w", notification.JobID, err)
	}

	uc.Logger.Log(ctx, log.LevelInfo, "Extraction mapping updated to completed",
		log.String("job_id", notification.JobID),
		log.String("report_id", mapping.ReportID))

	// Resume report generation: load template, render with extracted data, save
	reportID, err := uuid.Parse(mapping.ReportID)
	if err != nil {
		libOtel.HandleSpanError(span, "Invalid report ID in mapping", err)

		return pkg.ValidationError{
			Code:    "NOTIF-0003",
			Title:   "Invalid Report ID",
			Message: fmt.Sprintf("invalid report ID %s in extraction mapping: %s", mapping.ReportID, err.Error()),
		}
	}

	templateID, err := uuid.Parse(mapping.TemplateID)
	if err != nil {
		libOtel.HandleSpanError(span, "Invalid template ID in mapping", err)

		return pkg.ValidationError{
			Code:    "NOTIF-0004",
			Title:   "Invalid Template ID",
			Message: fmt.Sprintf("invalid template ID %s in extraction mapping: %s", mapping.TemplateID, err.Error()),
		}
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

	// Load template
	templateBytes, err := uc.loadTemplate(ctx, message, &span)
	if err != nil {
		uc.Logger.Log(ctx, log.LevelError, "Failed to load template during notification handling",
			log.String("job_id", notification.JobID),
			log.String("report_id", mapping.ReportID),
			log.String("template_id", mapping.TemplateID),
			log.Err(err))

		return err
	}

	// Validate that the storage path belongs to the correct tenant (multi-tenant isolation).
	// In MT mode, the path must start with the tenant ID from the extraction mapping.
	dataPath := notification.GetDataPath()
	if mapping.TenantID != "" && !strings.HasPrefix(dataPath, mapping.TenantID+"/") {
		err := fmt.Errorf("storage path tenant mismatch: path %q does not belong to tenant %q", dataPath, mapping.TenantID)
		libOtel.HandleSpanError(span, "Cross-tenant storage path rejected", err)

		return uc.handleErrorWithUpdate(ctx, reportID, &span, "Storage path tenant validation failed", err)
	}

	// Download extracted data from S3
	rawData, err := uc.downloadExtractedData(ctx, dataPath)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to download extracted data", err)
		return uc.handleErrorWithUpdate(ctx, reportID, &span, "Error downloading extracted data", err)
	}

	// Gap 10: Decrypt the data if APP_ENC_KEY is configured
	decryptedData, err := uc.decryptExtractedData(ctx, rawData)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to decrypt extracted data", err)
		return uc.handleErrorWithUpdate(ctx, reportID, &span, "Error decrypting extracted data", err)
	}

	// Gap 11: HMAC audit (log-only per D6)
	uc.auditHMAC(ctx, decryptedData, notification.GetHMAC())

	// Parse decrypted JSON into result map
	result, err := parseExtractedData(decryptedData)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to parse extracted data", err)
		return uc.handleErrorWithUpdate(ctx, reportID, &span, "Error parsing extracted data", err)
	}

	// Gap 12: Convert schema notation (schema.table -> schema__table) for Pongo2 compatibility
	result = convertResultSchemaNotation(result)

	// Render template with extracted data
	renderedOutput, err := uc.renderTemplate(ctx, templateBytes, result, message, &span)
	if err != nil {
		return err
	}

	// Convert to PDF if needed
	finalOutput, err := uc.convertToPDFIfNeeded(ctx, message, renderedOutput, &span)
	if err != nil {
		return err
	}

	// Save report
	if err := uc.saveReport(ctx, message, finalOutput); err != nil {
		return uc.handleErrorWithUpdate(ctx, reportID, &span, "Error saving report after extraction", err)
	}

	// Mark report as finished
	if err := uc.markReportAsFinished(ctx, reportID, &span); err != nil {
		return err
	}

	uc.Logger.Log(ctx, log.LevelInfo, "Report generation resumed and completed after Fetcher extraction",
		log.String("job_id", notification.JobID),
		log.String("report_id", mapping.ReportID))

	return nil
}

// handleFailedNotification processes a failed extraction notification.
// It updates the extraction mapping status to failed and marks the report as FAILED.
func (uc *UseCase) handleFailedNotification(
	ctx context.Context,
	notification fetcher.FetcherNotification,
	mapping *datasource.ExtractionMapping,
) error {
	ctx, span := uc.Tracer.Start(ctx, "service.notification.handle_failed")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.fetcher_job_id", notification.JobID),
		attribute.String("app.request.report_id", mapping.ReportID),
	)

	now := time.Now()

	// Update extraction mapping status to failed
	if err := uc.ExtractionMappingRepo.UpdateStatus(ctx, notification.JobID, constant.ExtractionStatusFailed, &now); err != nil {
		libOtel.HandleSpanError(span, "Failed to update extraction mapping status", err)
		uc.Logger.Log(ctx, log.LevelError, "Failed to update extraction mapping status",
			log.String("job_id", notification.JobID), log.Err(err))

		return fmt.Errorf("update extraction mapping status for job %s: %w", notification.JobID, err)
	}

	reportID, err := uuid.Parse(mapping.ReportID)
	if err != nil {
		libOtel.HandleSpanError(span, "Invalid report ID in mapping", err)

		return pkg.ValidationError{
			Code:    "NOTIF-0003",
			Title:   "Invalid Report ID",
			Message: fmt.Sprintf("invalid report ID %s in extraction mapping: %s", mapping.ReportID, err.Error()),
		}
	}

	// Update report status to Error with extraction failure metadata
	extractionErr := fmt.Errorf("fetcher extraction failed: %s", notification.GetErrorMessage())
	if errUpdate := uc.updateReportWithErrors(ctx, reportID, extractionErr); errUpdate != nil {
		libOtel.HandleSpanError(span, "Failed to update report status to Error", errUpdate)
		return errUpdate
	}

	uc.Logger.Log(ctx, log.LevelInfo, "Report marked as failed due to extraction failure",
		log.String("job_id", notification.JobID),
		log.String("report_id", mapping.ReportID),
		log.String("error", notification.GetErrorMessage()))

	return nil
}

// parseNotificationMessage unmarshals and validates a Fetcher notification message.
func parseNotificationMessage(body []byte) (fetcher.FetcherNotification, error) {
	var notification fetcher.FetcherNotification

	if err := json.Unmarshal(body, &notification); err != nil {
		return notification, fmt.Errorf("unmarshal fetcher notification: %w", err)
	}

	if notification.JobID == "" {
		return notification, fmt.Errorf("jobId is required in fetcher notification")
	}

	if notification.Status == "" {
		return notification, fmt.Errorf("status is required in fetcher notification")
	}

	if notification.Status != constant.FetcherStatusCompleted && notification.Status != constant.FetcherStatusFailed {
		return notification, fmt.Errorf("invalid notification status: %s (expected %q or %q)",
			notification.Status, constant.FetcherStatusCompleted, constant.FetcherStatusFailed)
	}

	if notification.Status == constant.FetcherStatusCompleted && notification.GetDataPath() == "" {
		return notification, fmt.Errorf("result.path is required when status is %q", constant.FetcherStatusCompleted)
	}

	return notification, nil
}
