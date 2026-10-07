// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/composition"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// CompositionIdempotency is the CRM idempotency slot the composition claims when
// the client sends X-Idempotency: claim or replay, store the answered response,
// and release a slot whose account was not created.
type CompositionIdempotency interface {
	CreateOrCheckCRMIdempotency(ctx context.Context, organizationID, internalKey, token string, ttl time.Duration) (*services.CRMIdempotencyResult, error)
	SetCRMIdempotencyValue(ctx context.Context, organizationID, internalKey, valueJSON string, ttl time.Duration)
	ReleaseCRMIdempotency(ctx context.Context, internalKey string)
}

// CompositionHandler exposes the holder-account composition route. It owns no
// domain logic: it binds the request, resolves the request scope, and delegates
// to the composition Service, which orchestrates the inherited account-create
// and instrument-create use cases. A nil Idempotency disables the slot.
type CompositionHandler struct {
	Service     *composition.Service
	Idempotency CompositionIdempotency
}

// createHolderAccount is the core for the holder-account composition. It owns the
// handler span (attributes + business/error-class recording + level-split logging)
// and the Service call, taking already-parsed UUIDs and an already-decoded and
// validated payload. Every canonical Midaz error it returns is rendered by the
// caller (http.HumaProblem). A partial failure (account committed, instrument
// failed) is returned as a nil-error 201 body by the Service, so it rides the
// success return here unchanged.
//
// Only a non-empty clientKey claims an idempotency slot. The answered 201,
// complete or partial, is stored for replay because the account was persisted;
// a failed account create releases the slot. replayed reports a cached answer.
func (handler *CompositionHandler) createHolderAccount(ctx context.Context, organizationID, ledgerID, holderID uuid.UUID, payload *mmodel.CreateHolderAccountInput, token, clientKey string, ttl time.Duration) (*mmodel.HolderAccountResponse, bool, error) {
	logger, tracer, reqID, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_holder_account")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.holder_id", holderID.String()),
	)

	if clientKey == "" || handler.Idempotency == nil {
		out, err := handler.callCreateHolderAccount(ctx, span, logger, organizationID, ledgerID, holderID, payload, token)

		return out, false, err
	}

	internalKey := services.CompositionIdempotencyKey(organizationID.String(), ledgerID.String(), holderID.String(), clientKey)

	result, err := handler.Idempotency.CreateOrCheckCRMIdempotency(ctx, organizationID.String(), internalKey, clientKey, ttl)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to claim holder account idempotency", err)

		return nil, false, err
	}

	if result.Replay != nil {
		replay := &mmodel.HolderAccountResponse{}
		if err := json.Unmarshal([]byte(*result.Replay), replay); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to deserialize replayed holder account", err)

			return nil, false, err
		}

		return replay, true, nil
	}

	out, err := handler.callCreateHolderAccount(ctx, span, logger, organizationID, ledgerID, holderID, payload, token)
	if err != nil {
		handler.Idempotency.ReleaseCRMIdempotency(ctx, internalKey)

		return nil, false, err
	}

	if value, err := libCommons.StructToJSONString(out); err == nil {
		handler.Idempotency.SetCRMIdempotencyValue(ctx, organizationID.String(), internalKey, value, ttl)
	} else {
		logger.Log(ctx, libLog.LevelWarn, "Holder account created but idempotency replay value could not be stored; a retry with the same key will conflict", libLog.Err(err))
	}

	return out, false, nil
}

// callCreateHolderAccount runs the composition and records a failure on span at
// the level of its error class.
func (handler *CompositionHandler) callCreateHolderAccount(ctx context.Context, span trace.Span, logger libLog.Logger, organizationID, ledgerID, holderID uuid.UUID, payload *mmodel.CreateHolderAccountInput, token string) (*mmodel.HolderAccountResponse, error) {
	out, err := handler.Service.CreateHolderAccount(ctx, organizationID, ledgerID, holderID, payload, token)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to create holder account", err)

		logLevel := libLog.LevelError
		if pkg.IsBusinessError(err) {
			logLevel = libLog.LevelWarn
		}

		logger.Log(
			ctx, logLevel, "Failed to create holder account",
			libLog.String("holder_id", holderID.String()),
			libLog.Err(err),
		)

		return nil, err
	}

	return out, nil
}
