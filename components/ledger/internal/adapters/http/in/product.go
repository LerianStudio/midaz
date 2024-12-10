package in

import (
	"go.mongodb.org/mongo-driver/bson"

	"github.com/LerianStudio/midaz/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/LerianStudio/midaz/pkg/mpostgres"
	"github.com/LerianStudio/midaz/pkg/net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ProductHandler struct contains a product use case for managing product related operations.
type ProductHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateProduct is a method that creates product information.
//
//	@Summary		Create a Product
//	@Description	Create a Product with the input payload
//	@Tags			Products
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string						true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header		string						false	"Request ID"
//	@Param			organization_id	path		string						true	"Organization ID"
//	@Param			ledger_id		path		string						true	"Ledger ID"
//	@Param			product			body		mmodel.CreateProductInput	true	"Product"
//	@Success		200				{object}	mmodel.Product
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/products [post]
func (handler *ProductHandler) CreateProduct(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_product")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	logger.Infof("Initiating create of Product with organization ID: %s and ledger ID: %s", organizationID.String(), ledgerID.String())

	payload := i.(*mmodel.CreateProductInput)
	logger.Infof("Request to create a Product with details: %#v", payload)

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	product, err := handler.Command.CreateProduct(ctx, organizationID, ledgerID, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create Product on command", err)

		return http.WithError(c, err)
	}

	logger.Infof("Successfully created Product")

	return http.Created(c, product)
}

// GetAllProducts is a method that retrieves all Products.
//
//	@Summary		Get all Products
//	@Description	Get all Products with the input metadata or without metadata
//	@Tags			Products
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			metadata		query		string	false	"Metadata"
//	@Success		200				{object}	mpostgres.Pagination{items=[]mmodel.Product}
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/products [get]
func (handler *ProductHandler) GetAllProducts(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_products")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	logger.Infof("Get Products with organization ID: %s and ledger ID: %s", organizationID.String(), ledgerID.String())

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	pagination := mpostgres.Pagination{
		Limit:     headerParams.Limit,
		Page:      headerParams.Page,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Products by metadata")

		products, err := handler.Query.GetAllMetadataProducts(ctx, organizationID, ledgerID, *headerParams)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to retrieve all Products on query", err)

			logger.Errorf("Failed to retrieve all Products, Error: %s", err.Error())

			return http.WithError(c, err)
		}

		logger.Infof("Successfully retrieved all Products by metadata")

		pagination.SetItems(products)

		return http.OK(c, pagination)
	}

	logger.Infof("Initiating retrieval of all Products ")

	headerParams.Metadata = &bson.M{}

	products, err := handler.Query.GetAllProducts(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve all Products on query", err)

		logger.Errorf("Failed to retrieve all Products, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Products")

	pagination.SetItems(products)

	return http.OK(c, pagination)
}

// GetProductByID is a method that retrieves Product information by a given id.
//
//	@Summary		Get a Product by ID
//	@Description	Get a Product with the input ID
//	@Tags			Products
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			id				path		string	true	"Product ID"
//	@Success		200				{object}	mmodel.Product
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/products/{id} [get]
func (handler *ProductHandler) GetProductByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_product_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)
	logger.Infof("Initiating retrieval of Product with Organization ID: %s and Ledger ID: %s and Product ID: %s", organizationID.String(), ledgerID.String(), id.String())

	product, err := handler.Query.GetProductByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve Product on query", err)

		logger.Errorf("Failed to retrieve Product with Ledger ID: %s and Product ID: %s, Error: %s", ledgerID.String(), id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Product with Organization ID: %s and Ledger ID: %s and Product ID: %s", organizationID.String(), ledgerID.String(), id.String())

	return http.OK(c, product)
}

// UpdateProduct is a method that updates Product information.
//
//	@Summary		Update a Product
//	@Description	Update a Product with the input payload
//	@Tags			Products
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string						true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header		string						false	"Request ID"
//	@Param			organization_id	path		string						true	"Organization ID"
//	@Param			ledger_id		path		string						true	"Ledger ID"
//	@Param			id				path		string						true	"Product ID"
//	@Param			product			body		mmodel.UpdateProductInput	true	"Product"
//	@Success		200				{object}	mmodel.Product
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/products/{id} [patch]
func (handler *ProductHandler) UpdateProduct(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_product")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)
	logger.Infof("Initiating update of Product with Organization ID: %s and Ledger ID: %s and Product ID: %s", organizationID.String(), ledgerID.String(), id.String())

	payload := i.(*mmodel.UpdateProductInput)
	logger.Infof("Request to update an Product with details: %#v", payload)

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	_, err = handler.Command.UpdateProductByID(ctx, organizationID, ledgerID, id, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update Product on command", err)

		logger.Errorf("Failed to update Product with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	product, err := handler.Query.GetProductByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve Product on query", err)

		logger.Errorf("Failed to retrieve Product with Ledger ID: %s and Product ID: %s, Error: %s", ledgerID.String(), id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully updated Product with Organization ID: %s and Ledger ID: %s and Product ID: %s", organizationID.String(), ledgerID.String(), id.String())

	return http.OK(c, product)
}

// DeleteProductByID is a method that removes Product information by a given ids.
//
//	@Summary		Delete a Product by ID
//	@Description	Delete a Product with the input ID
//	@Tags			Products
//	@Param			Authorization	header	string	true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header	string	false	"Request ID"
//	@Param			organization_id	path	string	true	"Organization ID"
//	@Param			ledger_id		path	string	true	"Ledger ID"
//	@Param			id				path	string	true	"Product ID"
//	@Success		204
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/products/{id} [delete]
func (handler *ProductHandler) DeleteProductByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_product_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating removal of Product with Organization ID: %s and Ledger ID: %s and Product ID: %s", organizationID.String(), ledgerID.String(), id.String())

	if err := handler.Command.DeleteProductByID(ctx, organizationID, ledgerID, id); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to remove Product on command", err)

		logger.Errorf("Failed to remove Product with Ledger ID: %s and Product ID: %s, Error: %s", ledgerID.String(), id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully removed Product with Organization ID: %s and Ledger ID: %s and Product ID: %s", organizationID.String(), ledgerID.String(), id.String())

	return http.NoContent(c)
}
