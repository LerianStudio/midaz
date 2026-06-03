// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"mime/multipart"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/ctxutil"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/datasource"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/template"
	pkgHTTP "github.com/LerianStudio/midaz/v3/components/reporter/pkg/net/http"
	templateUtils "github.com/LerianStudio/midaz/v3/components/reporter/pkg/templateutils"

	"github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

	// otel/attribute is used for span attribute types (no lib-commons wrapper available)
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// UpdateTemplateByID updates an existing template, optionally uploading a new file to storage,
// and returns the updated template with optional validation warnings (D7 pattern).
func (uc *UseCase) UpdateTemplateByID(ctx context.Context, outputFormat, description string, id uuid.UUID, fileHeader *multipart.FileHeader) (*template.Template, []datasource.ValidationWarning, error) {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.template.update")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.template_id", id.String()),
	)
	uc.Logger.Log(ctx, log.LevelInfo, "Updating template")

	_, mappedFields, rawMappedFields, currentTemplate, previousMappedFields, err := uc.prepareTemplateUpdate(ctx, span, id, outputFormat, fileHeader)
	if err != nil {
		return nil, nil, err
	}

	// Validate schema via DataSourceProvider when a file is provided (D7: warnings for unavailable datasources)
	var warnings []datasource.ValidationWarning

	if fileHeader != nil && rawMappedFields != nil {
		var errSchema error

		warnings, errSchema = uc.ValidateSchemaViaProvider(ctx, rawMappedFields)
		if errSchema != nil {
			return nil, nil, errSchema
		}
	}

	if err := uc.applyTemplateMetadataUpdate(ctx, span, id, description, outputFormat, mappedFields); err != nil {
		return nil, nil, err
	}

	if fileHeader != nil {
		if err := uc.uploadTemplateFileToStorage(ctx, currentTemplate, outputFormat, fileHeader, span); err != nil {
			if rollbackErr := uc.rollbackTemplateUpdate(ctx, id, currentTemplate, previousMappedFields); rollbackErr != nil {
				uc.Logger.Log(ctx, log.LevelError, "Failed to roll back template metadata after storage update failure",
					log.String("id", id.String()),
					log.Err(rollbackErr),
				)
			}

			return nil, nil, err
		}
	}

	updatedTemplate, err := uc.fetchUpdatedTemplate(ctx, span, id)
	if err != nil {
		return nil, nil, err
	}

	return updatedTemplate, warnings, nil
}

func (uc *UseCase) prepareTemplateUpdate(ctx context.Context, span trace.Span, id uuid.UUID, outputFormat string, fileHeader *multipart.FileHeader) (string, map[string]map[string][]string, map[string]map[string][]string, *template.Template, map[string]map[string][]string, error) {
	var (
		templateFile         string
		mappedFields         map[string]map[string][]string
		rawMappedFields      map[string]map[string][]string
		currentTemplate      *template.Template
		previousMappedFields map[string]map[string][]string
	)

	if fileHeader != nil {
		var err error

		templateFile, mappedFields, rawMappedFields, err = uc.processAndValidateTemplateFile(ctx, span, fileHeader)
		if err != nil {
			return "", nil, nil, nil, nil, err
		}
	}

	if err := uc.validateOutputFormatAndFile(ctx, id, fileHeader, outputFormat, templateFile); err != nil {
		return "", nil, nil, nil, nil, err
	}

	if fileHeader != nil {
		var err error

		currentTemplate, previousMappedFields, err = uc.getTemplateStateForUpdate(ctx, id)
		if err != nil {
			if pkgHTTP.IsBusinessError(err) {
				libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve current template state", err)
			} else {
				libOpentelemetry.HandleSpanError(span, "Failed to retrieve current template state", err)
			}

			return "", nil, nil, nil, nil, err
		}
	}

	return templateFile, mappedFields, rawMappedFields, currentTemplate, previousMappedFields, nil
}

func (uc *UseCase) processAndValidateTemplateFile(ctx context.Context, span trace.Span, fileHeader *multipart.FileHeader) (string, map[string]map[string][]string, map[string]map[string][]string, error) {
	templateFile, mappedFields, err := uc.processTemplateFile(ctx, fileHeader)
	if err != nil {
		if pkgHTTP.IsBusinessError(err) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to process template file", err)
		} else {
			libOpentelemetry.HandleSpanError(span, "Failed to process template file", err)
		}

		return "", nil, nil, err
	}

	// Schema validation is now handled solely by ValidateSchemaViaProvider
	// (called by the parent function). No mutation of mappedFields occurs.
	return templateFile, mappedFields, mappedFields, nil
}

