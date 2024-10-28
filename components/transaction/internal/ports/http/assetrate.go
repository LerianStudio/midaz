package http

import (
	"github.com/LerianStudio/midaz/common/mlog"
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
	logger := mlog.NewLoggerFromContext(c.UserContext())

	organizationID := c.Locals("organization_id").(uuid.UUID)
	logger.Infof("Initiating create of AssetRate with organization ID: %s", organizationID.String())

	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	logger.Infof("Initiating create of AssetRate with ledger ID: %s", ledgerID.String())

	payload := p.(*ar.CreateAssetRateInput)
	logger.Infof("Request to create an AssetRate with details: %#v", payload)

	assetRate, err := handler.Command.CreateAssetRate(c.UserContext(), organizationID, ledgerID, payload)
	if err != nil {
		logger.Infof("Error to created Asset: %s", err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully created AssetRate")

	return commonHTTP.Created(c, assetRate)
}

// GetAssetRate retrieves an asset rate.
func (handler *AssetRateHandler) GetAssetRate(c *fiber.Ctx) error {
	logger := mlog.NewLoggerFromContext(c.UserContext())

	organizationID := c.Locals("organization_id").(uuid.UUID)
	logger.Infof("Initiating get of AssetRate with organization ID: %s", organizationID.String())

	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	logger.Infof("Initiating get of AssetRate with ledger ID: %s", ledgerID.String())

	assetRateID := c.Locals("asset_rate_id").(uuid.UUID)
	logger.Infof("Initiating get of AssetRate with asset rate ID: %s", assetRateID.String())

	assetRate, err := handler.Query.GetAssetRateByID(c.UserContext(), organizationID, ledgerID, assetRateID)
	if err != nil {
		logger.Infof("Error to get AssetRate: %s", err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully get AssetRate")

	return commonHTTP.OK(c, assetRate)
}
