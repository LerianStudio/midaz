// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"time"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libConstants "github.com/LerianStudio/lib-commons/v5/commons/constants"
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

type HolderHandler struct {
	Service *services.UseCase
}

// createHolder is the transport-agnostic core for the holder create. It runs the
// full idempotency dance (claim + replay-or-create + store) using an
// already-resolved client key + TTL, so both the Fiber wrapper (CreateHolder) and
// the Huma shell (CreateHolderHuma) share one implementation and neither touches
// the other's request/response object. It returns replayed=true when the response
// was served from a cached idempotency slot so the caller can set the
// X-Idempotency-Replayed header on its own transport.
func (handler *HolderHandler) createHolder(ctx context.Context, organizationID uuid.UUID, payload *mmodel.CreateHolderInput, clientKey string, ttl time.Duration) (holder *mmodel.Holder, replayed bool, err error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_holder")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	body, err := libCommons.StructToJSONString(payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to serialize holder idempotency payload", err)

		return nil, false, err
	}

	hash := libCommons.HashSHA256(body)

	key := clientKey
	if key == "" {
		key = hash
	}

	internalKey := services.HolderIdempotencyKey(organizationID.String(), key)

	result, err := handler.Service.CreateOrCheckCRMIdempotency(ctx, internalKey, hash, ttl)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to claim holder idempotency", err)

		return nil, false, err
	}

	if result.Replay != nil {
		replay := &mmodel.Holder{}
		if err := json.Unmarshal([]byte(*result.Replay), replay); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to deserialize replayed holder", err)

			return nil, false, err
		}

		return replay, true, nil
	}

	out, err := handler.Service.CreateHolder(ctx, organizationID.String(), payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to create holder", err)

		return nil, false, err
	}

	if value, err := libCommons.StructToJSONString(out); err == nil {
		handler.Service.SetCRMIdempotencyValue(ctx, internalKey, value, ttl)
	} else {
		logger.Log(ctx, libLog.LevelWarn, "Holder created but idempotency replay value could not be stored; a retry with the same key will conflict", libLog.Err(err))
	}

	return out, false, nil
}

// CreateHolder is a method that creates Holder information.
//
//	@Summary		Create a Holder
//	@Description	Creates a new holder with the provided details.
//	@Tags			Holders
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id		header		string						false	"Request ID for tracing"
//	@Param			X-Idempotency-Key	header		string						false	"Idempotency key to safely retry the create; an identical retry returns the original holder"
//	@Param			organization_id		path		string						true	"Organization ID in UUID format"
//	@Param			holder				body		mmodel.CreateHolderInput	true	"Holder Input"
//	@Success		201					{object}	mmodel.Holder				"Successfully created holder"
//	@Failure		400					{object}	mmodel.Error				"Invalid input, validation errors"
//	@Failure		401					{object}	mmodel.Error				"Unauthorized access"
//	@Failure		403					{object}	mmodel.Error				"Forbidden access"
//	@Failure		404					{object}	mmodel.Error				"Organization not found"
//	@Failure		409					{object}	mmodel.Error				"Conflict: the document is already associated with another holder in this organization"
//	@Failure		500					{object}	mmodel.Error				"Internal server error"
//	@Router			/v1/organizations/{organization_id}/holders [post]
func (handler *HolderHandler) CreateHolder(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	payload, ok := p.(*mmodel.CreateHolderInput)
	if !ok || payload == nil {
		return http.WithError(c, pkg.ValidateInternalError(nil, cn.EntityHolder))
	}

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	clientKey, ttl := http.GetIdempotencyKeyAndTTL(c)

	c.Set(libConstants.IdempotencyReplayed, "false")

	holder, replayed, err := handler.createHolder(ctx, organizationID, payload, clientKey, ttl)
	if err != nil {
		return http.WithError(c, err)
	}

	if replayed {
		c.Set(libConstants.IdempotencyReplayed, "true")
	}

	return http.Created(c, holder)
}

