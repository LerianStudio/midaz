// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"strings"

	"github.com/LerianStudio/reporter/pkg"
	"github.com/LerianStudio/reporter/pkg/constant"
	"github.com/LerianStudio/reporter/pkg/ctxutil"
	"github.com/LerianStudio/reporter/pkg/datasource"
	"github.com/LerianStudio/reporter/pkg/mongodb/template"
	pkgHTTP "github.com/LerianStudio/reporter/pkg/net/http"
	templateUtils "github.com/LerianStudio/reporter/pkg/templateutils"

	"github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// CreateTemplate creates a new template with specified parameters, stores it in the repository,
// uploads the file to object storage, and performs a compensating transaction on storage failure.
//
// The third return value contains validation warnings. When a data source is
// unavailable during schema validation, warnings are returned instead of errors, allowing
// the template to be created with partial validation.
func (uc *UseCase) CreateTemplate(ctx context.Context, templateFile, outFormat, description string, fileHeader *multipart.FileHeader) (*template.Template, []datasource.ValidationWarning, error) {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	var (
		idempotencyKey   string
		shouldCleanupKey bool
	)

	ctx, span := uc.Tracer.Start(ctx, "service.template.create")
	defer span.End()
	defer func() {
		if shouldCleanupKey {
			uc.releaseTemplateIdempotencyKey(ctx, idempotencyKey)
		}
	}()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.output_format", outFormat),
		attribute.Bool("app.request.has_description", description != ""),
		attribute.Int("app.request.template_size_bytes", len(templateFile)),
	)
	uc.Logger.Log(ctx, log.LevelInfo, "Creating template",
		log.String("output_format", outFormat),
		log.Bool("has_description", description != ""),
		log.Int("template_size_bytes", len(templateFile)),
	)

	// Idempotency check: acquire lock via Redis SetNX before proceeding
	if uc.RedisRepo != nil {
		cachedResult, key, acquired, err := uc.checkTemplateIdempotency(ctx, templateFile, outFormat, description, span)
		if err != nil {
			return nil, nil, err
		}

		idempotencyKey = key
		shouldCleanupKey = acquired

		if cachedResult != nil {
			return cachedResult, nil, nil
		}
	}

	transformedMappedFields, mappedFields, err := uc.prepareTemplateCreation(ctx, span, templateFile)
	if err != nil {
		return nil, nil, err
	}

	// Validate schema via DataSourceProvider
	warnings, errSchema := uc.ValidateSchemaViaProvider(ctx, mappedFields)
	if errSchema != nil {
		return nil, nil, errSchema
	}

	resultTemplateModel, err := uc.persistTemplate(ctx, span, outFormat, description, transformedMappedFields)
	if err != nil {
		return nil, nil, err
	}

	if err := uc.storeCreatedTemplateFile(ctx, span, resultTemplateModel, fileHeader, outFormat); err != nil {
		return nil, nil, err
	}

	uc.finalizeTemplateIdempotency(ctx, templateFile, outFormat, description, resultTemplateModel, &idempotencyKey, &shouldCleanupKey)

	return resultTemplateModel, warnings, nil
}

func (uc *UseCase) prepareTemplateCreation(ctx context.Context, span trace.Span, templateFile string) (map[string]map[string][]string, map[string]map[string][]string, error) {
	if err := templateUtils.ValidateNoScriptTag(templateFile); err != nil {
		errScript := pkg.ValidateBusinessError(constant.ErrScriptTagDetected, "")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Script tag detected in template", errScript)

		return nil, nil, errScript
	}

	mappedFields := templateUtils.MappedFieldsOfTemplate(templateFile)
	uc.Logger.Log(ctx, log.LevelInfo, "Mapped Fields extracted from template", log.Any("mapped_fields", mappedFields))

	var midazOrgID string

	if _, hasPluginCRM := mappedFields[pluginCRMDataSourceID]; hasPluginCRM {
		if ds, exists := uc.ExternalDataSources.Get(pluginCRMDataSourceID); exists {
			midazOrgID = ds.MidazOrganizationID
		}
	}

	transformedMappedFields := TransformMappedFieldsForStorage(mappedFields, midazOrgID)
	uc.Logger.Log(ctx, log.LevelInfo, "Transformed Mapped Fields for storage", log.Any("transformed_mapped_fields", transformedMappedFields))

	return transformedMappedFields, mappedFields, nil
}

