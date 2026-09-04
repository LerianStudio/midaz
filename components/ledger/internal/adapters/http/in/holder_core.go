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

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

type HolderHandler struct {
	Service *services.UseCase
}

// --- Transport-agnostic cores -------------------------------------------------
//
// The createHolder/getHolderByID/... methods below own the span, the service call
// and — for the create — the idempotency dance. They take primitive args — parsed
// UUIDs, the decoded payload, the query map — so nothing transport-shaped reaches
// them; the handlers in holder_handler.go pull those out of the request envelope.
// Every canonical Midaz error a core returns is rendered by its caller via
// http.HumaProblem, which fixes the code + HTTP status. Body decode+validation
// happens BEFORE these cores, in the handler, via http.DecodeAndValidate.

// createHolder owns the span + the full idempotency dance (claim + replay-or-create
// + store) for an already-decoded payload, using an already-resolved client key +
// TTL. It returns replayed=true when the response came from a cached idempotency
// slot, so the handler can set the X-Idempotency-Replayed header.
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

// getHolderByID reads a single holder by ID.
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

// updateHolder applies an already-decoded merge patch. fieldsToRemove carries the
// RFC 7396 null-field paths the handler derived from the raw body via
// http.FindNilFields.
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

// deleteHolder removes a holder, soft by default and physically when hardDelete.
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

// getAllHolders lists holders for an organization. queries is the raw query the
// handler rebuilt as a map; http.ValidateParameters is the sole query binder.
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
