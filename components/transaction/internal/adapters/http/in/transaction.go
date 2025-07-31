package in

import (
	"encoding/json"
	"reflect"
	"time"

	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	"go.opentelemetry.io/otel/attribute"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	goldTransaction "github.com/LerianStudio/midaz/pkg/gold/transaction"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

// TransactionHandler struct that handle transaction
type TransactionHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateTransactionJSON method that create transaction using JSON
//
//	@Summary		Create a Transaction using JSON
//	@Description	Create a Transaction with the input payload
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string								true	"Authorization Bearer Token"
//	@Param			X-Request-Id		header		string								false	"Request ID"
//	@Param			organization_id	path		string								true	"Organization ID"
//	@Param			ledger_id		path		string								true	"Ledger ID"
//	@Param			transaction		body		transaction.CreateTransactionSwaggerModel	true	"Transaction Input"
//	@Success		201				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/json [post]
func (handler *TransactionHandler) CreateTransactionJSON(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_transaction")
	defer span.End()

	c.SetUserContext(ctx)

	input := p.(*transaction.CreateTransactionInput)
	parserDSL := input.FromDSL()
	logger.Infof("Request to create an transaction with details: %#v", parserDSL)

	transactionStatus := constant.CREATED
	if parserDSL.Pending {
		transactionStatus = constant.PENDING
	}

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.transaction.transaction_status", transactionStatus),
		attribute.String("app.request.transaction.chart_of_accounts_group_name", parserDSL.ChartOfAccountsGroupName),
		attribute.String("app.request.transaction.description", parserDSL.Description),
		attribute.String("app.request.transaction.code", parserDSL.Code),
		attribute.Bool("app.request.transaction.pending", parserDSL.Pending),
		attribute.String("app.request.transaction.route", parserDSL.Route),
	)

	err := libOpentelemetry.SetSpanAttributesFromStructWithObfuscation(&span, "app.request.transaction.send", parserDSL.Send)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert transaction input to JSON string", err)
	}

	return handler.createTransaction(c, logger, *parserDSL, transactionStatus)
}

// CreateTransactionInflow method that creates a transaction without specifying a source
//
//	@Summary		Create a Transaction without passing from source
//	@Description	Create a Transaction with the input payload
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string								true	"Authorization Bearer Token"
//	@Param			X-Request-Id		header		string								false	"Request ID"
//	@Param			organization_id	path		string								true	"Organization ID"
//	@Param			ledger_id		path		string								true	"Ledger ID"
//	@Param			transaction		body		transaction.CreateTransactionInflowSwaggerModel	true	"Transaction Input"
//	@Success		201				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/inflow [post]
func (handler *TransactionHandler) CreateTransactionInflow(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_transaction_inflow")
	defer span.End()

	c.SetUserContext(ctx)

	input := p.(*transaction.CreateTransactionInflowInput)
	parserDSL := input.InflowFromDSL()
	logger.Infof("Request to create an transaction inflow with details: %#v", parserDSL)

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.transaction.chart_of_accounts_group_name", input.ChartOfAccountsGroupName),
		attribute.String("app.request.transaction.description", input.Description),
		attribute.String("app.request.transaction.code", input.Code),
		attribute.String("app.request.transaction.route", input.Route),
	)

	err := libOpentelemetry.SetSpanAttributesFromStructWithObfuscation(&span, "app.request.transaction.send", input.Send)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert transaction input to JSON string", err)
	}

	response := handler.createTransaction(c, logger, *parserDSL, constant.CREATED)

	return response
}

// CreateTransactionOutflow method that creates a transaction without specifying a distribution
//
//	@Summary		Create a Transaction without passing to distribution
//	@Description	Create a Transaction with the input payload
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string								true	"Authorization Bearer Token"
//	@Param			X-Request-Id		header		string								false	"Request ID"
//	@Param			organization_id	path		string								true	"Organization ID"
//	@Param			ledger_id		path		string								true	"Ledger ID"
//	@Param			transaction		body		transaction.CreateTransactionOutflowSwaggerModel	true	"Transaction Input"
//	@Success		201				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/outflow [post]
func (handler *TransactionHandler) CreateTransactionOutflow(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_transaction_outflow")
	defer span.End()

	c.SetUserContext(ctx)

	input := p.(*transaction.CreateTransactionOutflowInput)
	parserDSL := input.OutflowFromDSL()
	logger.Infof("Request to create an transaction outflow with details: %#v", parserDSL)

	transactionStatus := constant.CREATED
	if parserDSL.Pending {
		transactionStatus = constant.PENDING
	}

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.transaction.transaction_status", transactionStatus),
		attribute.String("app.request.transaction.chart_of_accounts_group_name", input.ChartOfAccountsGroupName),
		attribute.String("app.request.transaction.description", input.Description),
		attribute.String("app.request.transaction.code", input.Code),
		attribute.String("app.request.transaction.route", input.Route),
	)

	err := libOpentelemetry.SetSpanAttributesFromStructWithObfuscation(&span, "app.request.transaction.send", input.Send)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert transaction input to JSON string", err)
	}

	return handler.createTransaction(c, logger, *parserDSL, transactionStatus)
}

