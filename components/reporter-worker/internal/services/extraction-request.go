// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/datasource"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/fetcher"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// requestFetcherExtraction dispatches data extraction to the Fetcher service,
// creates an ExtractionMapping in MongoDB, and sets the report status to
// PENDING_EXTRACTION. This is the "Fetcher mode" path of dual-mode dispatch.
//
// The method returns nil on success because the report generation is now
// asynchronous -- the Fetcher will notify back when extraction completes (T9).
func (uc *UseCase) requestFetcherExtraction(
	ctx context.Context,
	message GenerateReportMessage,
	span *trace.Span,
) error {
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, extractSpan := uc.Tracer.Start(ctx, "service.report.request_fetcher_extraction")
	defer extractSpan.End()

	extractSpan.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.report_id", message.ReportID.String()),
		attribute.String("app.request.template_id", message.TemplateID.String()),
	)

	uc.Logger.Log(ctx, log.LevelInfo, "Dispatching extraction to Fetcher (dual-mode)",
		log.String("report_id", message.ReportID.String()),
		log.String("template_id", message.TemplateID.String()))

	if err := uc.dispatchExtraction(ctx, message); err != nil {
		return uc.handleErrorWithUpdate(ctx, message.ReportID, span, "Error dispatching extraction to Fetcher", err)
	}

	// Update report status to PENDING_EXTRACTION
	if err := uc.ReportDataRepo.UpdateReportStatusById(
		ctx,
		constant.PendingExtractionStatus,
		message.ReportID,
		time.Time{}, // zero time -- not completed yet
		nil,
	); err != nil {
		libOtel.HandleSpanError(extractSpan, "Failed to update report status to PendingExtraction", err)
		return uc.handleErrorWithUpdate(ctx, message.ReportID, span, "Error updating report to PendingExtraction", err)
	}

	uc.Logger.Log(ctx, log.LevelInfo, "Report moved to PendingExtraction, awaiting Fetcher completion",
		log.String("report_id", message.ReportID.String()))

	return nil
}

// dispatchExtraction creates a single extraction job with all datasources
// as MappedFields and saves the corresponding ExtractionMapping in MongoDB.
func (uc *UseCase) dispatchExtraction(
	ctx context.Context,
	message GenerateReportMessage,
) error {
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.report.dispatch_extraction")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.report_id", message.ReportID.String()),
		attribute.Int("app.request.datasource_count", len(message.DataQueries)),
	)

	// Resolve tenant ID from context (empty string in single-tenant mode)
	tenantID := resolveTenantID(ctx)

	// Convert reporter filters to Fetcher's nested format.
	fetcherFilters := convertToFetcherFilters(message.Filters)

	// Convert Pongo2 notation (schema__table) back to dot notation (schema.table)
	// for the Fetcher API. The reporter stores mappedFields with __ separators
	// for Pongo2 template compatibility, but the Fetcher expects dot notation.
	fetcherMappedFields := convertMappedFieldsToDotNotation(message.DataQueries)

	jobReq := fetcher.CreateExtractionJobRequest{
		DataRequest: fetcher.ExtractionDataRequest{
			MappedFields: fetcherMappedFields,
			Filters:      fetcherFilters,
		},
		Metadata: map[string]any{
			"source":     constant.FetcherNotificationRoutingSource,
			"reportId":   message.ReportID.String(),
			"templateId": message.TemplateID.String(),
			"tenantId":   tenantID,
		},
	}

	uc.Logger.Log(ctx, log.LevelInfo, "Creating extraction job via Fetcher",
		log.String("report_id", message.ReportID.String()),
		log.Int("datasource_count", len(message.DataQueries)))

	resp, err := uc.FetcherClient.CreateExtractionJob(ctx, jobReq)
	if err != nil {
		libOtel.HandleSpanError(span, "Fetcher CreateExtractionJob failed", err)
		return fmt.Errorf("fetcher extraction job creation failed: %w", err)
	}

	if resp == nil {
		return pkg.FailedPreconditionError{Code: constant.ErrCodeInvalidFetcherResponse, Title: "Invalid Fetcher Response", Message: "fetcher returned nil response for extraction job"}
	}

	span.SetAttributes(attribute.String("app.request.fetcher_job_id", resp.JobID))

	// Save the extraction mapping
	mapping := &datasource.ExtractionMapping{
		JobID:        resp.JobID,
		ReportID:     message.ReportID.String(),
		TemplateID:   message.TemplateID.String(),
		TenantID:     tenantID,
		OutputFormat: message.OutputFormat,
		Status:       constant.ExtractionStatusPending,
		CreatedAt:    time.Now(),
	}

	if err := uc.ExtractionMappingRepo.Create(ctx, mapping); err != nil {
		libOtel.HandleSpanError(span, "Failed to save extraction mapping", err)
		return fmt.Errorf("failed to save extraction mapping for job %s: %w", resp.JobID, err)
	}

	uc.Logger.Log(ctx, log.LevelInfo, "Extraction mapping saved",
		log.String("job_id", resp.JobID),
		log.String("report_id", message.ReportID.String()))

	return nil
}

