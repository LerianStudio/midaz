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

type InstrumentHandler struct {
	Service *services.UseCase
}

// createInstrument is the transport-agnostic core for the instrument create. It runs
// the full idempotency dance (claim + replay-or-create + store) using an
// already-resolved client key + TTL, so both the Fiber wrapper (CreateInstrument) and
// the Huma shell (CreateInstrumentHuma) share one implementation and neither touches
// the other's request/response object. It returns replayed=true when the response was
// served from a cached idempotency slot so the caller can set the
// X-Idempotency-Replayed header on its own transport. Instruments are namespaced by
// (organization, holder), matching services.InstrumentIdempotencyKey.
func (handler *InstrumentHandler) createInstrument(ctx context.Context, organizationID, holderID uuid.UUID, payload *mmodel.CreateInstrumentInput, clientKey string, ttl time.Duration) (instrument *mmodel.Instrument, replayed bool, err error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_instrument")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.holder_id", holderID.String()),
	)

	body, err := libCommons.StructToJSONString(payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to serialize instrument idempotency payload", err)

		return nil, false, err
	}

	hash := libCommons.HashSHA256(body)

	key := clientKey
	if key == "" {
		key = hash
	}

	internalKey := services.InstrumentIdempotencyKey(organizationID.String(), holderID.String(), key)

	result, err := handler.Service.CreateOrCheckCRMIdempotency(ctx, internalKey, hash, ttl)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to claim instrument idempotency", err)

		return nil, false, err
	}

	if result.Replay != nil {
		replay := &mmodel.Instrument{}
		if err := json.Unmarshal([]byte(*result.Replay), replay); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to deserialize replayed instrument", err)

			return nil, false, err
		}

		return replay, true, nil
	}

	out, err := handler.Service.CreateInstrument(ctx, organizationID.String(), holderID, payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to create instrument", err)

		return nil, false, err
	}

	if value, err := libCommons.StructToJSONString(out); err == nil {
		handler.Service.SetCRMIdempotencyValue(ctx, internalKey, value, ttl)
	} else {
		logger.Log(ctx, libLog.LevelWarn, "Instrument created but idempotency replay value could not be stored; a retry with the same key will conflict", libLog.Err(err))
	}

	return out, false, nil
}

// CreateInstrument is a method that creates Instrument information linked with a specified Holder.
func (handler *InstrumentHandler) CreateInstrument(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

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

	clientKey, ttl := http.GetIdempotencyKeyAndTTL(c)

	c.Set(libConstants.IdempotencyReplayed, "false")

	instrument, replayed, err := handler.createInstrument(ctx, organizationID, holderID, payload, clientKey, ttl)
	if err != nil {
		return http.WithError(c, err)
	}

	if replayed {
		c.Set(libConstants.IdempotencyReplayed, "true")
	}

	return http.Created(c, instrument)
}

// getInstrumentByID is the transport-agnostic core for the instrument read.
func (handler *InstrumentHandler) getInstrumentByID(ctx context.Context, organizationID, holderID, id uuid.UUID, includeDeleted bool) (*mmodel.Instrument, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_instrument_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.instrument_id", id.String()),
		attribute.Bool("app.request.include_deleted", includeDeleted),
	)

	instrument, err := handler.Service.GetInstrumentByID(ctx, organizationID.String(), holderID, id, includeDeleted)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to retrieve instrument", err)

		return nil, err
	}

	return instrument, nil
}

// GetInstrumentByID retrieves Instrument details by a given id
func (handler *InstrumentHandler) GetInstrumentByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

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

	instrument, err := handler.getInstrumentByID(ctx, organizationID, holderID, id, includeDeleted)
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, instrument)
}

// updateInstrument is the transport-agnostic core for the instrument update.
// fieldsToRemove carries the RFC 7396 merge-patch null-field paths; the Fiber wrapper
// reads them from the patchRemove local produced by http.WithBody, the Huma shell
// derives them from the parsed body via http.FindNilFields — both feed this one core.
func (handler *InstrumentHandler) updateInstrument(ctx context.Context, organizationID, holderID, id uuid.UUID, payload *mmodel.UpdateInstrumentInput, fieldsToRemove []string) (*mmodel.Instrument, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_instrument")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.instrument_id", id.String()),
		attribute.Int("app.request.fields_to_remove_count", len(fieldsToRemove)),
	)

	instrument, err := handler.Service.UpdateInstrumentByID(ctx, organizationID.String(), holderID, id, payload, fieldsToRemove)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to update instrument", err)

		return nil, err
	}

	return instrument, nil
}

