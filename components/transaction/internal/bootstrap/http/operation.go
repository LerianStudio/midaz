package http

import (
	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"github.com/LerianStudio/midaz/common/mpostgres"
	"github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/database/postgres/operation"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/query"
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

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_operations_by_account")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	accountID := c.Locals("account_id").(uuid.UUID)

	headerParams := http.ValidateParameters(c.Queries())

	pagination := mpostgres.Pagination{
		Limit: headerParams.Limit,
		Page:  headerParams.Page,
	}

	logger.Infof("Initiating retrieval of all Operations by account")

	headerParams.Metadata = &bson.M{}

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "headerParams", headerParams)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert headerParams to JSON string", err)

		return http.WithError(c, err)
	}

	operations, err := handler.Query.GetAllOperationsByAccount(ctx, organizationID, ledgerID, accountID, *headerParams)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve all Operations by account", err)

		logger.Errorf("Failed to retrieve all Operations by account, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Operations by account")

	pagination.SetItems(operations)

	return http.OK(c, pagination)
}

func (handler *OperationHandler) GetOperationByAccount(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_operation_by_account")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	accountID := c.Locals("account_id").(uuid.UUID)
	operationID := c.Locals("operation_id").(uuid.UUID)

	logger.Infof("Initiating retrieval of Operation by account")

	operation, err := handler.Query.GetOperationByAccount(ctx, organizationID, ledgerID, accountID, operationID)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve Operation by account", err)

		logger.Errorf("Failed to retrieve Operation by account, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Operation by account")

	return http.OK(c, operation)
}

func (handler *OperationHandler) GetAllOperationsByPortfolio(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_operations_by_portfolio")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	portfolioID := c.Locals("portfolio_id").(uuid.UUID)

	headerParams := http.ValidateParameters(c.Queries())

	pagination := mpostgres.Pagination{
		Limit: headerParams.Limit,
		Page:  headerParams.Page,
	}

	logger.Infof("Initiating retrieval of all Operations by portfolio")

	headerParams.Metadata = &bson.M{}

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "headerParams", headerParams)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert headerParams to JSON string", err)

		return http.WithError(c, err)
	}

	operations, err := handler.Query.GetAllOperationsByPortfolio(ctx, organizationID, ledgerID, portfolioID, *headerParams)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve all Operations by portfolio", err)

		logger.Errorf("Failed to retrieve all Operations by portfolio, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Operations by portfolio")

	pagination.SetItems(operations)

	return http.OK(c, pagination)
}

func (handler *OperationHandler) GetOperationByPortfolio(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_operation_by_portfolio")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	portfolioID := c.Locals("portfolio_id").(uuid.UUID)
	operationID := c.Locals("operation_id").(uuid.UUID)

	logger.Infof("Initiating retrieval of Operation by portfolio")

	operation, err := handler.Query.GetOperationByPortfolio(ctx, organizationID, ledgerID, portfolioID, operationID)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve Operation by portfolio", err)

		logger.Errorf("Failed to retrieve Operation by portfolio, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Operation by portfolio")

	return http.OK(c, operation)
}

// UpdateOperation method that patch operation created before
func (handler *OperationHandler) UpdateOperation(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_operation")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	transactionID := c.Locals("transaction_id").(uuid.UUID)
	operationID := c.Locals("operation_id").(uuid.UUID)

	logger.Infof("Initiating update of Operation with Organization ID: %s, Ledger ID: %s, Transaction ID: %s and ID: %s", organizationID.String(), ledgerID.String(), transactionID.String(), operationID.String())

	payload := p.(*operation.UpdateOperationInput)
	logger.Infof("Request to update an Operation with details: %#v", payload)

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	_, err = handler.Command.UpdateOperation(ctx, organizationID, ledgerID, transactionID, operationID, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update Operation on command", err)

		logger.Errorf("Failed to update Operation with ID: %s, Error: %s", transactionID, err.Error())

		return http.WithError(c, err)
	}

	operation, err := handler.Query.GetOperationByID(ctx, organizationID, ledgerID, transactionID, operationID)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve Operation on query", err)

		logger.Errorf("Failed to retrieve Operation with ID: %s, Error: %s", operationID, err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully updated Operation with Organization ID: %s, Ledger ID: %s, Transaction ID: %s and ID: %s", organizationID, ledgerID, transactionID, operationID)

	return http.OK(c, operation)
}
