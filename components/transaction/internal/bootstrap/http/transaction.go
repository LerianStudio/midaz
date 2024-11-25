package http

import (
	"context"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/constant"
	goldTransaction "github.com/LerianStudio/midaz/common/gold/transaction"
	goldModel "github.com/LerianStudio/midaz/common/gold/transaction/model"
	"github.com/LerianStudio/midaz/common/mgrpc/account"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"github.com/LerianStudio/midaz/common/mpostgres"
	"github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/database/postgres/operation"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/database/postgres/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/query"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.opentelemetry.io/otel/trace"
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
//
//	@Param			organization_id	path		string								true	"Organization ID"
//	@Param			ledger_id		path		string								true	"Ledger ID"
//
//	@Param			transaction		body		transaction.CreateTransactionInput	true	"Transaction Input"
//	@Param			Midaz-Id		header		string								false	"Request ID"
//	@Success		200				{object}	transaction.Transaction
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/json [post]
func (handler *TransactionHandler) CreateTransactionJSON(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_transaction")
	defer span.End()

	c.SetUserContext(ctx)

	input := p.(*transaction.CreateTransactionInput)
	parserDSL := input.FromDSl()
	logger.Infof("Request to create an transaction with details: %#v", parserDSL)

	response := handler.createTransaction(c, logger, *parserDSL)

	return response
}

// CreateTransactionDSL method that create transaction using DSL
//
//	@Summary		Create a Transaction using DSL
//	@Description	Create a Transaction with the input DSL file
//	@Tags			Transactions
//	@Accept			mpfd
//	@Produce		json
//
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//
//	@Param			transaction		formData	file	true	"Transaction DSL file"
//	@Param			Midaz-Id		header		string	false	"Request ID"
//	@Success		200				{object}	transaction.Transaction
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/dsl [post]
func (handler *TransactionHandler) CreateTransactionDSL(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_transaction_dsl")
	defer span.End()

	c.SetUserContext(ctx)

	_ = http.ValidateParameters(c.Queries())

	dsl, err := http.GetFileFromHeader(c)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get file from Header", err)

		logger.Error("Failed to get file from Header: ", err.Error())

		return http.WithError(c, err)
	}

	errListener := goldTransaction.Validate(dsl)
	if errListener != nil && len(errListener.Errors) > 0 {
		err := common.ValidateBusinessError(constant.ErrInvalidDSLFileFormat, reflect.TypeOf(transaction.Transaction{}).Name())

		mopentelemetry.HandleSpanError(&span, "Failed to validate script in DSL", err)

		return http.WithError(c, err)
	}

	parsed := goldTransaction.Parse(dsl)

	parserDSL, ok := parsed.(goldModel.Transaction)
	if !ok {
		err := common.ValidateBusinessError(constant.ErrInvalidDSLFileFormat, reflect.TypeOf(transaction.Transaction{}).Name())

		mopentelemetry.HandleSpanError(&span, "Failed to parse script in DSL", err)

		return http.WithError(c, err)
	}

	response := handler.createTransaction(c, logger, parserDSL)

	return response
}

// CreateTransactionTemplate method that create transaction template
//
// TODO: Implement this method and the swagger documentation related to it
func (handler *TransactionHandler) CreateTransactionTemplate(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	tracer.Start(ctx, "handler.create_transaction_template")

	payload := p.(*transaction.InputDSL)
	logger.Infof("Request to create an transaction with details: %#v", payload)

	return http.Created(c, payload)
}

// CommitTransaction method that commit transaction created before
//
// TODO: Implement this method and the swagger documentation related to it
func (handler *TransactionHandler) CommitTransaction(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.commit_transaction")
	defer span.End()

	return http.Created(c, logger)
}

// RevertTransaction method that revert transaction created before
//
// TODO: Implement this method and the swagger documentation related to it
func (handler *TransactionHandler) RevertTransaction(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.revert_transaction")
	defer span.End()

	return http.Created(c, logger)
}

