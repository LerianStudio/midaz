package ports

import (
	"context"
	"net/http"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/gold/transaction"
	gold "github.com/LerianStudio/midaz/common/gold/transaction/model"
	"github.com/LerianStudio/midaz/common/mgrpc/account"
	"github.com/LerianStudio/midaz/common/mlog"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/transaction/internal/app/command"
	"github.com/LerianStudio/midaz/components/transaction/internal/app/query"
	o "github.com/LerianStudio/midaz/components/transaction/internal/domain/operation"
	t "github.com/LerianStudio/midaz/components/transaction/internal/domain/transaction"
	"github.com/gofiber/fiber/v2"
)

// TransactionHandler struct that handle transaction
type TransactionHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateTransaction method that create transaction
func (handler *TransactionHandler) CreateTransaction(c *fiber.Ctx) error {
	logger := mlog.NewLoggerFromContext(c.UserContext())

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")

	dsl, err := commonHTTP.GetFileFromHeader(c)
	if err != nil {
		logger.Error("Failed to validate and parse transaction", err.Error())
		return commonHTTP.WithError(c, err)
	}

	errListener := transaction.Validate(dsl)
	if errListener != nil && len(errListener.Errors) > 0 {
		var err []fiber.Map
		for _, e := range errListener.Errors {
			err = append(err, fiber.Map{
				"line":    e.Line,
				"column":  e.Column,
				"message": e.Message,
				"source":  errListener.Source,
			})
		}

		return c.Status(http.StatusBadRequest).JSON(err)
	}

	parsed := transaction.Parse(dsl)

	logger.Infof("Transaction parsed and validated")

	parserDSL, ok := parsed.(gold.Transaction)
	if !ok {
		return c.Status(http.StatusBadRequest).JSON("Type assertion failed")
	}

	source := make([]string, len(parserDSL.Send.Source.From))

	for i, fr := range parserDSL.Send.Source.From {
		source[i] = fr.Account
	}

	destination := make([]string, len(parserDSL.Distribute.To))

	for i, ot := range parserDSL.Distribute.To {
		destination[i] = ot.Account
	}

	var alias []string
	alias = append(alias, destination...)
	alias = append(alias, source...)

	accounts, err := handler.processAccounts(c.Context(), logger, alias)
	if err != nil {
		return commonHTTP.WithError(c, err)
	}

	tran, err := handler.Command.CreateTransaction(c.Context(), organizationID, ledgerID, &parserDSL)
	if err != nil {
		logger.Error("Failed to create transaction", err.Error())
		return commonHTTP.WithError(c, err)
	}

	tran.Source = source
	tran.Destination = destination

	e := make(chan error)
	result := make(chan []*o.Operation)

	var operations []*o.Operation

	go handler.Command.CreateOperation(c.Context(), accounts, tran, &parserDSL, result, e)
	select {
	case ops := <-result:
		operations = append(operations, ops...)
	case err := <-e:
		logger.Error("Failed to create operations: ", err.Error())
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

// ProcessAccounts is a function that split alias and isd, call the properly function and return Accounts
func (handler *TransactionHandler) processAccounts(c context.Context, logger mlog.Logger, input []string) ([]*account.Account, error) {
	var ids []string

	var alias []string

	for _, item := range input {
		if common.IsUUID(item) {
			ids = append(ids, item)
		} else {
			alias = append(alias, item)
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

	if len(alias) > 0 {
		gRPCAccounts, err := handler.Query.AccountGRPCRepo.GetAccountsByAlias(c, alias)
		if err != nil {
			logger.Error("Failed to get account by alias gRPC on Ledger", err.Error())
			return nil, err
		}

		accounts = append(accounts, gRPCAccounts.GetAccounts()...)
	}

	return accounts, nil
}
