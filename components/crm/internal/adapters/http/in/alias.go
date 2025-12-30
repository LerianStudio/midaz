// Package in provides HTTP handlers for incoming requests to the CRM service.
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
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// AliasHandler handles HTTP requests for alias operations.
type AliasHandler struct {
	Service *services.UseCase
}

// CreateAlias is a method that creates Alias information linked with a specified Holder.
//
//	@Summary		Create an Alias Account
//	@Description	Enables a creation of an alias account, which represents an account in the ledger. The alias account is linked to specific business information, making it easier to manage and abstract account data within the system.
//	@Tags			Aliases
//	@Accept			json
//	@Produce		json
//	@Param			Authorization		header		string					false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id	header		string					true	"The unique identifier of the Organization associated with the Ledger."
//	@Param			holder_id			path		string					true	"The unique identifier of the Holder."
//	@Param			alias				body		mmodel.CreateAliasInput	true	"Alias Input"
//	@Success		201					{object}	mmodel.Alias
//	@Failure		400					{object}	pkg.HTTPError
//	@Failure		404					{object}	pkg.HTTPError
//	@Failure		500					{object}	pkg.HTTPError
//	@Router			/v1/holders/{holder_id}/aliases [post]
func (handler *AliasHandler) CreateAlias(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_alias")
	defer span.End()

	payload := http.Payload[*mmodel.CreateAliasInput](c, p)
	holderID := http.LocalUUID(c, "holder_id")
	organizationID := http.LocalHeaderUUID(c, "X-Organization-Id")

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
	)

	err := libOpenTelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", payload)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)
	}

	out, err := handler.Service.CreateAlias(ctx, organizationID, holderID, payload)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to create alias", err)

		logger.Errorf("Failed to create alias: %v", err)

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	if err := http.Created(c, out); err != nil {
		return pkg.ValidateInternalError(err, "Alias")
	}

	return nil
}

// GetAliasByID retrieves Alias details by a given id
//
//	@Summary		Retrieve Alias details
//	@Description	Retrieves detailed information about a specific alias using its unique identifier.
//	@Tags			Aliases
//	@Produce		json
//	@Param			Authorization		header		string	false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id	header		string	true	"The unique identifier of the Organization associated with the Ledger."
//	@Param			holder_id			path		string	true	"The unique identifier of the Holder."
//	@Param			alias_id			path		string	true	"The unique identifier of the Alias account."
//	@Param			include_deleted		query		string	false	"Returns the alias even if it was logically deleted."
//	@Success		200					{object}	mmodel.Alias
//	@Failure		400					{object}	pkg.HTTPError
//	@Failure		404					{object}	pkg.HTTPError
//	@Failure		500					{object}	pkg.HTTPError
//	@Router			/v1/holders/{holder_id}/aliases/{alias_id} [get]
func (handler *AliasHandler) GetAliasByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_alias_by_id")
	defer span.End()

	id := http.LocalUUID(c, "id")
	holderID := http.LocalUUID(c, "holder_id")
	organizationID := http.LocalHeaderUUID(c, "X-Organization-Id")
	includeDeleted := http.GetBooleanParam(c, "include_deleted")

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.alias_id", id.String()),
		attribute.Bool("app.request.include_deleted", includeDeleted),
	)

	logger.Infof("Initiating retrieval of Alias with ID: %s", id.String())

	alias, err := handler.Service.GetAliasByID(ctx, organizationID, holderID, id, includeDeleted)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to retrieve alias", err)

		logger.Errorf("Failed to retrieve Alias with ID: %s from Holder %s, Error: %s", id.String(), holderID.String(), err.Error())

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	if err := http.OK(c, alias); err != nil {
		return pkg.ValidateInternalError(err, "Alias")
	}

	return nil
}