// UpdateTransaction method that patch transaction created before
//
//	@Summary		Update a Transaction
//	@Description	Update a Transaction with the input payload
//	@Tags			Transactions
//	@Accept			json
//	@Produce		json
//
//	@Param			organization_id	path		string								true	"Organization ID"
//	@Param			ledger_id		path		string								true	"Ledger ID"
//	@Param			transaction_id	path		string								true	"Transaction ID"
//
//	@Param			transaction		body		transaction.UpdateTransactionInput	true	"Transaction Input"
//	@Param			Midaz-Id		header		string								false	"Request ID"
//	@Success		200				{object}	transaction.Transaction
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id} [patch]
func (handler *TransactionHandler) UpdateTransaction(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_transaction")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	transactionID := c.Locals("transaction_id").(uuid.UUID)

	logger.Infof("Initiating update of Transaction with Organization ID: %s, Ledger ID: %s and ID: %s", organizationID.String(), ledgerID.String(), transactionID.String())

	payload := p.(*transaction.UpdateTransactionInput)
	logger.Infof("Request to update an Transaction with details: %#v", payload)

	_, err := handler.Command.UpdateTransaction(ctx, organizationID, ledgerID, transactionID, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update transaction on command", err)

		logger.Errorf("Failed to update Transaction with ID: %s, Error: %s", transactionID.String(), err.Error())

		return http.WithError(c, err)
	}

	trans, err := handler.Query.GetTransactionByID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve transaction on query", err)

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
//
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			transaction_id	path		string	true	"Transaction ID"
//
//	@Param			Midaz-Id		header		string	false	"Request ID"
//	@Success		200				{object}	transaction.Transaction
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id} [get]
func (handler *TransactionHandler) GetTransaction(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_transaction")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	transactionID := c.Locals("transaction_id").(uuid.UUID)

	tran, err := handler.Query.GetTransactionByID(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve transaction on query", err)

		logger.Errorf("Failed to retrieve Transaction with ID: %s, Error: %s", transactionID.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Transaction with ID: %s", transactionID.String())

	return http.OK(c, tran)
}

// GetAllTransactions method that get all transactions created before
//
//	@Summary		Get all Transactions
//	@Description	Get all Transactions with the input metadata or without metadata
//	@Tags			Transactions
//	@Produce		json
//
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//
//	@Param			Midaz-Id		header		string	false	"Request ID"
//	@Success		200				{object}	mpostgres.Pagination{items=[]transaction.Transaction}
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions [get]
func (handler *TransactionHandler) GetAllTransactions(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_transactions")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	headerParams := http.ValidateParameters(c.Queries())

	pagination := mpostgres.Pagination{
		Limit: headerParams.Limit,
		Page:  headerParams.Page,
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Transactions by metadata")

		err := mopentelemetry.SetSpanAttributesFromStruct(&span, "headerParams", headerParams)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to convert metadata headerParams to JSON string", err)

			return http.WithError(c, err)
		}

		trans, err := handler.Query.GetAllMetadataTransactions(ctx, organizationID, ledgerID, *headerParams)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to retrieve all Transactions by metadata", err)

			logger.Errorf("Failed to retrieve all Transactions, Error: %s", err.Error())

			return http.WithError(c, err)
		}

		logger.Infof("Successfully retrieved all Transactions by metadata")

		pagination.SetItems(trans)

		return http.OK(c, pagination)
	}

	logger.Infof("Initiating retrieval of all Transactions ")

	headerParams.Metadata = &bson.M{}

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "headerParams", headerParams)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert headerParams to JSON string", err)

		return http.WithError(c, err)
	}

	trans, err := handler.Query.GetAllTransactions(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve all Transactions", err)

		logger.Errorf("Failed to retrieve all Transactions, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Transactions")

	pagination.SetItems(trans)

	return http.OK(c, pagination)
}

