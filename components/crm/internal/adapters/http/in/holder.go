package in

import (
	"github.com/LerianStudio/midaz/v3/components/crm/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/attribute"
)

// HolderHandler handles HTTP requests for holder operations.
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
//	@Param			Authorization		header		string						false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id	header		string						true	"The unique identifier of the Organization associated with the Ledger."
//	@Param			holder				body		mmodel.CreateHolderInput	true	"Holder Input"
//	@Success		201					{object}	mmodel.Holder
//	@Failure		400					{object}	pkg.HTTPError
//	@Failure		404					{object}	pkg.HTTPError
//	@Failure		500					{object}	pkg.HTTPError
//	@Router			/v1/holders [post]
func (handler *HolderHandler) CreateHolder(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_holder")
	defer span.End()

	payload := http.Payload[*mmodel.CreateHolderInput](c, p)
	organizationID := c.Get("X-Organization-Id")

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
	)

	err := libOpenTelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", payload)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)
	}

	out, err := handler.Service.CreateHolder(ctx, organizationID, payload)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to create holder", err)

		logger.Errorf("Failed to create holder: %v", err)

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	if err := http.Created(c, out); err != nil {
		return pkg.ValidateInternalError(err, "Holder")
	}

	return nil
}

// GetHolderByID retrieves Holder details by a given id
//
//	@Summary		Retrieve Holder details
//	@Description	Retrieves detailed information about a specific holder using its unique identifier.
//	@Tags			Holders
//	@Produce		json
//	@Param			Authorization		header		string	false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id	header		string	true	"The unique identifier of the Organization associated with the Ledger."
//	@Param			id					path		string	true	"The unique identifier of the Holder."
//	@Param			include_deleted		query		string	false	"Returns the holder even if it was logically deleted"
//	@Success		200					{object}	mmodel.Holder
//	@Failure		400					{object}	pkg.HTTPError
//	@Failure		404					{object}	pkg.HTTPError
//	@Failure		500					{object}	pkg.HTTPError
//	@Router			/v1/holders/{id} [get]
func (handler *HolderHandler) GetHolderByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_holder_by_id")
	defer span.End()

	id := http.LocalUUID(c, "id")
	organizationID := c.Get("X-Organization-Id")
	includeDeleted := http.GetBooleanParam(c, "include_deleted")

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", id.String()),
		attribute.Bool("app.request.include_deleted", includeDeleted),
	)

	logger.Infof("Initiating retrieval of Holder with ID: %s", id.String())

	holder, err := handler.Service.GetHolderByID(ctx, organizationID, id, includeDeleted)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to retrieve holder", err)

		logger.Errorf("Failed to retrieve Holder with ID: %s, Error: %s", id.String(), err.Error())

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	if err := http.OK(c, holder); err != nil {
		return pkg.ValidateInternalError(err, "Holder")
	}

	return nil
}

// UpdateHolder is a method that updates Holder information.
//
//	@Summary		Update a Holder
//	@Description	Update details of a holder.
//	@Tags			Holders
//	@Accept			json
//	@Produce		json
//	@Param			Authorization		header		string						false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id	header		string						true	"The unique identifier of the Organization associated with the Ledger."
//	@Param			id					path		string						true	"The unique identifier of the Holder."
//	@Param			holder				body		mmodel.UpdateHolderInput	true	"Holder Input"
//	@Success		200					{object}	mmodel.Holder
//	@Failure		400					{object}	pkg.HTTPError
//	@Failure		404					{object}	pkg.HTTPError
//	@Failure		500					{object}	pkg.HTTPError
//	@Router			/v1/holders/{id} [patch]
func (handler *HolderHandler) UpdateHolder(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_holder")
	defer span.End()

	id := http.LocalUUID(c, "id")
	organizationID := c.Get("X-Organization-Id")
	payload := http.Payload[*mmodel.UpdateHolderInput](c, p)

	fieldsToRemove := http.LocalStringSlice(c, "patchRemove")

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", id.String()),
	)

	err := libOpenTelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", payload)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)
	}

	err = libOpenTelemetry.SetSpanAttributesFromStruct(&span, "app.request.fields_to_remove", fieldsToRemove)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to convert fields_to_remove to JSON string", err)
	}

	logger.Infof("Request to update holder with id: %v", id.String())

	holder, err := handler.Service.UpdateHolderByID(ctx, organizationID, id, payload, fieldsToRemove)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to update holder", err)

		logger.Errorf("Failed to update Holder with ID: %s, Error: %s", id.String(), err.Error())

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	if err := http.OK(c, holder); err != nil {
		return pkg.ValidateInternalError(err, "Holder")
	}

	return nil
}