// getHolderByID is the transport-agnostic core for the holder read.
func (handler *HolderHandler) getHolderByID(ctx context.Context, organizationID, id uuid.UUID, includeDeleted bool) (*mmodel.Holder, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_holder_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.holder_id", id.String()),
		attribute.Bool("app.request.include_deleted", includeDeleted),
	)

	holder, err := handler.Service.GetHolderByID(ctx, organizationID.String(), id, includeDeleted)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to retrieve holder", err)

		return nil, err
	}

	return holder, nil
}

// GetHolderByID retrieves Holder details by a given id
//
//	@Summary		Retrieve Holder details
//	@Description	Retrieves detailed information about a specific holder using its unique identifier.
//	@Tags			Holders
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id		header		string	false	"Request ID for tracing"
//	@Param			organization_id		path		string	true	"Organization ID in UUID format"
//	@Param			id					path		string	true	"Holder ID in UUID format"
//	@Param			include_deleted		query		string	false	"Returns the holder even if it was logically deleted"	Enums(true,false)
//	@Success		200					{object}	mmodel.Holder	"Successfully retrieved holder"
//	@Failure		400					{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401					{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403					{object}	mmodel.Error	"Forbidden access"
//	@Failure		404					{object}	mmodel.Error	"Holder not found"
//	@Failure		500					{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/holders/{id} [get]
func (handler *HolderHandler) GetHolderByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	includeDeleted := http.GetBooleanParam(c, "include_deleted")

	holder, err := handler.getHolderByID(ctx, organizationID, id, includeDeleted)
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, holder)
}

// updateHolder is the transport-agnostic core for the holder update. fieldsToRemove
// carries the RFC 7396 merge-patch null-field paths; the Fiber wrapper reads them
// from the patchRemove local produced by http.WithBody, the Huma shell derives them
// from the parsed body via http.FindNilFields — both feed this one core.
func (handler *HolderHandler) updateHolder(ctx context.Context, organizationID, id uuid.UUID, payload *mmodel.UpdateHolderInput, fieldsToRemove []string) (*mmodel.Holder, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_holder")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.holder_id", id.String()),
		attribute.Int("app.request.fields_to_remove_count", len(fieldsToRemove)),
	)

	holder, err := handler.Service.UpdateHolderByID(ctx, organizationID.String(), id, payload, fieldsToRemove)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to update holder", err)

		return nil, err
	}

	return holder, nil
}

// UpdateHolder is a method that updates Holder information.
//
//	@Summary		Update a Holder
//	@Description	Update details of a holder.
//	@Tags			Holders
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id		header		string						false	"Request ID for tracing"
//	@Param			organization_id		path		string						true	"Organization ID in UUID format"
//	@Param			id					path		string						true	"Holder ID in UUID format"
//	@Param			holder				body		mmodel.UpdateHolderInput	true	"Holder Input"
//	@Success		200					{object}	mmodel.Holder				"Successfully updated holder"
//	@Failure		400					{object}	mmodel.Error				"Invalid input, validation errors"
//	@Failure		401					{object}	mmodel.Error				"Unauthorized access"
//	@Failure		403					{object}	mmodel.Error				"Forbidden access"
//	@Failure		404					{object}	mmodel.Error				"Holder not found"
//	@Failure		500					{object}	mmodel.Error				"Internal server error"
//	@Router			/v1/organizations/{organization_id}/holders/{id} [patch]
func (handler *HolderHandler) UpdateHolder(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_holder_fiber")
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

	holder, err := handler.updateHolder(ctx, organizationID, id, payload, fieldsToRemove)
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, holder)
}

// deleteHolder is the transport-agnostic core for the holder delete.
func (handler *HolderHandler) deleteHolder(ctx context.Context, organizationID, id uuid.UUID, hardDelete bool) error {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.remove_holder_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.holder_id", id.String()),
		attribute.Bool("app.request.hard_delete", hardDelete),
	)

	if err := handler.Service.DeleteHolderByID(ctx, organizationID.String(), id, hardDelete); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to delete holder", err)

		return err
	}

	return nil
}

