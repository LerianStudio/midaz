package in

import (
	"fmt"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/commons/postgres"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services/command"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services/query"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/LerianStudio/midaz/pkg/constant"
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
//	@Summary		Create a new asset
//	@Description	Creates a new asset within the specified ledger. Assets represent currencies, cryptocurrencies, commodities, or other financial instruments tracked in the ledger.
//	@Tags			Assets
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string					true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string					false	"Request ID for tracing"
//	@Param			organization_id	path		string					true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string					true	"Ledger ID in UUID format"
//	@Param			asset			body		mmodel.CreateAssetInput	true	"Asset details including name, code, type, status, and optional metadata"
//	@Success		201				{object}	mmodel.Asset			"Successfully created asset"
//	@Failure		400				{object}	mmodel.Error			"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error			"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error			"Forbidden access"
//	@Failure		404				{object}	mmodel.Error			"Organization or ledger not found"
//	@Failure		409				{object}	mmodel.Error			"Conflict: Asset with the same name or code already exists"
//	@Failure		500				{object}	mmodel.Error			"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/assets [post]
func (handler *AssetHandler) CreateAsset(a any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_asset")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	logger.Infof("Initiating create of Asset with organization ID: %s", organizationID.String())

	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	logger.Infof("Initiating create of Asset with ledger ID: %s", ledgerID.String())

	payload := a.(*mmodel.CreateAssetInput)
	logger.Infof("Request to create a Asset with details: %#v", payload)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	asset, err := handler.Command.CreateAsset(ctx, organizationID, ledgerID, payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to create Asset on command", err)

		logger.Infof("Error to created Asset: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully created Asset")

	return http.Created(c, asset)
}

// GetAllAssets is a method that retrieves all Assets.
//
//	@Summary		List all assets
//	@Description	Returns a paginated list of assets within the specified ledger, optionally filtered by metadata, date range, and other criteria
//	@Tags			Assets
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			metadata		query		string	false	"JSON string to filter assets by metadata fields"
//	@Param			limit			query		int		false	"Maximum number of records to return per page"				default(10)	minimum(1)	maximum(100)
//	@Param			page			query		int		false	"Page number for pagination"									default(1)	minimum(1)
//	@Param			start_date		query		string	false	"Filter assets created on or after this date (format: YYYY-MM-DD)"
//	@Param			end_date		query		string	false	"Filter assets created on or before this date (format: YYYY-MM-DD)"
//	@Param			sort_order		query		string	false	"Sort direction for results based on creation date"			Enums(asc,desc)
//	@Success		200				{object}	libPostgres.Pagination{items=[]mmodel.Asset,page=int,limit=int}	"Successfully retrieved assets list"
//	@Failure		400				{object}	mmodel.Error	"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Organization or ledger not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/assets [get]
func (handler *AssetHandler) GetAllAssets(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_assets")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	logger.Infof("Initiating create of Asset with organization ID: %s", organizationID.String())

	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	logger.Infof("Initiating create of Asset with ledger ID: %s", ledgerID.String())

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	pagination := libPostgres.Pagination{
		Limit:     headerParams.Limit,
		Page:      headerParams.Page,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Assets by metadata")

		assets, err := handler.Query.GetAllMetadataAssets(ctx, organizationID, ledgerID, *headerParams)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to retrieve all Assets on query", err)

			logger.Errorf("Failed to retrieve all Assets, Error: %s", err.Error())

			return http.WithError(c, err)
		}

		logger.Infof("Successfully retrieved all Assets by metadata")

		pagination.SetItems(assets)

		return http.OK(c, pagination)
	}

	logger.Infof("Initiating retrieval of all Assets ")

	headerParams.Metadata = &bson.M{}

	assets, err := handler.Query.GetAllAssets(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve all Assets on query", err)

		logger.Errorf("Failed to retrieve all Assets, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Assets")

	pagination.SetItems(assets)

	return http.OK(c, pagination)
}

// GetAssetByID is a method that retrieves Asset information by a given id.
//
//	@Summary		Retrieve a specific asset
//	@Description	Returns detailed information about an asset identified by its UUID within the specified ledger
//	@Tags			Assets
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			id				path		string	true	"Asset ID in UUID format"
//	@Success		200				{object}	mmodel.Asset	"Successfully retrieved asset"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Asset, ledger, or organization not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/assets/{id} [get]
func (handler *AssetHandler) GetAssetByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_asset_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating retrieval of Asset with Ledger ID: %s and Asset ID: %s", ledgerID.String(), id.String())

	asset, err := handler.Query.GetAssetByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve Asset on query", err)

		logger.Errorf("Failed to retrieve Asset with Ledger ID: %s and Asset ID: %s, Error: %s", ledgerID.String(), id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Asset with Ledger ID: %s and Asset ID: %s", ledgerID.String(), id.String())

	return http.OK(c, asset)
}

// UpdateAsset is a method that updates Asset information.
//
//	@Summary		Update an asset
//	@Description	Updates an existing asset's properties such as name, status, and metadata within the specified ledger
//	@Tags			Assets
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string					true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string					false	"Request ID for tracing"
//	@Param			organization_id	path		string					true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string					true	"Ledger ID in UUID format"
//	@Param			id				path		string					true	"Asset ID in UUID format"
//	@Param			asset			body		mmodel.UpdateAssetInput	true	"Asset properties to update including name, status, and optional metadata"
//	@Success		200				{object}	mmodel.Asset			"Successfully updated asset"
//	@Failure		400				{object}	mmodel.Error			"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error			"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error			"Forbidden access"
//	@Failure		404				{object}	mmodel.Error			"Asset, ledger, or organization not found"
//	@Failure		409				{object}	mmodel.Error			"Conflict: Asset with the same name already exists"
//	@Failure		500				{object}	mmodel.Error			"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/assets/{id} [patch]
func (handler *AssetHandler) UpdateAsset(a any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_asset")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating update of Asset with Ledger ID: %s and Asset ID: %s", ledgerID.String(), id.String())

	payload := a.(*mmodel.UpdateAssetInput)
	logger.Infof("Request to update an Asset with details: %#v", payload)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	_, err = handler.Command.UpdateAssetByID(ctx, organizationID, ledgerID, id, payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update Asset on command", err)

		logger.Errorf("Failed to update Asset with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	asset, err := handler.Query.GetAssetByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get update Asset on query", err)

		logger.Errorf("Failed to get update Asset with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully updated Asset with Ledger ID: %s and Asset ID: %s", ledgerID.String(), id.String())

	return http.OK(c, asset)
}

// DeleteAssetByID is a method that removes Asset information by a given ids.
//
//	@Summary		Delete an asset
//	@Description	Permanently removes an asset from the specified ledger. This operation cannot be undone.
//	@Tags			Assets
//	@Param			Authorization	header	string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header	string	false	"Request ID for tracing"
//	@Param			organization_id	path	string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path	string	true	"Ledger ID in UUID format"
//	@Param			id				path	string	true	"Asset ID in UUID format"
//	@Success		204				{object}	nil	"Asset successfully deleted"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Asset, ledger, or organization not found"
//	@Failure		409				{object}	mmodel.Error	"Conflict: Asset cannot be deleted due to existing dependencies"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/assets/{id} [delete]
func (handler *AssetHandler) DeleteAssetByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_asset_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating removal of Asset with Ledger ID: %s and Asset ID: %s", ledgerID.String(), id.String())

	if err := handler.Command.DeleteAssetByID(ctx, organizationID, ledgerID, id); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to remove Asset on command", err)

		logger.Errorf("Failed to remove Asset with Ledger ID: %s and Asset ID: %s, Error: %s", ledgerID.String(), id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully removed Asset with Ledger ID: %s and Asset ID: %s", ledgerID.String(), id.String())

	return http.NoContent(c)
}

// CountAssets is a method that returns the total count of assets for a specific ledger in an organization.
//
//	@Summary		Count total assets
//	@Description	Returns the total count of assets for a specific ledger in an organization as a header without a response body
//	@Tags			Assets
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Success		204				{string}	string	"No content with X-Total-Count header containing the count"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Organization or ledger not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/assets/metrics/count [head]
func (handler *AssetHandler) CountAssets(c *fiber.Ctx) error {
	ctx := c.UserContext()

	tracer := libCommons.NewTracerFromContext(ctx)
	logger := libCommons.NewLoggerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.count_assets")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	logger.Infof("Initiating count of all assets for organization: %s, ledger: %s", organizationID, ledgerID)

	count, err := handler.Query.CountAssets(ctx, organizationID, ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to count assets", err)
		logger.Errorf("Failed to count assets, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully counted assets for organization %s, ledger %s: %d", organizationID, ledgerID, count)

	c.Set(constant.XTotalCount, fmt.Sprintf("%d", count))
	c.Set(constant.ContentLength, "0")

	return http.NoContent(c)
}
