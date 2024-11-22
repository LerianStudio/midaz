package http

import (
	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"github.com/LerianStudio/midaz/common/mpostgres"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/ledger_two/internal/services/command"
	"github.com/LerianStudio/midaz/components/ledger_two/internal/services/query"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

// ProductHandler struct contains a product use case for managing product related operations.
type ProductHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateProduct is a method that creates product information.
func (handler *ProductHandler) CreateProduct(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

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

		return commonHTTP.WithError(c, err)
	}

	product, err := handler.Command.CreateProduct(ctx, organizationID, ledgerID, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create Product on command", err)

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully created Product")

	return commonHTTP.Created(c, product)
}

// GetAllProducts is a method that retrieves all Products.
func (handler *ProductHandler) GetAllProducts(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_products")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	logger.Infof("Get Products with organization ID: %s and ledger ID: %s", organizationID.String(), ledgerID.String())

	headerParams := commonHTTP.ValidateParameters(c.Queries())

	pagination := mpostgres.Pagination{
		Limit: headerParams.Limit,
		Page:  headerParams.Page,
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Products by metadata")

		products, err := handler.Query.GetAllMetadataProducts(ctx, organizationID, ledgerID, *headerParams)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to retrieve all Products on query", err)

			logger.Errorf("Failed to retrieve all Products, Error: %s", err.Error())

			return commonHTTP.WithError(c, err)
		}

		logger.Infof("Successfully retrieved all Products by metadata")

		pagination.SetItems(products)

		return commonHTTP.OK(c, pagination)
	}

	logger.Infof("Initiating retrieval of all Products ")

	headerParams.Metadata = &bson.M{}

	products, err := handler.Query.GetAllProducts(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve all Products on query", err)

		logger.Errorf("Failed to retrieve all Products, Error: %s", err.Error())

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Products")

	pagination.SetItems(products)

	return commonHTTP.OK(c, pagination)
}

// GetProductByID is a method that retrieves Product information by a given id.
func (handler *ProductHandler) GetProductByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

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

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Product with Organization ID: %s and Ledger ID: %s and Product ID: %s", organizationID.String(), ledgerID.String(), id.String())

	return commonHTTP.OK(c, product)
}

// UpdateProduct is a method that updates Product information.
func (handler *ProductHandler) UpdateProduct(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

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

		return commonHTTP.WithError(c, err)
	}

	_, err = handler.Command.UpdateProductByID(ctx, organizationID, ledgerID, id, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update Product on command", err)

		logger.Errorf("Failed to update Product with ID: %s, Error: %s", id.String(), err.Error())

		return commonHTTP.WithError(c, err)
	}

	product, err := handler.Query.GetProductByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve Product on query", err)

		logger.Errorf("Failed to retrieve Product with Ledger ID: %s and Product ID: %s, Error: %s", ledgerID.String(), id.String(), err.Error())

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully updated Product with Organization ID: %s and Ledger ID: %s and Product ID: %s", organizationID.String(), ledgerID.String(), id.String())

	return commonHTTP.OK(c, product)
}

// DeleteProductByID is a method that removes Product information by a given ids.
func (handler *ProductHandler) DeleteProductByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_product_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating removal of Product with Organization ID: %s and Ledger ID: %s and Product ID: %s", organizationID.String(), ledgerID.String(), id.String())

	if err := handler.Command.DeleteProductByID(ctx, organizationID, ledgerID, id); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to remove Product on command", err)

		logger.Errorf("Failed to remove Product with Ledger ID: %s and Product ID: %s, Error: %s", ledgerID.String(), id.String(), err.Error())

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully removed Product with Organization ID: %s and Ledger ID: %s and Product ID: %s", organizationID.String(), ledgerID.String(), id.String())

	return commonHTTP.NoContent(c)
}