// CreateTransactionDSL method that create transaction using DSL
//
//	@Summary		Create a Transaction using DSL
//	@Description	Create a Transaction with the input DSL file
//	@Tags			Transactions
//	@Accept			mpfd
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			transaction		formData	file	true	"Transaction DSL file"
//	@Success		200				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid DSL file format or validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/dsl [post]
func (handler *TransactionHandler) CreateTransactionDSL(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_transaction_dsl")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	c.SetUserContext(ctx)

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
	}

	span.SetAttributes(attributes...)

	_, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to validate query parameters", err)

		logger.Error("Failed to validate query parameters: ", err.Error())

		return http.WithError(c, err)
	}

	dsl, err := http.GetFileFromHeader(c)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get file from Header", err)

		logger.Error("Failed to get file from Header: ", err.Error())

		return http.WithError(c, err)
	}

	errListener := goldTransaction.Validate(dsl)
	if errListener != nil && len(errListener.Errors) > 0 {
		err := pkg.ValidateBusinessError(constant.ErrInvalidDSLFileFormat, reflect.TypeOf(transaction.Transaction{}).Name(), errListener.Errors)

		libOpentelemetry.HandleSpanError(&span, "Failed to validate script in DSL", err)

		return http.WithError(c, err)
	}

	parsed := goldTransaction.Parse(dsl)

	parserDSL, ok := parsed.(libTransaction.Transaction)
	if !ok {
		err := pkg.ValidateBusinessError(constant.ErrInvalidDSLFileFormat, reflect.TypeOf(transaction.Transaction{}).Name())

		libOpentelemetry.HandleSpanError(&span, "Failed to parse script in DSL", err)

		return http.WithError(c, err)
	}

	dslAttributes := []attribute.KeyValue{
		attribute.String("app.request.transaction.chart_of_accounts_group_name", parserDSL.ChartOfAccountsGroupName),
		attribute.String("app.request.transaction.description", parserDSL.Description),
		attribute.String("app.request.transaction.code", parserDSL.Code),
		attribute.String("app.request.transaction.route", parserDSL.Route),
		attribute.Bool("app.request.transaction.pending", parserDSL.Pending),
	}

	attributes = append(attributes, dslAttributes...)

	span.SetAttributes(attributes...)

	err = libOpentelemetry.SetSpanAttributesFromStructWithObfuscation(&span, "app.request.transaction.send", parserDSL.Send)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert transaction input to JSON string", err)
	}

	response := handler.createTransaction(c, logger, parserDSL, constant.CREATED)

	return response
}

// CommitTransaction method that commit transaction created before
//
//	@Summary		Commit a Transaction
//	@Description	Commit a previously created transaction
//	@Tags			Transactions
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			transaction_id	path		string	true	"Transaction ID"
//	@Success		201				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid request or transaction cannot be reverted"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Transaction not found"
//	@Failure		409				{object}	mmodel.Error	"Transaction already has a parent transaction"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/commit [Post]
func (handler *TransactionHandler) CommitTransaction(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	transactionID := c.Locals("transaction_id").(uuid.UUID)

	_, span := tracer.Start(ctx, "handler.commit_transaction")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
	}

	span.SetAttributes(attributes...)

	tran, err := handler.Query.GetTransactionByID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve transaction on query", err)

		logger.Errorf("Failed to retrieve Transaction with ID: %s, Error: %s", transactionID.String(), err.Error())

		return http.WithError(c, err)
	}

	return handler.commitOrCancelTransaction(c, logger, tran, constant.APPROVED)
}

