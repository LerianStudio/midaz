package in

import (
	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/query"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// AssetRateHandler struct contains a cqrs use case for managing asset rate.
type AssetRateHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateAssetRate creates a new asset rate.
//
//	@Summary		Create an AssetRate
//	@Description	Create an AssetRate with the input payload
//	@Tags			Asset Rates
//	@Accept			json
//	@Produce		json
//
//	@Param			organization_id	path		string							true	"Organization ID"
//	@Param			ledger_id		path		string							true	"Ledger ID"
//
//	@Param			asset-rate		body		assetrate.CreateAssetRateInput	true	"AssetRate Input"
//	@Param			Midaz-Id		header		string							false	"Request ID"
//	@Success		200				{object}	assetrate.AssetRate
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/asset-rates [post]
func (handler *AssetRateHandler) CreateAssetRate(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

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

	assetRate, err := handler.Command.CreateAssetRate(ctx, organizationID, ledgerID, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create AssetRate on command", err)

		logger.Infof("Error to created Asset: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully created AssetRate")

	return http.Created(c, assetRate)
}

// GetAssetRate retrieves an asset rate.
//
//	@Summary		Get an AssetRate by ID
//	@Description	Get an AssetRate with the input ID
//	@Tags			Asset Rates
//	@Produce		json
//
//	@Param			organization_id	path		string							true	"Organization ID"
//	@Param			ledger_id		path		string							true	"Ledger ID"
//	@Param			asset_rate_id	path		string							true	"AssetRate ID"
//
//	@Param			asset-rate		body		assetrate.CreateAssetRateInput	true	"AssetRate Input"
//	@Param			Midaz-Id		header		string							false	"Request ID"
//	@Success		200				{object}	assetrate.AssetRate
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/asset-rates/{asset_rate_id} [get]
func (handler *AssetRateHandler) GetAssetRate(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_asset_rate")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	logger.Infof("Initiating get of AssetRate with organization ID: %s", organizationID.String())

	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	logger.Infof("Initiating get of AssetRate with ledger ID: %s", ledgerID.String())

	assetRateID := c.Locals("asset_rate_id").(uuid.UUID)
	logger.Infof("Initiating get of AssetRate with asset rate ID: %s", assetRateID.String())

	assetRate, err := handler.Query.GetAssetRateByID(ctx, organizationID, ledgerID, assetRateID)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get AssetRate on query", err)

		logger.Infof("Error to get AssetRate: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully get AssetRate")

	return http.OK(c, assetRate)
}
