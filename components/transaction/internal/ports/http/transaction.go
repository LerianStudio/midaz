package http

import (
	"context"
	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/gold/transaction"
	gold "github.com/LerianStudio/midaz/common/gold/transaction/model"
	"github.com/LerianStudio/midaz/common/mgrpc/account"
	"github.com/LerianStudio/midaz/common/mlog"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/transaction/internal/app/command"
	"github.com/LerianStudio/midaz/components/transaction/internal/app/query"
	v "github.com/LerianStudio/midaz/components/transaction/internal/domain/account"
	o "github.com/LerianStudio/midaz/components/transaction/internal/domain/operation"
	t "github.com/LerianStudio/midaz/components/transaction/internal/domain/transaction"
	"github.com/gofiber/fiber/v2"
	"net/http"
)

// TransactionHandler struct that handle transaction
type TransactionHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateTransaction method that create transaction
func (handler *TransactionHandler) CreateTransaction(c *fiber.Ctx) error {
	logger := mlog.NewLoggerFromContext(c.UserContext())

	_ = commonHTTP.ValidateParameters(c.Queries())

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")

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

	validate, err := v.ValidateSendSourceAndDistribute(parserDSL)
	if err != nil {
		logger.Error("Validation failed:", err.Error())
		return commonHTTP.WithError(c, err)
	}

	accounts, err := handler.getAccounts(c.Context(), logger, validate.Aliases)
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

	err = handler.processAccounts(c.Context(), logger, *validate, accounts)
	if err != nil {
		return commonHTTP.WithError(c, err)
	}

	tran.Operations = operations

	return commonHTTP.Created(c, tran)
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

// getAccounts is a function that split aliases and ids, call the properly function and return Accounts
func (handler *TransactionHandler) getAccounts(c context.Context, logger mlog.Logger, input []string) ([]*account.Account, error) {
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
		gRPCAccounts, err := handler.Query.AccountGRPCRepo.GetAccountsByIds(c, ids)
		if err != nil {
			logger.Error("Failed to get account gRPC by ids on Ledger", err.Error())
			return nil, err
		}

		accounts = append(accounts, gRPCAccounts.GetAccounts()...)
	}

	if len(aliases) > 0 {
		gRPCAccounts, err := handler.Query.AccountGRPCRepo.GetAccountsByAlias(c, aliases)
		if err != nil {
			logger.Error("Failed to get account by alias gRPC on Ledger", err.Error())
			return nil, err
		}

		accounts = append(accounts, gRPCAccounts.GetAccounts()...)
	}

	return accounts, nil
}

// processAccounts is a function that adjust balance on Accounts
func (handler *TransactionHandler) processAccounts(c context.Context, logger mlog.Logger, validate v.Responses, accounts []*account.Account) error {
	for _, acc := range accounts {
		var balance account.Balance

		for _, fo := range validate.From {
			_, b, err := v.OperateAmounts(fo, acc.Balance, "sub")
			if err != nil {
				return err
			}

			balance = account.Balance{
				Available: *b.Available,
				Scale:     *b.Scale,
				OnHold:    *b.OnHold,
			}

		}

		for _, to := range validate.To {
			_, b, err := v.OperateAmounts(to, acc.Balance, "add")
			if err != nil {
				return err
			}

			balance = account.Balance{
				Available: *b.Available,
				Scale:     *b.Scale,
				OnHold:    *b.OnHold,
			}
		}

		acc.Balance = &balance

	}

	/*
		acc, err := handler.Command.AccountGRPCRepo.UpdateAccounts(c, accounts)
		if err != nil {
			logger.Error("Failed to update accounts gRPC on Ledger", err.Error())
			return err
		}

		for _, a := range acc.Accounts {
			logger.Infof(a.UpdatedAt)
		}
	*/

	return nil
}