// createTransaction func that received struct from DSL parsed and create Transaction
func (handler *TransactionHandler) createTransaction(c *fiber.Ctx, logger mlog.Logger, parserDSL goldModel.Transaction) error {
	ctx := c.UserContext()
	tracer := common.NewTracerFromContext(ctx)

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	_, spanValidateDSL := tracer.Start(ctx, "handler.create_transaction_validate_dsl")

	validate, err := goldModel.ValidateSendSourceAndDistribute(parserDSL)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanValidateDSL, "Failed to validate send source and distribute", err)

		logger.Error("Validation failed:", err.Error())

		return http.WithError(c, err)
	}

	spanValidateDSL.End()

	token := http.GetTokenHeader(c)

	ctxGetAccounts, spanGetAccounts := tracer.Start(ctx, "handler.create_transaction.get_accounts")

	accounts, err := handler.getAccounts(ctxGetAccounts, logger, token, organizationID, ledgerID, validate.Aliases)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanGetAccounts, "Failed to get accounts", err)

		return http.WithError(c, err)
	}

	spanGetAccounts.End()

	_, spanValidateAccounts := tracer.Start(ctx, "handler.create_transaction.validate_accounts")

	err = goldModel.ValidateAccounts(*validate, accounts)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanValidateAccounts, "Failed to validate accounts", err)

		return http.WithError(c, err)
	}

	spanValidateAccounts.End()

	ctxCreateTransaction, spanCreateTransaction := tracer.Start(ctx, "handler.create_transaction.create_transaction")

	err = mopentelemetry.SetSpanAttributesFromStruct(&spanCreateTransaction, "payload_command_create_transaction", parserDSL)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanCreateTransaction, "Failed to convert parserDSL from struct to JSON string", err)

		return err
	}

	tran, err := handler.Command.CreateTransaction(ctxCreateTransaction, organizationID, ledgerID, &parserDSL)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanCreateTransaction, "Failed to create transaction", err)

		logger.Error("Failed to create transaction", err.Error())

		return http.WithError(c, err)
	}

	spanCreateTransaction.End()

	tran.Source = validate.Sources
	tran.Destination = validate.Destinations
	transactionID := uuid.MustParse(tran.ID)

	e := make(chan error)
	result := make(chan []*operation.Operation)

	var operations []*operation.Operation

	ctxCreateOperation, spanCreateOperation := tracer.Start(ctx, "handler.create_transaction.create_operation")

	go handler.Command.CreateOperation(ctxCreateOperation, accounts, tran.ID, &parserDSL, *validate, result, e)
	select {
	case ops := <-result:
		operations = append(operations, ops...)
	case err := <-e:
		mopentelemetry.HandleSpanError(&spanCreateOperation, "Failed to create operations", err)

		logger.Error("Failed to create operations: ", err.Error())

		return http.WithError(c, err)
	}

	spanCreateOperation.End()

	ctxProcessAccounts, spanProcessAccounts := tracer.Start(ctx, "handler.create_transaction.process_accounts")

	err = mopentelemetry.SetSpanAttributesFromStruct(&spanProcessAccounts, "payload_handler_process_accounts", accounts)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanProcessAccounts, "Failed to convert accounts from struct to JSON string", err)
	}

	err = handler.processAccounts(ctxProcessAccounts, logger, *validate, token, organizationID, ledgerID, accounts)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanProcessAccounts, "Failed to process accounts", err)

		return http.WithError(c, err)
	}

	spanProcessAccounts.End()

	ctxUpdateTransactionStatus, spanUpdateTransactionStatus := tracer.Start(ctx, "handler.create_transaction.update_transaction_status")

	//TODO: use event driven and broken and parts
	_, err = handler.Command.UpdateTransactionStatus(ctxUpdateTransactionStatus, organizationID, ledgerID, transactionID, constant.APPROVED)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanUpdateTransactionStatus, "Failed to update transaction status", err)

		logger.Errorf("Failed to update Transaction with ID: %s, Error: %s", tran.ID, err.Error())

		return http.WithError(c, err)
	}

	spanUpdateTransactionStatus.End()

	ctxGetTransaction, spanGetTransaction := tracer.Start(ctx, "handler.create_transaction.get_transaction")

	//TODO: use event driven and broken and parts
	tran, err = handler.Query.GetTransactionByID(ctxGetTransaction, organizationID, ledgerID, transactionID)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanGetTransaction, "Failed to retrieve transaction", err)

		logger.Errorf("Failed to retrieve Transaction with ID: %s, Error: %s", tran.ID, err.Error())

		return http.WithError(c, err)
	}

	spanGetTransaction.End()

	tran.Operations = operations

	logger.Infof("Successfully updated Transaction with Organization ID: %s, Ledger ID: %s and ID: %s", organizationID.String(), ledgerID.String(), tran.ID)

	return http.Created(c, tran)
}

