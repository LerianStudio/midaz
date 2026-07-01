// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	// AssetRateHandler struct contains a cqrs use case for managing asset rate.
)

type AssetRateHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// --- Transport-agnostic cores -------------------------------------------------
//
// The createOrUpdateAssetRate/getAssetRateByExternalID/getAllAssetRatesByAssetCode
// methods below own the span, the service call and the success log. They take
// primitive args (parsed UUIDs, the raw asset-code string, the decoded payload, the
// query map) so BOTH transports feed them: the Fiber wrappers pull those from
// *fiber.Ctx (Locals + the WithBody-decoded payload + c.Queries) and the Huma
// handlers (assetrate_handler_huma.go) pull them from the request envelope. Every
// canonical Midaz error the cores return is rendered by the caller — http.WithError
// on the Fiber path, http.HumaProblem on the Huma path — so the code + HTTP status
// are identical across both transports. assetrate is MONEY-adjacent (exchange
// rates): the response is byte-for-byte identical across transports.

// createOrUpdateAssetRate owns the span + service call + success log for an
// already-decoded payload. Body decode+validation happens BEFORE this core: the
// Fiber path decodes via the WithBody decorator (passing the struct as `p`), the
// Huma path decodes via http.DecodeAndValidate(RawBody). Both feed the SAME
// validated *CreateAssetRateInput here.
func (handler *AssetRateHandler) createOrUpdateAssetRate(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *assetrate.CreateAssetRateInput) (*assetrate.AssetRate, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_asset_rate")
	defer span.End()

	logSafePayload(ctx, logger, "Request to create an AssetRate", payload)
	recordSafePayloadAttributes(span, payload)

	assetRate, err := handler.Command.CreateOrUpdateAssetRate(ctx, organizationID, ledgerID, payload)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to create AssetRate on command", err)

		return nil, err
	}

	return assetRate, nil
}

// getAssetRateByExternalID retrieves a single asset rate by its external id.
func (handler *AssetRateHandler) getAssetRateByExternalID(ctx context.Context, organizationID, ledgerID, externalID uuid.UUID) (*assetrate.AssetRate, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_asset_rate_by_external_id")
	defer span.End()

	assetRate, err := handler.Query.GetAssetRateByExternalID(ctx, organizationID, ledgerID, externalID)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to get AssetRate on query", err)

		return nil, err
	}

	return assetRate, nil
}

// getAllAssetRatesByAssetCode binds the query map imperatively (http.ValidateParameters
// — the SAME binder the Fiber path used) so a bad query yields the canonical 400,
// then returns the cursor-paginated envelope. assetCode is a free-form string path
// segment (NOT a UUID), so it is passed through verbatim.
func (handler *AssetRateHandler) getAllAssetRatesByAssetCode(ctx context.Context, organizationID, ledgerID uuid.UUID, assetCode string, queries map[string]string) (http.Pagination, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_asset_rate_by_asset_code")
	defer span.End()

	headerParams, err := http.ValidateParameters(queries)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		return http.Pagination{}, err
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

		return http.Pagination{}, err
	}

	pagination.SetItems(assetRates)
	pagination.SetCursor(cur.Next, cur.Prev)

	return pagination, nil
}

// --- Fiber wrappers (thin) ----------------------------------------------------
//
// These stay so the legacy Fiber unit/integration tests keep exercising the
// handler methods directly; each pulls the transport inputs from *fiber.Ctx
// (Locals set by ParseUUIDPathParameters, the WithBody-decoded payload as `p`) and
// delegates to the shared core. The swaggo doc-comments below are preserved
// verbatim (the migration is ADDITIVE; swaggo is unchanged) so the generated api/
// spec keeps its per-op security. NOTE: the LIVE asset-rate routes are Huma now
// (see assetrate_handler_huma.go + RegisterAssetRateRoutesToApp); these Fiber
// wrappers are not mounted by the unified server.

// CreateOrUpdateAssetRate creates or updates an asset rate.
//
//	@Summary		Create or Update an AssetRate
//	@Description	Creates or updates the conversion factor (rate) between two asset codes for a ledger.
//	@Tags			Asset Rates
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id	header		string							false	"Request ID for tracing"
//	@Param			organization_id	path		string							true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string							true	"Ledger ID in UUID format"
//	@Param			asset-rate		body		assetrate.CreateAssetRateInput	true	"AssetRate Input"
//	@Success		201				{object}	assetrate.AssetRate	"Successfully created or updated asset rate"
//	@Failure		400				{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Ledger or organization not found"
//	@Failure		409				{object}	mmodel.Error	"Duplicate asset pair"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/asset-rates [put]
func (handler *AssetRateHandler) CreateOrUpdateAssetRate(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	assetRate, err := handler.createOrUpdateAssetRate(ctx, organizationID, ledgerID, p.(*assetrate.CreateAssetRateInput))
	if err != nil {
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
//	@Security		BearerAuth
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			external_id		path		string	true	"External ID in UUID format"
//	@Success		200				{object}	assetrate.AssetRate	"Successfully retrieved asset rate"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Asset rate not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/asset-rates/{external_id} [get]
func (handler *AssetRateHandler) GetAssetRateByExternalID(c *fiber.Ctx) error {
	ctx := c.UserContext()

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

	assetRate, err := handler.getAssetRateByExternalID(ctx, organizationID, ledgerID, externalID)
	if err != nil {
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
//	@Security		BearerAuth
//	@Param			X-Request-Id	header		string		false	"Request ID for tracing"
//	@Param			organization_id	path		string		true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string		true	"Ledger ID in UUID format"
//	@Param			asset_code		path		string		true	"From Asset Code"
//
//	@Param			to				query		[]string	false	"Filter by destination asset codes"
//	@Param			limit			query		int			false	"Maximum number of items to return (max 100)"	default(10)
//	@Param			start_date		query		string		false	"Filter asset rates created on or after this date (format: YYYY-MM-DD)"
//	@Param			end_date		query		string		false	"Filter asset rates created on or before this date (format: YYYY-MM-DD)"
//	@Param			sort_order		query		string		false	"Sort order by creation date"	Enums(asc,desc)
//	@Param			cursor			query		string		false	"Opaque cursor token for pagination"
//	@Success		200				{object}	http.Pagination{items=[]assetrate.AssetRate}	"Successfully retrieved asset rates list"
//	@Failure		400				{object}	mmodel.Error	"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Asset code not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/asset-rates/from/{asset_code} [get]
func (handler *AssetRateHandler) GetAllAssetRatesByAssetCode(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	assetCode := c.Params("asset_code")

	pagination, err := handler.getAllAssetRatesByAssetCode(ctx, organizationID, ledgerID, assetCode, c.Queries())
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, pagination)
}
