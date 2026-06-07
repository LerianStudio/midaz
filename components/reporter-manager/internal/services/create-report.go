// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	pkg "github.com/LerianStudio/midaz/v4/pkg"
	cnErr "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb/report"

	"github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
	goRedis "github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// CreateReport create a new report
func (uc *UseCase) CreateReport(ctx context.Context, reportInput *model.CreateReportInput) (*report.Report, error) {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	var (
		idempotencyKey   string
		shouldCleanupKey bool
	)

	ctx, span := uc.Tracer.Start(ctx, "service.report.create")
	defer span.End()
	defer func() {
		if shouldCleanupKey {
			uc.releaseIdempotencyKey(ctx, idempotencyKey)
		}
	}()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.template_id", reportInput.TemplateID),
		attribute.Int("app.request.filter_datasource_count", len(reportInput.Filters)),
	)

	uc.Logger.Log(ctx, log.LevelInfo, "Creating report",
		log.String("template_id", reportInput.TemplateID),
		log.Int("filter_datasource_count", len(reportInput.Filters)),
	)

	// Idempotency check: acquire lock via Redis SetNX before proceeding
	if uc.RedisRepo != nil {
		cachedResult, key, acquired, err := uc.checkReportIdempotency(ctx, reportInput, span)
		if err != nil {
			return nil, err
		}

		idempotencyKey = key
		shouldCleanupKey = acquired

		if cachedResult != nil {
			return cachedResult, nil
		}
	}

	templateID, outputFormat, mappedFields, templateDescription, err := uc.prepareReportCreation(ctx, span, reportInput)
	if err != nil {
		return nil, err
	}

	result, err := uc.persistReport(ctx, span, templateID, reportInput.Filters, outputFormat, templateDescription)
	if err != nil {
		return nil, err
	}

	if err := uc.sendReportMessage(ctx, span, result, templateID, outputFormat, mappedFields, reportInput.Filters); err != nil {
		return nil, err
	}

	uc.finalizeReportIdempotency(ctx, reportInput, result, &idempotencyKey, &shouldCleanupKey)

	return result, nil
}

func (uc *UseCase) prepareReportCreation(ctx context.Context, span trace.Span, reportInput *model.CreateReportInput) (uuid.UUID, *string, map[string]map[string][]string, string, error) {
	templateID, errParseUUID := uuid.Parse(reportInput.TemplateID)
	if errParseUUID != nil {
		errInvalidID := pkg.ValidateBusinessError(cnErr.ErrInvalidTemplateID, "")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid template ID format", errInvalidID)

		return uuid.Nil, nil, nil, "", errInvalidID
	}

	outputFormat, mappedFields, templateDescription, err := uc.TemplateRepo.FindMappedFieldsAndOutputFormatByID(ctx, templateID)
	if err != nil {
		uc.Logger.Log(ctx, log.LevelError, "Error to find template by id", log.Err(err))

		if nf := (pkg.EntityNotFoundError{}); errors.As(err, &nf) {
			errNotFound := pkg.ValidateBusinessError(cnErr.ErrEntityNotFound, "", constant.MongoCollectionTemplate)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Template not found", errNotFound)

			return uuid.Nil, nil, nil, "", errNotFound
		}

		libOpentelemetry.HandleSpanError(span, "Failed to find template by ID", err)

		return uuid.Nil, nil, nil, "", err
	}

	// Validate filter fields against schema (direct mode only).
	// In fetcher mode, filter validation is skipped because the fetcher handles
	// schema-qualified table names internally and the reporter's filter table
	// names (simple) don't match the fetcher's qualified names.
	if reportInput.Filters != nil && !uc.isFetcherMode() {
		if err := uc.validateReportFilters(ctx, reportInput.Filters, span); err != nil {
			return uuid.Nil, nil, nil, "", err
		}
	}

	return templateID, outputFormat, mappedFields, templateDescription, nil
}

func (uc *UseCase) persistReport(ctx context.Context, span trace.Span, templateID uuid.UUID, filters map[string]map[string]map[string]model.FilterCondition, outputFormat *string, templateDescription string) (*report.Report, error) {
	reportID, err := commons.GenerateUUIDv7()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to generate report ID", err)
		return nil, err
	}

	var outFmt string
	if outputFormat != nil {
		outFmt = *outputFormat
	}

	reportModel, err := report.NewReport(reportID, templateID, constant.ProcessingStatus, filters, outFmt, templateDescription)
	if err != nil {
		if pkg.IsBusinessError(err) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create report entity", err)
		} else {
			libOpentelemetry.HandleSpanError(span, "Failed to create report entity", err)
		}

		return nil, err
	}

	result, err := uc.ReportRepo.Create(ctx, reportModel)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to create report in repository", err)
		uc.Logger.Log(ctx, log.LevelError, "Error creating report in database", log.Err(err))

		return nil, err
	}

	return result, nil
}