// CancelTransaction method that cancel pre transaction created before
//
//	@Summary		Cancel a pre transaction
//	@Description	Cancel a previously created pre transaction
//	@Tags			Transactions
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			transaction_id	path		string	true	"Transaction ID"
//	@Success		201				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid request or transaction cannot be reverted"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Transaction not found"
//	@Failure		409				{object}	mmodel.Error	"Transaction already has a parent transaction"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/cancel [Post]
func (handler *TransactionHandler) CancelTransaction(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	transactionID := c.Locals("transaction_id").(uuid.UUID)

	_, span := tracer.Start(ctx, "handler.cancel_transaction")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
	}

	span.SetAttributes(attributes...)

	tran, err := handler.Query.GetTransactionByID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve transaction on query", err)

		logger.Errorf("Failed to retrieve Transaction with ID: %s, Error: %s", transactionID.String(), err.Error())

		return http.WithError(c, err)
	}

	return handler.commitOrCancelTransaction(c, logger, tran, constant.CANCELED)
}

// RevertTransaction method that revert transaction created before
//
//	@Summary		Revert a Transaction
//	@Description	Revert a Transaction with Transaction ID only
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string								true	"Authorization Bearer Token"
//	@Param			X-Request-Id		header		string								false	"Request ID"
//	@Param			organization_id	path		string								true	"Organization ID"
//	@Param			ledger_id		path		string								true	"Ledger ID"
//	@Param			transaction_id	path		string								true	"Transaction ID"
//	@Success		200				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid request or transaction cannot be reverted"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Transaction not found"
//	@Failure		409				{object}	mmodel.Error	"Transaction already has a parent transaction"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/revert [post]
func (handler *TransactionHandler) RevertTransaction(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	transactionID := c.Locals("transaction_id").(uuid.UUID)

	_, span := tracer.Start(ctx, "handler.revert_transaction")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
	}

	span.SetAttributes(attributes...)

	parent, err := handler.Query.GetParentByTransactionID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve Parent Transaction on query", err)

		logger.Errorf("Failed to retrieve Parent Transaction with ID: %s, Error: %s", transactionID.String(), err.Error())

		return http.WithError(c, err)
	}

	if parent != nil {
		err = pkg.ValidateBusinessError(constant.ErrTransactionIDHasAlreadyParentTransaction, "RevertTransaction")

		libOpentelemetry.HandleSpanError(&span, "Transaction Has Already Parent Transaction", err)

		logger.Errorf("Transaction Has Already Parent Transaction with ID: %s, Error: %s", transactionID.String(), err)

		return http.WithError(c, err)
	}

	tran, err := handler.Query.GetTransactionWithOperationsByID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve transaction on query", err)

		logger.Errorf("Failed to retrieve Transaction with ID: %s, Error: %s", transactionID.String(), err.Error())

		return http.WithError(c, err)
	}

	if tran.ParentTransactionID != nil {
		err = pkg.ValidateBusinessError(constant.ErrTransactionIDIsAlreadyARevert, "RevertTransaction")

		libOpentelemetry.HandleSpanError(&span, "Transaction Has Already Parent Transaction", err)

		logger.Errorf("Transaction Has Already Parent Transaction with ID: %s, Error: %s", transactionID.String(), err)

		return http.WithError(c, err)
	}

	if tran.Status.Code != constant.APPROVED {
		err = pkg.ValidateBusinessError(constant.ErrCommitTransactionNotPending, "RevertTransaction")

		libOpentelemetry.HandleSpanError(&span, "Transaction CantRevert Transaction", err)

		logger.Errorf("Transaction CantRevert Transaction with ID: %s, Error: %s", transactionID.String(), err)

		return http.WithError(c, err)
	}

	transactionReverted := tran.TransactionRevert()
	if transactionReverted.IsEmpty() {
		err = pkg.ValidateBusinessError(constant.ErrTransactionCantRevert, "RevertTransaction")

		libOpentelemetry.HandleSpanError(&span, "Transaction can't be reverted", err)

		logger.Errorf("Parent Transaction can't be reverted with ID: %s, Error: %s", transactionID.String(), err)

		return http.WithError(c, err)
	}

	response := handler.createTransaction(c, logger, transactionReverted, constant.CREATED)

	return response
}

