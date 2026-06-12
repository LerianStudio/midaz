// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
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
//	@Security		BearerAuth
//	@Param			X-Request-Id		header		string							false	"Request ID for tracing"
//	@Param			X-Idempotency-Key	header		string							false	"Idempotency key to safely retry the create; an identical retry returns the original instrument"
//	@Param			organization_id		path		string							true	"Organization ID in UUID format"
//	@Param			holder_id			path		string							true	"Holder ID in UUID format"
//	@Param			instrument				body		mmodel.CreateInstrumentInput	true	"Instrument Input"
//	@Success		201					{object}	mmodel.Instrument				"Successfully created instrument"
//	@Failure		400					{object}	mmodel.Error					"Invalid input, validation errors"
//	@Failure		401					{object}	mmodel.Error					"Unauthorized access"
//	@Failure		403					{object}	mmodel.Error					"Forbidden access"
//	@Failure		404					{object}	mmodel.Error					"Organization or holder not found"
//	@Failure		409					{object}	mmodel.Error					"Conflict: the account ID is already associated with another instrument"
//	@Failure		500					{object}	mmodel.Error					"Internal server error"
//	@Router			/v1/organizations/{organization_id}/holders/{holder_id}/instruments [post]
func (handler *InstrumentHandler) CreateInstrument(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_instrument")
	defer span.End()

	payload, ok := p.(*mmodel.CreateInstrumentInput)
	if !ok || payload == nil {
		return http.WithError(c, pkg.ValidateInternalError(nil, cn.EntityInstrument))
	}

	holderID, err := http.GetUUIDFromLocals(c, "holder_id")
	if err != nil {
		return http.WithError(c, err)
	}

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.holder_id", holderID.String()),
	)

	claim, err := claimCRMIdempotency(ctx, c, handler.Service, payload, func(key string) string {
		return services.InstrumentIdempotencyKey(organizationID.String(), holderID.String(), key)
	})
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to claim instrument idempotency", err)

		return http.WithError(c, err)
	}

	if claim.ReplayJSON != nil {
		replay := &mmodel.Instrument{}
		if err := json.Unmarshal([]byte(*claim.ReplayJSON), replay); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to deserialize replayed instrument", err)

			return http.WithError(c, err)
		}

		return http.Created(c, replay)
	}

	out, err := handler.Service.CreateInstrument(ctx, organizationID.String(), holderID, payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to create instrument", err)

		return http.WithError(c, err)
	}

	if value, err := libCommons.StructToJSONString(out); err == nil {
		handler.Service.SetCRMIdempotencyValue(ctx, claim.InternalKey, value, claim.TTL)
	}

	return http.Created(c, out)
}