func (uc *UseCase) sendReportMessage(ctx context.Context, span trace.Span, result *report.Report, templateID uuid.UUID, outputFormat *string, mappedFields map[string]map[string][]string, filters map[string]map[string]map[string]model.FilterCondition) error {
	reportMessage := model.ReportMessage{
		TemplateID:   templateID,
		ReportID:     result.ID,
		Filters:      filters,
		OutputFormat: *outputFormat,
		MappedFields: mappedFields,
	}

	uc.Logger.Log(ctx, log.LevelInfo, "Sending report to reports queue...")

	if err := uc.SendReportQueueReports(ctx, reportMessage); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to send report to queue", err)
		uc.Logger.Log(ctx, log.LevelError, "Error sending report to queue", log.Err(err))

		metadata := map[string]any{"error": "Failed to send report to queue"}
		if updateErr := uc.ReportRepo.UpdateReportStatusById(ctx, constant.ErrorStatus, result.ID, time.Now(), metadata); updateErr != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to update report status to error", updateErr)
			uc.Logger.Log(ctx, log.LevelError, "Error updating report status to error", log.Err(updateErr))
		}

		return err
	}

	return nil
}

func (uc *UseCase) finalizeReportIdempotency(ctx context.Context, reportInput *model.CreateReportInput, result *report.Report, idempotencyKey *string, shouldCleanupKey *bool) {
	if uc.RedisRepo == nil {
		return
	}

	if *idempotencyKey == "" {
		key, keyErr := uc.buildIdempotencyKey(ctx, reportInput)
		if keyErr == nil {
			*idempotencyKey = key
		}
	}

	if *idempotencyKey != "" {
		if cacheErr := uc.cacheIdempotencyResult(ctx, *idempotencyKey, result); cacheErr == nil {
			*shouldCleanupKey = false
		}
	}
}

// checkReportIdempotency acquires an idempotency lock via Redis SetNX.
// Returns a cached report if this is a duplicate request, or nil to proceed with creation.
func (uc *UseCase) checkReportIdempotency(ctx context.Context, reportInput *model.CreateReportInput, span trace.Span) (*report.Report, string, bool, error) {
	idempotencyKey, keyErr := uc.buildIdempotencyKey(ctx, reportInput)
	if keyErr != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to compute idempotency key", keyErr)

		return nil, "", false, keyErr
	}

	span.SetAttributes(attribute.String("app.idempotency.key", idempotencyKey))
	uc.Logger.Log(ctx, log.LevelInfo, "Checking idempotency", log.String("key", idempotencyKey))

	acquired, setNXErr := uc.RedisRepo.SetNX(ctx, idempotencyKey, "processing", constant.IdempotencyTTL)
	if setNXErr != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to acquire idempotency lock", setNXErr)

		return nil, idempotencyKey, false, setNXErr
	}

	if !acquired {
		cachedResult, err := uc.handleDuplicateRequest(ctx, idempotencyKey)
		return cachedResult, idempotencyKey, false, err
	}

	return nil, idempotencyKey, true, nil
}

// validateReportFilters validates that all filter fields exist on their respective tables
// using the DataSourceProvider interface.
func (uc *UseCase) validateReportFilters(ctx context.Context, filters map[string]map[string]map[string]model.FilterCondition, span trace.Span) error {
	filtersMapped := uc.convertFiltersToMappedFieldsType(filters)

	_, errValidate := uc.ValidateSchemaViaProvider(ctx, filtersMapped)
	if errValidate != nil {
		if pkg.IsBusinessError(errValidate) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate filter fields existence on tables", errValidate)
		} else {
			libOpentelemetry.HandleSpanError(span, "Failed to validate filter fields existence on tables", errValidate)
		}

		return errValidate
	}

	return nil
}

// buildIdempotencyKey resolves the idempotency key for the request.
// If a client-provided Idempotency-Key header value exists in context, it is used as-is.
// Otherwise, a SHA256 hash of the JSON-serialized request body is computed.
func (uc *UseCase) buildIdempotencyKey(ctx context.Context, reportInput *model.CreateReportInput) (string, error) {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.report.build_idempotency_key")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.request_id", reqId))

	// Check for client-provided idempotency key from context
	if clientKey, ok := ctx.Value(constant.IdempotencyKeyCtx).(string); ok && clientKey != "" {
		key := constant.IdempotencyKeyPrefix + ":" + clientKey
		uc.Logger.Log(ctx, log.LevelInfo, "Using client-provided idempotency key", log.String("key", key))

		return key, nil
	}

	// Compute SHA256 hash of the serialized request body
	data, err := json.Marshal(reportInput)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to marshal report input for idempotency hash", err)

		return "", fmt.Errorf("failed to marshal report input for idempotency hash: %w", err)
	}

	hash := commons.HashSHA256(string(data))
	key := constant.IdempotencyKeyPrefix + ":" + hash
	uc.Logger.Log(ctx, log.LevelInfo, "Computed idempotency key from request body hash", log.String("key", key))

	return key, nil
}