// UpdateTransaction method that patch transaction created before
//
//	@Summary		Update a Transaction
//	@Description	Update a Transaction with the input payload
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string								true	"Authorization Bearer Token"
//	@Param			X-Request-Id		header		string								false	"Request ID"
//	@Param			organization_id	path		string								true	"Organization ID"
//	@Param			ledger_id		path		string								true	"Ledger ID"
//	@Param			transaction_id	path		string								true	"Transaction ID"
//	@Param			transaction		body		transaction.UpdateTransactionInput	true	"Transaction Input"
//	@Success		200				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Transaction not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id} [patch]
func (handler *TransactionHandler) UpdateTransaction(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_transaction")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	transactionID := c.Locals("transaction_id").(uuid.UUID)

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.transaction_id", transactionID.String()),
	)

	logger.Infof("Initiating update of Transaction with Organization ID: %s, Ledger ID: %s and ID: %s", organizationID.String(), ledgerID.String(), transactionID.String())

	payload := p.(*transaction.UpdateTransactionInput)
	logger.Infof("Request to update an Transaction with details: %#v", payload)

	if err := libOpentelemetry.SetSpanAttributesFromStructWithObfuscation(&span, "app.request.payload", payload); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)
	}

	_, err := handler.Command.UpdateTransaction(ctx, organizationID, ledgerID, transactionID, payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update transaction on command", err)

		logger.Errorf("Failed to update Transaction with ID: %s, Error: %s", transactionID.String(), err.Error())

		return http.WithError(c, err)
	}

	trans, err := handler.Query.GetTransactionByID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve transaction on query", err)

		logger.Errorf("Failed to retrieve Transaction with ID: %s, Error: %s", transactionID.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully updated Transaction with Organization ID: %s, Ledger ID: %s and ID: %s", organizationID.String(), ledgerID.String(), transactionID.String())

	return http.OK(c, trans)
}

// GetTransaction method that get transaction created before
//
//	@Summary		Get a Transaction by ID
//	@Description	Get a Transaction with the input ID
//	@Tags			Transactions
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id		header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			transaction_id	path		string	true	"Transaction ID"
//	@Success		200				{object}	transaction.Transaction
//	@Failure		400				{object}	mmodel.Error	"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Transaction not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id} [get]
func (handler *TransactionHandler) GetTransaction(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_transaction")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	transactionID := c.Locals("transaction_id").(uuid.UUID)

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.transaction_id", transactionID.String()),
	)

	tran, err := handler.Query.GetTransactionByID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve transaction on query", err)

		logger.Errorf("Failed to retrieve Transaction with ID: %s, Error: %s", transactionID.String(), err.Error())

		return http.WithError(c, err)
	}

	ctxGetTransaction, spanGetTransaction := tracer.Start(ctx, "handler.get_transaction.get_operations")

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	headerParams.Metadata = &bson.M{}

	tran, err = handler.Query.GetOperationsByTransaction(ctxGetTransaction, organizationID, ledgerID, tran, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanGetTransaction, "Failed to retrieve Operations", err)

		logger.Errorf("Failed to retrieve Operations with ID: %s, Error: %s", tran.ID, err.Error())

		return http.WithError(c, err)
	}

	spanGetTransaction.End()

	logger.Infof("Successfully retrieved Transaction with ID: %s", transactionID.String())

	return http.OK(c, tran)
}

// GetAllTransactions method that get all transactions created before
//
//	@Summary		Get all Transactions
//	@Description	Get all Transactions with the input metadata or without metadata
//	@Tags			Transactions
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id		header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			limit			query		int		false	"Limit"			default(10)
//	@Param			start_date		query		string	false	"Start Date"	example "2021-01-01"
//	@Param			end_date		query		string	false	"End Date"		example "2021-01-01"
//	@Param			sort_order		query		string	false	"Sort Order"	Enums(asc,desc)
//	@Param			cursor			query		string	false	"Cursor"
//	@Success		200				{object}	libPostgres.Pagination{items=[]transaction.Transaction,next_cursor=string,prev_cursor=string,limit=int,page=nil}
//	@Failure		400				{object}	mmodel.Error	"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions [get]
func (handler *TransactionHandler) GetAllTransactions(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_transactions")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
	)

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	err = libOpentelemetry.SetSpanAttributesFromStructWithObfuscation(&span, "app.request.query_params", headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert metadata headerParams to JSON string", err)
	}

	pagination := libPostgres.Pagination{
		Limit:      headerParams.Limit,
		NextCursor: headerParams.Cursor,
		SortOrder:  headerParams.SortOrder,
		StartDate:  headerParams.StartDate,
		EndDate:    headerParams.EndDate,
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Transactions by metadata")

		trans, cur, err := handler.Query.GetAllMetadataTransactions(ctx, organizationID, ledgerID, *headerParams)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to retrieve all Transactions by metadata", err)

			logger.Errorf("Failed to retrieve all Transactions, Error: %s", err.Error())

			return http.WithError(c, err)
		}

		logger.Infof("Successfully retrieved all Transactions by metadata")

		pagination.SetItems(trans)
		pagination.SetCursor(cur.Next, cur.Prev)

		return http.OK(c, pagination)
	}

	logger.Infof("Initiating retrieval of all Transactions ")

	headerParams.Metadata = &bson.M{}

	trans, cur, err := handler.Query.GetAllTransactions(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve all Transactions", err)

		logger.Errorf("Failed to retrieve all Transactions, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Transactions")

	pagination.SetItems(trans)
	pagination.SetCursor(cur.Next, cur.Prev)

	return http.OK(c, pagination)
}

