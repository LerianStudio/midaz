// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"fmt"

	"github.com/LerianStudio/midaz/v3/components/crm/services"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpenTelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

type InstrumentHandler struct {
	Service *services.UseCase
}

// CreateInstrument is a method that creates Instrument information linked with a specified Holder.
//
//	@Summary		Create an Instrument Account
//	@Description	Enables a creation of an instrument account, which represents an account in the ledger. The instrument account is linked to specific business information, making it easier to manage and abstract account data within the system.
//	@Tags			Instruments
//	@Accept			json
//	@Produce		json
//	@Param			Authorization		header		string					false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id	header		string					true	"The unique identifier of the Organization associated with the Ledger."
//	@Param			holder_id			path		string					true	"The unique identifier of the Holder."
//	@Param			instrument				body		mmodel.CreateInstrumentInput	true	"Instrument Input"
//	@Success		201					{object}	mmodel.Instrument
//	@Failure		400					{object}	pkg.HTTPError
//	@Failure		404					{object}	pkg.HTTPError
//	@Failure		500					{object}	pkg.HTTPError
//	@Router			/v1/holders/{holder_id}/instruments [post]
func (handler *InstrumentHandler) CreateInstrument(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_instrument")
	defer span.End()

	payload, ok := p.(*mmodel.CreateInstrumentInput)
	if !ok || payload == nil {
		return http.WithError(c, cn.ErrInternalServer)
	}

	holderID, err := http.GetUUIDFromLocals(c, "holder_id")
	if err != nil {
		return http.WithError(c, err)
	}

	organizationID := c.Get("X-Organization-Id")

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
	)

	out, err := handler.Service.CreateInstrument(ctx, organizationID, holderID, payload)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to create alias", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to create alias: %v", err))

		return http.WithError(c, err)
	}

	return http.Created(c, out)
}

// GetInstrumentByID retrieves Instrument details by a given id
//
//	@Summary		Retrieve Instrument details
//	@Description	Retrieves detailed information about a specific instrument using its unique identifier.
//	@Tags			Instruments
//	@Produce		json
//	@Param			Authorization		header		string	false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id	header		string	true	"The unique identifier of the Organization associated with the Ledger."
//	@Param			holder_id			path		string	true	"The unique identifier of the Holder."
//	@Param			instrument_id		path		string	true	"The unique identifier of the Instrument account."
//	@Param			include_deleted		query		string	false	"Returns the instrument even if it was logically deleted."
//	@Success		200					{object}	mmodel.Instrument
//	@Failure		400					{object}	pkg.HTTPError
//	@Failure		404					{object}	pkg.HTTPError
//	@Failure		500					{object}	pkg.HTTPError
//	@Router			/v1/holders/{holder_id}/instruments/{instrument_id} [get]
func (handler *InstrumentHandler) GetInstrumentByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_instrument_by_id")
	defer span.End()

	id, err := http.GetUUIDFromLocals(c, "alias_id")
	if err != nil {
		return http.WithError(c, err)
	}

	holderID, err := http.GetUUIDFromLocals(c, "holder_id")
	if err != nil {
		return http.WithError(c, err)
	}

	organizationID := c.Get("X-Organization-Id")
	includeDeleted := http.GetBooleanParam(c, "include_deleted")

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.alias_id", id.String()),
		attribute.Bool("app.request.include_deleted", includeDeleted),
	)

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Initiating retrieval of Alias with ID: %s", id.String()))

	alias, err := handler.Service.GetInstrumentByID(ctx, organizationID, holderID, id, includeDeleted)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to retrieve alias", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to retrieve Alias with ID: %s from Holder %s, Error: %s", id.String(), holderID.String(), err.Error()))

		return http.WithError(c, err)
	}

	return http.OK(c, alias)
}

