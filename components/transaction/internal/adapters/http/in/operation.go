package in

import (
	"go.mongodb.org/mongo-driver/bson"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/LerianStudio/midaz/pkg/mpostgres"
	"github.com/LerianStudio/midaz/pkg/net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// OperationHandler struct contains a cqrs use case for managing operations.
type OperationHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// GetAllOperationsByAccount retrieves all operations by account.
//
//	@Summary		Get all Operations by account
//	@Description	Get all Operations with the input ID
//	@Tags			Operations
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			account_id		path		string	true	"Account ID"
//	@Success		200				{object}	mpostgres.Pagination{items=[]operation.Operation}
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/operations [get]
func (handler *OperationHandler) GetAllOperationsByAccount(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_operations_by_account")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	accountID := c.Locals("account_id").(uuid.UUID)

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	pagination := mpostgres.Pagination{
		Limit:      headerParams.Limit,
		NextCursor: headerParams.Cursor,
		Page:       headerParams.Page,
		SortOrder:  headerParams.SortOrder,
		StartDate:  headerParams.StartDate,
		EndDate:    headerParams.EndDate,
	}

	logger.Infof("Initiating retrieval of all Operations by account")

	headerParams.Metadata = &bson.M{}

	err = mopentelemetry.SetSpanAttributesFromStruct(&span, "headerParams", headerParams)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert headerParams to JSON string", err)

		return http.WithError(c, err)
	}

	operations, cur, err := handler.Query.GetAllOperationsByAccount(ctx, organizationID, ledgerID, accountID, *headerParams)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve all Operations by account", err)

		logger.Errorf("Failed to retrieve all Operations by account, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Operations by account")

	pagination.SetItems(operations)
	pagination.SetCursor(cur.Next, cur.Prev)

	return http.OK(c, pagination)
}

// GetOperationByAccount retrieves an operation by account.
//
//	@Summary		Get an Operation by account
//	@Description	Get an Operation with the input ID
//	@Tags			Operations
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			account_id		path		string	true	"Account ID"
//	@Param			operation_id	path		string	true	"Operation ID"
//	@Success		200				{object}	operation.Operation
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/operations/{operation_id} [get]
func (handler *OperationHandler) GetOperationByAccount(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_operation_by_account")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	accountID := c.Locals("account_id").(uuid.UUID)
	operationID := c.Locals("operation_id").(uuid.UUID)

	logger.Infof("Initiating retrieval of Operation by account")

	op, err := handler.Query.GetOperationByAccount(ctx, organizationID, ledgerID, accountID, operationID)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve Operation by account", err)

		logger.Errorf("Failed to retrieve Operation by account, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Operation by account")

	return http.OK(c, op)
}

// GetAllOperationsByPortfolio retrieves all operations by portfolio.
//
//	@Summary		Get all Operations by portfolio
//	@Description	Get all Operations with the input ID
//	@Tags			Operations
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			portfolio_id	path		string	true	"Portfolio ID"
//	@Success		200				{object}	mpostgres.Pagination{items=[]operation.Operation}
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{portfolio_id}/operations [get]
func (handler *OperationHandler) GetAllOperationsByPortfolio(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_operations_by_portfolio")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	portfolioID := c.Locals("portfolio_id").(uuid.UUID)

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	pagination := mpostgres.Pagination{
		Limit:      headerParams.Limit,
		NextCursor: headerParams.Cursor,
		Page:       headerParams.Page,
		SortOrder:  headerParams.SortOrder,
		StartDate:  headerParams.StartDate,
		EndDate:    headerParams.EndDate,
	}

	logger.Infof("Initiating retrieval of all Operations by portfolio")

	headerParams.Metadata = &bson.M{}

	err = mopentelemetry.SetSpanAttributesFromStruct(&span, "headerParams", headerParams)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert headerParams to JSON string", err)

		return http.WithError(c, err)
	}

	operations, cur, err := handler.Query.GetAllOperationsByPortfolio(ctx, organizationID, ledgerID, portfolioID, *headerParams)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve all Operations by portfolio", err)

		logger.Errorf("Failed to retrieve all Operations by portfolio, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Operations by portfolio")

	pagination.SetItems(operations)
	pagination.SetCursor(cur.Next, cur.Prev)

	return http.OK(c, pagination)
}

// GetOperationByPortfolio retrieves an operation by portfolio.
//
//	@Summary		Get an Operation by portfolio
//	@Description	Get an Operation with the input ID
//	@Tags			Operations
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			portfolio_id	path		string	true	"Portfolio ID"
//	@Param			operation_id	path		string	true	"Operation ID"
//	@Success		200				{object}	operation.Operation
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{portfolio_id}/operations/{operation_id} [get]
func (handler *OperationHandler) GetOperationByPortfolio(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_operation_by_portfolio")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	portfolioID := c.Locals("portfolio_id").(uuid.UUID)
	operationID := c.Locals("operation_id").(uuid.UUID)

	logger.Infof("Initiating retrieval of Operation by portfolio")

	op, err := handler.Query.GetOperationByPortfolio(ctx, organizationID, ledgerID, portfolioID, operationID)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve Operation by portfolio", err)

		logger.Errorf("Failed to retrieve Operation by portfolio, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Operation by portfolio")

	return http.OK(c, op)
}

// UpdateOperation method that patch operation created before
//
//	@Summary		Update an Operation
//	@Description	Update an Operation with the input payload
//	@Tags			Operations
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string							true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header		string							false	"Request ID"
//	@Param			organization_id	path		string							true	"Organization ID"
//	@Param			ledger_id		path		string							true	"Ledger ID"
//	@Param			transaction_id	path		string							true	"Transaction ID"
//	@Param			operation_id	path		string							true	"Operation ID"
//	@Param			operation		body		operation.UpdateOperationInput	true	"Operation Input"
//	@Success		200				{object}	operation.Operation
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/operations/{operation_id} [patch]
func (handler *OperationHandler) UpdateOperation(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

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

	op, err := handler.Query.GetOperationByID(ctx, organizationID, ledgerID, transactionID, operationID)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve Operation on query", err)

		logger.Errorf("Failed to retrieve Operation with ID: %s, Error: %s", operationID, err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully updated Operation with Organization ID: %s, Ledger ID: %s, Transaction ID: %s and ID: %s", organizationID, ledgerID, transactionID, operationID)

	return http.OK(c, op)
}
