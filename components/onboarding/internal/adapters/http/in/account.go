package in

import (
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.opentelemetry.io/otel/attribute"
)

// AccountHandler struct contains an account use case for managing account related operations.
type AccountHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateAccount is a method that creates account information.
//
//	@Summary		Create a new account
//	@Description	Creates a new account within the specified ledger. Accounts represent individual financial entities like bank accounts, credit cards, or expense categories.
//	@Tags			Accounts
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string						true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string						false	"Request ID for tracing"
//	@Param			organization_id	path		string						true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string						true	"Ledger ID in UUID format"
//	@Param			account			body		mmodel.CreateAccountInput	true	"Account details including name, type, asset code, and optional parent account, portfolio, segment, and metadata"
//	@Success		201				{object}	mmodel.Account				"Successfully created account"
//	@Failure		400				{object}	mmodel.Error				"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error				"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error				"Forbidden access"
//	@Failure		404				{object}	mmodel.Error				"Organization, ledger, parent account, portfolio, or segment not found"
//	@Failure		409				{object}	mmodel.Error				"Conflict: Account with the same alias already exists"
//	@Failure		500				{object}	mmodel.Error				"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts [post]
func (handler *AccountHandler) CreateAccount(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	metricFactory := libCommons.NewMetricFactoryFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_account")
	defer span.End()

	reqId := libCommons.NewHeaderIDFromContext(ctx)
	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	payload := i.(*mmodel.CreateAccountInput)
	portfolioID := payload.PortfolioID
	logger.Infof("Request to create a Account with details: %#v", payload)

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
	)

	labels := map[string]string{
		"organization_id": organizationID.String(),
		"ledger_id":       ledgerID.String(),
	}

	if !libCommons.IsNilOrEmpty(portfolioID) {
		logger.Infof("Initiating create of Account with Portfolio ID: %s", *portfolioID)
		span.SetAttributes(attribute.String("app.request.portfolio_id", *portfolioID))
	}

	err := libOpentelemetry.SetSpanAttributesFromStructWithObfuscation(&span, "app.request.payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	account, err := handler.Command.CreateAccount(ctx, organizationID, ledgerID, payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to create Account on command", err)

		return http.WithError(c, err)
	}

	metricFactory.Counter("account_created", libOpentelemetry.MetricOption{
		Description: "New Account created",
		Unit:        "1",
	}).WithLabels(labels).Add(ctx, 1)

	logger.Infof("Successfully created Account")

	return http.Created(c, account)
}

