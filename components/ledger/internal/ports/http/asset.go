package http

import (
	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"github.com/LerianStudio/midaz/common/mpostgres"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/query"
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
//
//		@Summary        Create an Asset
//		@Description    Create an Asset with the input payload
//		@Tags           Assets
//		@Accept         json
//		@Produce        json
//	 @Param 			organization_id path string true "Organization ID"
//	 @Param 			ledger_id path string true "Ledger ID"
//		@Param          asset body mmodel.CreateAssetInput true "Asset Input"
//		@Success        200 {object} mmodel.Asset
//		@Router         /v1/organizations/{organization_id}/ledgers/{ledger_id}/assets [post]
func (handler *AssetHandler) CreateAsset(a any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_asset")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	logger.Infof("Initiating create of Asset with organization ID: %s", organizationID.String())

	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	logger.Infof("Initiating create of Asset with ledger ID: %s", ledgerID.String())

	payload := a.(*mmodel.CreateAssetInput)
	logger.Infof("Request to create a Asset with details: %#v", payload)

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return commonHTTP.WithError(c, err)
	}

	asset, err := handler.Command.CreateAsset(ctx, organizationID, ledgerID, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create Asset on command", err)

		logger.Infof("Error to created Asset: %s", err.Error())

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully created Asset")

	return commonHTTP.Created(c, asset)
}

// GetAllAssets is a method that retrieves all Assets.
//
//		@Summary        Get all Assets
//		@Description    Get all Assets with the input metadata or without metadata
//		@Tags           Assets
//		@Produce        json
//	 @Param 			organization_id path string true "Organization ID"
//	 @Param 			ledger_id path string true "Ledger ID"
//		@Param          metadata query string false "Metadata"
//		@Success        200 {object} mpostgres.Pagination{items=[]mmodel.Asset}
//		@Router         /v1/organizations/{organization_id}/ledgers/{ledger_id}/assets [get]
func (handler *AssetHandler) GetAllAssets(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_assets")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	logger.Infof("Initiating create of Asset with organization ID: %s", organizationID.String())

	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	logger.Infof("Initiating create of Asset with ledger ID: %s", ledgerID.String())

	headerParams := commonHTTP.ValidateParameters(c.Queries())

	pagination := mpostgres.Pagination{
		Limit: headerParams.Limit,
		Page:  headerParams.Page,
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Assets by metadata")

		assets, err := handler.Query.GetAllMetadataAssets(ctx, organizationID, ledgerID, *headerParams)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to retrieve all Assets on query", err)

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
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve all Assets on query", err)

		logger.Errorf("Failed to retrieve all Assets, Error: %s", err.Error())

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Assets")

	pagination.SetItems(assets)

	return commonHTTP.OK(c, pagination)
}

// GetAssetByID is a method that retrieves Asset information by a given id.
//
//		@Summary        Get an Asset by ID
//		@Description    Get an Asset with the input ID
//		@Tags           Assets
//		@Produce        json
//	 @Param 			organization_id path string true "Organization ID"
//	 @Param 			ledger_id path string true "Ledger ID"
//		@Param          id path string true "Asset ID"
//		@Success        200 {object} mmodel.Asset
//		@Router         /v1/organizations/{organization_id}/ledgers/{ledger_id}/assets/{id} [get]
func (handler *AssetHandler) GetAssetByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_asset_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating retrieval of Asset with Ledger ID: %s and Asset ID: %s", ledgerID.String(), id.String())

	asset, err := handler.Query.GetAssetByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve Asset on query", err)

		logger.Errorf("Failed to retrieve Asset with Ledger ID: %s and Asset ID: %s, Error: %s", ledgerID.String(), id.String(), err.Error())

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Asset with Ledger ID: %s and Asset ID: %s", ledgerID.String(), id.String())

	return commonHTTP.OK(c, asset)
}

// UpdateAsset is a method that updates Asset information.
//
//		@Summary        Update an Asset
//		@Description    Update an Asset with the input payload
//		@Tags           Assets
//		@Accept         json
//		@Produce        json
//	 @Param 			organization_id path string true "Organization ID"
//	 @Param 			ledger_id path string true "Ledger ID"
//		@Param          id path string true "Asset ID"
//		@Param          asset body mmodel.UpdateAssetInput true "Asset Input"
//		@Success        200 {object} mmodel.Asset
//		@Router         /v1/organizations/{organization_id}/ledgers/{ledger_id}/assets/{id} [patch]
func (handler *AssetHandler) UpdateAsset(a any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_asset")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating update of Asset with Ledger ID: %s and Asset ID: %s", ledgerID.String(), id.String())

	payload := a.(*mmodel.UpdateAssetInput)
	logger.Infof("Request to update an Asset with details: %#v", payload)

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return commonHTTP.WithError(c, err)
	}

	_, err = handler.Command.UpdateAssetByID(ctx, organizationID, ledgerID, id, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update Asset on command", err)

		logger.Errorf("Failed to update Asset with ID: %s, Error: %s", id.String(), err.Error())

		return commonHTTP.WithError(c, err)
	}

	asset, err := handler.Query.GetAssetByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get update Asset on query", err)

		logger.Errorf("Failed to get update Asset with ID: %s, Error: %s", id.String(), err.Error())

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully updated Asset with Ledger ID: %s and Asset ID: %s", ledgerID.String(), id.String())

	return commonHTTP.OK(c, asset)
}

// DeleteAssetByID is a method that removes Asset information by a given ids.
//
//		@Summary        Delete an Asset by ID
//		@Description    Delete an Asset with the input ID
//		@Tags           Assets
//	 @Param 			organization_id path string true "Organization ID"
//	 @Param 			ledger_id path string true "Ledger ID"
//		@Param          id path string true "Asset ID"
//		@Success        204
//		@Router         /v1/organizations/{organization_id}/ledgers/{ledger_id}/assets/{id} [delete]
func (handler *AssetHandler) DeleteAssetByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_asset_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating removal of Asset with Ledger ID: %s and Asset ID: %s", ledgerID.String(), id.String())

	if err := handler.Command.DeleteAssetByID(ctx, organizationID, ledgerID, id); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to remove Asset on command", err)

		logger.Errorf("Failed to remove Asset with Ledger ID: %s and Asset ID: %s, Error: %s", ledgerID.String(), id.String(), err.Error())

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully removed Asset with Ledger ID: %s and Asset ID: %s", ledgerID.String(), id.String())

	return commonHTTP.NoContent(c)
}