func (uc *UseCase) applyTemplateMetadataUpdate(ctx context.Context, span trace.Span, id uuid.UUID, description, outputFormat string, mappedFields map[string]map[string][]string) error {
	setFields := uc.buildSetFields(description, outputFormat, mappedFields)

	updateFields := bson.M{}
	if len(setFields) > 0 {
		updateFields["$set"] = setFields
	}

	if errUpdate := uc.TemplateRepo.Update(ctx, id, &updateFields); errUpdate != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to update template in repository", errUpdate)
		uc.Logger.Log(ctx, log.LevelError, "Error into updating a template", log.Err(errUpdate))

		return errUpdate
	}

	return nil
}

func (uc *UseCase) fetchUpdatedTemplate(ctx context.Context, span trace.Span, id uuid.UUID) (*template.Template, error) {
	templateUpdated, err := uc.GetTemplateByID(ctx, id)
	if err != nil {
		if pkgHTTP.IsBusinessError(err) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve updated template", err)
		} else {
			libOpentelemetry.HandleSpanError(span, "Failed to retrieve updated template", err)
		}

		uc.Logger.Log(ctx, log.LevelError, "Failed to retrieve template", log.String("id", id.String()), log.Err(err))

		return nil, err
	}

	return templateUpdated, nil
}

// uploadTemplateFileToStorage reads the file bytes and uploads the new file content to object storage.
func (uc *UseCase) uploadTemplateFileToStorage(ctx context.Context, currentTemplate *template.Template, outputFormat string, fileHeader *multipart.FileHeader, span trace.Span) error {
	fileBytes, errRead := pkgHTTP.ReadMultipartFile(fileHeader)
	if errRead != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to read multipart file", errRead)
		uc.Logger.Log(ctx, log.LevelError, "Error to get file content", log.Err(errRead))

		return errRead
	}

	// Determine the contentType for storage: use the new outputFormat if provided, otherwise use existing
	storageContentType := outputFormat
	if commons.IsNilOrEmpty(&storageContentType) {
		storageContentType = currentTemplate.OutputFormat
	}

	errPutStorage := uc.TemplateSeaweedFS.Put(ctx, currentTemplate.FileName, storageContentType, fileBytes)
	if errPutStorage != nil {
		libOpentelemetry.HandleSpanError(span, "Error putting template file on storage", errPutStorage)
		uc.Logger.Log(ctx, log.LevelError, "Error putting template file on storage", log.Err(errPutStorage))

		return errPutStorage
	}

	return nil
}

func (uc *UseCase) getTemplateStateForUpdate(ctx context.Context, id uuid.UUID) (*template.Template, map[string]map[string][]string, error) {
	currentTemplate, err := uc.TemplateRepo.FindByID(ctx, id)
	if err != nil {
		uc.Logger.Log(ctx, log.LevelError, "Failed to retrieve template", log.String("id", id.String()), log.Err(err))
		return nil, nil, err
	}

	_, previousMappedFields, _, err := uc.TemplateRepo.FindMappedFieldsAndOutputFormatByID(ctx, id)
	if err != nil {
		uc.Logger.Log(ctx, log.LevelError, "Failed to retrieve template mapped fields", log.String("id", id.String()), log.Err(err))
		return nil, nil, err
	}

	return currentTemplate, previousMappedFields, nil
}

func (uc *UseCase) rollbackTemplateUpdate(ctx context.Context, id uuid.UUID, currentTemplate *template.Template, previousMappedFields map[string]map[string][]string) error {
	rollbackFields := bson.M{
		"description":   currentTemplate.Description,
		"output_format": currentTemplate.OutputFormat,
		"mapped_fields": previousMappedFields,
		"updated_at":    currentTemplate.UpdatedAt,
	}

	updateFields := bson.M{"$set": rollbackFields}

	return uc.TemplateRepo.Update(ctx, id, &updateFields)
}