// GetAllAccounts is a method that retrieves all Accounts.
//
//	@Summary		List all accounts
//	@Description	Returns a paginated list of accounts within the specified ledger, optionally filtered by metadata, date range, and other criteria
//	@Tags			Accounts
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			metadata		query		string	false	"JSON string to filter accounts by metadata fields"
//	@Param			limit			query		int		false	"Maximum number of records to return per page"				default(10)	minimum(1)	maximum(100)
//	@Param			page			query		int		false	"Page number for pagination"									default(1)	minimum(1)
//	@Param			start_date		query		string	false	"Filter accounts created on or after this date (format: YYYY-MM-DD)"
//	@Param			end_date		query		string	false	"Filter accounts created on or before this date (format: YYYY-MM-DD)"
//	@Param			sort_order		query		string	false	"Sort direction for results based on creation date"			Enums(asc,desc)
//	@Success		200				{object}	libPostgres.Pagination{items=[]mmodel.Account,page=int,limit=int}	"Successfully retrieved accounts list"
//	@Failure		400				{object}	mmodel.Error	"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Organization or ledger not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts [get]
func (handler *AccountHandler) GetAllAccounts(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_accounts")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
	)

	var portfolioID *uuid.UUID

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to validate query parameters", err)
		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	err = libOpentelemetry.SetSpanAttributesFromStructWithObfuscation(&span, "app.request.query_params", headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert query params to JSON string", err)
	}

	pagination := libPostgres.Pagination{
		Limit:     headerParams.Limit,
		Page:      headerParams.Page,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	if !libCommons.IsNilOrEmpty(&headerParams.PortfolioID) {
		parsedID := uuid.MustParse(headerParams.PortfolioID)
		portfolioID = &parsedID

		logger.Infof("Search of all Accounts with Portfolio ID: %s", portfolioID)
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Accounts by metadata")

		accounts, err := handler.Query.GetAllMetadataAccounts(ctx, organizationID, ledgerID, portfolioID, *headerParams)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to retrieve all Accounts on query", err)

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
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve all Accounts on query", err)

		logger.Errorf("Failed to retrieve all Accounts, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Accounts")

	pagination.SetItems(accounts)

	return http.OK(c, pagination)
}

// GetAccountByID is a method that retrieves Account information by a given account id.
//
//	@Summary		Retrieve a specific account
//	@Description	Returns detailed information about an account identified by its UUID within the specified ledger
//	@Tags			Accounts
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			id				path		string	true	"Account ID in UUID format"
//	@Success		200				{object}	mmodel.Account	"Successfully retrieved account"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Account, ledger, or organization not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id} [get]
func (handler *AccountHandler) GetAccountByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_account_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_id", id.String()),
	)

	logger.Infof("Initiating retrieval of Account with Account ID: %s", id.String())

	account, err := handler.Query.GetAccountByID(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve Account on query", err)

		logger.Errorf("Failed to retrieve Account with Account ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Account with Account ID: %s", id.String())

	return http.OK(c, account)
}

// GetAccountExternalByCode is a method that retrieves External Account information by a given asset code.
//
//	@Summary		Retrieve an account by alias
//	@Description	Returns detailed information about an account identified by its alias within the specified ledger
//	@Tags			Accounts
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			code			path		string	true	"Account External Code (e.g. BRL)"
//	@Success		200				{object}	mmodel.Account	"Successfully retrieved account"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Account with the specified alias, ledger, or organization not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/external/{code} [get]
func (handler *AccountHandler) GetAccountExternalByCode(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_account_external_by_code")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	code := c.Params("code")

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_code", code),
	)

	alias := constant.DefaultExternalAccountAliasPrefix + code

	logger.Infof("Initiating retrieval of Account with Account Alias: %s", alias)

	account, err := handler.Query.GetAccountByAlias(ctx, organizationID, ledgerID, nil, alias)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve Account on query", err)

		logger.Errorf("Failed to retrieve Account with Account Alias: %s, Error: %s", alias, err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Account with Account Alias: %s", alias)

	return http.OK(c, account)
}

// GetAccountByAlias is a method that retrieves Account information by a given account alias.
//
//	@Summary		Retrieve an account by alias
//	@Description	Returns detailed information about an account identified by its alias within the specified ledger
//	@Tags			Accounts
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			alias			path		string	true	"Account alias (e.g. @person1)"
//	@Success		200				{object}	mmodel.Account	"Successfully retrieved account"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Account with the specified alias, ledger, or organization not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/alias/{alias} [get]
func (handler *AccountHandler) GetAccountByAlias(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_account_by_alias")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	alias := c.Params("alias")

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_alias", alias),
	)

	logger.Infof("Initiating retrieval of Account with Account Alias: %s", alias)

	account, err := handler.Query.GetAccountByAlias(ctx, organizationID, ledgerID, nil, alias)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve Account on query", err)

		logger.Errorf("Failed to retrieve Account with Account Alias: %s, Error: %s", alias, err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Account with Account Alias: %s", alias)

	return http.OK(c, account)
}