// UpdateAlias is a method that updates Holder information.
//
//	@Summary		Update an Alias
//	@Description	Update details of an alias.
//	@Tags			Aliases
//	@Accept			json
//	@Produce		json
//	@Param			Authorization		header		string					false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id	header		string					true	"The unique identifier of the Organization associated with the Ledger."
//	@Param			holder_id			path		string					true	"The unique identifier of the Holder."
//	@Param			alias_id			path		string					true	"The unique identifier of the Alias account."
//	@Param			alias				body		mmodel.UpdateAliasInput	true	"Alias Input"
//	@Success		200					{object}	mmodel.Alias
//	@Failure		400					{object}	pkg.HTTPError
//	@Failure		404					{object}	pkg.HTTPError
//	@Failure		500					{object}	pkg.HTTPError
//	@Router			/v1/holders/{holder_id}/aliases/{alias_id} [patch]
func (handler *AliasHandler) UpdateAlias(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_alias")
	defer span.End()

	id := http.LocalUUID(c, "id")
	holderID := http.LocalUUID(c, "holder_id")
	organizationID := http.LocalHeaderUUID(c, "X-Organization-Id")
	payload := http.Payload[*mmodel.UpdateAliasInput](c, p)

	fieldsToRemove := http.LocalStringSlice(c, "patchRemove")

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.alias_id", id.String()),
	)

	err := libOpenTelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", payload)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)
	}

	err = libOpenTelemetry.SetSpanAttributesFromStruct(&span, "app.request.fields_to_remove", fieldsToRemove)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to convert fields_to_remove to JSON string", err)
	}

	logger.Infof("Request to update alias %s from holder %s", id.String(), holderID.String())

	alias, err := handler.Service.UpdateAliasByID(ctx, organizationID, holderID, id, payload, fieldsToRemove)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to update alias", err)

		logger.Errorf("Failed to update alias %s from holder %s, Error: %s", id.String(), holderID.String(), err.Error())

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	if err := http.OK(c, alias); err != nil {
		return pkg.ValidateInternalError(err, "Alias")
	}

	return nil
}

// DeleteAliasByID removes an alias by a given id
//
//	@Summary		Delete an Alias
//	@Description	Delete an Alias. **Note:** By default, the delete endpoint performs a logical deletion (soft delete) of the entity in the system. If a physical deletion (hard delete) is required, you can use the query parameter outlined in the documentation.
//	@Tags			Aliases
//	@Param			Authorization		header	string	false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id	header	string	true	"The unique identifier of the Organization associated with the Ledger."
//	@Param			holder_id			path	string	true	"The unique identifier of the Holder."
//	@Param			alias_id			path	string	true	"The unique identifier of the Alias account."
//	@Param			hard_delete			query	string	false	"Use only to perform a physical deletion of the data. This action is irreversible."
//	@Success		204
//	@Failure		400	{object}	pkg.HTTPError
//	@Failure		404	{object}	pkg.HTTPError
//	@Failure		500	{object}	pkg.HTTPError
//	@Router			/v1/holders/{holder_id}/aliases/{alias_id} [delete]
func (handler *AliasHandler) DeleteAliasByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.remove_alias_by_id")
	defer span.End()

	id := http.LocalUUID(c, "id")
	holderID := http.LocalUUID(c, "holder_id")
	organizationID := http.LocalHeaderUUID(c, "X-Organization-Id")
	hardDelete := http.GetBooleanParam(c, "hard_delete")

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.alias_id", id.String()),
		attribute.Bool("app.request.hard_delete", hardDelete),
	)

	logger.Infof("Initiating removal of alias with ID: %s", id.String())

	err := handler.Service.DeleteAliasByID(ctx, organizationID, holderID, id, hardDelete)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to delete alias", err)

		logger.Errorf("Failed to delete alias with ID: %s, Error: %s", id.String(), err.Error())

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	if err := http.NoContent(c); err != nil {
		return pkg.ValidateInternalError(err, "Alias")
	}

	return nil
}