// handleAccountFields processes account and accountAlias fields for transaction entries
// accountAlias is deprecated but still needs to be handled for backward compatibility
func (handler *TransactionHandler) handleAccountFields(entries []libTransaction.FromTo, isConcat bool) []libTransaction.FromTo {
	result := make([]libTransaction.FromTo, 0, len(entries))

	for i := range entries {
		var newAlias string
		if isConcat {
			newAlias = entries[i].ConcatAlias(i)
		} else {
			newAlias = entries[i].SplitAlias()
		}

		entries[i].AccountAlias = newAlias

		result = append(result, entries[i])
	}

	return result
}

// createTransaction func that received struct from DSL parsed and create Transaction
func (handler *TransactionHandler) createTransaction(c *fiber.Ctx, logger libLog.Logger, parserDSL libTransaction.Transaction, transactionStatus string) error {
	ctx := c.UserContext()
	tracer := libCommons.NewTracerFromContext(ctx)
	metricFactory := libCommons.NewMetricFactoryFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.create_transaction")
	defer span.End()

	c.Set(libConstants.IdempotencyReplayed, "false")

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	transactionID, _ := c.Locals("transaction_id").(uuid.UUID)

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.transaction_id", transactionID.String()),
		attribute.String("app.request.transaction.transaction_status", transactionStatus),
		attribute.String("app.request.transaction.chart_of_accounts_group_name", parserDSL.ChartOfAccountsGroupName),
		attribute.String("app.request.transaction.description", parserDSL.Description),
		attribute.String("app.request.transaction.code", parserDSL.Code),
		attribute.String("app.request.transaction.route", parserDSL.Route),
		attribute.Bool("app.request.transaction.pending", parserDSL.Pending),
	}

	span.SetAttributes(attributes...)

	err := libOpentelemetry.SetSpanAttributesFromStructWithObfuscation(&span, "app.request.transaction.send", parserDSL.Send)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert transaction to JSON string", err)

		logger.Errorf("Failed to convert transaction to JSON string, Error: %s", err.Error())
	}

	var fromTo []libTransaction.FromTo

	fromTo = append(fromTo, handler.handleAccountFields(parserDSL.Send.Source.From, true)...)
	if transactionStatus != constant.PENDING {
		fromTo = append(fromTo, handler.handleAccountFields(parserDSL.Send.Distribute.To, true)...)
	}

	ctxIdempotency, spanIdempotency := tracer.Start(ctx, "handler.create_transaction_idempotency")

	c.SetUserContext(ctxIdempotency)

	spanIdempotency.SetAttributes(attributes...)

	ts, _ := libCommons.StructToJSONString(parserDSL)
	hash := libCommons.HashSHA256(ts)
	key, ttl := http.GetIdempotencyKeyAndTTL(c)

	value, err := handler.Command.CreateOrCheckIdempotencyKey(ctx, organizationID, ledgerID, key, hash, ttl)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanIdempotency, "Error on create or check redis idempotency key", err)
		spanIdempotency.End()

		logger.Infof("Error on create or check redis idempotency key: %v", err.Error())

		return http.WithError(c, err)
	} else if !libCommons.IsNilOrEmpty(value) {
		t := transaction.Transaction{}
		if err = json.Unmarshal([]byte(*value), &t); err != nil {
			libOpentelemetry.HandleSpanError(&spanIdempotency, "Error to deserialization idempotency transaction json on redis", err)
			spanIdempotency.End()

			logger.Errorf("Error to deserialization idempotency transaction json on redis: %v", err)

			return http.WithError(c, err)
		}

		spanIdempotency.End()
		c.Set(libConstants.IdempotencyReplayed, "true")

		return http.Created(c, t)
	}

	spanIdempotency.End()

	validate, err := libTransaction.ValidateSendSourceAndDistribute(parserDSL, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to validate send source and distribute", err)

		logger.Error("Validation failed:", err.Error())

		if err.Error() == constant.ErrTransactionAmbiguous.Error() {
			err = pkg.ValidateBusinessError(constant.ErrTransactionAmbiguous, "ValidateSendSourceAndDistribute")
		} else if err.Error() == constant.ErrTransactionValueMismatch.Error() {
			err = pkg.ValidateBusinessError(constant.ErrTransactionValueMismatch, "ValidateSendSourceAndDistribute")
		}

		_ = handler.Command.RedisRepo.Del(ctx, key)

		return http.WithError(c, err)
	}

	_, spanGetBalances := tracer.Start(ctx, "handler.create_transaction.get_balances")

	spanGetBalances.SetAttributes(attributes...)

	balances, err := handler.Query.GetBalances(ctx, organizationID, ledgerID, validate, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanGetBalances, "Failed to get balances", err)
		spanGetBalances.End()

		logger.Errorf("Failed to get balances: %v", err.Error())

		_ = handler.Command.RedisRepo.Del(ctx, key)

		return http.WithError(c, err)
	}

	spanGetBalances.End()

	_, spanValidateBalances := tracer.Start(ctx, "handler.create_transaction.validate_balances")

	spanValidateBalances.SetAttributes(attributes...)

	blcs := mmodel.ConvertBalancesToLibBalances(balances)

	err = libTransaction.ValidateBalancesRules(ctx, parserDSL, *validate, blcs)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanValidateBalances, "Failed to validate balances", err)
		spanValidateBalances.End()

		_ = handler.Command.RedisRepo.Del(ctx, key)

		return http.WithError(c, err)
	}

	spanValidateBalances.End()

	status := transaction.Status{
		Code:        transactionStatus,
		Description: &transactionStatus,
	}

	var parentTransactionID *string

	if transactionID != uuid.Nil {
		value := transactionID.String()
		parentTransactionID = &value
	}

	fromTo = append(fromTo, handler.handleAccountFields(parserDSL.Send.Source.From, false)...)
	if transactionStatus != constant.PENDING {
		fromTo = append(fromTo, handler.handleAccountFields(parserDSL.Send.Distribute.To, false)...)
	}

	tran := &transaction.Transaction{
		ID:                       libCommons.GenerateUUIDv7().String(),
		ParentTransactionID:      parentTransactionID,
		OrganizationID:           organizationID.String(),
		LedgerID:                 ledgerID.String(),
		Description:              parserDSL.Description,
		Status:                   status,
		Amount:                   &parserDSL.Send.Value,
		AssetCode:                parserDSL.Send.Asset,
		ChartOfAccountsGroupName: parserDSL.ChartOfAccountsGroupName,
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
		Route:                    parserDSL.Route,
		Metadata:                 parserDSL.Metadata,
	}

	var operations []*operation.Operation

	_, spanCreateTransactionOperations := tracer.Start(ctx, "handler.create_transaction.create_transaction_operations")

	spanCreateTransactionOperations.SetAttributes(attributes...)

	for _, blc := range balances {
		for i := range fromTo {
			if blc.Alias == fromTo[i].AccountAlias {
				logger.Infof("Creating operation for account id: %s", blc.ID)

				balance := operation.Balance{
					Available: &blc.Available,
					OnHold:    &blc.OnHold,
				}

				amt, bat, er := libTransaction.ValidateFromToOperation(fromTo[i], *validate, blc.ConvertToLibBalance())
				if er != nil {
					libOpentelemetry.HandleSpanError(&spanCreateTransactionOperations, "Failed to validate balances", er)
					spanCreateTransactionOperations.End()

					logger.Errorf("Failed to validate balance: %v", er.Error())

					return http.WithError(c, err)
				}

				amount := operation.Amount{
					Value: &amt.Value,
				}

				balanceAfter := operation.Balance{
					Available: &bat.Available,
					OnHold:    &bat.OnHold,
				}

				descr := fromTo[i].Description
				if libCommons.IsNilOrEmpty(&fromTo[i].Description) {
					descr = parserDSL.Description
				}

				operations = append(operations, &operation.Operation{
					ID:              libCommons.GenerateUUIDv7().String(),
					TransactionID:   tran.ID,
					Description:     descr,
					Type:            amt.Operation,
					AssetCode:       parserDSL.Send.Asset,
					ChartOfAccounts: fromTo[i].ChartOfAccounts,
					Amount:          amount,
					Balance:         balance,
					BalanceAfter:    balanceAfter,
					BalanceID:       blc.ID,
					AccountID:       blc.AccountID,
					AccountAlias:    libTransaction.SplitAlias(blc.Alias),
					OrganizationID:  blc.OrganizationID,
					LedgerID:        blc.LedgerID,
					CreatedAt:       time.Now(),
					UpdatedAt:       time.Now(),
					Route:           fromTo[i].Route,
					Metadata:        fromTo[i].Metadata,
				})

				labels := map[string]string{
					"organization_id": organizationID.String(),
					"ledger_id":       ledgerID.String(),
				}

				metricFactory.Counter("transaction_created", libOpentelemetry.MetricOption{
					Description: "New transaction created",
					Unit:        "1",
				}).WithLabels(labels).Add(ctx, 1)
			}
		}
	}

	spanCreateTransactionOperations.End()

	tran.Source = validate.Sources
	tran.Destination = validate.Destinations
	tran.Operations = operations

	err = handler.Command.TransactionExecute(ctx, organizationID, ledgerID, &parserDSL, validate, balances, tran)
	if err != nil {
		err := pkg.ValidateBusinessError(constant.ErrMessageBrokerUnavailable, "failed to update BTO")

		libOpentelemetry.HandleSpanError(&span, "failed to update BTO", err)

		logger.Errorf("failed to update BTO - transaction: %s - Error: %v", tran.ID, err)

		return http.WithError(c, err)
	}

	go handler.Command.SetValueOnExistingIdempotencyKey(ctx, organizationID, ledgerID, key, hash, *tran, ttl)

	go handler.Command.SendLogTransactionAuditQueue(ctx, operations, organizationID, ledgerID, tran.IDtoUUID())

	return http.Created(c, tran)
}