// UpdateInstrument is a method that updates Instrument information.
//
//	@Summary		Update an Instrument
//	@Description	Update details of an instrument.
//	@Tags			Instruments
//	@Accept			json
//	@Produce		json
//	@Param			Authorization		header		string					false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id	header		string					true	"The unique identifier of the Organization associated with the Ledger."
//	@Param			holder_id			path		string					true	"The unique identifier of the Holder."
//	@Param			instrument_id		path		string					true	"The unique identifier of the Instrument account."
//	@Param			instrument				body		mmodel.UpdateInstrumentInput	true	"Instrument Input"
//	@Success		200					{object}	mmodel.Instrument
//	@Failure		400					{object}	pkg.HTTPError
//	@Failure		404					{object}	pkg.HTTPError
//	@Failure		500					{object}	pkg.HTTPError
//	@Router			/v1/holders/{holder_id}/instruments/{instrument_id} [patch]
func (handler *InstrumentHandler) UpdateInstrument(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_instrument")
	defer span.End()

	id, err := http.GetUUIDFromLocals(c, "alias_id")
	if err != nil {
		return http.WithError(c, err)
	}

	holderID, err := http.GetUUIDFromLocals(c, "holder_id")
	if err != nil {
		return http.WithError(c, err)
	}

	organizationID := c.Get("X-Organization-Id")

	payload, ok := p.(*mmodel.UpdateInstrumentInput)
	if !ok || payload == nil {
		return http.WithError(c, cn.ErrInternalServer)
	}

	fieldsToRemove, ok := c.Locals("patchRemove").([]string)
	if !ok {
		libOpenTelemetry.HandleSpanError(span, "Failed to get fields to remove", cn.ErrInternalServer)

		logger.Log(ctx, libLog.LevelError, "Failed to get fields to remove")

		return http.WithError(c, cn.ErrInternalServer)
	}

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.alias_id", id.String()),
	)

	err = libOpenTelemetry.SetSpanAttributesFromValue(span, "app.request.fields_to_remove", fieldsToRemove, nil)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to convert fields_to_remove to JSON string", err)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Request to update alias %s from holder %s", id.String(), holderID.String()))

	alias, err := handler.Service.UpdateInstrumentByID(ctx, organizationID, holderID, id, payload, fieldsToRemove)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to update alias", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to update alias %s from holder %s, Error: %s", id.String(), holderID.String(), err.Error()))

		return http.WithError(c, err)
	}

	return http.OK(c, alias)
}

// DeleteInstrumentByID removes an instrument by a given id
//
//	@Summary		Delete an Instrument
//	@Description	Delete an Instrument. **Note:** By default, the delete endpoint performs a logical deletion (soft delete) of the entity in the system. If a physical deletion (hard delete) is required, you can use the query parameter outlined in the documentation.
//	@Tags			Instruments
//	@Param			Authorization		header	string	false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id	header	string	true	"The unique identifier of the Organization associated with the Ledger."
//	@Param			holder_id			path	string	true	"The unique identifier of the Holder."
//	@Param			instrument_id		path	string	true	"The unique identifier of the Instrument account."
//	@Param			hard_delete			query	string	false	"Use only to perform a physical deletion of the data. This action is irreversible."
//	@Success		204
//	@Failure		400	{object}	pkg.HTTPError
//	@Failure		404	{object}	pkg.HTTPError
//	@Failure		500	{object}	pkg.HTTPError
//	@Router			/v1/holders/{holder_id}/instruments/{instrument_id} [delete]
func (handler *InstrumentHandler) DeleteInstrumentByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.remove_instrument_by_id")
	defer span.End()

	id, err := http.GetUUIDFromLocals(c, "alias_id")
	if err != nil {
		return http.WithError(c, err)
	}

	holderID, err := http.GetUUIDFromLocals(c, "holder_id")
	if err != nil {
		return http.WithError(c, err)
	}

	organizationID := c.Get("X-Organization-Id")
	hardDelete := http.GetBooleanParam(c, "hard_delete")

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.alias_id", id.String()),
		attribute.Bool("app.request.hard_delete", hardDelete),
	)

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Initiating removal of alias with ID: %s", id.String()))

	err = handler.Service.DeleteInstrumentByID(ctx, organizationID, holderID, id, hardDelete)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to delete alias", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to delete alias with ID: %s, Error: %s", id.String(), err.Error()))

		return http.WithError(c, err)
	}

	return http.NoContent(c)
}