// UpdateInstrument is a method that updates Instrument information.
func (handler *InstrumentHandler) UpdateInstrument(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_instrument_fiber")
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

	instrument, err := handler.updateInstrument(ctx, organizationID, holderID, id, payload, fieldsToRemove)
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, instrument)
}

// deleteInstrument is the transport-agnostic core for the instrument delete.
func (handler *InstrumentHandler) deleteInstrument(ctx context.Context, organizationID, holderID, id uuid.UUID, hardDelete bool) error {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.remove_instrument_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.instrument_id", id.String()),
		attribute.Bool("app.request.hard_delete", hardDelete),
	)

	if err := handler.Service.DeleteInstrumentByID(ctx, organizationID.String(), holderID, id, hardDelete); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to delete instrument", err)

		return err
	}

	return nil
}

// DeleteInstrumentByID removes an instrument by a given id
func (handler *InstrumentHandler) DeleteInstrumentByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

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

	if err := handler.deleteInstrument(ctx, organizationID, holderID, id, hardDelete); err != nil {
		return http.WithError(c, err)
	}

	return http.NoContent(c)
}

// getAllInstruments is the transport-agnostic core for the instrument list. queries is
// the map[string]string the caller derived from its transport (Fiber c.Queries() or the
// Huma raw-query rebuild); http.ValidateParameters is the sole query binder so the two
// transports validate identically. The holder filter, when present, is parsed from the
// bound query params (mirroring the Fiber wrapper).
func (handler *InstrumentHandler) getAllInstruments(ctx context.Context, organizationID uuid.UUID, queries map[string]string, includeDeleted bool) (http.Pagination, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_instruments")
	defer span.End()

	headerParams, err := http.ValidateParameters(queries)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to validate query parameters", err)

		return http.Pagination{}, err
	}

	var holderID uuid.UUID
	if !libCommons.IsNilOrEmpty(headerParams.HolderID) {
		holderID, err = uuid.Parse(*headerParams.HolderID)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to parse holder ID", err)

			return http.Pagination{}, err
		}
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

	if !libCommons.IsNilOrEmpty(headerParams.HolderID) {
		span.SetAttributes(
			attribute.String("app.request.holder_id", holderID.String()),
		)
	}

	recordSafeQueryAttributes(span, headerParams)

	instruments, err := handler.Service.GetAllInstruments(ctx, organizationID.String(), holderID, *headerParams, includeDeleted)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get all instruments", err)

		return http.Pagination{}, err
	}

	pagination.SetItems(instruments)

	return pagination, nil
}

// GetAllInstruments retrieves instruments
func (handler *InstrumentHandler) GetAllInstruments(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	includeDeleted := http.GetBooleanParam(c, "include_deleted")

	pagination, err := handler.getAllInstruments(ctx, organizationID, c.Queries(), includeDeleted)
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, pagination)
}

// deleteRelatedParty is the transport-agnostic core for the related-party delete.
func (handler *InstrumentHandler) deleteRelatedParty(ctx context.Context, organizationID, holderID, instrumentID, relatedPartyID uuid.UUID) error {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_related_party")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.holder_id", holderID.String()),
		attribute.String("app.request.instrument_id", instrumentID.String()),
		attribute.String("app.request.related_party_id", relatedPartyID.String()),
	)

	if err := handler.Service.DeleteRelatedPartyByID(ctx, organizationID.String(), holderID, instrumentID, relatedPartyID); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to delete related party", err)

		return err
	}

	return nil
}

// DeleteRelatedParty removes a related party from an instrument
func (handler *InstrumentHandler) DeleteRelatedParty(c *fiber.Ctx) error {
	ctx := c.UserContext()

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

	if err := handler.deleteRelatedParty(ctx, organizationID, holderID, instrumentID, relatedPartyID); err != nil {
		return http.WithError(c, err)
	}

	return http.NoContent(c)
}