// commitOrCancelTransaction func that is responsible to commit or cancel transaction.
func (handler *TransactionHandler) commitOrCancelTransaction(c *fiber.Ctx, logger libLog.Logger, tran *transaction.Transaction, transactionStatus string) error {
	ctx := c.UserContext()
	tracer := libCommons.NewTracerFromContext(ctx)
	metricFactory := libCommons.NewMetricFactoryFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.commit_or_cancel_transaction")
	defer span.End()

	attributes := []attribute.KeyValue{
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", tran.OrganizationID),
		attribute.String("app.request.ledger_id", tran.LedgerID),
		attribute.String("app.request.transaction_id", tran.ID),
		attribute.String("app.request.transaction.transaction_status", transactionStatus),
	}

	span.SetAttributes(attributes...)

	organizationID := uuid.MustParse(tran.OrganizationID)
	ledgerID := uuid.MustParse(tran.LedgerID)

	parserDSL := tran.Body

	var fromTo []libTransaction.FromTo

	fromTo = append(fromTo, handler.handleAccountFields(parserDSL.Send.Source.From, true)...)
	if transactionStatus != constant.CANCELED {
		fromTo = append(fromTo, handler.handleAccountFields(parserDSL.Send.Distribute.To, true)...)
	}

	if tran.Status.Code != constant.PENDING {
		err := pkg.ValidateBusinessError(constant.ErrCommitTransactionNotPending, "ValidateTransactionNotPending")

		libOpentelemetry.HandleSpanError(&span, "Transaction is not pending", err)

		logger.Errorf("Failed, Transaction: %s is not pending, Error: %s", tran.ID, err.Error())

		return http.WithError(c, err)
	}

	validate, err := libTransaction.ValidateSendSourceAndDistribute(parserDSL, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to validate send source and distribute", err)

		logger.Error("Validation failed:", err.Error())

		if err.Error() == constant.ErrTransactionAmbiguous.Error() {
			err = pkg.ValidateBusinessError(constant.ErrTransactionAmbiguous, "ValidateSendSourceAndDistribute")
		} else if err.Error() == constant.ErrTransactionValueMismatch.Error() {
			err = pkg.ValidateBusinessError(constant.ErrTransactionValueMismatch, "ValidateSendSourceAndDistribute")
		}

		return http.WithError(c, err)
	}

	_, spanGetBalances := tracer.Start(ctx, "handler.create_transaction.get_balances")

	spanGetBalances.SetAttributes(attributes...)

	balances, err := handler.Query.GetBalances(ctx, organizationID, ledgerID, validate, transactionStatus)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanGetBalances, "Failed to get balances", err)
		spanGetBalances.End()

		logger.Errorf("Failed to get balances: %v", err.Error())

		return http.WithError(c, err)
	}

	spanGetBalances.End()

	status := transaction.Status{
		Code:        transactionStatus,
		Description: &transactionStatus,
	}

	tran.Status = status
	tran.UpdatedAt = time.Now()

	fromTo = append(fromTo, handler.handleAccountFields(parserDSL.Send.Source.From, false)...)
	if transactionStatus != constant.CANCELED {
		fromTo = append(fromTo, handler.handleAccountFields(parserDSL.Send.Distribute.To, false)...)
	}

	var operations []*operation.Operation

	var preBalances []*mmodel.Balance

	_, spanCreateTransactionOperations := tracer.Start(ctx, "handler.commit_or_cancel_transaction.create_transaction_operations")

	spanCreateTransactionOperations.SetAttributes(attributes...)

	for _, blc := range balances {
		for i := range fromTo {
			if blc.Alias == fromTo[i].AccountAlias {
				logger.Infof("Creating operation for account id: %s", blc.ID)

				preBalances = append(preBalances, blc)

				amt, bat, er := libTransaction.ValidateFromToOperation(fromTo[i], *validate, blc.ConvertToLibBalance())
				if er != nil {
					libOpentelemetry.HandleSpanError(&spanCreateTransactionOperations, "Failed to validate balances", er)
					spanCreateTransactionOperations.End()

					logger.Errorf("Failed to validate balance: %v", er.Error())

					return http.WithError(c, err)
				}

				amount := operation.Amount{
					Value: &amt.Value,
				}

				balance := operation.Balance{
					Available: &blc.Available,
					OnHold:    &blc.OnHold,
				}

				balanceAfter := operation.Balance{
					Available: &bat.Available,
					OnHold:    &bat.OnHold,
				}

				descr := fromTo[i].Description
				if libCommons.IsNilOrEmpty(&fromTo[i].Description) {
					descr = parserDSL.Description
				}

				operations = append(operations, &operation.Operation{
					ID:              libCommons.GenerateUUIDv7().String(),
					TransactionID:   tran.ID,
					Description:     descr,
					Type:            amt.Operation,
					AssetCode:       parserDSL.Send.Asset,
					ChartOfAccounts: fromTo[i].ChartOfAccounts,
					Amount:          amount,
					Balance:         balance,
					BalanceAfter:    balanceAfter,
					BalanceID:       blc.ID,
					AccountID:       blc.AccountID,
					AccountAlias:    libTransaction.SplitAlias(blc.Alias),
					OrganizationID:  blc.OrganizationID,
					LedgerID:        blc.LedgerID,
					CreatedAt:       time.Now(),
					UpdatedAt:       time.Now(),
					Route:           fromTo[i].Route,
					Metadata:        fromTo[i].Metadata,
				})

				labels := map[string]string{
					"organization_id": organizationID.String(),
					"ledger_id":       ledgerID.String(),
				}

				metricFactory.Counter("transaction_created", libOpentelemetry.MetricOption{
					Description: "New transaction created",
					Unit:        "1",
				}).WithLabels(labels).Add(ctx, 1)
			}
		}
	}

	spanCreateTransactionOperations.End()

	tran.Source = validate.Sources
	tran.Destination = validate.Destinations
	tran.Operations = operations

	err = handler.Command.TransactionExecute(ctx, organizationID, ledgerID, &parserDSL, validate, preBalances, tran)
	if err != nil {
		err := pkg.ValidateBusinessError(constant.ErrMessageBrokerUnavailable, "failed to update BTO")

		libOpentelemetry.HandleSpanError(&span, "failed to update BTO", err)

		logger.Errorf("failed to update BTO - transaction: %s - Error: %v", tran.ID, err)

		return http.WithError(c, err)
	}

	go handler.Command.SendLogTransactionAuditQueue(ctx, operations, organizationID, ledgerID, tran.IDtoUUID())

	return http.Created(c, tran)
}
