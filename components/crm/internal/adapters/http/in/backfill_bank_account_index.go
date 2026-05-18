// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/attribute"
)

// BackfillBankAccountIndex repairs the tenant-wide alias bank-account resolver index.
//
//	@Summary		Backfill Alias Bank Account Resolver Index
//	@Description	Scans tenant alias collections and rebuilds alias_bank_account_index rows. Report contains counts and alias IDs only; no document, account, or bank identity values.
//	@Tags			Aliases
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string							false	"The authorization token in the 'Bearer access_token' format. Only required when auth plugin is enabled."
//	@Param			backfill		body		mmodel.BackfillBankAccountIndexInput	true	"Backfill Input"
//	@Success		200				{object}	mmodel.BankAccountIndexBackfillReport
//	@Failure		400				{object}	pkg.HTTPError
//	@Failure		500				{object}	pkg.HTTPError
//	@Router			/v1/aliases/backfill-bank-account-index [post]
func (handler *AliasHandler) BackfillBankAccountIndex(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger, tracer, reqID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.backfill_bank_account_index")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.request_id", reqID))

	payload, ok := p.(*mmodel.BackfillBankAccountIndexInput)
	if !ok || payload == nil {
		return http.WithError(c, cn.ErrInternalServer)
	}

	result, err := handler.Service.BackfillBankAccountIndex(ctx, payload.DryRun)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to backfill bank account index", err)
		logger.Log(ctx, libLog.LevelError, "Failed to backfill bank account index", libLog.Err(err))

		return http.WithError(c, err)
	}

	return http.OK(c, result)
}