// GetAllInstruments retrieves instruments
//
//	@Summary		List Instruments
//	@Description	List all Instruments with or without filters. CRM listing endpoints support pagination using the page, limit, and sort parameters. The sort parameter orders results by the entity ID using the UUID v7 standard, which is time-sortable, ensuring chronological ordering of the results.
//	@Tags			Instruments
//	@Produce		json
//	@Param			Authorization			header		string	false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id		header		string	true	"The unique identifier of the Organization associated with the Ledger."
//	@Param			holder_id				query		string	false	"The unique identifier of the Holder."
//	@Param			metadata				query		string	false	"Metadata"
//	@Param			limit					query		int		false	"Limit"			default(10)
//	@Param			page					query		int		false	"Page"			default(1)
//	@Param			sort_order				query		string	false	"Sort Order"	Enums(asc,desc)
//	@Param			include_deleted			query		string	false	"Return includes logically deleted instruments."
//	@Param			account_id				query		string	false	"Filter instrument by accountID"
//	@Param			ledger_id				query		string	false	"Filter instrument by ledgerID"
//	@Param			document				query		string	false	"Filter instrument by document"
//	@Param			banking_details_branch					query		string	false	"Filter instrument by banking details branch"
//	@Param			banking_details_account					query		string	false	"Filter instrument by banking details account"
//	@Param			banking_details_iban					query		string	false	"Filter instrument by banking details iban"
//	@Param			regulatory_fields_participant_document	query		string	false	"Filter instrument by regulatory fields participant document"
//	@Param			related_party_document					query		string	false	"Filter instrument by related party document"
//	@Param			related_party_role						query		string	false	"Filter instrument by related party role"
//	@Success		200										{object}	http.Pagination{items=[]mmodel.Instrument}
//	@Failure		400						{object}	pkg.HTTPError
//	@Failure		404						{object}	pkg.HTTPError
//	@Failure		500						{object}	pkg.HTTPError
//	@Router			/v1/instruments [get]
func (handler *InstrumentHandler) GetAllInstruments(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_instruments")
	defer span.End()

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to validate query parameters", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to validate query parameters, Error: %s", err.Error()))

		return http.WithError(c, err)
	}

	var holderID uuid.UUID
	if !libCommons.IsNilOrEmpty(headerParams.HolderID) {
		holderID, err = uuid.Parse(*headerParams.HolderID)
		if err != nil {
			libOpenTelemetry.HandleSpanError(span, "Failed to parse holder ID", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to parse holder ID, Error: %s", err.Error()))

			return http.WithError(c, err)
		}
	}

	pagination := http.Pagination{
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

	if !libCommons.IsNilOrEmpty(headerParams.HolderID) {
		span.SetAttributes(
			attribute.String("app.request.holder_id", holderID.String()),
		)
	}

	recordSafeQueryAttributes(span, headerParams)

	aliases, err := handler.Service.GetAllInstruments(ctx, organizationID, holderID, *headerParams, includeDeleted)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to get all aliases", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get all aliases, Error: %v", err.Error()))

		return http.WithError(c, err)
	}

	pagination.SetItems(aliases)

	return http.OK(c, pagination)
}

// DeleteRelatedParty removes a related party from an instrument
//
//	@Summary		Delete a Related Party
//	@Description	Delete a Related Party from an Instrument. This operation performs a physical deletion (hard delete) of the related party.
//	@Tags			Instruments
//	@Param			Authorization		header	string	false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id	header	string	true	"The unique identifier of the Organization associated with the Ledger."
//	@Param			holder_id			path	string	true	"The unique identifier of the Holder."
//	@Param			instrument_id		path	string	true	"The unique identifier of the Instrument account."
//	@Param			related_party_id	path	string	true	"The unique identifier of the Related Party."
//	@Success		204
//	@Failure		400	{object}	pkg.HTTPError
//	@Failure		404	{object}	pkg.HTTPError
//	@Failure		500	{object}	pkg.HTTPError
//	@Router			/v1/holders/{holder_id}/instruments/{instrument_id}/related-parties/{related_party_id} [delete]
func (handler *InstrumentHandler) DeleteRelatedParty(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_related_party")
	defer span.End()

	holderID, err := http.GetUUIDFromLocals(c, "holder_id")
	if err != nil {
		return http.WithError(c, err)
	}

	aliasID, err := http.GetUUIDFromLocals(c, "alias_id")
	if err != nil {
		return http.WithError(c, err)
	}

	relatedPartyID, err := http.GetUUIDFromLocals(c, "related_party_id")
	if err != nil {
		return http.WithError(c, err)
	}

	organizationID := c.Get("X-Organization-Id")

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.alias_id", aliasID.String()),
		attribute.String("app.request.related_party_id", relatedPartyID.String()),
	)

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Initiating removal of related party with ID: %s from alias: %s", relatedPartyID.String(), aliasID.String()))

	err = handler.Service.DeleteRelatedPartyByID(ctx, organizationID, holderID, aliasID, relatedPartyID)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to delete related party", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to delete related party with ID: %s, Error: %s", relatedPartyID.String(), err.Error()))

		return http.WithError(c, err)
	}

	return http.NoContent(c)
}