// getAccounts is a function that split aliases and ids, call the properly function and return Accounts
func (handler *TransactionHandler) getAccounts(ctx context.Context, logger mlog.Logger, token string, organizationID, ledgerID uuid.UUID, input []string) ([]*account.Account, error) {
	span := trace.SpanFromContext(ctx)

	var ids []string

	var aliases []string

	for _, item := range input {
		if common.IsUUID(item) {
			ids = append(ids, item)
		} else {
			aliases = append(aliases, item)
		}
	}

	var accounts []*account.Account

	if len(ids) > 0 {
		gRPCAccounts, err := handler.Query.AccountGRPCRepo.GetAccountsByIds(ctx, token, organizationID, ledgerID, ids)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get account by ids gRPC on Ledger", err)

			logger.Error("Failed to get account gRPC by ids on Ledger", err.Error())

			return nil, err
		}

		accounts = append(accounts, gRPCAccounts.GetAccounts()...)
	}

	if len(aliases) > 0 {
		gRPCAccounts, err := handler.Query.AccountGRPCRepo.GetAccountsByAlias(ctx, token, organizationID, ledgerID, aliases)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get account by alias gRPC on Ledger", err)

			logger.Error("Failed to get account by alias gRPC on Ledger", err.Error())

			return nil, err
		}

		accounts = append(accounts, gRPCAccounts.GetAccounts()...)
	}

	return accounts, nil
}

// processAccounts is a function that adjust balance on Accounts
func (handler *TransactionHandler) processAccounts(ctx context.Context, logger mlog.Logger, validate goldModel.Responses, token string, organizationID, ledgerID uuid.UUID, accounts []*account.Account) error {
	span := trace.SpanFromContext(ctx)

	e := make(chan error)
	result := make(chan []*account.Account)

	var accountsToUpdate []*account.Account

	go goldModel.UpdateAccounts(constant.DEBIT, validate.From, accounts, result, e)
	select {
	case r := <-result:
		accountsToUpdate = append(accountsToUpdate, r...)
	case err := <-e:
		mopentelemetry.HandleSpanError(&span, "Failed to update debit accounts", err)

		return err
	}

	go goldModel.UpdateAccounts(constant.CREDIT, validate.To, accounts, result, e)
	select {
	case r := <-result:
		accountsToUpdate = append(accountsToUpdate, r...)
	case err := <-e:
		mopentelemetry.HandleSpanError(&span, "Failed to update credit accounts", err)

		return err
	}

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "payload_grpc_update_accounts", accountsToUpdate)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert accountsToUpdate from struct to JSON string", err)

		return err
	}

	acc, err := handler.Command.AccountGRPCRepo.UpdateAccounts(ctx, token, organizationID, ledgerID, accountsToUpdate)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update accounts gRPC on Ledger", err)

		logger.Error("Failed to update accounts gRPC on Ledger", err.Error())

		return err
	}

	for _, a := range acc.Accounts {
		logger.Infof(a.UpdatedAt)
	}

	return nil
}
