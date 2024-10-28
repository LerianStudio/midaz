package http

import (
	"context"
	"github.com/LerianStudio/midaz/common/constant"
	"github.com/google/uuid"
	"net/http"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/gold/transaction"
	gold "github.com/LerianStudio/midaz/common/gold/transaction/model"
	"github.com/LerianStudio/midaz/common/mgrpc/account"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/common/mpostgres"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/transaction/internal/app/command"
	"github.com/LerianStudio/midaz/components/transaction/internal/app/query"
	v "github.com/LerianStudio/midaz/components/transaction/internal/domain/account"
	o "github.com/LerianStudio/midaz/components/transaction/internal/domain/operation"
	t "github.com/LerianStudio/midaz/components/transaction/internal/domain/transaction"
	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
)

// TransactionHandler struct that handle transaction
type TransactionHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateTransactionJSON method that create transaction using JSON
func (handler *TransactionHandler) CreateTransactionJSON(p any, c *fiber.Ctx) error {
	logger := mlog.NewLoggerFromContext(c.UserContext())

	input := p.(*t.CreateTransactionInput)
	parserDSL := input.FromDSl()
	logger.Infof("Request to create an transaction with details: %#v", parserDSL)

	return handler.createTransaction(c, logger, *parserDSL)
}

// CreateTransactionDSL method that create transaction using DSL
func (handler *TransactionHandler) CreateTransactionDSL(c *fiber.Ctx) error {
	logger := mlog.NewLoggerFromContext(c.UserContext())

	_ = commonHTTP.ValidateParameters(c.Queries())

	dsl, err := commonHTTP.GetFileFromHeader(c)
	if err != nil {
		logger.Error("Failed to get file from Header: ", err.Error())

		return c.Status(http.StatusBadRequest).JSON(err)
	}

	errListener := transaction.Validate(dsl)
	if errListener != nil && len(errListener.Errors) > 0 {
		err := common.ValidationError{
			Code:    "0017",
			Title:   "Invalid Script Error",
			Message: "The script provided in the request is invalid or in an unsupported format. Please verify the script and try again.",
		}

		return c.Status(http.StatusBadRequest).JSON(err)
	}

	parsed := transaction.Parse(dsl)

	parserDSL, ok := parsed.(gold.Transaction)
	if !ok {
		err := common.ValidationError{
			Code:    "0017",
			Title:   "Invalid Script Error",
			Message: "The script provided in the request is invalid or in an unsupported format. Please verify the script and try again.",
		}

		return c.Status(http.StatusBadRequest).JSON(err)
	}

	return handler.createTransaction(c, logger, parserDSL)
}

// CreateTransactionTemplate method that create transaction template
func (handler *TransactionHandler) CreateTransactionTemplate(p any, c *fiber.Ctx) error {
	logger := mlog.NewLoggerFromContext(c.UserContext())

	payload := p.(*t.InputDSL)
	logger.Infof("Request to create an transaction with details: %#v", payload)

	return commonHTTP.Created(c, payload)
}

// CommitTransaction method that commit transaction created before
func (handler *TransactionHandler) CommitTransaction(c *fiber.Ctx) error {
	logger := mlog.NewLoggerFromContext(c.UserContext())

	return commonHTTP.Created(c, logger)
}

// RevertTransaction method that revert transaction created before
func (handler *TransactionHandler) RevertTransaction(c *fiber.Ctx) error {
	logger := mlog.NewLoggerFromContext(c.UserContext())

	return commonHTTP.Created(c, logger)
}

