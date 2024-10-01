package ports

import (
	"net/http"

	"github.com/LerianStudio/midaz/common/gold/transaction"
	gold "github.com/LerianStudio/midaz/common/gold/transaction/model"
	"github.com/LerianStudio/midaz/common/mlog"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/transaction/internal/app/command"
	"github.com/LerianStudio/midaz/components/transaction/internal/app/query"
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

	//status = {"CREATED", "APPROVED", "PRE_APPROVED", "SENT", "CANCELED", "DECLINED"}

	ret, err := handler.Query.AccountGRPCRepo.GetAccountsByAlias(c.Context(), nil)
	if err != nil {
		logger.Error("Failed to get account gRPC on Ledger", err.Error())
		return commonHTTP.WithError(c, err)
	}

	for _, ac := range ret.Accounts {
		logger.Infof("Account %s founded on Ledger", ac.Alias)
	}

	entity, err := handler.Command.CreateTransaction(c.Context(), &transactionParsed)
	if err != nil {
		logger.Error("Failed to create transaction", err.Error())
		return commonHTTP.WithError(c, err)
	}

	return commonHTTP.Created(c, entity)
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
