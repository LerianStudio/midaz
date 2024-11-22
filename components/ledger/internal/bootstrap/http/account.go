package http

import (
	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"github.com/LerianStudio/midaz/common/mpostgres"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/services/query"
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

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_account")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	payload := i.(*mmodel.CreateAccountInput)
	logger.Infof("Request to create a Account with details: %#v", payload)

	if !common.IsNilOrEmpty(payload.PortfolioID) {
		logger.Infof("Initiating create of Account with Portfolio ID: %s", *payload.PortfolioID)
	}

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return commonHTTP.WithError(c, err)
	}

	account, err := handler.Command.CreateAccount(ctx, organizationID, ledgerID, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create Account on command", err)

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully created Account")

	return commonHTTP.Created(c, account)
}

// GetAllAccounts is a method that retrieves all Accounts.
func (handler *AccountHandler) GetAllAccounts(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_accounts")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	var portfolioID *uuid.UUID

	headerParams := commonHTTP.ValidateParameters(c.Queries())

	pagination := mpostgres.Pagination{
		Limit: headerParams.Limit,
		Page:  headerParams.Page,
	}

	if !common.IsNilOrEmpty(&headerParams.PortfolioID) {
		parsedID := uuid.MustParse(headerParams.PortfolioID)
		portfolioID = &parsedID

		logger.Infof("Search of all Accounts with Portfolio ID: %s", portfolioID)
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Accounts by metadata")

		accounts, err := handler.Query.GetAllMetadataAccounts(ctx, organizationID, ledgerID, portfolioID, *headerParams)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to retrieve all Accounts on query", err)

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
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve all Accounts on query", err)

		logger.Errorf("Failed to retrieve all Accounts, Error: %s", err.Error())

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Accounts")

	pagination.SetItems(accounts)

	return commonHTTP.OK(c, pagination)
}

// GetAccountByID is a method that retrieves Account information by a given account id.
func (handler *AccountHandler) GetAccountByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_account_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating retrieval of Account with Account ID: %s", id.String())

	account, err := handler.Query.GetAccountByIDWithDeleted(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve Account on query", err)

		logger.Errorf("Failed to retrieve Account with Account ID: %s, Error: %s", id.String(), err.Error())

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Account with Account ID: %s", id.String())

	return commonHTTP.OK(c, account)
}

// UpdateAccount is a method that updates Account information.
func (handler *AccountHandler) UpdateAccount(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_account")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating update of Account with ID: %s", id.String())

	payload := i.(*mmodel.UpdateAccountInput)
	logger.Infof("Request to update an Account with details: %#v", payload)

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return commonHTTP.WithError(c, err)
	}

	_, err = handler.Command.UpdateAccount(ctx, organizationID, ledgerID, nil, id, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update Account on command", err)

		logger.Errorf("Failed to update Account with ID: %s, Error: %s", id.String(), err.Error())

		return commonHTTP.WithError(c, err)
	}

	account, err := handler.Query.GetAccountByID(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve Account on query", err)

		logger.Errorf("Failed to retrieve Account with ID: %s, Error: %s", id, err.Error())

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully updated Account with ID: %s", id.String())

	return commonHTTP.OK(c, account)
}

// DeleteAccountByID is a method that removes Account information by a given account id.
func (handler *AccountHandler) DeleteAccountByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_account_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating removal of Account with ID: %s", id.String())

	if err := handler.Command.DeleteAccountByID(ctx, organizationID, ledgerID, nil, id); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to remove Account on command", err)

		logger.Errorf("Failed to remove Account with ID: %s, Error: %s", id.String(), err.Error())

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully removed Account with ID: %s", id.String())

	return commonHTTP.NoContent(c)
}

// TODO: Remove the following methods after the deprecation period.