// processTemplateFile handles file extraction, script tag validation, and mapped fields extraction.
func (uc *UseCase) processTemplateFile(ctx context.Context, fileHeader *multipart.FileHeader) (string, map[string]map[string][]string, error) {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	_, span := uc.Tracer.Start(ctx, "service.template.process_template_file")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
	)

	templateFile, errFile := pkgHTTP.GetFileFromHeader(fileHeader)
	if errFile != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get file from header", errFile)
		return "", nil, errFile
	}

	if err := templateUtils.ValidateNoScriptTag(templateFile); err != nil {
		errBusiness := pkg.ValidateBusinessError(constant.ErrScriptTagDetected, "")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Script tag detected in template file", errBusiness)

		return "", nil, errBusiness
	}

	mappedFields := templateUtils.MappedFieldsOfTemplate(templateFile)
	uc.Logger.Log(ctx, log.LevelInfo, "Mapped Fields is valid to continue", log.Any("mapped_fields", mappedFields))

	return templateFile, mappedFields, nil
}

// buildSetFields builds the setFields map for the update operation.
func (uc *UseCase) buildSetFields(description, outputFormat string, mappedFields map[string]map[string][]string) bson.M {
	setFields := bson.M{}
	if !commons.IsNilOrEmpty(&description) {
		setFields["description"] = description
	}

	if !commons.IsNilOrEmpty(&outputFormat) {
		setFields["output_format"] = strings.ToLower(outputFormat)
	}

	if mappedFields != nil {
		setFields["mapped_fields"] = mappedFields
	}

	setFields["updated_at"] = time.Now()

	return setFields
}

// validateOutputFormatAndFile validates output format and file format compatibility.
func (uc *UseCase) validateOutputFormatAndFile(ctx context.Context, id uuid.UUID, fileHeader *multipart.FileHeader, outputFormat, templateFile string) error {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.template.validate_output_format")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.template_id", id.String()),
	)

	// If file is provided without explicit outputFormat, validate against existing template's outputFormat
	if fileHeader != nil && commons.IsNilOrEmpty(&outputFormat) {
		outputFormatExistentTemplate, err := uc.TemplateRepo.FindOutputFormatByID(ctx, id)
		if err != nil {
			if pkgHTTP.IsBusinessError(err) {
				libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get outputFormat of template by ID", err)
			} else {
				libOpentelemetry.HandleSpanError(span, "Failed to get outputFormat of template by ID", err)
			}

			uc.Logger.Log(ctx, log.LevelError, "Error to get outputFormat of template by ID", log.Err(err))

			return err
		}

		if outputFormatExistentTemplate == nil {
			err := fmt.Errorf("output format not found for template %s", id)
			libOpentelemetry.HandleSpanError(span, "Output format is nil", err)
			uc.Logger.Log(ctx, log.LevelError, "Output format is nil for template", log.String("id", id.String()))

			return err
		}

		if errFileFormat := pkg.ValidateFileFormat(*outputFormatExistentTemplate, templateFile); errFileFormat != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "File format validation failed", errFileFormat)
			uc.Logger.Log(ctx, log.LevelError, "Error to validate file format", log.Err(errFileFormat))

			return errFileFormat
		}
	}

	// If outputFormat is explicitly provided, validate it
	if !commons.IsNilOrEmpty(&outputFormat) {
		if !pkg.IsOutputFormatValuesValid(&outputFormat) {
			errInvalidFormat := pkg.ValidateBusinessError(constant.ErrInvalidOutputFormat, "")

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid output format value", errInvalidFormat)
			uc.Logger.Log(ctx, log.LevelError, "Error invalid outputFormat value", log.String("output_format", outputFormat))

			return errInvalidFormat
		}

		if fileHeader == nil {
			errMissingFile := pkg.ValidateBusinessError(constant.ErrOutputFormatWithoutTemplateFile, "")

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Output format provided without template file", errMissingFile)
			uc.Logger.Log(ctx, log.LevelError, "Can not update outputFormat without passing the file template")

			return errMissingFile
		}

		if errFileFormat := pkg.ValidateFileFormat(outputFormat, templateFile); errFileFormat != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "File format validation failed", errFileFormat)
			uc.Logger.Log(ctx, log.LevelError, "Error to validate file format", log.Err(errFileFormat))

			return errFileFormat
		}
	}

	return nil
}
