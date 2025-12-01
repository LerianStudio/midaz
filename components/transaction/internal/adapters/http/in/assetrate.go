package in

import (
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

// AssetRateHandler struct contains a cqrs use case for managing asset rate.
type AssetRateHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateOrUpdateAssetRate creates or updates an asset rate.
//
//	@ID				createOrUpdateAssetRate
//	@Summary		Create or update an asset rate
//	@Description	Creates a new asset rate or updates an existing one for currency conversion. Asset rates define exchange rates between assets within a ledger for multi-currency transactions.
//	@Tags			Asset Rates
//	@Security		BearerAuth
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string							true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string							false	"Request ID for tracing"
//	@Param			organization_id	path		string							true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string							true	"Ledger ID in UUID format"
//	@Param			asset-rate		body		assetrate.CreateAssetRateInput	true	"Asset rate details including from/to assets, rate, scale, and TTL"
//	@Success		200				{object}	assetrate.AssetRate				"Successfully created or updated asset rate"
//	@Example		response	{"id":"ar123456-89ab-cdef-0123-456789abcdef","organizationId":"a1b2c3d4-e5f6-7890-abcd-1234567890ab","ledgerId":"b2c3d4e5-f6a1-7890-bcde-2345678901cd","externalId":"USDBRL-2024","from":"USD","to":"BRL","rate":"5.25","scale":2,"source":"Central Bank","ttl":3600,"createdAt":"2024-01-15T09:30:00Z","updatedAt":"2024-01-15T09:30:00Z"}
//	@Failure		400				{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Organization or ledger not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/asset-rates [put]
func (handler *AssetRateHandler) CreateOrUpdateAssetRate(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_asset_rate")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	logger.Infof("Initiating create of AssetRate with organization ID: %s", organizationID.String())
	logger.Infof("Initiating create of AssetRate with ledger ID: %s", ledgerID.String())

	payload := p.(*assetrate.CreateAssetRateInput)
	logger.Infof("Request to create an AssetRate with details: %#v", payload)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)
	}

	assetRate, err := handler.Command.CreateOrUpdateAssetRate(ctx, organizationID, ledgerID, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create AssetRate on command", err)

		logger.Infof("Error to created Asset: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully created AssetRate")

	return http.Created(c, assetRate)
}

// GetAssetRateByExternalID retrieves an asset rate.
//
//	@ID				getAssetRateByExternalID
//	@Summary		Retrieve an asset rate by external ID
//	@Description	Returns detailed information about an asset rate identified by its external ID within the specified ledger
//	@Tags			Asset Rates
//	@Security		BearerAuth
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			external_id		path		string	true	"External ID in UUID format"
//	@Success		200				{object}	assetrate.AssetRate	"Successfully retrieved asset rate"
//	@Example		response	{"id":"ar123456-89ab-cdef-0123-456789abcdef","organizationId":"a1b2c3d4-e5f6-7890-abcd-1234567890ab","ledgerId":"b2c3d4e5-f6a1-7890-bcde-2345678901cd","externalId":"USDBRL-2024","from":"USD","to":"BRL","rate":"5.25","scale":2,"source":"Central Bank","ttl":3600,"createdAt":"2024-01-15T09:30:00Z","updatedAt":"2024-01-15T09:30:00Z"}
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Asset rate not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/asset-rates/{external_id} [get]
func (handler *AssetRateHandler) GetAssetRateByExternalID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_asset_rate_by_external_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	externalID := c.Locals("external_id").(uuid.UUID)

	logger.Infof("Initiating get of AssetRate with organization ID '%s', ledger ID: '%s', and external ID: '%s'",
		organizationID.String(), ledgerID.String(), externalID.String())

	assetRate, err := handler.Query.GetAssetRateByExternalID(ctx, organizationID, ledgerID, externalID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get AssetRate on query", err)

		logger.Infof("Error to get AssetRate: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully get AssetRate")

	return http.OK(c, assetRate)
}

// GetAllAssetRatesByAssetCode retrieves an asset rate.
//
//	@ID				listAssetRatesByAssetCode
//	@Summary		List asset rates by source asset code
//	@Description	Retrieves all asset rates for a specific source asset code within the ledger. Supports filtering by target asset codes, date range, cursor-based pagination, and sorting.
//	@Tags			Asset Rates
//	@Security		BearerAuth
//	@Produce		json
//	@Param			Authorization	header		string		true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string		false	"Request ID for tracing"
//	@Param			organization_id	path		string		true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string		true	"Ledger ID in UUID format"
//	@Param			asset_code		path		string		true	"Source asset code (e.g., USD, BRL)"
//	@Param			to				query		[]string	false	"Filter by target asset codes"
//	@Param			limit			query		int			false	"Maximum number of records to return per page"				default(10)	minimum(1)	maximum(100)
//	@Param			start_date		query		string		false	"Filter records created on or after this date (format: YYYY-MM-DD)"
//	@Param			end_date		query		string		false	"Filter records created on or before this date (format: YYYY-MM-DD)"
//	@Param			sort_order		query		string		false	"Sort direction for results based on creation date"			Enums(asc,desc)
//	@Param			cursor			query		string		false	"Cursor for pagination to fetch the next set of results"
//	@Success		200				{object}	libPostgres.Pagination{items=[]assetrate.AssetRate,next_cursor=string,prev_cursor=string,limit=int}	"Successfully retrieved asset rates"
//	@Example		response	{"items":[{"id":"ar123456-89ab-cdef-0123-456789abcdef","organizationId":"a1b2c3d4-e5f6-7890-abcd-1234567890ab","ledgerId":"b2c3d4e5-f6a1-7890-bcde-2345678901cd","externalId":"USDBRL-2024","from":"USD","to":"BRL","rate":"5.25","scale":2,"source":"Central Bank","ttl":3600,"createdAt":"2024-01-15T09:30:00Z","updatedAt":"2024-01-15T09:30:00Z"}],"limit":10,"nextCursor":"eyJpZCI6ImFyMTIzNDU2In0="}
//	@Failure		400				{object}	mmodel.Error	"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Asset code not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/asset-rates/from/{asset_code} [get]
func (handler *AssetRateHandler) GetAllAssetRatesByAssetCode(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_asset_rate_by_asset_code")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	assetCode := c.Params("asset_code")

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	err = libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.query_params", headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert query parameters to JSON string", err)
	}

	pagination := libPostgres.Pagination{
		Limit:     headerParams.Limit,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	logger.Infof("Initiating get of AssetRate with organization ID '%s', ledger ID: '%s', and asset_code: '%s'",
		organizationID.String(), ledgerID.String(), assetCode)

	headerParams.Metadata = &bson.M{}

	assetRates, cur, err := handler.Query.GetAllAssetRatesByAssetCode(ctx, organizationID, ledgerID, assetCode, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get AssetRate on query", err)

		logger.Infof("Error to get AssetRate: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully get AssetRate")

	pagination.SetItems(assetRates)
	pagination.SetCursor(cur.Next, cur.Prev)

	return http.OK(c, pagination)
}