// DeleteHolderByID is a method that removes Holder information by a given id.
//
//	@Summary		Delete a Holder
//	@Description	Delete a Holder. **Note:** By default, the delete endpoint performs a logical deletion (soft delete) of the entity in the system. If a physical deletion (hard delete) is required, you can use the query parameter outlined in the documentation.
//	@Tags			Holders
//	@Param			Authorization		header	string	false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id	header	string	true	"The unique identifier of the Organization associated with the Ledger."
//	@Param			id					path	string	true	"The unique identifier of the Holder."
//	@Param			hard_delete			query	string	false	"Use only to perform a physical deletion of the data. This action is irreversible."
//	@Success		204
//	@Failure		400	{object}	pkg.HTTPError
//	@Failure		404	{object}	pkg.HTTPError
//	@Failure		500	{object}	pkg.HTTPError
//	@Router			/v1/holders/{id} [delete]
func (handler *HolderHandler) DeleteHolderByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.remove_holder_by_id")
	defer span.End()

	id := http.LocalUUID(c, "id")
	organizationID := c.Get("X-Organization-Id")
	hardDelete := http.GetBooleanParam(c, "hard_delete")

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", id.String()),
		attribute.Bool("app.request.hard_delete", hardDelete),
	)

	logger.Infof("Initiating removal of holder with ID: %s", id.String())

	err := handler.Service.DeleteHolderByID(ctx, organizationID, id, hardDelete)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to delete holder", err)

		logger.Errorf("Failed to delete Holder with ID: %s, Error: %s", id.String(), err.Error())

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	if err := http.NoContent(c); err != nil {
		return pkg.ValidateInternalError(err, "Holder")
	}

	return nil
}

// GetAllHolders retrieves Holder details by a given id
//
//	@Summary		List Holders
//	@Description	List all Holders. CRM listing endpoints support pagination using the page, limit, and sort parameters. The sort parameter orders results by the entity ID using the UUID v7 standard, which is time-sortable, ensuring chronological ordering of the results.
//	@Tags			Holders
//	@Produce		json
//	@Param			Authorization		header		string	false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id	header		string	true	"The unique identifier of the Organization associated with the Ledger."
//	@Param			metadata			query		string	false	"Metadata"
//	@Param			limit				query		int		false	"Limit"			default(10)
//	@Param			page				query		int		false	"Page"			default(1)
//	@Param			sort_order			query		string	false	"Sort Order"	Enums(asc,desc)
//	@Param			include_deleted		query		string	false	"Return includes logically deleted holders."
//	@Param			external_id			query		string	false	"Filter holders by externalID"
//	@Param			document			query		string	false	"Filter holders by document"
//	@Success		200					{object}	libPostgres.Pagination{items=[]mmodel.Holder,page=int,limit=int}
//	@Failure		400					{object}	pkg.HTTPError
//	@Failure		404					{object}	pkg.HTTPError
//	@Failure		500					{object}	pkg.HTTPError
//	@Router			/v1/holders [get]
func (handler *HolderHandler) GetAllHolders(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_holders")
	defer span.End()

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to validate query parameters", err)
		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	pagination := libPostgres.Pagination{
		Limit:     headerParams.Limit,
		Page:      headerParams.Page,
		SortOrder: headerParams.SortOrder,
	}

	organizationID := c.Get("X-Organization-Id")
	includeDeleted := http.GetBooleanParam(c, "include_deleted")

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.Bool("app.request.include_deleted", includeDeleted),
	)

	err = libOpenTelemetry.SetSpanAttributesFromStruct(&span, "app.request.query_params", headerParams)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to convert query_params to JSON string", err)
	}

	holders, err := handler.Service.GetAllHolders(ctx, organizationID, *headerParams, includeDeleted)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to get all holders", err)
		logger.Errorf("Failed to get all holders, Error: %v", err.Error())

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	pagination.SetItems(holders)

	if err := http.OK(c, pagination); err != nil {
		return pkg.ValidateInternalError(err, "Holder")
	}

	return nil
}
