// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/bson"
	// AssetRateHandler struct contains a cqrs use case for managing asset rate.
)

type AssetRateHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateOrUpdateAssetRate creates or updates an asset rate.
//
//	@Summary		Create or Update an AssetRate
//	@Description	Create or Update an AssetRate with the input details
//	@Tags			Asset Rates
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string							false	"Bearer token authentication. Format: Bearer {access_token}. Only required when auth plugin is enabled."
//	@Param			X-Request-Id	header		string							false	"Request ID"
//	@Param			organization_id	path		string							true	"Organization ID"
//	@Param			ledger_id		path		string							true	"Ledger ID"
//	@Param			asset-rate		body		assetrate.CreateAssetRateInput	true	"AssetRate Input"
//	@Success		200				{object}	assetrate.AssetRate
//	@Failure		400				{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Ledger or organization not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/asset-rates [Put]
func (handler *AssetRateHandler) CreateOrUpdateAssetRate(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_asset_rate")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	payload := p.(*assetrate.CreateAssetRateInput)
	logSafePayload(ctx, logger, "Request to create an AssetRate", payload)

	recordSafePayloadAttributes(span, payload)

	assetRate, err := handler.Command.CreateOrUpdateAssetRate(ctx, organizationID, ledgerID, payload)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to create AssetRate on command", err)

		return http.WithError(c, err)
	}

	return http.Created(c, assetRate)
}

// GetAssetRateByExternalID retrieves an asset rate.
//
//	@Summary		Get an AssetRate by External ID
//	@Description	Get an AssetRate by External ID with the input details
//	@Tags			Asset Rates
//	@Produce		json
//	@Param			Authorization	header		string	false	"Bearer token authentication. Format: Bearer {access_token}. Only required when auth plugin is enabled."
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			external_id		path		string	true	"External ID"
//	@Success		200				{object}	assetrate.AssetRate
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Asset rate not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/asset-rates/{external_id} [get]
func (handler *AssetRateHandler) GetAssetRateByExternalID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_asset_rate_by_external_id")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	externalID, err := http.GetUUIDFromLocals(c, "external_id")
	if err != nil {
		return http.WithError(c, err)
	}

	assetRate, err := handler.Query.GetAssetRateByExternalID(ctx, organizationID, ledgerID, externalID)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to get AssetRate on query", err)

		return http.WithError(c, err)
	}

	return http.OK(c, assetRate)
}

// GetAllAssetRatesByAssetCode retrieves an asset rate.
//
//	@Summary		Get an AssetRate by the Asset Code
//	@Description	Get an AssetRate by the Asset Code with the input details
//	@Tags			Asset Rates
//	@Produce		json
//	@Param			Authorization	header		string		false	"Bearer token authentication. Format: Bearer {access_token}. Only required when auth plugin is enabled."
//	@Param			X-Request-Id	header		string		false	"Request ID"
//	@Param			organization_id	path		string		true	"Organization ID"
//	@Param			ledger_id		path		string		true	"Ledger ID"
//	@Param			asset_code		path		string		true	"From Asset Code"
//
//	@Param			to				query		[]string	false	"To Asset Codes"	example	"BRL,USD,SGD"
//	@Param			limit			query		int			false	"Limit"				default(10)
//	@Param			start_date		query		string		false	"Start Date"		example	"2021-01-01"
//	@Param			end_date		query		string		false	"End Date"			example	"2021-01-01"
//	@Param			sort_order		query		string		false	"Sort Order"		Enums(asc,desc)
//	@Param			cursor			query		string		false	"Cursor"
//	@Success		200				{object}	http.Pagination{items=[]assetrate.AssetRate}
//	@Failure		400				{object}	mmodel.Error	"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Asset code not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/asset-rates/from/{asset_code} [get]
func (handler *AssetRateHandler) GetAllAssetRatesByAssetCode(c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_asset_rate_by_asset_code")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	assetCode := c.Params("asset_code")

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		return http.WithError(c, err)
	}

	recordSafeQueryAttributes(span, headerParams)

	pagination := http.Pagination{
		Limit:     headerParams.Limit,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	headerParams.Metadata = &bson.M{}

	assetRates, cur, err := handler.Query.GetAllAssetRatesByAssetCode(ctx, organizationID, ledgerID, assetCode, *headerParams)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to get AssetRate on query", err)

		return http.WithError(c, err)
	}

	pagination.SetItems(assetRates)
	pagination.SetCursor(cur.Next, cur.Prev)

	return http.OK(c, pagination)
}
