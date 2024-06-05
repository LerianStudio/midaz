package ports

import (
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/common/mpostgres"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/query"
	r "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/product"
	"github.com/gofiber/fiber/v2"
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

	logger := mlog.NewLoggerFromContext(ctx)

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")
	logger.Infof("Initiating create of Product with organization ID: %s and ledger ID: %s", organizationID, ledgerID)

	payload := i.(*r.CreateProductInput)
	logger.Infof("Request to create a Product with details: %#v", payload)

	product, err := handler.Command.CreateProduct(ctx, organizationID, ledgerID, payload)
	if err != nil {
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully created Product")

	return commonHTTP.Created(c, product)
}

// GetAllProducts is a method that retrieves all Products.
func (handler *ProductHandler) GetAllProducts(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := mlog.NewLoggerFromContext(ctx)

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")
	logger.Infof("Get Products with organization ID: %s and ledger ID: %s", organizationID, ledgerID)

	headerParams := commonHTTP.ValidateParameters(c.Queries())

	pagination := mpostgres.Pagination{
		Limit: headerParams.Limit,
		Page:  headerParams.Page,
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Products by metadata")

		products, err := handler.Query.GetAllMetadataProducts(ctx, organizationID, ledgerID, *headerParams)
		if err != nil {
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
	logger := mlog.NewLoggerFromContext(ctx)

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")
	id := c.Params("id")
	logger.Infof("Initiating retrieval of Product with Organization ID: %s and Ledger ID: %s and Product ID: %s", organizationID, ledgerID, id)

	product, err := handler.Query.GetProductByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		logger.Errorf("Failed to retrieve Product with Ledger ID: %s and Product ID: %s, Error: %s", ledgerID, id, err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Product with Organization ID: %s and Ledger ID: %s and Product ID: %s", organizationID, ledgerID, id)

	return commonHTTP.OK(c, product)
}

// UpdateProduct is a method that updates Product information.
func (handler *ProductHandler) UpdateProduct(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := mlog.NewLoggerFromContext(ctx)

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")
	id := c.Params("id")
	logger.Infof("Initiating update of Product with Organization ID: %s and Ledger ID: %s and Product ID: %s", organizationID, ledgerID, id)

	payload := i.(*r.UpdateProductInput)
	logger.Infof("Request to update an Product with details: %#v", payload)

	_, err := handler.Command.UpdateProductByID(ctx, organizationID, ledgerID, id, payload)
	if err != nil {
		logger.Errorf("Failed to update Product with ID: %s, Error: %s", id, err.Error())
		return commonHTTP.WithError(c, err)
	}

	product, err := handler.Query.GetProductByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		logger.Errorf("Failed to retrieve Product with Ledger ID: %s and Product ID: %s, Error: %s", ledgerID, id, err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully updated Product with Organization ID: %s and Ledger ID: %s and Product ID: %s", organizationID, ledgerID, id)

	return commonHTTP.OK(c, product)
}

// DeleteProductByID is a method that removes Product information by a given ids.
func (handler *ProductHandler) DeleteProductByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := mlog.NewLoggerFromContext(ctx)

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")
	id := c.Params("id")

	logger.Infof("Initiating removal of Product with Organization ID: %s and Ledger ID: %s and Product ID: %s", organizationID, ledgerID, id)

	if err := handler.Command.DeleteProductByID(ctx, organizationID, ledgerID, id); err != nil {
		logger.Errorf("Failed to remove Product with Ledger ID: %s and Product ID: %s, Error: %s", ledgerID, id, err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully removed Product with Organization ID: %s and Ledger ID: %s and Product ID: %s", organizationID, ledgerID, id)

	return commonHTTP.NoContent(c)
}