// UpdateAccount is a method that updates Account information.
//
//	@Summary		Update an account
//	@Description	Updates an existing account's properties such as name, status, portfolio, segment, and metadata within the specified ledger
//	@Tags			Accounts
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string						true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string						false	"Request ID for tracing"
//	@Param			organization_id	path		string						true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string						true	"Ledger ID in UUID format"
//	@Param			id				path		string						true	"Account ID in UUID format"
//	@Param			account			body		mmodel.UpdateAccountInput	true	"Account properties to update including name, status, portfolio, segment, and optional metadata"
//	@Success		200				{object}	mmodel.Account				"Successfully updated account"
//	@Failure		400				{object}	mmodel.Error				"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error				"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error				"Forbidden access"
//	@Failure		404				{object}	mmodel.Error				"Account, ledger, organization, portfolio, or segment not found"
//	@Failure		409				{object}	mmodel.Error				"Conflict: Account with the same name already exists"
//	@Failure		500				{object}	mmodel.Error				"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id} [patch]
func (handler *AccountHandler) UpdateAccount(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_account")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating update of Account with ID: %s", id.String())

	payload := i.(*mmodel.UpdateAccountInput)
	logger.Infof("Request to update an Account with details: %#v", payload)

	err := libOpentelemetry.SetSpanAttributesFromStructWithObfuscation(&span, "app.request.payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)
	}

	_, err = handler.Command.UpdateAccount(ctx, organizationID, ledgerID, nil, id, payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update Account on command", err)

		logger.Errorf("Failed to update Account with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	account, err := handler.Query.GetAccountByID(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve Account on query", err)

		logger.Errorf("Failed to retrieve Account with ID: %s, Error: %s", id, err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully updated Account with ID: %s", id.String())

	return http.OK(c, account)
}

// DeleteAccountByID is a method that removes Account information by a given account id.
//
//	@Summary		Delete an account
//	@Description	Permanently removes an account from the specified ledger. This operation cannot be undone.
//	@Tags			Accounts
//	@Param			Authorization	header	string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header	string	false	"Request ID for tracing"
//	@Param			organization_id	path	string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path	string	true	"Ledger ID in UUID format"
//	@Param			id				path	string	true	"Account ID in UUID format"
//	@Success		204				{object}	nil	"Account successfully deleted"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Account, ledger, or organization not found"
//	@Failure		409				{object}	mmodel.Error	"Conflict: Account cannot be deleted due to existing dependencies"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{id} [delete]
func (handler *AccountHandler) DeleteAccountByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_account_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_id", id.String()),
	)

	logger.Infof("Initiating removal of Account with ID: %s", id.String())

	if err := handler.Command.DeleteAccountByID(ctx, organizationID, ledgerID, nil, id); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to remove Account on command", err)

		logger.Errorf("Failed to remove Account with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully removed Account with ID: %s", id.String())

	return http.NoContent(c)
}

// CountAccounts is a method that counts all accounts for a given organization and ledger, with an optional portfolio ID.
//
//	@Summary		Count accounts
//	@Description	Returns the total count of accounts for the specified organization, ledger, and optional portfolio
//	@Tags			Accounts
//	@Param			Authorization	header	string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header	string	false	"Request ID for tracing"
//	@Param			organization_id	path	string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path	string	true	"Ledger ID in UUID format"
//	@Success		200				{object}	nil	"Successfully retrieved accounts count"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Organization or ledger not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/metrics/count [head]
func (handler *AccountHandler) CountAccounts(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.count_accounts")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
	)

	logger.Infof("Counting accounts for organization %s and ledger %s", organizationID, ledgerID)

	count, err := handler.Query.CountAccounts(ctx, organizationID, ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to count accounts", err)
		logger.Errorf("Error counting accounts: %v", err)

		return http.WithError(c, err)
	}

	logger.Infof("Successfully counted accounts for organization %s and ledger %s: %d", organizationID, ledgerID, count)

	c.Set(constant.XTotalCount, fmt.Sprintf("%d", count))
	c.Set(constant.ContentLength, "0")

	return http.NoContent(c)
}