// DeleteHolderByID is a method that removes Holder information by a given id.
//
//	@Summary		Delete a Holder
//	@Description	Delete a Holder. **Note:** By default, the delete endpoint performs a logical deletion (soft delete) of the entity in the system. If a physical deletion (hard delete) is required, you can use the query parameter outlined in the documentation.
//	@Tags			Holders
//	@Security		BearerAuth
//	@Param			X-Request-Id		header	string	false	"Request ID for tracing"
//	@Param			organization_id		path	string	true	"Organization ID in UUID format"
//	@Param			id					path	string	true	"Holder ID in UUID format"
//	@Param			hard_delete			query	string	false	"Use only to perform a physical deletion of the data. This action is irreversible."	Enums(true,false)
//	@Success		204	"Holder successfully deleted"
//	@Failure		400	{object}	mmodel.Error	"Invalid input or holder has associated instruments that must be removed first"
//	@Failure		401	{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403	{object}	mmodel.Error	"Forbidden access"
//	@Failure		404	{object}	mmodel.Error	"Holder not found"
//	@Failure		500	{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/holders/{id} [delete]
func (handler *HolderHandler) DeleteHolderByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	hardDelete := http.GetBooleanParam(c, "hard_delete")

	if err := handler.deleteHolder(ctx, organizationID, id, hardDelete); err != nil {
		return http.WithError(c, err)
	}

	return http.NoContent(c)
}

// getAllHolders is the transport-agnostic core for the holder list. queries is the
// map[string]string the caller derived from its transport (Fiber c.Queries() or the
// Huma raw-query rebuild); http.ValidateParameters is the sole query binder so the
// two transports validate identically.
func (handler *HolderHandler) getAllHolders(ctx context.Context, organizationID uuid.UUID, queries map[string]string, includeDeleted bool) (http.Pagination, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_holders")
	defer span.End()

	headerParams, err := http.ValidateParameters(queries)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to validate query parameters", err)

		return http.Pagination{}, err
	}

	pagination := http.Pagination{
		Limit:     headerParams.Limit,
		Page:      headerParams.Page,
		SortOrder: headerParams.SortOrder,
	}

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.Bool("app.request.include_deleted", includeDeleted),
	)

	recordSafeQueryAttributes(span, headerParams)

	holders, err := handler.Service.GetAllHolders(ctx, organizationID.String(), *headerParams, includeDeleted)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get all holders", err)

		return http.Pagination{}, err
	}

	pagination.SetItems(holders)

	return pagination, nil
}

// GetAllHolders retrieves Holder details by a given id
//
//	@Summary		List Holders
//	@Description	List all Holders. CRM listing endpoints support pagination using the page, limit, and sort parameters. The sort parameter orders results by the entity ID using the UUID v7 standard, which is time-sortable, ensuring chronological ordering of the results.
//	@Tags			Holders
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id		header		string	false	"Request ID for tracing"
//	@Param			organization_id		path		string	true	"Organization ID in UUID format"
//	@Param			metadata			query		string	false	"JSON string to filter holders by metadata fields"
//	@Param			limit				query		int		false	"Maximum number of records to return per page"	default(10)	minimum(1)	maximum(100)
//	@Param			page				query		int		false	"Page number for pagination"					default(1)	minimum(1)
//	@Param			sort_order			query		string	false	"Sort direction for results based on creation date"	Enums(asc,desc)
//	@Param			include_deleted		query		string	false	"Return includes logically deleted holders"	Enums(true,false)
//	@Param			external_id			query		string	false	"Filter holders by externalID"
//	@Param			document			query		string	false	"Filter holders by document"
//	@Success		200					{object}	http.Pagination{items=[]mmodel.Holder}	"Successfully retrieved holders list"
//	@Failure		400					{object}	mmodel.Error	"Invalid query parameters"
//	@Failure		401					{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403					{object}	mmodel.Error	"Forbidden access"
//	@Failure		404					{object}	mmodel.Error	"Organization not found"
//	@Failure		500					{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/holders [get]
func (handler *HolderHandler) GetAllHolders(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	includeDeleted := http.GetBooleanParam(c, "include_deleted")

	pagination, err := handler.getAllHolders(ctx, organizationID, c.Queries(), includeDeleted)
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, pagination)
}