// GetInstrumentByID retrieves Instrument details by a given id
//
//	@Summary		Retrieve Instrument details
//	@Description	Retrieves detailed information about a specific instrument using its unique identifier.
//	@Tags			Instruments
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id		header		string	false	"Request ID for tracing"
//	@Param			organization_id		path		string	true	"Organization ID in UUID format"
//	@Param			holder_id			path		string	true	"Holder ID in UUID format"
//	@Param			instrument_id		path		string	true	"Instrument ID in UUID format"
//	@Param			include_deleted		query		string	false	"Returns the instrument even if it was logically deleted"	Enums(true,false)
//	@Success		200					{object}	mmodel.Instrument	"Successfully retrieved instrument"
//	@Failure		400					{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401					{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403					{object}	mmodel.Error	"Forbidden access"
//	@Failure		404					{object}	mmodel.Error	"Instrument not found"
//	@Failure		500					{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/holders/{holder_id}/instruments/{instrument_id} [get]
func (handler *InstrumentHandler) GetInstrumentByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_instrument_by_id")
	defer span.End()

	id, err := http.GetUUIDFromLocals(c, "instrument_id")
	if err != nil {
		return http.WithError(c, err)
	}

	holderID, err := http.GetUUIDFromLocals(c, "holder_id")
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
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.instrument_id", id.String()),
		attribute.Bool("app.request.include_deleted", includeDeleted),
	)

	alias, err := handler.Service.GetInstrumentByID(ctx, organizationID.String(), holderID, id, includeDeleted)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to retrieve instrument", err)

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
//	@Security		BearerAuth
//	@Param			X-Request-Id		header		string							false	"Request ID for tracing"
//	@Param			organization_id		path		string							true	"Organization ID in UUID format"
//	@Param			holder_id			path		string							true	"Holder ID in UUID format"
//	@Param			instrument_id		path		string							true	"Instrument ID in UUID format"
//	@Param			instrument				body		mmodel.UpdateInstrumentInput	true	"Instrument Input"
//	@Success		200					{object}	mmodel.Instrument				"Successfully updated instrument"
//	@Failure		400					{object}	mmodel.Error					"Invalid input, validation errors"
//	@Failure		401					{object}	mmodel.Error					"Unauthorized access"
//	@Failure		403					{object}	mmodel.Error					"Forbidden access"
//	@Failure		404					{object}	mmodel.Error					"Instrument not found"
//	@Failure		500					{object}	mmodel.Error					"Internal server error"
//	@Router			/v1/organizations/{organization_id}/holders/{holder_id}/instruments/{instrument_id} [patch]
func (handler *InstrumentHandler) UpdateInstrument(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_instrument")
	defer span.End()

	id, err := http.GetUUIDFromLocals(c, "instrument_id")
	if err != nil {
		return http.WithError(c, err)
	}

	holderID, err := http.GetUUIDFromLocals(c, "holder_id")
	if err != nil {
		return http.WithError(c, err)
	}

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	payload, ok := p.(*mmodel.UpdateInstrumentInput)
	if !ok || payload == nil {
		return http.WithError(c, pkg.ValidateInternalError(nil, cn.EntityInstrument))
	}

	fieldsToRemove, ok := c.Locals("patchRemove").([]string)
	if !ok {
		libOpentelemetry.HandleSpanError(span, "Failed to get fields to remove", cn.ErrInternalServer)

		logger.Log(ctx, libLog.LevelError, "Failed to get fields to remove")

		return http.WithError(c, pkg.ValidateInternalError(nil, cn.EntityInstrument))
	}

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.instrument_id", id.String()),
		attribute.Int("app.request.fields_to_remove_count", len(fieldsToRemove)),
	)

	alias, err := handler.Service.UpdateInstrumentByID(ctx, organizationID.String(), holderID, id, payload, fieldsToRemove)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to update instrument", err)

		return http.WithError(c, err)
	}

	return http.OK(c, alias)
}

