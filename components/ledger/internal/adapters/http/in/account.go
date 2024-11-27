package in

import (
	"go.mongodb.org/mongo-driver/bson"

	"github.com/LerianStudio/midaz/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/LerianStudio/midaz/pkg/mpostgres"
	"github.com/LerianStudio/midaz/pkg/net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// AccountHandler struct contains an account use case for managing account related operations.
type AccountHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateAccount is a method that creates account information.
//
//	@Summary		Create an Account
//	@Description	Create an Account with the input payload
//	@Tags			Accounts
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header	string	true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header	string	false	"Request ID"
//	@Param			organization_id	path		string						true	"Organization ID"
//	@Param			ledger_id		path		string						true	"Ledger ID"
//	@Param			account			body		mmodel.CreateAccountInput	true	"Account"
//	@Success		200				{object}	mmodel.Account
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts [post]
func (handler *AccountHandler) CreateAccount(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_account")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	payload := i.(*mmodel.CreateAccountInput)
	logger.Infof("Request to create a Account with details: %#v", payload)

	if !pkg.IsNilOrEmpty(payload.PortfolioID) {
		logger.Infof("Initiating create of Account with Portfolio ID: %s", *payload.PortfolioID)
	}

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	account, err := handler.Command.CreateAccount(ctx, organizationID, ledgerID, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create Account on command", err)

		return http.WithError(c, err)
	}

	logger.Infof("Successfully created Account")

	return http.Created(c, account)
}

// GetAllAccounts is a method that retrieves all Accounts.
//
//	@Summary		Get all Accounts
//	@Description	Get all Accounts with the input metadata or without metadata
//	@Tags			Accounts
//	@Produce		json
//	@Param			Authorization	header	string	true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header	string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			metadata		query		string	false	"Metadata"
//	@Success		200				{object}	mpostgres.Pagination{items=[]mmodel.Account}
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts [get]
func (handler *AccountHandler) GetAllAccounts(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_accounts")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	var portfolioID *uuid.UUID

	headerParams := http.ValidateParameters(c.Queries())

	pagination := mpostgres.Pagination{
		Limit: headerParams.Limit,
		Page:  headerParams.Page,
	}

	if !pkg.IsNilOrEmpty(&headerParams.PortfolioID) {
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

			return http.WithError(c, err)
		}

		logger.Infof("Successfully retrieved all Accounts by metadata")

		pagination.SetItems(accounts)

		return http.OK(c, pagination)
	}

	logger.Infof("Initiating retrieval of all Accounts ")

	headerParams.Metadata = &bson.M{}

	accounts, err := handler.Query.GetAllAccount(ctx, organizationID, ledgerID, portfolioID, *headerParams)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve all Accounts on query", err)

		logger.Errorf("Failed to retrieve all Accounts, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Accounts")

	pagination.SetItems(accounts)

	return http.OK(c, pagination)
}

// GetAccountByID is a method that retrieves Account information by a given account id.
//
//	@Summary		Get an Account by ID
//	@Description	Get an Account with the input ID
//	@Tags			Accounts
//	@Produce		json
//	@Param			Authorization	header	string	true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header	string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			id				path		string	true	"Account ID"
//	@Success		200				{object}	mmodel.Account
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id} [get]
func (handler *AccountHandler) GetAccountByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

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

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Account with Account ID: %s", id.String())

	return http.OK(c, account)
}

// UpdateAccount is a method that updates Account information.
//
//	@Summary		Update an Account
//	@Description	Update an Account with the input payload
//	@Tags			Accounts
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header	string	true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header	string	false	"Request ID"
//	@Param			organization_id	path		string						true	"Organization ID"
//	@Param			ledger_id		path		string						true	"Ledger ID"
//	@Param			id				path		string						true	"Account ID"
//	@Param			account			body		mmodel.UpdateAccountInput	true	"Account"
//	@Success		200				{object}	mmodel.Account
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id} [patch]
func (handler *AccountHandler) UpdateAccount(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

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

		return http.WithError(c, err)
	}

	_, err = handler.Command.UpdateAccount(ctx, organizationID, ledgerID, nil, id, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update Account on command", err)

		logger.Errorf("Failed to update Account with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	account, err := handler.Query.GetAccountByID(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve Account on query", err)

		logger.Errorf("Failed to retrieve Account with ID: %s, Error: %s", id, err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully updated Account with ID: %s", id.String())

	return http.OK(c, account)
}

// DeleteAccountByID is a method that removes Account information by a given account id.
//
//	@Summary		Delete an Account by ID
//	@Description	Delete an Account with the input ID
//	@Tags			Accounts
//	@Param			Authorization	header	string	true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header	string	false	"Request ID"
//	@Param			organization_id	path	string	true	"Organization ID"
//	@Param			ledger_id		path	string	true	"Ledger ID"
//	@Param			id				path	string	true	"Account ID"
//	@Success		204
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id} [delete]
func (handler *AccountHandler) DeleteAccountByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_account_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating removal of Account with ID: %s", id.String())

	if err := handler.Command.DeleteAccountByID(ctx, organizationID, ledgerID, nil, id); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to remove Account on command", err)

		logger.Errorf("Failed to remove Account with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully removed Account with ID: %s", id.String())

	return http.NoContent(c)
}