// GetAllAliases retrieves aliases
//
//	@Summary		List Aliases
//	@Description	List all Aliases with or without filters. CRM listing endpoints support pagination using the page, limit, and sort parameters. The sort parameter orders results by the entity ID using the UUID v7 standard, which is time-sortable, ensuring chronological ordering of the results.
//	@Tags			Aliases
//	@Produce		json
//	@Param			Authorization			header		string	false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id		header		string	true	"The unique identifier of the Organization associated with the Ledger."
//	@Param			holder_id				query		string	false	"The unique identifier of the Holder."
//	@Param			metadata				query		string	false	"Metadata"
//	@Param			limit					query		int		false	"Limit"			default(10)
//	@Param			page					query		int		false	"Page"			default(1)
//	@Param			sort_order				query		string	false	"Sort Order"	Enums(asc,desc)
//	@Param			include_deleted			query		string	false	"Return includes logically deleted aliases."
//	@Param			account_id				query		string	false	"Filter alias by accountID"
//	@Param			ledger_id				query		string	false	"Filter alias by ledgerID"
//	@Param			document				query		string	false	"Filter alias by document"
//	@Param			banking_details_branch	query		string	false	"Filter alias by banking details branch"
//	@Param			banking_details_account	query		string	false	"Filter alias by banking details account"
//	@Param			banking_details_iban	query		string	false	"Filter alias by banking details iban"
//	@Success		200						{object}	libPostgres.Pagination{items=[]mmodel.Alias,page=int,limit=int}
//	@Failure		400						{object}	pkg.HTTPError
//	@Failure		404						{object}	pkg.HTTPError
//	@Failure		500						{object}	pkg.HTTPError
//	@Router			/v1/aliases [get]
func (handler *AliasHandler) GetAllAliases(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_aliases")
	defer span.End()

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to validate query parameters", err)
		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		return writeCRMError(c, err)
	}

	holderID, hasHolderID, err := parseOptionalHolderID(headerParams.HolderID)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to parse holder ID", err)
		logger.Errorf("Failed to parse holder ID, Error: %s", err.Error())

		return writeCRMError(c, err)
	}

	pagination := libPostgres.Pagination{
		Limit:     headerParams.Limit,
		Page:      headerParams.Page,
		SortOrder: headerParams.SortOrder,
	}

	organizationID := http.LocalHeaderUUID(c, "X-Organization-Id")
	includeDeleted := http.GetBooleanParam(c, "include_deleted")

	attrs := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.Bool("app.request.include_deleted", includeDeleted),
	}
	if hasHolderID {
		attrs = append(attrs, attribute.String("app.request.holder_id", holderID.String()))
	}

	span.SetAttributes(attrs...)

	err = libOpenTelemetry.SetSpanAttributesFromStruct(&span, "app.request.query_params", headerParams)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to convert query_params to JSON string", err)
	}

	aliases, err := handler.Service.GetAllAliases(ctx, organizationID, holderID, *headerParams, includeDeleted)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to get all aliases", err)
		logger.Errorf("Failed to get all aliases, Error: %v", err.Error())

		return writeCRMError(c, err)
	}

	pagination.SetItems(aliases)

	if err := http.OK(c, pagination); err != nil {
		return pkg.ValidateInternalError(err, "Alias")
	}

	return nil
}

func writeCRMError(c *fiber.Ctx, err error) error {
	if httpErr := http.WithError(c, err); httpErr != nil {
		return httpErr
	}

	return nil
}

func parseOptionalHolderID(holderID *string) (uuid.UUID, bool, error) {
	if libCommons.IsNilOrEmpty(holderID) {
		return uuid.UUID{}, false, nil
	}

	parsedHolderID, err := uuid.Parse(*holderID)
	if err != nil {
		//nolint:wrapcheck // Error is handled at the HTTP layer via http.WithError.
		return uuid.UUID{}, false, err
	}

	return parsedHolderID, true, nil
}
