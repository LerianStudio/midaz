// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/attribute"
)

// ResolveAccount resolves an active alias by tenant-wide ledger account ID.
//
//	@Summary		Resolve Alias by Account ID
//	@Description	Resolves an active alias across the current tenant by account ID. Does not require X-Organization-Id.
//	@Tags			Aliases
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string					false	"The authorization token in the 'Bearer access_token' format. Only required when auth plugin is enabled."
//	@Param			resolver		body		mmodel.ResolveAccountInput	true	"Account Resolver Input"
//	@Success		200				{object}	mmodel.ResolveAliasResponse
//	@Failure		400				{object}	pkg.HTTPError
//	@Failure		404				{object}	pkg.HTTPError
//	@Failure		409				{object}	pkg.HTTPError
//	@Failure		500				{object}	pkg.HTTPError
//	@Router			/v1/aliases/resolve-account [post]
func (handler *AliasHandler) ResolveAccount(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, reqID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.resolve_account")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.request_id", reqID))

	payload, ok := p.(*mmodel.ResolveAccountInput)
	if !ok || payload == nil {
		return http.WithError(c, cn.ErrInternalServer)
	}

	result, err := handler.Service.ResolveAccount(ctx, payload)
	if err != nil {
		handleAliasResolverHandlerError(ctx, span, logger, "Failed to resolve account", err)

		return http.WithError(c, err)
	}

	return http.OK(c, result)
}