// handleDuplicateRequest handles the case where SetNX returned false (key already exists).
// It attempts to retrieve the cached response from Redis. If a cached response exists,
// it is unmarshaled and returned. If no cached response exists yet (in-flight request),
// an error is returned indicating a duplicate in-flight request.
func (uc *UseCase) handleDuplicateRequest(ctx context.Context, idempotencyKey string) (*report.Report, error) {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, childSpan := uc.Tracer.Start(ctx, "service.report.handle_duplicate_request")
	defer childSpan.End()

	childSpan.SetAttributes(attribute.String("app.request.request_id", reqId))
	uc.Logger.Log(ctx, log.LevelInfo, "Duplicate request detected", log.String("idempotency_key", idempotencyKey))

	cachedResponse, getErr := uc.RedisRepo.Get(ctx, idempotencyKey)
	if getErr != nil {
		// The lock existed at SetNX but the key is gone by the time we Get it:
		// the first request released it after a failure, or its TTL lapsed in
		// the window between SetNX-false and Get. There is no cached result to
		// replay; surface the same in-flight conflict so the client retries
		// rather than leaking the raw driver sentinel as a 500.
		if errors.Is(getErr, goRedis.Nil) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(childSpan, "Idempotency key vanished between SetNX and Get", cnErr.ErrDuplicateRequestInFlight)

			return nil, pkg.ValidateBusinessError(cnErr.ErrDuplicateRequestInFlight, "report")
		}

		libOpentelemetry.HandleSpanError(childSpan, "Failed to retrieve cached idempotency response", getErr)

		return nil, getErr
	}

	// If the cached value is empty or still "processing", the first request is still in-flight
	if cachedResponse == "" || cachedResponse == "processing" {
		libOpentelemetry.HandleSpanBusinessErrorEvent(childSpan, "Duplicate in-flight request detected", cnErr.ErrDuplicateRequestInFlight)

		return nil, pkg.ValidateBusinessError(cnErr.ErrDuplicateRequestInFlight, "report")
	}

	// Unmarshal the cached response
	var cachedReport report.Report
	if unmarshalErr := json.Unmarshal([]byte(cachedResponse), &cachedReport); unmarshalErr != nil {
		libOpentelemetry.HandleSpanError(childSpan, "Failed to unmarshal cached idempotency response", unmarshalErr)

		return nil, fmt.Errorf("failed to unmarshal cached idempotency response: %w", unmarshalErr)
	}

	// Signal to the handler that this is a replayed response via pointer mutation in context
	if replayedPtr, ok := ctx.Value(constant.IdempotencyReplayedCtx).(*bool); ok && replayedPtr != nil {
		*replayedPtr = true
	}

	uc.Logger.Log(ctx, log.LevelInfo, "Returning cached idempotent response", log.String("key", idempotencyKey))

	return &cachedReport, nil
}

// cacheIdempotencyResult caches the successful report creation result in Redis
// so that future duplicate requests can return the cached response.
func (uc *UseCase) cacheIdempotencyResult(ctx context.Context, idempotencyKey string, result *report.Report) error {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.report.cache_idempotency_result")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.request_id", reqId))

	data, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to marshal report result for idempotency cache", marshalErr)
		uc.Logger.Log(ctx, log.LevelError, "Failed to marshal report result for idempotency cache", log.Err(marshalErr))

		return marshalErr
	}

	if setErr := uc.RedisRepo.Set(ctx, idempotencyKey, string(data), constant.IdempotencyTTL); setErr != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to cache idempotency result", setErr)
		uc.Logger.Log(ctx, log.LevelError, "Failed to cache idempotency result", log.Err(setErr))

		return setErr
	}

	return nil
}

func (uc *UseCase) releaseIdempotencyKey(ctx context.Context, idempotencyKey string) {
	if uc.RedisRepo == nil || idempotencyKey == "" {
		return
	}

	if err := uc.RedisRepo.Del(ctx, idempotencyKey); err != nil {
		uc.Logger.Log(ctx, log.LevelWarn, "Failed to release idempotency key", log.String("key", idempotencyKey), log.Err(err))
	}
}

// convertFiltersToMappedFieldsType transforms a deeply nested filter map into a mapped fields structure with limited keys per level.
func (uc *UseCase) convertFiltersToMappedFieldsType(filters map[string]map[string]map[string]model.FilterCondition) map[string]map[string][]string {
	output := make(map[string]map[string][]string)

	for topKey, nested := range filters {
		output[topKey] = make(map[string][]string)

		for midKey, inner := range nested {
			var keys []string

			count := 0

			for innerKey := range inner {
				keys = append(keys, innerKey)

				count++
				if count == constant.MaxSchemaPreviewKeys {
					break
				}
			}

			output[topKey][midKey] = keys
		}
	}

	return output
}