// TODO: Remove the following methods after the deprecation period.

// CreateAccountFromPortfolio is a method that creates account information from a given portfolio id.
//
// Will be deprecated in the future. Use CreateAccount instead.
//
//	@Summary		Create an Account from Portfolio
//	@Description	## This endpoint will be deprecated soon. Use Create an Account instead. ##
//	@Description	---
//	@Description	Create an Account with the input payload from a Portfolio
//	@Tags			Accounts
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header	string	true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header	string	false	"Request ID"
//	@Param			organization_id	path		string						true	"Organization ID"
//	@Param			ledger_id		path		string						true	"Ledger ID"
//	@Param			portfolio_id	path		string						true	"Portfolio ID"
//	@Param			account			body		mmodel.CreateAccountInput	true	"Account"
//	@Success		200				{object}	mmodel.Account
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{portfolio_id}/accounts [post]
func (handler *AccountHandler) CreateAccountFromPortfolio(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := pkg.NewLoggerFromContext(ctx)

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
		return http.WithError(c, err)
	}

	logger.Infof("Successfully created Account")

	return http.Created(c, account)
}

// GetAllAccountsByIDFromPortfolio is a method that retrieves all Accounts by a given portfolio id.
//
// Will be deprecated in the future. Use GetAllAccounts instead.
//
//	@Summary		Get all Accounts from Portfolio
//	@Description	## This endpoint will be deprecated soon. Use Get all Accounts instead. ##
//	@Description	---
//	@Description	Get all Accounts with the input metadata or without metadata from a Portfolio
//	@Tags			Accounts
//	@Produce		json
//	@Param			Authorization	header	string	true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header	string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			portfolio_id	path		string	true	"Portfolio ID"
//	@Param			metadata		query		string	false	"Metadata"
//	@Success		200				{object}	mpostgres.Pagination{items=[]mmodel.Account}
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{portfolio_id}/accounts [get]
func (handler *AccountHandler) GetAllAccountsByIDFromPortfolio(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := pkg.NewLoggerFromContext(ctx)

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	portfolioID := c.Locals("portfolio_id").(uuid.UUID)

	logger.Infof("Get Accounts with Portfolio ID: %s", portfolioID.String())

	headerParams := http.ValidateParameters(c.Queries())

	pagination := mpostgres.Pagination{
		Limit: headerParams.Limit,
		Page:  headerParams.Page,
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Accounts by metadata")

		accounts, err := handler.Query.GetAllMetadataAccounts(ctx, organizationID, ledgerID, &portfolioID, *headerParams)
		if err != nil {
			logger.Errorf("Failed to retrieve all Accounts, Error: %s", err.Error())

			return http.WithError(c, err)
		}

		logger.Infof("Successfully retrieved all Accounts by metadata")

		pagination.SetItems(accounts)

		return http.OK(c, pagination)
	}

	logger.Infof("Initiating retrieval of all Accounts ")

	headerParams.Metadata = &bson.M{}

	accounts, err := handler.Query.GetAllAccount(ctx, organizationID, ledgerID, &portfolioID, *headerParams)
	if err != nil {
		logger.Errorf("Failed to retrieve all Accounts, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Accounts")

	pagination.SetItems(accounts)

	return http.OK(c, pagination)
}

// GetAccountByIDFromPortfolio is a method that retrieves Account information by a given portfolio id and account id.
//
// Will be deprecated in the future. Use GetAccountByID instead.
//
//	@Summary		Get an Account by ID from Portfolio
//	@Description	## This endpoint will be deprecated soon. Use Get an Account by ID instead. ##
//	@Description	---
//	@Description	Get an Account with the input ID from a Portfolio.
//	@Tags			Accounts
//	@Produce		json
//	@Param			Authorization	header	string	true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header	string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			portfolio_id	path		string	true	"Portfolio ID"
//	@Param			id				path		string	true	"Account ID"
//	@Success		200				{object}	mmodel.Account
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{portfolio_id}/accounts/{id} [get]
func (handler *AccountHandler) GetAccountByIDFromPortfolio(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	portfolioID := c.Locals("portfolio_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger := pkg.NewLoggerFromContext(ctx)
	logger.Infof("Initiating retrieval of Account with Portfolio ID: %s and Account ID: %s", portfolioID.String(), id.String())

	account, err := handler.Query.GetAccountByIDWithDeleted(ctx, organizationID, ledgerID, &portfolioID, id)
	if err != nil {
		logger.Errorf("Failed to retrieve Account with Portfolio ID: %s and Account ID: %s, Error: %s", portfolioID.String(), id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Account with Portfolio ID: %s and Account ID: %s", portfolioID.String(), id.String())

	return http.OK(c, account)
}

// UpdateAccountFromPortfolio is a method that updates Account information from a given portfolio id and account id.
//
// Will be deprecated in the future. Use UpdateAccount instead.
//
//	@Summary		Update an Account from Portfolio
//	@Description	## This endpoint will be deprecated soon. Use Update an Account instead. ##
//	@Description	---
//	@Description	Update an Account with the input payload from a Portfolio
//	@Tags			Accounts
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header	string	true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header	string	false	"Request ID"
//	@Param			organization_id	path		string						true	"Organization ID"
//	@Param			ledger_id		path		string						true	"Ledger ID"
//	@Param			portfolio_id	path		string						true	"Portfolio ID"
//	@Param			id				path		string						true	"Account ID"
//	@Param			account			body		mmodel.UpdateAccountInput	true	"Account"
//	@Success		200				{object}	mmodel.Account
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{portfolio_id}/accounts/{id} [patch]
func (handler *AccountHandler) UpdateAccountFromPortfolio(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := pkg.NewLoggerFromContext(ctx)

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

		return http.WithError(c, err)
	}

	account, err := handler.Query.GetAccountByID(ctx, organizationID, ledgerID, &portfolioID, id)
	if err != nil {
		logger.Errorf("Failed to retrieve Account with ID: %s, Error: %s", id, err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully updated Account with Portfolio ID: %s and Account ID: %s", portfolioID.String(), id.String())

	return http.OK(c, account)
}

// DeleteAccountByIDFromPortfolio is a method that removes Account information by a given portfolio id and account id.
//
// Will be deprecated in the future. Use DeleteAccountByID instead.
//
//	@Summary		Delete an Account by ID from Portfolio
//	@Description	## This endpoint will be deprecated soon. Use Delete an Account by ID instead. ##
//	@Description	---
//	@Description	Delete an Account with the input ID from a Portfolio
//	@Tags			Accounts
//	@Param			Authorization	header	string	true	"Authorization Bearer Token"
//	@Param			Midaz-Id		header	string	false	"Request ID"
//	@Param			organization_id	path	string	true	"Organization ID"
//	@Param			ledger_id		path	string	true	"Ledger ID"
//	@Param			portfolio_id	path	string	true	"Portfolio ID"
//	@Param			id				path	string	true	"Account ID"
//	@Success		204
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{portfolio_id}/accounts/{id} [delete]
func (handler *AccountHandler) DeleteAccountByIDFromPortfolio(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := pkg.NewLoggerFromContext(ctx)

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	portfolioID := c.Locals("portfolio_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating removal of Account with Portfolio ID: %s and Account ID: %s", portfolioID.String(), id.String())

	if err := handler.Command.DeleteAccountByID(ctx, organizationID, ledgerID, &portfolioID, id); err != nil {
		logger.Errorf("Failed to remove Account with Portfolio ID: %s and Account ID: %s, Error: %s", portfolioID.String(), id.String(), err.Error())
		return http.WithError(c, err)
	}

	logger.Infof("Successfully removed Account with Portfolio ID: %s and Account ID: %s", portfolioID.String(), id.String())

	return http.NoContent(c)
}