// UpdateTransaction method that patch transaction created before
func (handler *TransactionHandler) UpdateTransaction(p any, c *fiber.Ctx) error {
	logger := mlog.NewLoggerFromContext(c.UserContext())

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")
	transactionID := c.Params("transaction_id")

	logger.Infof("Initiating update of Transaction with Organization ID: %s, Ledger ID: %s and ID: %s", organizationID, ledgerID, transactionID)

	payload := p.(*t.UpdateTransactionInput)
	logger.Infof("Request to update an Transaction with details: %#v", payload)

	_, err := handler.Command.UpdateTransaction(c.Context(), organizationID, ledgerID, transactionID, payload)
	if err != nil {
		logger.Errorf("Failed to update Transaction with ID: %s, Error: %s", transactionID, err.Error())
		return commonHTTP.WithError(c, err)
	}

	trans, err := handler.Query.GetTransactionByID(c.Context(), organizationID, ledgerID, transactionID)
	if err != nil {
		logger.Errorf("Failed to retrieve Transaction with ID: %s, Error: %s", transactionID, err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully updated Transaction with Organization ID: %s, Ledger ID: %s and ID: %s", organizationID, ledgerID, transactionID)

	return commonHTTP.OK(c, trans)
}

// GetTransaction method that get transaction created before
func (handler *TransactionHandler) GetTransaction(c *fiber.Ctx) error {
	logger := mlog.NewLoggerFromContext(c.UserContext())

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")
	transactionID := c.Params("transaction_id")

	tran, err := handler.Query.GetTransactionByID(c.Context(), organizationID, ledgerID, transactionID)
	if err != nil {
		logger.Errorf("Failed to retrieve Transaction with ID: %s, Error: %s", transactionID, err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Transaction with ID: %s", transactionID)

	return commonHTTP.OK(c, tran)
}

func (handler *TransactionHandler) GetAllTransactions(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := mlog.NewLoggerFromContext(c.UserContext())

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")

	headerParams := commonHTTP.ValidateParameters(c.Queries())

	pagination := mpostgres.Pagination{
		Limit: headerParams.Limit,
		Page:  headerParams.Page,
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Transactions by metadata")

		trans, err := handler.Query.GetAllMetadataTransactions(ctx, organizationID, ledgerID, *headerParams)
		if err != nil {
			logger.Errorf("Failed to retrieve all Transactions, Error: %s", err.Error())
			return commonHTTP.WithError(c, err)
		}

		logger.Infof("Successfully retrieved all Transactions by metadata")

		pagination.SetItems(trans)

		return commonHTTP.OK(c, pagination)
	}

	logger.Infof("Initiating retrieval of all Transactions ")

	headerParams.Metadata = &bson.M{}

	trans, err := handler.Query.GetAllTransactions(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		logger.Errorf("Failed to retrieve all Transactions, Error: %s", err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Transactions")

	pagination.SetItems(trans)

	return commonHTTP.OK(c, pagination)
}

// createTransaction func that received struct from DSL parsed and create Transaction
func (handler *TransactionHandler) createTransaction(c *fiber.Ctx, logger mlog.Logger, parserDSL gold.Transaction) error {
	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")

	validate, err := v.ValidateSendSourceAndDistribute(parserDSL)
	if err != nil {
		logger.Error("Validation failed:", err.Error())
		return commonHTTP.WithError(c, err)
	}

	token := commonHTTP.GetTokenHeader(c)

	accounts, err := handler.getAccounts(c.Context(), logger, token, organizationID, ledgerID, validate.Aliases)
	if err != nil {
		return commonHTTP.WithError(c, err)
	}

	err = v.ValidateAccounts(*validate, accounts)
	if err != nil {
		return commonHTTP.WithError(c, err)
	}

	tran, err := handler.Command.CreateTransaction(c.Context(), organizationID, ledgerID, &parserDSL)
	if err != nil {
		logger.Error("Failed to create transaction", err.Error())
		return commonHTTP.WithError(c, err)
	}

	tran.Source = validate.Sources
	tran.Destination = validate.Destinations

	e := make(chan error)
	result := make(chan []*o.Operation)

	var operations []*o.Operation

	go handler.Command.CreateOperation(c.Context(), accounts, tran.ID, &parserDSL, *validate, result, e)
	select {
	case ops := <-result:
		operations = append(operations, ops...)
	case err := <-e:
		logger.Error("Failed to create operations: ", err.Error())
		return commonHTTP.WithError(c, err)
	}

	err = handler.processAccounts(c.Context(), logger, *validate, token, organizationID, ledgerID, accounts)
	if err != nil {
		return commonHTTP.WithError(c, err)
	}

	//TODO: use event driven and broken and parts
	_, err = handler.Command.UpdateTransactionStatus(c.Context(), organizationID, ledgerID, tran.ID, constant.APPROVED)
	if err != nil {
		logger.Errorf("Failed to update Transaction with ID: %s, Error: %s", tran.ID, err.Error())
		return commonHTTP.WithError(c, err)
	}

	//TODO: use event driven and broken and parts
	tran, err = handler.Query.GetTransactionByID(c.Context(), organizationID, ledgerID, tran.ID)
	if err != nil {
		logger.Errorf("Failed to retrieve Transaction with ID: %s, Error: %s", tran.ID, err.Error())
		return commonHTTP.WithError(c, err)
	}

	tran.Operations = operations

	logger.Infof("Successfully updated Transaction with Organization ID: %s, Ledger ID: %s and ID: %s", organizationID, ledgerID, tran.ID)

	return commonHTTP.Created(c, tran)
}

// getAccounts is a function that split aliases and ids, call the properly function and return Accounts
func (handler *TransactionHandler) getAccounts(c context.Context, logger mlog.Logger, token, organizationID, ledgerID string, input []string) ([]*account.Account, error) {
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
		gRPCAccounts, err := handler.Query.AccountGRPCRepo.GetAccountsByIds(c, token, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), ids)
		if err != nil {
			logger.Error("Failed to get account gRPC by ids on Ledger", err.Error())
			return nil, err
		}

		accounts = append(accounts, gRPCAccounts.GetAccounts()...)
	}

	if len(aliases) > 0 {
		gRPCAccounts, err := handler.Query.AccountGRPCRepo.GetAccountsByAlias(c, token, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), aliases)
		if err != nil {
			logger.Error("Failed to get account by alias gRPC on Ledger", err.Error())
			return nil, err
		}

		accounts = append(accounts, gRPCAccounts.GetAccounts()...)
	}

	return accounts, nil
}

// processAccounts is a function that adjust balance on Accounts
func (handler *TransactionHandler) processAccounts(c context.Context, logger mlog.Logger, validate v.Responses, token, organizationID, ledgerID string, accounts []*account.Account) error {
	e := make(chan error)
	result := make(chan []*account.Account)

	var update []*account.Account

	go v.UpdateAccounts(constant.DEBIT, validate.From, accounts, result, e)
	select {
	case r := <-result:
		update = append(update, r...)
	case err := <-e:
		return err
	}

	go v.UpdateAccounts(constant.CREDIT, validate.To, accounts, result, e)
	select {
	case r := <-result:
		update = append(update, r...)
	case err := <-e:
		return err
	}

	acc, err := handler.Command.AccountGRPCRepo.UpdateAccounts(c, token, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), update)
	if err != nil {
		logger.Error("Failed to update accounts gRPC on Ledger", err.Error())
		return err
	}

	for _, a := range acc.Accounts {
		logger.Infof(a.UpdatedAt)
	}

	return nil
}
