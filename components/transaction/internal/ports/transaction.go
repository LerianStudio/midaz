package ports

import (
	"net/http"

	"github.com/LerianStudio/midaz/common/gold/transaction"
	gold "github.com/LerianStudio/midaz/common/gold/transaction/model"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/common/mpostgres"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/transaction/internal/app/command"
	"github.com/LerianStudio/midaz/components/transaction/internal/app/query"
	t "github.com/LerianStudio/midaz/components/transaction/internal/domain/transaction"
	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
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

	transactionParsed, ok := parsed.(gold.Transaction)
	if !ok {
		return c.Status(http.StatusBadRequest).JSON("Type assertion failed")
	}

	alias := make([]string, len(transactionParsed.Send.Source.From)+len(transactionParsed.Distribute.To))
	for _, from := range transactionParsed.Send.Source.From {
		alias = append(alias, from.Account)
	}

	for _, to := range transactionParsed.Distribute.To {
		alias = append(alias, to.Account)
	}

	ret, err := handler.Query.AccountGRPCRepo.GetAccountsByAlias(c.Context(), alias)
	if err != nil {
		logger.Error("Failed to get account gRPC on Ledger", err.Error())
		return commonHTTP.WithError(c, err)
	}

	for _, ac := range ret.Accounts {
		logger.Infof("Account %s founded on Ledger", ac.Alias)
	}

	entity, err := handler.Command.CreateTransaction(c.Context(), organizationID, ledgerID, &transactionParsed)
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

// UpdateTransaction method that patch transaction created before
func (handler *TransactionHandler) UpdateTransaction(p any, c *fiber.Ctx) error {
	logger := mlog.NewLoggerFromContext(c.UserContext())

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")
	transactionID := c.Params("transaction_id")

	logger.Infof("Initiating update of Transaction with Organization ID: %s, Ledger ID: %s and ID: %s", organizationID, ledgerID, transactionID)

	payload := p.(*t.UpdateTransactionInput)
	logger.Infof("Request to update an Transaction with details: %#v", payload)

	trans, err := handler.Command.UpdateTransaction(c.Context(), organizationID, ledgerID, transactionID, payload)
	if err != nil {
		logger.Errorf("Failed to update TRansaction with ID: %s, Error: %s", transactionID, err.Error())
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

func (handler *TransactionHandler) GetAllTTransactions(c *fiber.Ctx) error {
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
