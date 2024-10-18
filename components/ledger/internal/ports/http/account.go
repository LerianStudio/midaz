package http

import (
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/common/mpostgres"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/query"
	a "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/account"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

// AccountHandler struct contains an account use case for managing account related operations.
type AccountHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateAccount is a method that creates account information.
func (handler *AccountHandler) CreateAccount(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := mlog.NewLoggerFromContext(ctx)

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	portfolioID := c.Locals("portfolio_id").(uuid.UUID)

	logger.Infof("Initiating create of Account with Portfolio ID: %s", portfolioID.String())

	payload := i.(*a.CreateAccountInput)
	logger.Infof("Request to create a Account with details: %#v", payload)

	account, err := handler.Command.CreateAccount(ctx, organizationID, ledgerID, portfolioID, payload)
	if err != nil {
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully created Account")

	return commonHTTP.Created(c, account)
}

// GetAllAccounts is a method that retrieves all Accounts.
func (handler *AccountHandler) GetAllAccounts(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := mlog.NewLoggerFromContext(ctx)

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")
	portfolioID := c.Params("portfolio_id")

	logger.Infof("Get Accounts with Portfolio ID: %s", portfolioID)

	headerParams := commonHTTP.ValidateParameters(c.Queries())

	pagination := mpostgres.Pagination{
		Limit: headerParams.Limit,
		Page:  headerParams.Page,
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Accounts by metadata")

		accounts, err := handler.Query.GetAllMetadataAccounts(ctx, organizationID, ledgerID, portfolioID, *headerParams)
		if err != nil {
			logger.Errorf("Failed to retrieve all Accounts, Error: %s", err.Error())
			return commonHTTP.WithError(c, err)
		}

		logger.Infof("Successfully retrieved all Accounts by metadata")

		pagination.SetItems(accounts)

		return commonHTTP.OK(c, pagination)
	}

	logger.Infof("Initiating retrieval of all Accounts ")

	headerParams.Metadata = &bson.M{}

	accounts, err := handler.Query.GetAllAccount(ctx, organizationID, ledgerID, portfolioID, *headerParams)
	if err != nil {
		logger.Errorf("Failed to retrieve all Accounts, Error: %s", err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Accounts")

	pagination.SetItems(accounts)

	return commonHTTP.OK(c, pagination)
}

// GetAccountByID is a method that retrieves Account information by a given id.
func (handler *AccountHandler) GetAccountByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")
	portfolioID := c.Params("portfolio_id")
	id := c.Params("id")

	logger := mlog.NewLoggerFromContext(ctx)

	logger.Infof("Initiating retrieval of Account with Portfolio ID: %s and Account ID: %s", portfolioID, id)

	account, err := handler.Query.GetAccountByID(ctx, organizationID, ledgerID, portfolioID, id)
	if err != nil {
		logger.Errorf("Failed to retrieve Account with Portfolio ID: %s and Account ID: %s, Error: %s", portfolioID, id, err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Account with Portfolio ID: %s and Account ID: %s", portfolioID, id)

	return commonHTTP.OK(c, account)
}

// UpdateAccount is a method that updates Account information.
func (handler *AccountHandler) UpdateAccount(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := mlog.NewLoggerFromContext(ctx)

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")
	portfolioID := c.Params("portfolio_id")
	id := c.Params("id")

	logger.Infof("Initiating update of Account with Portfolio ID: %s and Account ID: %s", portfolioID, id)

	payload := i.(*a.UpdateAccountInput)
	logger.Infof("Request to update an Account with details: %#v", payload)

	account, err := handler.Command.UpdateAccount(ctx, organizationID, ledgerID, portfolioID, id, payload)
	if err != nil {
		logger.Errorf("Failed to update Account with ID: %s, Error: %s", id, err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully updated Account with Portfolio ID: %s and Account ID: %s", portfolioID, id)

	return commonHTTP.OK(c, account)
}

// DeleteAccountByID is a method that removes Account information by a given ids.
func (handler *AccountHandler) DeleteAccountByID(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := mlog.NewLoggerFromContext(ctx)

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")
	portfolioID := c.Params("portfolio_id")
	id := c.Params("id")

	logger.Infof("Initiating removal of Account with Portfolio ID: %s and Account ID: %s", portfolioID, id)

	if err := handler.Command.DeleteAccountByID(ctx, organizationID, ledgerID, portfolioID, id); err != nil {
		logger.Errorf("Failed to remove Account with Portfolio ID: %s and Account ID: %s, Error: %s", portfolioID, id, err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully removed Account with Portfolio ID: %s and Account ID: %s", portfolioID, id)

	return commonHTTP.NoContent(c)
}