// CreateAccountFromPortfolio is a method that creates account information from a given portfolio id.
//
// Will be deprecated in the future. Use CreateAccount instead.
func (handler *AccountHandler) CreateAccountFromPortfolio(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := common.NewLoggerFromContext(ctx)

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	portfolioID := c.Locals("portfolio_id").(uuid.UUID)
	portfolioIDStr := portfolioID.String()

	logger.Infof("Initiating create of Account with Portfolio ID: %s", portfolioIDStr)

	payload := i.(*mmodel.CreateAccountInput)
	payload.PortfolioID = &portfolioIDStr

	logger.Infof("Request to create a Account with details: %#v", payload)

	account, err := handler.Command.CreateAccount(ctx, organizationID, ledgerID, payload)
	if err != nil {
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully created Account")

	return commonHTTP.Created(c, account)
}

// GetAllAccountsByIDFromPortfolio is a method that retrieves all Accounts by a given portfolio id.
//
// Will be deprecated in the future. Use GetAllAccounts instead.
func (handler *AccountHandler) GetAllAccountsByIDFromPortfolio(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := common.NewLoggerFromContext(ctx)

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	portfolioID := c.Locals("portfolio_id").(uuid.UUID)

	logger.Infof("Get Accounts with Portfolio ID: %s", portfolioID.String())

	headerParams := commonHTTP.ValidateParameters(c.Queries())

	pagination := mpostgres.Pagination{
		Limit: headerParams.Limit,
		Page:  headerParams.Page,
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Accounts by metadata")

		accounts, err := handler.Query.GetAllMetadataAccounts(ctx, organizationID, ledgerID, &portfolioID, *headerParams)
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

	accounts, err := handler.Query.GetAllAccount(ctx, organizationID, ledgerID, &portfolioID, *headerParams)
	if err != nil {
		logger.Errorf("Failed to retrieve all Accounts, Error: %s", err.Error())

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Accounts")

	pagination.SetItems(accounts)

	return commonHTTP.OK(c, pagination)
}

// GetAccountByIDFromPortfolio is a method that retrieves Account information by a given portfolio id and account id.
//
// Will be deprecated in the future. Use GetAccountByID instead.
func (handler *AccountHandler) GetAccountByIDFromPortfolio(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	portfolioID := c.Locals("portfolio_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger := common.NewLoggerFromContext(ctx)
	logger.Infof("Initiating retrieval of Account with Portfolio ID: %s and Account ID: %s", portfolioID.String(), id.String())

	account, err := handler.Query.GetAccountByIDWithDeleted(ctx, organizationID, ledgerID, &portfolioID, id)
	if err != nil {
		logger.Errorf("Failed to retrieve Account with Portfolio ID: %s and Account ID: %s, Error: %s", portfolioID.String(), id.String(), err.Error())

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Account with Portfolio ID: %s and Account ID: %s", portfolioID.String(), id.String())

	return commonHTTP.OK(c, account)
}

// UpdateAccountFromPortfolio is a method that updates Account information from a given portfolio id and account id.
//
// Will be deprecated in the future. Use UpdateAccount instead.
func (handler *AccountHandler) UpdateAccountFromPortfolio(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := common.NewLoggerFromContext(ctx)

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	portfolioID := c.Locals("portfolio_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating update of Account with Portfolio ID: %s and Account ID: %s", portfolioID.String(), id.String())

	payload := i.(*mmodel.UpdateAccountInput)
	logger.Infof("Request to update an Account with details: %#v", payload)

	_, err := handler.Command.UpdateAccount(ctx, organizationID, ledgerID, &portfolioID, id, payload)
	if err != nil {
		logger.Errorf("Failed to update Account with ID: %s, Error: %s", id.String(), err.Error())

		return commonHTTP.WithError(c, err)
	}

	account, err := handler.Query.GetAccountByID(ctx, organizationID, ledgerID, &portfolioID, id)
	if err != nil {
		logger.Errorf("Failed to retrieve Account with ID: %s, Error: %s", id, err.Error())

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully updated Account with Portfolio ID: %s and Account ID: %s", portfolioID.String(), id.String())

	return commonHTTP.OK(c, account)
}

// DeleteAccountByIDFromPortfolio is a method that removes Account information by a given portfolio id and account id.
//
// Will be deprecated in the future. Use DeleteAccountByID instead.
func (handler *AccountHandler) DeleteAccountByIDFromPortfolio(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := common.NewLoggerFromContext(ctx)

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	portfolioID := c.Locals("portfolio_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating removal of Account with Portfolio ID: %s and Account ID: %s", portfolioID.String(), id.String())

	if err := handler.Command.DeleteAccountByID(ctx, organizationID, ledgerID, &portfolioID, id); err != nil {
		logger.Errorf("Failed to remove Account with Portfolio ID: %s and Account ID: %s, Error: %s", portfolioID.String(), id.String(), err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully removed Account with Portfolio ID: %s and Account ID: %s", portfolioID.String(), id.String())

	return commonHTTP.NoContent(c)
}
