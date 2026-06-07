// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"errors"

	"github.com/LerianStudio/midaz/v4/components/reporter-manager/internal/services"
	_ "github.com/LerianStudio/midaz/v4/pkg/reporter" // swag: resolves pkg.HTTPError in annotations
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/reporter/net/http"
	_ "github.com/LerianStudio/midaz/v4/pkg/reporter/pongo"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/template_builder"

	commonsHttp "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/attribute"
)

// TemplateBuilderHandler handles HTTP requests for template builder operations.
type TemplateBuilderHandler struct {
	service *services.UseCase
}

// NewTemplateBuilderHandler creates a new TemplateBuilderHandler with the given service dependency.
func NewTemplateBuilderHandler(service *services.UseCase) (*TemplateBuilderHandler, error) {
	if service == nil {
		return nil, errors.New("service must not be nil for TemplateBuilderHandler")
	}

	return &TemplateBuilderHandler{service: service}, nil
}

// GetBlocksConfig returns all available block definitions for the template builder.
//
//	@Summary		Get block definitions for template builder
//	@Description	Returns all available block type definitions including properties and categories
//	@Tags			Template Builder
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200		{object}	pongo.BlocksConfigResponse
//	@Failure		401		{object}	pkg.HTTPError
//	@Failure		403		{object}	pkg.HTTPError
//	@Failure		500		{object}	pkg.HTTPError
//	@Router			/v1/templates/blocks-config [get]
func (h *TemplateBuilderHandler) GetBlocksConfig(c *fiber.Ctx) error {
	ctx := c.UserContext()
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := h.service.Tracer.Start(ctx, "handler.template_builder.get_blocks_config")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
	)

	h.service.Logger.Log(ctx, log.LevelDebug, "Request to get block definitions")

	result := h.service.GetBlocksConfig(ctx)

	h.service.Logger.Log(ctx, log.LevelDebug, "Successfully retrieved block definitions")

	return commonsHttp.Respond(c, fiber.StatusOK, result)
}

// GetFiltersConfig returns all available filter definitions for the template builder.
//
//	@Summary		Get filter definitions for template builder
//	@Description	Returns all available Pongo2 filter definitions including DIMP filters
//	@Tags			Template Builder
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200		{object}	pongo.FiltersResponse
//	@Failure		401		{object}	pkg.HTTPError
//	@Failure		403		{object}	pkg.HTTPError
//	@Failure		500		{object}	pkg.HTTPError
//	@Router			/v1/templates/filters [get]
func (h *TemplateBuilderHandler) GetFiltersConfig(c *fiber.Ctx) error {
	ctx := c.UserContext()
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := h.service.Tracer.Start(ctx, "handler.template_builder.get_filters_config")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
	)

	h.service.Logger.Log(ctx, log.LevelDebug, "Request to get filter definitions")

	result := h.service.GetFiltersConfig(ctx)

	h.service.Logger.Log(ctx, log.LevelDebug, "Successfully retrieved filter definitions")

	return commonsHttp.Respond(c, fiber.StatusOK, result)
}

// GenerateCode generates Pongo2 template code from a JSON block structure.
//
//	@Summary		Generate Pongo2 code from template blocks
//	@Description	Converts a JSON block structure into Pongo2 template code and extracts mapped fields
//	@Tags			Template Builder
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body	template_builder.GenerateCodeInput	true	"Block structure"
//	@Success		200		{object}	template_builder.GenerateCodeResponse
//	@Failure		400		{object}	pkg.HTTPError
//	@Failure		401		{object}	pkg.HTTPError
//	@Failure		403		{object}	pkg.HTTPError
//	@Failure		500		{object}	pkg.HTTPError
//	@Router			/v1/templates/generate-code [post]
//
//nolint:dupword // swag @Param syntax is "name location", here both are "body"
func (h *TemplateBuilderHandler) GenerateCode(c *fiber.Ctx) error {
	ctx := c.UserContext()
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := h.service.Tracer.Start(ctx, "handler.template_builder.generate_code")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
	)

	h.service.Logger.Log(ctx, log.LevelDebug, "Request to generate code from template blocks")

	var input template_builder.GenerateCodeInput
	if err := c.BodyParser(&input); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to parse request body", err)

		return pkgHTTP.BadRequest(c, err)
	}

	if len(input.Blocks) == 0 {
		err := errors.New("blocks must not be empty")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Empty blocks in request", err)

		return pkgHTTP.BadRequest(c, err)
	}

	result, err := h.service.GenerateCode(ctx, &input)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to generate code", err)
		h.service.Logger.Log(ctx, log.LevelError, "Failed to generate code", log.Err(err))

		return pkgHTTP.BadRequest(c, err)
	}

	h.service.Logger.Log(ctx, log.LevelDebug, "Successfully generated code from template blocks")

	return commonsHttp.Respond(c, fiber.StatusOK, result)
}

// ValidateBlocks validates template blocks and returns detailed validation errors.
//
//	@Summary		Validate template blocks
//	@Description	Validates a sequence of template blocks. Validation errors are returned in the **200 envelope** with `valid:false` and per-block error messages. HTTP **400** is reserved for malformed request bodies (JSON parse failures).
//	@Tags			Template Builder
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body	template_builder.ValidateBlocksInput	true	"Blocks to validate"
//	@Success		200		{object}	template_builder.ValidateBlocksResponse
//	@Failure		400		{object}	pkg.HTTPError
//	@Failure		401		{object}	pkg.HTTPError
//	@Failure		403		{object}	pkg.HTTPError
//	@Failure		500		{object}	pkg.HTTPError
//	@Router			/v1/templates/validate [post]
//
//nolint:dupword // swag @Param syntax is "name location", here both are "body"
func (h *TemplateBuilderHandler) ValidateBlocks(c *fiber.Ctx) error {
	ctx := c.UserContext()
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := h.service.Tracer.Start(ctx, "handler.template_builder.validate_blocks")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
	)

	h.service.Logger.Log(ctx, log.LevelDebug, "Request to validate template blocks")

	var input template_builder.ValidateBlocksInput
	if err := c.BodyParser(&input); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to parse request body", err)

		return pkgHTTP.BadRequest(c, err)
	}

	if len(input.Blocks) == 0 {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Empty blocks in validate request",
			errors.New("blocks must not be empty"))

		return commonsHttp.Respond(c, fiber.StatusOK, &template_builder.ValidateBlocksResponse{
			Valid: false,
			Errors: []template_builder.ValidationError{
				{BlockID: "", Field: "blocks", Message: "blocks must not be empty"},
			},
		})
	}

	result := h.service.ValidateBlocks(ctx, &input)

	h.service.Logger.Log(ctx, log.LevelDebug, "Block validation completed",
		log.Bool("valid", result.Valid),
		log.Int("error_count", len(result.Errors)))

	return commonsHttp.Respond(c, fiber.StatusOK, result)
}
