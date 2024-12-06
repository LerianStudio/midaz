package in

import (
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/LerianStudio/midaz/pkg/mpostgres"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"go.mongodb.org/mongo-driver/bson"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// AssetRateHandler struct contains a cqrs use case for managing asset rate.
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
//	@Param			Authorization	header		string							true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header		string							false	"Request ID"
//	@Param			organization_id	path		string							true	"Organization ID"
//	@Param			ledger_id		path		string							true	"Ledger ID"
//	@Param			asset-rate		body		assetrate.CreateAssetRateInput	true	"AssetRate Input"
//	@Success		200				{object}	assetrate.AssetRate
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/asset-rates [post]
func (handler *AssetRateHandler) CreateOrUpdateAssetRate(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_asset_rate")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	logger.Infof("Initiating create of AssetRate with organization ID: %s", organizationID.String())

	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	logger.Infof("Initiating create of AssetRate with ledger ID: %s", ledgerID.String())

	payload := p.(*assetrate.CreateAssetRateInput)
	logger.Infof("Request to create an AssetRate with details: %#v", payload)

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	assetRate, err := handler.Command.CreateOrUpdateAssetRate(ctx, organizationID, ledgerID, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create AssetRate on command", err)

		logger.Infof("Error to created Asset: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully created AssetRate")

	return http.Created(c, assetRate)
}

// GetAssetRateByExternalID retrieves an asset rate.
//
//	@Summary		Get an AssetRate by External ID
//	@Description	Get an AssetRate by External ID with the input details
//	@Tags			Asset Rates
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			external_id		path		string	true	"External ID"
//	@Success		200				{object}	assetrate.AssetRate
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/asset-rates/{external_id} [get]
func (handler *AssetRateHandler) GetAssetRateByExternalID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_asset_rate_by_external_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	externalID := c.Locals("external_id").(uuid.UUID)

	logger.Infof("Initiating get of AssetRate with organization ID '%s', ledger ID: '%s', and external ID: '%s'",
		organizationID.String(), ledgerID.String(), externalID.String())

	assetRate, err := handler.Query.GetAssetRateByExternalID(ctx, organizationID, ledgerID, externalID)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get AssetRate on query", err)

		logger.Infof("Error to get AssetRate: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully get AssetRate")

	return http.OK(c, assetRate)
}

// GetAllAssetRatesByAssetCode retrieves an asset rate.
//
//	@Summary		Get an AssetRate by the Asset Code
//	@Description	Get an AssetRate by the Asset Code with the input details
//	@Tags			Asset Rates
//	@Produce		json
//	@Param			Authorization	header		string		true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header		string		false	"Request ID"
//	@Param			organization_id	path		string		true	"Organization ID"
//	@Param			ledger_id		path		string		true	"Ledger ID"
//	@Param			asset_code		path		string		true	"From Asset Code"
//
//	@Param			to				query		[]string	false	"To Asset Codes"	example("BRL,USD,SGD")
//
//	@Success		200				{object}	mpostgres.Pagination{items=[]assetrate.AssetRate}
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/asset-rates/from/{asset_code} [get]
func (handler *AssetRateHandler) GetAllAssetRatesByAssetCode(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_asset_rate_by_asset_code")
	defer span.End()

	headerParams := http.ValidateParameters(c.Queries())

	pagination := mpostgres.Pagination{
		Limit: headerParams.Limit,
		Page:  headerParams.Page,
	}

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	assetCode := c.Locals("asset_code").(string)

	logger.Infof("Initiating get of AssetRate with organization ID '%s', ledger ID: '%s', and asset_code: '%s'",
		organizationID.String(), ledgerID.String(), assetCode)

	headerParams.Metadata = &bson.M{}

	assetRates, err := handler.Query.GetAllAssetRatesByAssetCode(ctx, organizationID, ledgerID, assetCode, *headerParams)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get AssetRate on query", err)

		logger.Infof("Error to get AssetRate: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully get AssetRate")

	pagination.SetItems(assetRates)

	return http.OK(c, pagination)
}
