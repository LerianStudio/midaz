// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"

	libConstants "github.com/LerianStudio/lib-commons/v5/commons/constants"
	commonsHttp "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"

	"github.com/LerianStudio/midaz/v3/components/reporter-manager/internal/services"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/ctxutil"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/model"
	template "github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/template"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel/attribute"
)

// TemplateHandler handles HTTP requests for template operations.
type TemplateHandler struct {
	service *services.UseCase
}

// NewTemplateHandler creates a new TemplateHandler with the given service dependency.
func NewTemplateHandler(service *services.UseCase) (*TemplateHandler, error) {
	if service == nil {
		return nil, errors.New("service must not be nil for TemplateHandler")
	}

	return &TemplateHandler{service: service}, nil
}

// CreateTemplate is a method that creates a template.
//
//	@Summary		Create a Template
//	@Description	Create a Template with the input payload (multipart form)
//	@Tags			Templates
//	@Accept			multipart/form-data
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Idempotency	header		string	false	"Client-provided idempotency key to prevent duplicate template creation"
//	@Param			template		formData	file	true	"Template file"
//	@Param			outputFormat	formData	string	true	"Output format (html, pdf, txt, xml)"
//	@Param			description		formData	string	false	"Template description"
//	@Success		201				{object}	TemplateResponse
//	@Failure		400				{object}	pkg.HTTPError
//	@Failure		401				{object}	pkg.HTTPError
//	@Failure		403				{object}	pkg.HTTPError
//	@Failure		500				{object}	pkg.HTTPError
//	@Router			/v1/templates [post]
func (th *TemplateHandler) CreateTemplate(c *fiber.Ctx) error {
	ctx := c.UserContext()
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := th.service.Tracer.Start(ctx, "handler.template.create")
	defer span.End()

	if idempotencyKey := c.Get(libConstants.IdempotencyKey); idempotencyKey != "" {
		ctx = context.WithValue(ctx, constant.IdempotencyKeyCtx, idempotencyKey)
	}

	replayed := false
	ctx = context.WithValue(ctx, constant.IdempotencyReplayedCtx, &replayed)
	c.SetUserContext(ctx)

	th.service.Logger.Log(ctx, log.LevelInfo, "Request to create template")

	outputFormat := c.FormValue("outputFormat")
	description := c.FormValue("description")
	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.output_format", outputFormat),
		attribute.Bool("app.request.has_description", description != ""),
	)

	fileHeader, err := c.FormFile("template")
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get template file from form", err)
		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidFileUploaded, "", err))
	}

	templateFile, errFile := http.GetFileFromHeader(fileHeader)
	if errFile != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get file from header", errFile)
		return http.WithError(c, errFile)
	}

	if errValidate := pkg.ValidateFormDataFields(&outputFormat, &description); errValidate != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate form data fields", errValidate)
		th.service.Logger.Log(ctx, log.LevelError, "Error validating form data fields", log.Err(errValidate))

		return http.WithError(c, errValidate)
	}

	if errValidateFile := pkg.ValidateFileFormat(outputFormat, templateFile); errValidateFile != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate file format", errValidateFile)
		th.service.Logger.Log(ctx, log.LevelError, "Error validating file format", log.Err(errValidateFile))

		return http.WithError(c, errValidateFile)
	}

	templateOut, warnings, err := th.service.CreateTemplate(ctx, templateFile, outputFormat, description, fileHeader)
	if err != nil {
		if http.IsBusinessError(err) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create template", err)
		} else {
			libOpentelemetry.HandleSpanError(span, "Failed to create template", err)
		}

		return http.WithError(c, err)
	}

	th.service.Logger.Log(ctx, log.LevelInfo, "Successfully created template", log.String("template_id", templateOut.ID.String()))

	if replayed {
		c.Set(libConstants.IdempotencyReplayed, "true")
	}

	response := newTemplateResponse(templateOut, warnings)

	return commonsHttp.Respond(c, fiber.StatusCreated, response)
}

