package ports

import (
	"net/http"

	"github.com/LerianStudio/midaz/common/gold/transaction"
	gold "github.com/LerianStudio/midaz/common/gold/transaction/model"
	"github.com/LerianStudio/midaz/common/mlog"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/transaction/internal/app/command"
	"github.com/LerianStudio/midaz/components/transaction/internal/app/query"
	o "github.com/LerianStudio/midaz/components/transaction/internal/domain/operation"
	t "github.com/LerianStudio/midaz/components/transaction/internal/domain/transaction"
	"github.com/gofiber/fiber/v2"
)

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

	transactionParsed, ok := parsed.(gold.Transaction)
	if !ok {
		return c.Status(http.StatusBadRequest).JSON("Type assertion failed")
	}

	from := transactionParsed.Send.Source.From
	source := make([]string, len(from))
	for _, fr := range from {
		source = append(source, fr.Account)
	}

	to := transactionParsed.Distribute.To
	destination := make([]string, len(to))
	for _, ot := range to {
		destination = append(destination, ot.Account)
	}
	alias := make([]string, len(source)+len(destination))
	alias = append(alias, source...)
	alias = append(alias, destination...)

	//if common.IsUUID() {
	//	gRPCAccounts, err := handler.Query.AccountGRPCRepo.GetAccountsByIds(c.Context(), uuids)
	//} esle {
	gRPCAccounts, err := handler.Query.AccountGRPCRepo.GetAccountsByAlias(c.Context(), alias)
	if err != nil {
		logger.Error("Failed to get account gRPC on Ledger", err.Error())
		return commonHTTP.WithError(c, err)
	}

	tran, err := handler.Command.CreateTransaction(c.Context(), organizationID, ledgerID, &transactionParsed)
	if err != nil {
		logger.Error("Failed to create transaction", err.Error())
		return commonHTTP.WithError(c, err)
	}

	tran.Source = source
	tran.Destination = destination

	e := make(chan error)
	result := make(chan []*o.Operation)
	go handler.Command.CreateOperation(c.Context(), gRPCAccounts, tran, from, result, e)
	select {
	case of := <-result:
		for _, fo := range of {
			logger.Info(fo.ID)
		}
	case err := <-e:
		logger.Error("Failed to create FROM operation: ", err.Error())
		return commonHTTP.WithError(c, err)
	}

	go handler.Command.CreateOperation(c.Context(), gRPCAccounts, tran, to, result, e)
	select {
	case to := <-result:
		for _, ot := range to {
			logger.Info(ot.ID)
		}
	case err := <-e:
		logger.Error("Failed to create TO operation: ", err.Error())
		return commonHTTP.WithError(c, err)
	}

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

	return commonHTTP.Created(c, logger)
}
