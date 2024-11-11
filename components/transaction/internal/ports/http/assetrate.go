package http

import (
	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/transaction/internal/app/command"
	"github.com/LerianStudio/midaz/components/transaction/internal/app/query"
	ar "github.com/LerianStudio/midaz/components/transaction/internal/domain/assetrate"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// AssetRateHandler struct contains a cqrs use case for managing asset rate.
type AssetRateHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateAssetRate creates a new asset rate.
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

	payload := p.(*ar.CreateAssetRateInput)
	logger.Infof("Request to create an AssetRate with details: %#v", payload)

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return commonHTTP.WithError(c, err)
	}

	assetRate, err := handler.Command.CreateAssetRate(c.UserContext(), organizationID, ledgerID, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create AssetRate on command", err)

		logger.Infof("Error to created Asset: %s", err.Error())

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully created AssetRate")

	return commonHTTP.Created(c, assetRate)
}

// GetAssetRate retrieves an asset rate.
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

	assetRate, err := handler.Query.GetAssetRateByID(c.UserContext(), organizationID, ledgerID, assetRateID)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get AssetRate on query", err)

		logger.Infof("Error to get AssetRate: %s", err.Error())

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully get AssetRate")

	return commonHTTP.OK(c, assetRate)
}