func (uc *UseCase) persistTemplate(ctx context.Context, span trace.Span, outFormat, description string, transformedMappedFields map[string]map[string][]string) (*template.Template, error) {
	templateID, err := commons.GenerateUUIDv7()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to generate template ID", err)
		return nil, err
	}

	fileName := fmt.Sprintf("%s.tpl", templateID.String())

	templateEntity, err := template.NewTemplate(templateID, strings.ToLower(outFormat), description, fileName)
	if err != nil {
		if pkgHTTP.IsBusinessError(err) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create template entity", err)
		} else {
			libOpentelemetry.HandleSpanError(span, "Failed to create template entity", err)
		}

		return nil, err
	}

	templateModel := template.FromTemplateEntity(templateEntity, transformedMappedFields)

	resultTemplateModel, err := uc.TemplateRepo.Create(ctx, templateModel)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to create template in repository", err)
		uc.Logger.Log(ctx, log.LevelError, "Error into creating a template", log.Err(err))

		return nil, err
	}

	return resultTemplateModel, nil
}

func (uc *UseCase) storeCreatedTemplateFile(ctx context.Context, span trace.Span, resultTemplateModel *template.Template, fileHeader *multipart.FileHeader, outFormat string) error {
	fileBytes, err := pkgHTTP.ReadMultipartFile(fileHeader)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to read multipart file", err)
		uc.Logger.Log(ctx, log.LevelError, "Error to get the file content", log.Err(err))

		// Roll back the repository insert since we cannot store the file
		if errDelete := uc.DeleteTemplateByID(ctx, resultTemplateModel.ID, true); errDelete != nil {
			uc.Logger.Log(ctx, log.LevelError, "Failed to roll back template creation after file read failure",
				log.String("id", resultTemplateModel.ID.String()),
				log.Err(errDelete),
			)
		}

		return err
	}

	errPutStorage := uc.TemplateSeaweedFS.Put(ctx, resultTemplateModel.FileName, outFormat, fileBytes)
	if errPutStorage != nil {
		libOpentelemetry.HandleSpanError(span, "Error putting template file on storage", errPutStorage)

		if errDelete := uc.DeleteTemplateByID(ctx, resultTemplateModel.ID, true); errDelete != nil {
			uc.Logger.Log(ctx, log.LevelError, "Failed to roll back template creation after storage failure",
				log.String("id", resultTemplateModel.ID.String()),
				log.Err(errDelete),
			)
		}

		uc.Logger.Log(ctx, log.LevelError, "Error putting template file on storage", log.Err(errPutStorage))

		return errPutStorage
	}

	return nil
}

func (uc *UseCase) finalizeTemplateIdempotency(ctx context.Context, templateFile, outFormat, description string, resultTemplateModel *template.Template, idempotencyKey *string, shouldCleanupKey *bool) {
	if uc.RedisRepo == nil {
		return
	}

	if *idempotencyKey == "" {
		key, keyErr := uc.buildTemplateIdempotencyKey(ctx, templateFile, outFormat, description)
		if keyErr == nil {
			*idempotencyKey = key
		}
	}

	if *idempotencyKey != "" {
		if cacheErr := uc.cacheTemplateIdempotencyResult(ctx, *idempotencyKey, resultTemplateModel); cacheErr == nil {
			*shouldCleanupKey = false
		}
	}
}

// checkTemplateIdempotency acquires an idempotency lock via Redis SetNX.
// Returns a cached template if this is a duplicate request, or nil to proceed with creation.
func (uc *UseCase) checkTemplateIdempotency(ctx context.Context, templateFile, outFormat, description string, span trace.Span) (*template.Template, string, bool, error) {
	idempotencyKey, keyErr := uc.buildTemplateIdempotencyKey(ctx, templateFile, outFormat, description)
	if keyErr != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to compute template idempotency key", keyErr)

		return nil, "", false, keyErr
	}

	span.SetAttributes(attribute.String("app.idempotency.key_prefix", idempotencyKey[:min(len(idempotencyKey), 30)]+"..."))
	uc.Logger.Log(ctx, log.LevelInfo, "Checking idempotency")

	acquired, setNXErr := uc.RedisRepo.SetNX(ctx, idempotencyKey, "processing", constant.IdempotencyTTL)
	if setNXErr != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to acquire template idempotency lock", setNXErr)

		return nil, idempotencyKey, false, setNXErr
	}

	if !acquired {
		cachedResult, err := uc.handleDuplicateTemplateRequest(ctx, idempotencyKey)
		return cachedResult, idempotencyKey, false, err
	}

	return nil, idempotencyKey, true, nil
}

// templateIdempotencyInput is the internal struct used to compute idempotency hashes
// for template creation requests. It captures the unique combination of template content,
// output format, and description that defines a distinct template.
type templateIdempotencyInput struct {
	TemplateFile string `json:"templateFile"`
	OutputFormat string `json:"outputFormat"`
	Description  string `json:"description"`
}

