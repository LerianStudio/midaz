// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/midaz/v4/components/crm/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/attribute"
)

type HolderHandler struct {
	Service *services.UseCase
}

// CreateHolder is a method that creates Holder information.
//
//	@Summary		Create a Holder
//	@Description	Creates a new holder with the provided details.
//	@Tags			Holders
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			organization_id		path		string						true	"The unique identifier of the Organization."
//	@Param			holder				body		mmodel.CreateHolderInput	true	"Holder Input"
//	@Success		201					{object}	mmodel.Holder
//	@Failure		400					{object}	mmodel.Error
//	@Failure		404					{object}	mmodel.Error
//	@Failure		500					{object}	mmodel.Error
//	@Router			/v1/organizations/{organization_id}/holders [post]
func (handler *HolderHandler) CreateHolder(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_holder")
	defer span.End()

	payload, ok := p.(*mmodel.CreateHolderInput)
	if !ok || payload == nil {
		return http.WithError(c, pkg.ValidateInternalError(nil, cn.EntityHolder))
	}

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	out, err := handler.Service.CreateHolder(ctx, organizationID.String(), payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to create holder", err)

		return http.WithError(c, err)
	}

	return http.Created(c, out)
}

// GetHolderByID retrieves Holder details by a given id
//
//	@Summary		Retrieve Holder details
//	@Description	Retrieves detailed information about a specific holder using its unique identifier.
//	@Tags			Holders
//	@Produce		json
//	@Security		BearerAuth
//	@Param			organization_id		path		string	true	"The unique identifier of the Organization."
//	@Param			id					path		string	true	"The unique identifier of the Holder."
//	@Param			include_deleted		query		string	false	"Returns the holder even if it was logically deleted"
//	@Success		200					{object}	mmodel.Holder
//	@Failure		400					{object}	mmodel.Error
//	@Failure		404					{object}	mmodel.Error
//	@Failure		500					{object}	mmodel.Error
//	@Router			/v1/organizations/{organization_id}/holders/{id} [get]
func (handler *HolderHandler) GetHolderByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_holder_by_id")
	defer span.End()

	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	includeDeleted := http.GetBooleanParam(c, "include_deleted")

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.holder_id", id.String()),
		attribute.Bool("app.request.include_deleted", includeDeleted),
	)

	holder, err := handler.Service.GetHolderByID(ctx, organizationID.String(), id, includeDeleted)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to retrieve holder", err)

		return http.WithError(c, err)
	}

	return http.OK(c, holder)
}

// UpdateHolder is a method that updates Holder information.
//
//	@Summary		Update a Holder
//	@Description	Update details of a holder.
//	@Tags			Holders
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			organization_id		path		string						true	"The unique identifier of the Organization."
//	@Param			id					path		string						true	"The unique identifier of the Holder."
//	@Param			holder				body		mmodel.UpdateHolderInput	true	"Holder Input"
//	@Success		200					{object}	mmodel.Holder
//	@Failure		400					{object}	mmodel.Error
//	@Failure		404					{object}	mmodel.Error
//	@Failure		500					{object}	mmodel.Error
//	@Router			/v1/organizations/{organization_id}/holders/{id} [patch]
func (handler *HolderHandler) UpdateHolder(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_holder")
	defer span.End()

	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	payload, ok := p.(*mmodel.UpdateHolderInput)
	if !ok || payload == nil {
		return http.WithError(c, pkg.ValidateInternalError(nil, cn.EntityHolder))
	}

	fieldsToRemove, ok := c.Locals("patchRemove").([]string)
	if !ok {
		libOpentelemetry.HandleSpanError(span, "Failed to get fields to remove", cn.ErrInternalServer)

		logger.Log(ctx, libLog.LevelError, "Failed to get fields to remove")

		return http.WithError(c, pkg.ValidateInternalError(nil, cn.EntityHolder))
	}

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.holder_id", id.String()),
		attribute.Int("app.request.fields_to_remove_count", len(fieldsToRemove)),
	)

	holder, err := handler.Service.UpdateHolderByID(ctx, organizationID.String(), id, payload, fieldsToRemove)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to update holder", err)

		return http.WithError(c, err)
	}

	return http.OK(c, holder)
}

// DeleteHolderByID is a method that removes Holder information by a given id.
//
//	@Summary		Delete a Holder
//	@Description	Delete a Holder. **Note:** By default, the delete endpoint performs a logical deletion (soft delete) of the entity in the system. If a physical deletion (hard delete) is required, you can use the query parameter outlined in the documentation.
//	@Tags			Holders
//	@Security		BearerAuth
//	@Param			organization_id		path	string	true	"The unique identifier of the Organization."
//	@Param			id					path	string	true	"The unique identifier of the Holder."
//	@Param			hard_delete			query	string	false	"Use only to perform a physical deletion of the data. This action is irreversible."
//	@Success		204
//	@Failure		400	{object}	mmodel.Error
//	@Failure		404	{object}	mmodel.Error
//	@Failure		500	{object}	mmodel.Error
//	@Router			/v1/organizations/{organization_id}/holders/{id} [delete]
func (handler *HolderHandler) DeleteHolderByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.remove_holder_by_id")
	defer span.End()

	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	hardDelete := http.GetBooleanParam(c, "hard_delete")

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.holder_id", id.String()),
		attribute.Bool("app.request.hard_delete", hardDelete),
	)

	err = handler.Service.DeleteHolderByID(ctx, organizationID.String(), id, hardDelete)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to delete holder", err)

		return http.WithError(c, err)
	}

	return http.NoContent(c)
}

// GetAllHolders retrieves Holder details by a given id
//
//	@Summary		List Holders
//	@Description	List all Holders. CRM listing endpoints support pagination using the page, limit, and sort parameters. The sort parameter orders results by the entity ID using the UUID v7 standard, which is time-sortable, ensuring chronological ordering of the results.
//	@Tags			Holders
//	@Produce		json
//	@Security		BearerAuth
//	@Param			organization_id		path		string	true	"The unique identifier of the Organization."
//	@Param			metadata			query		string	false	"Metadata"
//	@Param			limit				query		int		false	"Limit"			default(10)
//	@Param			page				query		int		false	"Page"			default(1)
//	@Param			sort_order			query		string	false	"Sort Order"	Enums(asc,desc)
//	@Param			include_deleted		query		string	false	"Return includes logically deleted holders."
//	@Param			external_id			query		string	false	"Filter holders by externalID"
//	@Param			document			query		string	false	"Filter holders by document"
//	@Success		200					{object}	http.Pagination{items=[]mmodel.Holder}
//	@Failure		400					{object}	mmodel.Error
//	@Failure		404					{object}	mmodel.Error
//	@Failure		500					{object}	mmodel.Error
//	@Router			/v1/organizations/{organization_id}/holders [get]
func (handler *HolderHandler) GetAllHolders(c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_holders")
	defer span.End()

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to validate query parameters", err)

		return http.WithError(c, err)
	}

	pagination := http.Pagination{
		Limit:     headerParams.Limit,
		Page:      headerParams.Page,
		SortOrder: headerParams.SortOrder,
	}

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	includeDeleted := http.GetBooleanParam(c, "include_deleted")

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.Bool("app.request.include_deleted", includeDeleted),
	)

	recordSafeQueryAttributes(span, headerParams)

	holders, err := handler.Service.GetAllHolders(ctx, organizationID.String(), *headerParams, includeDeleted)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get all holders", err)

		return http.WithError(c, err)
	}

	pagination.SetItems(holders)

	return http.OK(c, pagination)
}