// GetTemplateByID is a method to get a template by ID.
//
//	@Summary		Get a Template
//	@Description	Get information of a Template passing the ID
//	@Tags			Templates
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string	true	"Template ID"
//	@Success		200		{object}	template.Template
//	@Failure		400		{object}	pkg.HTTPError
//	@Failure		401		{object}	pkg.HTTPError
//	@Failure		403		{object}	pkg.HTTPError
//	@Failure		404		{object}	pkg.HTTPError
//	@Failure		500		{object}	pkg.HTTPError
//	@Router			/v1/templates/{id} [get]
func (th *TemplateHandler) GetTemplateByID(c *fiber.Ctx) error {
	ctx := c.UserContext()
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := th.service.Tracer.Start(ctx, "handler.template.get")
	defer span.End()

	id, ok := c.Locals("id").(uuid.UUID)
	if !ok {
		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, "", "id"))
	}

	th.service.Logger.Log(ctx, log.LevelInfo, "Initiating get template", log.String("template_id", id.String()))
	span.SetAttributes(attribute.String("app.request.request_id", reqId), attribute.String("app.request.template_id", id.String()))

	templateModel, err := th.service.GetTemplateByID(ctx, id)
	if err != nil {
		if http.IsBusinessError(err) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve template on query", err)
		} else {
			libOpentelemetry.HandleSpanError(span, "Failed to retrieve template on query", err)
		}

		th.service.Logger.Log(ctx, log.LevelError, "Failed to retrieve template", log.String("template_id", id.String()), log.Err(err))

		return http.WithError(c, err)
	}

	th.service.Logger.Log(ctx, log.LevelInfo, "Successfully retrieved template", log.String("template_id", id.String()))

	return commonsHttp.Respond(c, fiber.StatusOK, templateModel)
}

// GetAllTemplates is a method that retrieves all templates.
//
//	@Summary		Get all Templates
//	@Description	List all the templates
//	@Tags			Templates
//	@Produce		json
//	@Security		BearerAuth
//	@Param			limit		query		int		false	"Limit"		default(10)
//	@Param			page		query		int		false	"Page"		default(1)
//	@Success		200			{object}	model.Pagination{items=[]template.Template,page=int,limit=int,total=int}
//	@Failure		400			{object}	pkg.HTTPError
//	@Failure		401			{object}	pkg.HTTPError
//	@Failure		403			{object}	pkg.HTTPError
//	@Failure		500			{object}	pkg.HTTPError
//	@Router			/v1/templates [get]
func (th *TemplateHandler) GetAllTemplates(c *fiber.Ctx) error {
	ctx := c.UserContext()
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := th.service.Tracer.Start(ctx, "handler.template.get_all")
	defer span.End()

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)
		th.service.Logger.Log(ctx, log.LevelError, "Failed to validate query parameters", log.Err(err))

		return http.WithError(c, err)
	}

	pagination := model.Pagination{Limit: headerParams.Limit, Page: headerParams.Page}

	th.service.Logger.Log(ctx, log.LevelInfo, "Initiating retrieval all templates")
	span.SetAttributes(attribute.String("app.request.request_id", reqId))

	err = libOpentelemetry.SetSpanAttributesFromValue(span, "app.request.query_params", headerParams, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert query params to JSON string", err)
	}

	templates, err := th.service.GetAllTemplates(ctx, *headerParams)
	if err != nil {
		if http.IsBusinessError(err) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve all Templates on query", err)
		} else {
			libOpentelemetry.HandleSpanError(span, "Failed to retrieve all Templates on query", err)
		}

		th.service.Logger.Log(ctx, log.LevelError, "Failed to retrieve all templates", log.Err(err))

		return http.WithError(c, err)
	}

	if templates == nil {
		templates = []*template.Template{}
	}

	total, err := th.service.TemplateRepo.Count(ctx, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to count templates on query", err)
		th.service.Logger.Log(ctx, log.LevelError, "Failed to count templates", log.Err(err))

		return http.WithError(c, err)
	}

	th.service.Logger.Log(ctx, log.LevelInfo, "Successfully retrieved all templates")
	pagination.SetItems(templates)
	pagination.SetTotal(int(total))

	return commonsHttp.Respond(c, fiber.StatusOK, pagination)
}

