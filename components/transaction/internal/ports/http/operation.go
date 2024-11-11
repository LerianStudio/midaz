package http

import (
	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mpostgres"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/transaction/internal/app/command"
	"github.com/LerianStudio/midaz/components/transaction/internal/app/query"
	o "github.com/LerianStudio/midaz/components/transaction/internal/domain/operation"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

// OperationHandler struct contains a cqrs use case for managing operations.
type OperationHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// GetAllOperationsByAccount retrieves all operations by account.
func (handler *OperationHandler) GetAllOperationsByAccount(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := common.NewLoggerFromContext(c.UserContext())

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	accountID := c.Locals("account_id").(uuid.UUID)

	headerParams := commonHTTP.ValidateParameters(c.Queries())

	pagination := mpostgres.Pagination{
		Limit: headerParams.Limit,
		Page:  headerParams.Page,
	}

	logger.Infof("Initiating retrieval of all Operations by account")

	headerParams.Metadata = &bson.M{}

	operations, err := handler.Query.GetAllOperationsByAccount(ctx, organizationID, ledgerID, accountID, *headerParams)
	if err != nil {
		logger.Errorf("Failed to retrieve all Operations by account, Error: %s", err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Operations by account")

	pagination.SetItems(operations)

	return commonHTTP.OK(c, pagination)
}

func (handler *OperationHandler) GetOperationByAccount(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := common.NewLoggerFromContext(c.UserContext())

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	accountID := c.Locals("account_id").(uuid.UUID)
	operationID := c.Locals("operation_id").(uuid.UUID)

	logger.Infof("Initiating retrieval of Operation by account")

	operation, err := handler.Query.GetOperationByAccount(ctx, organizationID, ledgerID, accountID, operationID)
	if err != nil {
		logger.Errorf("Failed to retrieve Operation by account, Error: %s", err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Operation by account")

	return commonHTTP.OK(c, operation)
}

func (handler *OperationHandler) GetAllOperationsByPortfolio(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := common.NewLoggerFromContext(c.UserContext())

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	portfolioID := c.Locals("portfolio_id").(uuid.UUID)

	headerParams := commonHTTP.ValidateParameters(c.Queries())

	pagination := mpostgres.Pagination{
		Limit: headerParams.Limit,
		Page:  headerParams.Page,
	}

	logger.Infof("Initiating retrieval of all Operations by portfolio")

	headerParams.Metadata = &bson.M{}

	operations, err := handler.Query.GetAllOperationsByPortfolio(ctx, organizationID, ledgerID, portfolioID, *headerParams)
	if err != nil {
		logger.Errorf("Failed to retrieve all Operations by portfolio, Error: %s", err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Operations by portfolio")

	pagination.SetItems(operations)

	return commonHTTP.OK(c, pagination)
}

func (handler *OperationHandler) GetOperationByPortfolio(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := common.NewLoggerFromContext(c.UserContext())

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	portfolioID := c.Locals("portfolio_id").(uuid.UUID)
	operationID := c.Locals("operation_id").(uuid.UUID)

	logger.Infof("Initiating retrieval of Operation by portfolio")

	operation, err := handler.Query.GetOperationByPortfolio(ctx, organizationID, ledgerID, portfolioID, operationID)
	if err != nil {
		logger.Errorf("Failed to retrieve Operation by portfolio, Error: %s", err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Operation by portfolio")

	return commonHTTP.OK(c, operation)
}

// UpdateOperation method that patch operation created before
func (handler *OperationHandler) UpdateOperation(p any, c *fiber.Ctx) error {
	logger := common.NewLoggerFromContext(c.UserContext())

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	transactionID := c.Locals("transaction_id").(uuid.UUID)
	operationID := c.Locals("operation_id").(uuid.UUID)

	logger.Infof("Initiating update of Operation with Organization ID: %s, Ledger ID: %s, Transaction ID: %s and ID: %s", organizationID.String(), ledgerID.String(), transactionID.String(), operationID.String())

	payload := p.(*o.UpdateOperationInput)
	logger.Infof("Request to update an Operation with details: %#v", payload)

	_, err := handler.Command.UpdateOperation(c.Context(), organizationID, ledgerID, transactionID, operationID, payload)
	if err != nil {
		logger.Errorf("Failed to update Operation with ID: %s, Error: %s", transactionID, err.Error())
		return commonHTTP.WithError(c, err)
	}

	operation, err := handler.Query.GetOperationByID(c.Context(), organizationID, ledgerID, transactionID, operationID)
	if err != nil {
		logger.Errorf("Failed to retrieve Operation with ID: %s, Error: %s", operationID, err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully updated Operation with Organization ID: %s, Ledger ID: %s, Transaction ID: %s and ID: %s", organizationID, ledgerID, transactionID, operationID)

	return commonHTTP.OK(c, operation)
}