// buildTemplateIdempotencyKey resolves the idempotency key for the template creation request.
// If a client-provided Idempotency-Key header value exists in context, it is used as-is.
// Otherwise, a SHA256 hash of the JSON-serialized request fields is computed.
func (uc *UseCase) buildTemplateIdempotencyKey(ctx context.Context, templateFile, outFormat, description string) (string, error) {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.template.build_idempotency_key")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.request_id", reqId))

	// Check for client-provided idempotency key from context
	if clientKey, ok := ctx.Value(constant.IdempotencyKeyCtx).(string); ok && clientKey != "" {
		key := constant.IdempotencyKeyPrefix + ":template:" + clientKey
		uc.Logger.Log(ctx, log.LevelInfo, "Using client-provided template idempotency key", log.String("key", key))

		return key, nil
	}

	// Compute SHA256 hash of the serialized request fields
	input := templateIdempotencyInput{
		TemplateFile: templateFile,
		OutputFormat: outFormat,
		Description:  description,
	}

	data, err := json.Marshal(input)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to marshal template input for idempotency hash", err)

		return "", fmt.Errorf("failed to marshal template input for idempotency hash: %w", err)
	}

	hash := commons.HashSHA256(string(data))
	key := constant.IdempotencyKeyPrefix + ":template:" + hash
	uc.Logger.Log(ctx, log.LevelInfo, "Computed template idempotency key from request body hash", log.String("key", key))

	return key, nil
}

// handleDuplicateTemplateRequest handles the case where SetNX returned false (key already exists).
// It attempts to retrieve the cached response from Redis. If a cached response exists,
// it is unmarshaled and returned. If no cached response exists yet (in-flight request),
// an error is returned indicating a duplicate in-flight request.
func (uc *UseCase) handleDuplicateTemplateRequest(ctx context.Context, idempotencyKey string) (*template.Template, error) {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, childSpan := uc.Tracer.Start(ctx, "service.template.handle_duplicate_request")
	defer childSpan.End()

	childSpan.SetAttributes(attribute.String("app.request.request_id", reqId))
	uc.Logger.Log(ctx, log.LevelInfo, "Duplicate template request detected", log.String("idempotency_key", idempotencyKey))

	cachedResponse, getErr := uc.RedisRepo.Get(ctx, idempotencyKey)
	if getErr != nil {
		libOpentelemetry.HandleSpanError(childSpan, "Failed to retrieve cached template idempotency response", getErr)

		return nil, getErr
	}

	// If the cached value is empty or still "processing", the first request is still in-flight
	if cachedResponse == "" || cachedResponse == "processing" {
		libOpentelemetry.HandleSpanBusinessErrorEvent(childSpan, "Duplicate in-flight template request detected", constant.ErrDuplicateRequestInFlight)

		return nil, pkg.ValidateBusinessError(constant.ErrDuplicateRequestInFlight, "template")
	}

	// Unmarshal the cached response
	var cachedTemplate template.Template
	if unmarshalErr := json.Unmarshal([]byte(cachedResponse), &cachedTemplate); unmarshalErr != nil {
		libOpentelemetry.HandleSpanError(childSpan, "Failed to unmarshal cached template idempotency response", unmarshalErr)

		return nil, fmt.Errorf("failed to unmarshal cached template idempotency response: %w", unmarshalErr)
	}

	// Signal to the handler that this is a replayed response via pointer mutation in context
	if replayedPtr, ok := ctx.Value(constant.IdempotencyReplayedCtx).(*bool); ok && replayedPtr != nil {
		*replayedPtr = true
	}

	uc.Logger.Log(ctx, log.LevelInfo, "Returning cached idempotent template response", log.String("key", idempotencyKey))

	return &cachedTemplate, nil
}

// cacheTemplateIdempotencyResult caches the successful template creation result in Redis
// so that future duplicate requests can return the cached response.
func (uc *UseCase) cacheTemplateIdempotencyResult(ctx context.Context, idempotencyKey string, result *template.Template) error {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.template.cache_idempotency_result")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.request_id", reqId))

	data, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to marshal template result for idempotency cache", marshalErr)
		uc.Logger.Log(ctx, log.LevelError, "Failed to marshal template result for idempotency cache", log.Err(marshalErr))

		return marshalErr
	}

	if setErr := uc.RedisRepo.Set(ctx, idempotencyKey, string(data), constant.IdempotencyTTL); setErr != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to cache template idempotency result", setErr)
		uc.Logger.Log(ctx, log.LevelError, "Failed to cache template idempotency result", log.Err(setErr))

		return setErr
	}

	return nil
}

func (uc *UseCase) releaseTemplateIdempotencyKey(ctx context.Context, idempotencyKey string) {
	if uc.RedisRepo == nil || idempotencyKey == "" {
		return
	}

	if err := uc.RedisRepo.Del(ctx, idempotencyKey); err != nil {
		uc.Logger.Log(ctx, log.LevelWarn, "Failed to release template idempotency key", log.String("key", idempotencyKey), log.Err(err))
	}
}
