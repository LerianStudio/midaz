package http

import (
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/common/mpostgres"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/transaction/internal/app/command"
	"github.com/LerianStudio/midaz/components/transaction/internal/app/query"
	"github.com/gofiber/fiber/v2"
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
	logger := mlog.NewLoggerFromContext(c.UserContext())

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")
	accountID := c.Params("account_id")

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
	logger := mlog.NewLoggerFromContext(c.UserContext())

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")
	accountID := c.Params("account_id")
	operationID := c.Params("operation_id")

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
	logger := mlog.NewLoggerFromContext(c.UserContext())

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")
	portfolioID := c.Params("portfolio_id")

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
