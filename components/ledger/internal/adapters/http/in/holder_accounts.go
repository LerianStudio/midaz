// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// HolderAccountsReader lists the accounts owned by a holder. Ownership is
// org-global (the holder collection is per-organization, not per-ledger), so
// the contract is scoped by organization ID only. The implementation is an
// adapter over the ledger account-query use case, wired at the composition
// root; defining the port here keeps the CRM HTTP layer free of any import of
// ledger internals.
type HolderAccountsReader interface {
	ListAccountsByHolder(ctx context.Context, organizationID string, holderID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Account, error)
}

// HolderAccountsHandler serves the holder-scoped account listing route. It is a
// dedicated handler so the ledger reader dependency stays isolated from the
// Mongo-backed HolderHandler.
type HolderAccountsHandler struct {
	Reader HolderAccountsReader
}

// GetAccountsByHolder lists the accounts owned by a holder.
//
//	@Summary		List Accounts by Holder
//	@Description	Lists the accounts owned by a holder, identified by the holder's ownership link.
//	@Tags			Holders
//	@Produce		json
//	@Param			Authorization		header		string	false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			organization_id		path		string	true	"The unique identifier of the Organization."
//	@Param			id					path		string	true	"The unique identifier of the Holder."
//	@Param			limit				query		int		false	"Limit"			default(10)
//	@Param			page				query		int		false	"Page"			default(1)
//	@Param			sort_order			query		string	false	"Sort Order"	Enums(asc,desc)
//	@Success		200					{object}	http.Pagination{items=[]mmodel.Account}
//	@Failure		400					{object}	mmodel.Error
//	@Failure		404					{object}	mmodel.Error
//	@Failure		500					{object}	mmodel.Error
//	@Router			/v1/organizations/{organization_id}/holders/{id}/accounts [get]
func (handler *HolderAccountsHandler) GetAccountsByHolder(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_accounts_by_holder")
	defer span.End()

	holderID, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to validate query parameters", err)

		logger.Log(ctx, libLog.LevelWarn, "Failed to validate query parameters", libLog.Err(err))

		return http.WithError(c, err)
	}

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	holderIDStr := holderID.String()
	headerParams.HolderID = &holderIDStr

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.holder_id", holderIDStr),
	)

	recordSafeQueryAttributes(span, headerParams)

	accounts, err := handler.Reader.ListAccountsByHolder(ctx, organizationID.String(), holderID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to list accounts by holder", err)

		logger.Log(ctx, libLog.LevelError, "Failed to list accounts by holder", libLog.Err(err))

		return http.WithError(c, err)
	}

	pagination := http.Pagination{
		Limit:     headerParams.Limit,
		Page:      headerParams.Page,
		SortOrder: headerParams.SortOrder,
	}
	pagination.SetItems(accounts)

	return http.OK(c, pagination)
}
