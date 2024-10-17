package http

import (
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/common/mpostgres"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/query"
	s "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/asset"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

// AssetHandler struct contains a cqrs use case for managing asset in related operations.
type AssetHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateAsset is a method that creates asset information.
func (handler *AssetHandler) CreateAsset(a any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := mlog.NewLoggerFromContext(ctx)

	organizationID := c.Locals("organization_id").(uuid.UUID)
	logger.Infof("Initiating create of Asset with organization ID: %s", organizationID)

	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	logger.Infof("Initiating create of Asset with ledger ID: %s", ledgerID)

	payload := a.(*s.CreateAssetInput)
	logger.Infof("Request to create a Asset with details: %#v", payload)

	asset, err := handler.Command.CreateAsset(ctx, organizationID, ledgerID, payload)
	if err != nil {
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully created Asset")

	return commonHTTP.Created(c, asset)
}

// GetAllAssets is a method that retrieves all Assets.
func (handler *AssetHandler) GetAllAssets(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := mlog.NewLoggerFromContext(ctx)

	organizationID := c.Params("organization_id")
	logger.Infof("Initiating create of Asset with organization ID: %s", organizationID)

	ledgerID := c.Params("ledger_id")
	logger.Infof("Initiating create of Asset with ledger ID: %s", ledgerID)

	headerParams := commonHTTP.ValidateParameters(c.Queries())

	pagination := mpostgres.Pagination{
		Limit: headerParams.Limit,
		Page:  headerParams.Page,
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Assets by metadata")

		assets, err := handler.Query.GetAllMetadataAssets(ctx, organizationID, ledgerID, *headerParams)
		if err != nil {
			logger.Errorf("Failed to retrieve all Assets, Error: %s", err.Error())
			return commonHTTP.WithError(c, err)
		}

		logger.Infof("Successfully retrieved all Assets by metadata")

		pagination.SetItems(assets)

		return commonHTTP.OK(c, pagination)
	}

	logger.Infof("Initiating retrieval of all Assets ")

	headerParams.Metadata = &bson.M{}

	assets, err := handler.Query.GetAllAssets(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		logger.Errorf("Failed to retrieve all Assets, Error: %s", err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Assets")

	pagination.SetItems(assets)

	return commonHTTP.OK(c, pagination)
}

// GetAssetByID is a method that retrieves Asset information by a given id.
func (handler *AssetHandler) GetAssetByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger := mlog.NewLoggerFromContext(ctx)

	logger.Infof("Initiating retrieval of Asset with Ledger ID: %s and Asset ID: %s", ledgerID, id)

	asset, err := handler.Query.GetAssetByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		logger.Errorf("Failed to retrieve Asset with Ledger ID: %s and Asset ID: %s, Error: %s", ledgerID, id, err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Asset with Ledger ID: %s and Asset ID: %s", ledgerID, id)

	return commonHTTP.OK(c, asset)
}

// UpdateAsset is a method that updates Asset information.
func (handler *AssetHandler) UpdateAsset(a any, c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := mlog.NewLoggerFromContext(ctx)

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating update of Asset with Ledger ID: %s and Asset ID: %s", ledgerID, id)

	payload := a.(*s.UpdateAssetInput)
	logger.Infof("Request to update an Asset with details: %#v", payload)

	_, err := handler.Command.UpdateAssetByID(ctx, organizationID, ledgerID, id, payload)
	if err != nil {
		logger.Errorf("Failed to update Asset with ID: %s, Error: %s", id, err.Error())
		return commonHTTP.WithError(c, err)
	}

	asset, err := handler.Query.GetAssetByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		logger.Errorf("Failed to get update Asset with ID: %s, Error: %s", id, err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully updated Asset with Ledger ID: %s and Asset ID: %s", ledgerID, id)

	return commonHTTP.OK(c, asset)
}

// DeleteAssetByID is a method that removes Asset information by a given ids.
func (handler *AssetHandler) DeleteAssetByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := mlog.NewLoggerFromContext(ctx)

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")
	id := c.Params("id")

	logger.Infof("Initiating removal of Asset with Ledger ID: %s and Asset ID: %s", ledgerID, id)

	if err := handler.Command.DeleteAssetByID(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuid.MustParse(id)); err != nil {
		logger.Errorf("Failed to remove Asset with Ledger ID: %s and Asset ID: %s, Error: %s", ledgerID, id, err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully removed Asset with Ledger ID: %s and Asset ID: %s", ledgerID, id)

	return commonHTTP.NoContent(c)
}
