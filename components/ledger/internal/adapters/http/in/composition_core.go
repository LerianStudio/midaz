// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/composition"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// CompositionHandler exposes the holder-account composition route. It owns no
// domain logic: it binds the request, resolves the request scope, and delegates
// to the composition Service, which orchestrates the inherited account-create
// and instrument-create use cases.
type CompositionHandler struct {
	Service *composition.Service
}

// createHolderAccount is the core for the holder-account composition. It owns the
// handler span (attributes + business/error-class recording + level-split logging)
// and the Service call, taking already-parsed UUIDs and an already-decoded and
// validated payload. Every canonical Midaz error it returns is rendered by the
// caller (http.HumaProblem). A partial failure (account committed, instrument
// failed) is returned as a nil-error 201 body by the Service, so it rides the
// success return here unchanged.
func (handler *CompositionHandler) createHolderAccount(ctx context.Context, organizationID, ledgerID, holderID uuid.UUID, payload *mmodel.CreateHolderAccountInput, token string) (*mmodel.HolderAccountResponse, error) {
	logger, tracer, reqID, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_holder_account")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.holder_id", holderID.String()),
	)

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