// UpdateTemplateByID is a method that updates a template by ID.
//
//	@Summary		Update a Template
//	@Description	Update a Template by its ID (multipart form)
//	@Tags			Templates
//	@Accept			multipart/form-data
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id				path		string	true	"Template ID"
//	@Param			template		formData	file	false	"Template file"
//	@Param			outputFormat	formData	string	false	"Output format (html, pdf, txt, xml)"
//	@Param			description		formData	string	false	"Template description"
//	@Success		200				{object}	TemplateResponse
//	@Failure		400				{object}	pkg.HTTPError
//	@Failure		401				{object}	pkg.HTTPError
//	@Failure		403				{object}	pkg.HTTPError
//	@Failure		404				{object}	pkg.HTTPError
//	@Failure		500				{object}	pkg.HTTPError
//	@Router			/v1/templates/{id} [patch]
func (th *TemplateHandler) UpdateTemplateByID(c *fiber.Ctx) error {
	ctx := c.UserContext()
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := th.service.Tracer.Start(ctx, "handler.template.update")
	defer span.End()

	id, ok := c.Locals("id").(uuid.UUID)
	if !ok {
		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, "", "id"))
	}

	th.service.Logger.Log(ctx, log.LevelInfo, "Initiating update of template", log.String("template_id", id.String()))

	outputFormat := c.FormValue("outputFormat")
	description := c.FormValue("description")
	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.template_id", id.String()),
		attribute.String("app.request.output_format", outputFormat),
		attribute.Bool("app.request.has_description", description != ""),
	)

	fileHeader, err := c.FormFile("template")
	if err != nil {
		if !errors.Is(err, fasthttp.ErrMissingFile) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get template file from form", err)
			return http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidFileUploaded, "", err))
		}

		fileHeader = nil
	}

	templateUpdated, warnings, errUpdate := th.service.UpdateTemplateByID(ctx, outputFormat, description, id, fileHeader)
	if errUpdate != nil {
		if http.IsBusinessError(errUpdate) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update template", errUpdate)
		} else {
			libOpentelemetry.HandleSpanError(span, "Failed to update template", errUpdate)
		}

		th.service.Logger.Log(ctx, log.LevelError, "Failed to update template", log.String("template_id", id.String()), log.Err(errUpdate))

		return http.WithError(c, errUpdate)
	}

	th.service.Logger.Log(ctx, log.LevelInfo, "Successfully updated template", log.String("template_id", id.String()))

	response := newTemplateResponse(templateUpdated, warnings)

	return commonsHttp.Respond(c, fiber.StatusOK, response)
}

// DeleteTemplateByID is a method that deletes a template by ID.
//
//	@Summary		Delete a Template
//	@Description	Delete a Template by its ID
//	@Tags			Templates
//	@Security		BearerAuth
//	@Param			id		path		string	true	"Template ID"
//	@Success		204
//	@Failure		400		{object}	pkg.HTTPError
//	@Failure		401		{object}	pkg.HTTPError
//	@Failure		403		{object}	pkg.HTTPError
//	@Failure		404		{object}	pkg.HTTPError
//	@Failure		500		{object}	pkg.HTTPError
//	@Router			/v1/templates/{id} [delete]
func (th *TemplateHandler) DeleteTemplateByID(c *fiber.Ctx) error {
	ctx := c.UserContext()
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := th.service.Tracer.Start(ctx, "handler.template.delete")
	defer span.End()

	id, ok := c.Locals("id").(uuid.UUID)
	if !ok {
		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, "", "id"))
	}

	th.service.Logger.Log(ctx, log.LevelInfo, "Initiating removal of template", log.String("template_id", id.String()))
	span.SetAttributes(attribute.String("app.request.request_id", reqId), attribute.String("app.request.template_id", id.String()))

	if err := th.service.DeleteTemplateByID(ctx, id, false); err != nil {
		if http.IsBusinessError(err) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to remove template on database", err)
		} else {
			libOpentelemetry.HandleSpanError(span, "Failed to remove template on database", err)
		}

		th.service.Logger.Log(ctx, log.LevelError, "Failed to remove template", log.String("template_id", id.String()), log.Err(err))

		return http.WithError(c, err)
	}

	th.service.Logger.Log(ctx, log.LevelInfo, "Successfully removed template", log.String("template_id", id.String()))

	return commonsHttp.RespondStatus(c, fiber.StatusNoContent)
}
