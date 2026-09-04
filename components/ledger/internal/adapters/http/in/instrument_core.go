// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"time"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

type InstrumentHandler struct {
	Service *services.UseCase
}

// --- Transport-agnostic cores -------------------------------------------------
//
// The createInstrument/updateInstrument/... methods below own the span, the service
// call and — for the create — the idempotency dance. They take primitive args —
// parsed UUIDs, the decoded payload, the query map — so nothing transport-shaped
// reaches them; the handlers in instrument_handler.go pull those out of the request
// envelope. Every canonical Midaz error a core returns is rendered by its caller via
// http.HumaProblem, which fixes the code + HTTP status. Body decode+validation happens
// BEFORE these cores, in the handler, via http.DecodeAndValidate.

// createInstrument owns the span + the full idempotency dance (claim +
// replay-or-create + store) for an already-decoded payload, using an already-resolved
// client key + TTL. It returns replayed=true when the response came from a cached
// idempotency slot, so the handler can set the X-Idempotency-Replayed header.
// Instruments are namespaced by (organization, holder), matching
// services.InstrumentIdempotencyKey.
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

// getInstrumentByID owns the span + service call for the instrument read.
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

// updateInstrument owns the span + service call for the instrument update.
// fieldsToRemove carries the RFC 7396 merge-patch null-field paths, which the handler
// derives from the parsed body via http.FindNilFields.
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

// deleteInstrument owns the span + service call for the instrument delete.
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

// getAllInstruments binds the query map imperatively via http.ValidateParameters — the
// sole query binder, so a bad query yields the canonical 400 — then returns the
// assembled pagination envelope. queries carries the same last-value-wins semantics as
// Fiber's c.Queries(). The holder filter, when present, is parsed from the bound query
// params.
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

		span.SetAttributes(attribute.String("app.request.holder_id", holderID.String()))
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

	instruments, err := handler.Service.GetAllInstruments(ctx, organizationID.String(), holderID, *headerParams, includeDeleted)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get all instruments", err)

		return http.Pagination{}, err
	}

	pagination.SetItems(instruments)

	return pagination, nil
}

// deleteRelatedParty owns the span + service call for the related-party delete.
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