// DeleteInstrumentByID removes an instrument by a given id
//
//	@Summary		Delete an Instrument
//	@Description	Delete an Instrument. **Note:** By default, the delete endpoint performs a logical deletion (soft delete) of the entity in the system. If a physical deletion (hard delete) is required, you can use the query parameter outlined in the documentation.
//	@Tags			Instruments
//	@Security		BearerAuth
//	@Param			X-Request-Id		header	string	false	"Request ID for tracing"
//	@Param			organization_id		path	string	true	"Organization ID in UUID format"
//	@Param			holder_id			path	string	true	"Holder ID in UUID format"
//	@Param			instrument_id		path	string	true	"Instrument ID in UUID format"
//	@Param			hard_delete			query	string	false	"Use only to perform a physical deletion of the data. This action is irreversible."	Enums(true,false)
//	@Success		204	"Instrument successfully deleted"
//	@Failure		400	{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401	{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403	{object}	mmodel.Error	"Forbidden access"
//	@Failure		404	{object}	mmodel.Error	"Instrument not found"
//	@Failure		500	{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/holders/{holder_id}/instruments/{instrument_id} [delete]
func (handler *InstrumentHandler) DeleteInstrumentByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.remove_instrument_by_id")
	defer span.End()

	id, err := http.GetUUIDFromLocals(c, "instrument_id")
	if err != nil {
		return http.WithError(c, err)
	}

	holderID, err := http.GetUUIDFromLocals(c, "holder_id")
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
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.instrument_id", id.String()),
		attribute.Bool("app.request.hard_delete", hardDelete),
	)

	err = handler.Service.DeleteInstrumentByID(ctx, organizationID.String(), holderID, id, hardDelete)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to delete instrument", err)

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
//	@Security		BearerAuth
//	@Param			X-Request-Id			header		string	false	"Request ID for tracing"
//	@Param			organization_id			path		string	true	"Organization ID in UUID format"
//	@Param			holder_id				query		string	false	"Filter instruments by holder ID in UUID format"
//	@Param			metadata				query		string	false	"JSON string to filter instruments by metadata fields"
//	@Param			limit					query		int		false	"Maximum number of records to return per page"	default(10)	minimum(1)	maximum(100)
//	@Param			page					query		int		false	"Page number for pagination"					default(1)	minimum(1)
//	@Param			sort_order				query		string	false	"Sort direction for results based on creation date"	Enums(asc,desc)
//	@Param			include_deleted			query		string	false	"Return includes logically deleted instruments"	Enums(true,false)
//	@Param			account_id				query		string	false	"Filter instrument by accountID"
//	@Param			ledger_id				query		string	false	"Filter instrument by ledgerID"
//	@Param			document				query		string	false	"Filter instrument by document"
//	@Param			banking_details_branch					query		string	false	"Filter instrument by banking details branch"
//	@Param			banking_details_account					query		string	false	"Filter instrument by banking details account"
//	@Param			banking_details_iban					query		string	false	"Filter instrument by banking details iban"
//	@Param			regulatory_fields_participant_document	query		string	false	"Filter instrument by regulatory fields participant document"
//	@Param			related_party_document					query		string	false	"Filter instrument by related party document"
//	@Param			related_party_role						query		string	false	"Filter instrument by related party role"	Enums(PRIMARY_HOLDER,LEGAL_REPRESENTATIVE,RESPONSIBLE_PARTY)
//	@Success		200										{object}	http.Pagination{items=[]mmodel.Instrument}	"Successfully retrieved instruments list"
//	@Failure		400						{object}	mmodel.Error	"Invalid query parameters"
//	@Failure		401						{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403						{object}	mmodel.Error	"Forbidden access"
//	@Failure		404						{object}	mmodel.Error	"Organization not found"
//	@Failure		500						{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/instruments [get]
func (handler *InstrumentHandler) GetAllInstruments(c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_instruments")
	defer span.End()

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to validate query parameters", err)

		return http.WithError(c, err)
	}

	var holderID uuid.UUID
	if !libCommons.IsNilOrEmpty(headerParams.HolderID) {
		holderID, err = uuid.Parse(*headerParams.HolderID)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to parse holder ID", err)

			return http.WithError(c, err)
		}
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

	if !libCommons.IsNilOrEmpty(headerParams.HolderID) {
		span.SetAttributes(
			attribute.String("app.request.holder_id", holderID.String()),
		)
	}

	recordSafeQueryAttributes(span, headerParams)

	aliases, err := handler.Service.GetAllInstruments(ctx, organizationID.String(), holderID, *headerParams, includeDeleted)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get all aliases", err)

		return http.WithError(c, err)
	}

	pagination.SetItems(aliases)

	return http.OK(c, pagination)
}

// DeleteRelatedParty removes a related party from an instrument
//
//	@Summary		Delete a Related Party
//	@Description	Delete a Related Party from an Instrument. This operation performs a physical deletion (hard delete) of the related party. Related parties are created inline via the instrument body (CreateInstrumentInput.RelatedParties) and retrieved via GetInstrumentByID; only deletion is exposed as a distinct sub-resource route.
//	@Tags			Instruments
//	@Security		BearerAuth
//	@Param			X-Request-Id		header	string	false	"Request ID for tracing"
//	@Param			organization_id		path	string	true	"Organization ID in UUID format"
//	@Param			holder_id			path	string	true	"Holder ID in UUID format"
//	@Param			instrument_id		path	string	true	"Instrument ID in UUID format"
//	@Param			related_party_id	path	string	true	"Related Party ID in UUID format"
//	@Success		204	"Related party successfully deleted"
//	@Failure		400	{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401	{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403	{object}	mmodel.Error	"Forbidden access"
//	@Failure		404	{object}	mmodel.Error	"Related party not found"
//	@Failure		500	{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/holders/{holder_id}/instruments/{instrument_id}/related-parties/{related_party_id} [delete]
func (handler *InstrumentHandler) DeleteRelatedParty(c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_related_party")
	defer span.End()

	holderID, err := http.GetUUIDFromLocals(c, "holder_id")
	if err != nil {
		return http.WithError(c, err)
	}

	instrumentID, err := http.GetUUIDFromLocals(c, "instrument_id")
	if err != nil {
		return http.WithError(c, err)
	}

	relatedPartyID, err := http.GetUUIDFromLocals(c, "related_party_id")
	if err != nil {
		return http.WithError(c, err)
	}

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.instrument_id", instrumentID.String()),
		attribute.String("app.request.related_party_id", relatedPartyID.String()),
	)

	err = handler.Service.DeleteRelatedPartyByID(ctx, organizationID.String(), holderID, instrumentID, relatedPartyID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to delete related party", err)

		return http.WithError(c, err)
	}

	return http.NoContent(c)
}
