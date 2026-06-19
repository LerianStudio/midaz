// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/composition"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/attribute"
)

// CompositionHandler exposes the holder-account composition route. It owns no
// domain logic: it binds the request, resolves the request scope, and delegates
// to the composition Service, which orchestrates the inherited account-create
// and instrument-create use cases.
type CompositionHandler struct {
	Service *composition.Service
}

// CreateHolderAccount opens a holder-owned account and, when instrument fields
// are present, an instrument linked to it, in a single call.
//
//	@Summary		Open a holder-owned account (with optional instrument)
//	@Description	Opens an account owned by the holder identified in the path and, when banking/regulatory/related-party fields are present, an instrument linked to the new account. The account is created first; if it commits but the instrument write fails the account remains persisted and a typed instrumentError block is returned (no rollback). The holder is always taken from the path, never the body.
//	@Tags			Composition
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id		header		string							false	"Request ID for tracing"
//	@Param			organization_id		path		string							true	"Organization ID in UUID format"
//	@Param			ledger_id			path		string							true	"Ledger ID in UUID format"
//	@Param			id					path		string							true	"Holder ID in UUID format"
//	@Param			composition			body		mmodel.CreateHolderAccountInput	true	"Composite account (and optional instrument) details"
//	@Success		201					{object}	mmodel.HolderAccountResponse	"Successfully opened holder account"
//	@Failure		400					{object}	mmodel.Error					"Invalid input, validation errors"
//	@Failure		401					{object}	mmodel.Error					"Unauthorized access"
//	@Failure		403					{object}	mmodel.Error					"Forbidden access"
//	@Failure		404					{object}	mmodel.Error					"Organization, ledger, or holder not found"
//	@Failure		409					{object}	mmodel.Error					"Conflict: account alias already in use"
//	@Failure		422					{object}	mmodel.Error					"Business validation failed (e.g. invalid account configuration)"
//	@Failure		500					{object}	mmodel.Error					"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/holders/{id}/accounts [post]
func (handler *CompositionHandler) CreateHolderAccount(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqID, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_holder_account")
	defer span.End()

	payload, ok := p.(*mmodel.CreateHolderAccountInput)
	if !ok || payload == nil {
		return http.WithError(c, pkg.ValidateInternalError(nil, constant.EntityAccount))
	}

	// Path param is :id; ParseUUIDPathParameters("holder") parses it (it is a
	// known UUID path param) and stores it in locals under the param name "id".
	holderID, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.holder_id", holderID.String()),
	)

	token := c.Get("Authorization")

	out, err := handler.Service.CreateHolderAccount(ctx, organizationID, ledgerID, holderID, payload, token)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to create holder account", err)

		logLevel := libLog.LevelError
		if pkg.IsBusinessError(err) {
			logLevel = libLog.LevelWarn
		}

		logger.Log(ctx, logLevel, "Failed to create holder account",
			libLog.String("holder_id", holderID.String()),
			libLog.Err(err),
		)

		return http.WithError(c, err)
	}

	return http.Created(c, out)
}