// convertToFetcherFilters converts the reporter's filter structure to the Fetcher's
// NestedFilters format: map[datasource]map[table]map[field]FilterCondition.
func convertToFetcherFilters(
	filters map[string]map[string]map[string]model.FilterCondition,
) map[string]map[string]map[string]fetcher.FilterCondition {
	if len(filters) == 0 {
		return nil
	}

	result := make(map[string]map[string]map[string]fetcher.FilterCondition)

	for dsName, tables := range filters {
		dsTables := make(map[string]map[string]fetcher.FilterCondition)

		for tableName, fields := range tables {
			tableFields := make(map[string]fetcher.FilterCondition)

			for fieldName, condition := range fields {
				tableFields[fieldName] = fetcher.FilterCondition{
					Equals:         condition.Equals,
					GreaterThan:    condition.GreaterThan,
					GreaterOrEqual: condition.GreaterOrEqual,
					LessThan:       condition.LessThan,
					LessOrEqual:    condition.LessOrEqual,
					Between:        condition.Between,
					In:             condition.In,
					NotIn:          condition.NotIn,
				}
			}

			// Convert Pongo2 notation (schema__table) to dot notation (schema.table)
			// to match the same conversion applied to mappedFields table keys.
			convertedTableName := tableName
			if strings.Contains(tableName, "__") {
				convertedTableName = strings.ReplaceAll(tableName, "__", ".")
			}

			dsTables[convertedTableName] = tableFields
		}

		result[dsName] = dsTables
	}

	return result
}

// resolveTenantID extracts the tenant ID from context using the lib-commons
// tenant-manager API. Returns empty string in single-tenant mode (no tenant
// in context).
func resolveTenantID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	return tmcore.GetTenantIDContext(ctx)
}

// convertMappedFieldsToDotNotation converts table keys from Pongo2 notation
// (schema__table) back to SQL dot notation (schema.table) for the Fetcher API.
func convertMappedFieldsToDotNotation(dataQueries map[string]map[string][]string) map[string]map[string][]string {
	result := make(map[string]map[string][]string, len(dataQueries))

	for dsName, tables := range dataQueries {
		convertedTables := make(map[string][]string, len(tables))

		for tableKey, fields := range tables {
			dotKey := strings.ReplaceAll(tableKey, "__", ".")
			convertedTables[dotKey] = fields
		}

		result[dsName] = convertedTables
	}

	return result
}

// isFetcherMode returns true if the UseCase is configured for Fetcher-based extraction.
func (uc *UseCase) isFetcherMode() bool {
	return uc.FetcherClient != nil
}

// generateReportData orchestrates data retrieval using either Fetcher mode or direct mode.
// This is the dual-mode dispatch entry point called from GenerateReport.
func (uc *UseCase) generateReportData(
	ctx context.Context,
	message GenerateReportMessage,
	result map[string]map[string][]map[string]any,
	span *trace.Span,
	reportID uuid.UUID,
) error {
	if uc.isFetcherMode() {
		return uc.requestFetcherExtraction(ctx, message, span)
	}

	// Direct mode: query external datasources directly (legacy path)
	if err := uc.queryExternalData(ctx, message, result); err != nil {
		return uc.handleErrorWithUpdate(ctx, reportID, span, "Error querying external data", err)
	}

	return nil
}
