package in

import (
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

// BalanceHandler struct contains a cqrs use case for managing balances.
type BalanceHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// GetAllBalances retrieves all balances.
//
//	@ID				listBalances
//	@Summary		Get all balances
//	@Description	Retrieves all balance records for the specified ledger. Supports filtering by date range, cursor-based pagination, and sorting. Returns comprehensive balance information including account details, asset codes, and available/on-hold amounts.
//	@Tags			Balances
//	@Security		BearerAuth
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			limit			query		int		false	"Maximum number of records to return per page"				default(10)	minimum(1)	maximum(100)
//	@Param			start_date		query		string	false	"Filter records created on or after this date (format: YYYY-MM-DD)"
//	@Param			end_date		query		string	false	"Filter records created on or before this date (format: YYYY-MM-DD)"
//	@Param			sort_order		query		string	false	"Sort direction for results based on creation date"			Enums(asc,desc)
//	@Param			cursor			query		string	false	"Cursor for pagination to fetch the next set of results"
//	@Success		200				{object}	libPostgres.Pagination{items=[]mmodel.Balance, next_cursor=string, prev_cursor=string,limit=int}
//	@Example		response	{"items":[{"id":"b1234567-89ab-cdef-0123-456789abcdef","organizationId":"a1b2c3d4-e5f6-7890-abcd-1234567890ab","ledgerId":"b2c3d4e5-f6a1-7890-bcde-2345678901cd","accountId":"c3d4e5f6-a1b2-7890-cdef-3456789012de","alias":"@operating","key":"default","assetCode":"USD","available":"15000.00","onHold":"500.00","version":1,"accountType":"deposit","allowSending":true,"allowReceiving":true,"createdAt":"2024-01-15T09:30:00Z","updatedAt":"2024-01-15T09:30:00Z"}],"limit":10,"nextCursor":"eyJpZCI6ImIxMjM0NTY3In0="}
//	@Failure		400				{object}	mmodel.Error	"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/balances [get]
func (handler *BalanceHandler) GetAllBalances(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_balances")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	err = libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.query_params", headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert headerParams to JSON string", err)
	}

	pagination := libPostgres.Pagination{
		Limit:     headerParams.Limit,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	logger.Infof("Initiating retrieval of all Balances")

	headerParams.Metadata = &bson.M{}

	balances, cur, err := handler.Query.GetAllBalances(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve all Balances", err)

		logger.Errorf("Failed to retrieve all Balances, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Balances")

	pagination.SetItems(balances)
	pagination.SetCursor(cur.Next, cur.Prev)

	return http.OK(c, pagination)
}

// GetAllBalancesByAccountID retrieves all balances.
//
//	@ID				listBalancesByAccountID
//	@Summary		Get all balances by account id
//	@Description	Retrieves all balance records associated with a specific account. Supports filtering by date range, cursor-based pagination, and sorting. Useful for viewing multi-currency balances or balance keys for a single account.
//	@Tags			Balances
//	@Security		BearerAuth
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			account_id		path		string	true	"Account ID in UUID format"
//	@Param			limit			query		int		false	"Maximum number of records to return per page"				default(10)	minimum(1)	maximum(100)
//	@Param			start_date		query		string	false	"Filter records created on or after this date (format: YYYY-MM-DD)"
//	@Param			end_date		query		string	false	"Filter records created on or before this date (format: YYYY-MM-DD)"
//	@Param			sort_order		query		string	false	"Sort direction for results based on creation date"			Enums(asc,desc)
//	@Param			cursor			query		string	false	"Cursor for pagination to fetch the next set of results"
//	@Success		200				{object}	libPostgres.Pagination{items=[]mmodel.Balance, next_cursor=string, prev_cursor=string,limit=int}
//	@Example		response	{"items":[{"id":"b1234567-89ab-cdef-0123-456789abcdef","organizationId":"a1b2c3d4-e5f6-7890-abcd-1234567890ab","ledgerId":"b2c3d4e5-f6a1-7890-bcde-2345678901cd","accountId":"c3d4e5f6-a1b2-7890-cdef-3456789012de","alias":"@savings","key":"default","assetCode":"USD","available":"25000.00","onHold":"1000.00","version":2,"accountType":"savings","allowSending":true,"allowReceiving":true,"createdAt":"2024-01-15T09:30:00Z","updatedAt":"2024-01-15T10:00:00Z"}],"limit":10,"nextCursor":"eyJpZCI6ImIxMjM0NTY3In0="}
//	@Failure		400				{object}	mmodel.Error	"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Account not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/balances [get]
func (handler *BalanceHandler) GetAllBalancesByAccountID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_balances_by_account_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	accountID := c.Locals("account_id").(uuid.UUID)

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	err = libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.query_params", headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert headerParams to JSON string", err)
	}

	pagination := libPostgres.Pagination{
		Limit:     headerParams.Limit,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	logger.Infof("Initiating retrieval of all Balances by account id")

	headerParams.Metadata = &bson.M{}

	balances, cur, err := handler.Query.GetAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve all Balances by account id", err)

		logger.Errorf("Failed to retrieve all Balances by account id, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Balances by account id")

	pagination.SetItems(balances)
	pagination.SetCursor(cur.Next, cur.Prev)

	return http.OK(c, pagination)
}

// GetBalanceByID retrieves a balance by ID.
//
//	@ID				getBalanceByID
//	@Summary		Retrieve a specific balance
//	@Description	Returns detailed information about a balance identified by its UUID within the specified ledger, including available amount, on-hold amount, and account information
//	@Tags			Balances
//	@Security		BearerAuth
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			balance_id		path		string	true	"Balance ID in UUID format"
//	@Success		200				{object}	mmodel.Balance	"Successfully retrieved balance"
//	@Example		response	{"id":"b1234567-89ab-cdef-0123-456789abcdef","organizationId":"a1b2c3d4-e5f6-7890-abcd-1234567890ab","ledgerId":"b2c3d4e5-f6a1-7890-bcde-2345678901cd","accountId":"c3d4e5f6-a1b2-7890-cdef-3456789012de","alias":"@operating","key":"default","assetCode":"USD","available":"15000.00","onHold":"500.00","version":1,"accountType":"deposit","allowSending":true,"allowReceiving":true,"createdAt":"2024-01-15T09:30:00Z","updatedAt":"2024-01-15T09:30:00Z"}
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Balance not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/balances/{balance_id} [get]
func (handler *BalanceHandler) GetBalanceByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_balance_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	balanceID := c.Locals("balance_id").(uuid.UUID)

	logger.Infof("Initiating retrieval of balance by id")

	op, err := handler.Query.GetBalanceByID(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve balance by id", err)

		logger.Errorf("Failed to retrieve balance by id, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved balance by id")

	return http.OK(c, op)
}

// DeleteBalanceByID delete a balance by ID.
//
//	@ID				deleteBalance
//	@Summary		Delete a balance
//	@Description	Permanently removes a balance identified by its UUID. This operation cannot be undone and requires the balance to have no active operations.
//	@Tags			Balances
//	@Security		BearerAuth
//	@Produce		json
//	@Param			Authorization	header		string			true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string			false	"Request ID for tracing"
//	@Param			organization_id	path		string			true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string			true	"Ledger ID in UUID format"
//	@Param			balance_id		path		string			true	"Balance ID in UUID format"
//	@Success		204				{string}	string			"Balance successfully deleted"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Balance not found"
//	@Failure		409				{object}	mmodel.Error	"Conflict: Cannot delete balance with active operations"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/balances/{balance_id} [delete]
func (handler *BalanceHandler) DeleteBalanceByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_balance_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	balanceID := c.Locals("balance_id").(uuid.UUID)

	logger.Infof("Initiating delete balance by id")

	err := handler.Command.DeleteBalance(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete balance by id", err)

		logger.Errorf("Failed to delete balance by id, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully delete balance by id")

	return http.NoContent(c)
}

// UpdateBalance method that patch balance created before
//
//	@ID				updateBalance
//	@Summary		Update a balance
//	@Description	Updates an existing balance's properties such as allowSending and allowReceiving flags. Only supplied fields will be updated.
//	@Tags			Balances
//	@Security		BearerAuth
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string					true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string					false	"Request ID for tracing"
//	@Param			organization_id	path		string					true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string					true	"Ledger ID in UUID format"
//	@Param			balance_id		path		string					true	"Balance ID in UUID format"
//	@Param			balance			body		mmodel.UpdateBalance	true	"Balance properties to update"
//	@Success		200				{object}	mmodel.Balance			"Successfully updated balance"
//	@Example		response	{"id":"b1234567-89ab-cdef-0123-456789abcdef","organizationId":"a1b2c3d4-e5f6-7890-abcd-1234567890ab","ledgerId":"b2c3d4e5-f6a1-7890-bcde-2345678901cd","accountId":"c3d4e5f6-a1b2-7890-cdef-3456789012de","alias":"@operating","key":"default","assetCode":"USD","available":"18000.00","onHold":"200.00","version":2,"accountType":"deposit","allowSending":true,"allowReceiving":true,"createdAt":"2024-01-15T09:30:00Z","updatedAt":"2024-01-15T14:45:00Z"}
//	@Failure		400				{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Balance not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/balances/{balance_id} [patch]
func (handler *BalanceHandler) UpdateBalance(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_balance")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	balanceID := c.Locals("balance_id").(uuid.UUID)

	logger.Infof("Initiating update of Balance with Organization ID: %s, Ledger ID: %s, and ID: %s", organizationID.String(), ledgerID.String(), balanceID.String())

	payload := p.(*mmodel.UpdateBalance)
	logger.Infof("Request to update a Balance with details: %#v", payload)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)
	}

	err = handler.Command.Update(ctx, organizationID, ledgerID, balanceID, *payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update Balance on command", err)

		logger.Errorf("Failed to update Balance with ID: %s, Error: %s", balanceID, err.Error())

		return http.WithError(c, err)
	}

	op, err := handler.Query.GetBalanceByID(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve Balance on query", err)

		logger.Errorf("Failed to retrieve Balance with ID: %s, Error: %s", balanceID, err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully updated Balance with Organization ID: %s, Ledger ID: %s, and ID: %s", organizationID, ledgerID, balanceID)

	return http.OK(c, op)
}

// GetBalancesByAlias retrieves balances by Alias.
//
//	@ID				getBalancesByAlias
//	@Summary		Get balances by account alias
//	@Description	Retrieves all balance records associated with an account identified by its alias. Useful for querying balances when you know the account alias but not the account ID.
//	@Tags			Balances
//	@Security		BearerAuth
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			alias			path		string	true	"Account alias (e.g., @person1)"
//	@Success		200				{object}	libPostgres.Pagination{items=[]mmodel.Balance, next_cursor=string, prev_cursor=string,limit=int}	"Successfully retrieved balances"
//	@Example		response	{"items":[{"id":"b1234567-89ab-cdef-0123-456789abcdef","organizationId":"a1b2c3d4-e5f6-7890-abcd-1234567890ab","ledgerId":"b2c3d4e5-f6a1-7890-bcde-2345678901cd","accountId":"c3d4e5f6-a1b2-7890-cdef-3456789012de","alias":"@person1","key":"default","assetCode":"USD","available":"5000.00","onHold":"100.00","version":1,"accountType":"checking","allowSending":true,"allowReceiving":true,"createdAt":"2024-01-15T09:30:00Z","updatedAt":"2024-01-15T09:30:00Z"}],"limit":10}
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Account with alias not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/alias/{alias}/balances [get]
func (handler *BalanceHandler) GetBalancesByAlias(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_balances_by_alias")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	alias := c.Params("alias")

	logger.Infof("Initiating retrieval of balances by alias")

	balances, err := handler.Query.GetAllBalancesByAlias(ctx, organizationID, ledgerID, alias)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve balances by alias", err)

		logger.Errorf("Failed to retrieve balances by alias, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved balances by alias")

	if len(balances) == 0 {
		balances = []*mmodel.Balance{}
	}

	return http.OK(c, libPostgres.Pagination{
		Limit: 10,
		Items: balances,
	})
}

// GetBalancesExternalByCode retrieves external balances by code.
//
//	@ID				getBalancesExternalByCode
//	@Summary		Get external account balances by asset code
//	@Description	Retrieves balance records for an external account identified by asset code. External accounts are system accounts used for tracking external funds flow.
//	@Tags			Balances
//	@Security		BearerAuth
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			code			path		string	true	"Asset code (e.g., BRL, USD)"
//	@Success		200				{object}	libPostgres.Pagination{items=[]mmodel.Balance, next_cursor=string, prev_cursor=string,limit=int}	"Successfully retrieved external balances"
//	@Example		response	{"items":[{"id":"b1234567-89ab-cdef-0123-456789abcdef","organizationId":"a1b2c3d4-e5f6-7890-abcd-1234567890ab","ledgerId":"b2c3d4e5-f6a1-7890-bcde-2345678901cd","accountId":"c3d4e5f6-a1b2-7890-cdef-3456789012de","alias":"@external/BRL","key":"default","assetCode":"BRL","available":"50000.00","onHold":"0.00","version":1,"accountType":"external","allowSending":true,"allowReceiving":true,"createdAt":"2024-01-15T09:30:00Z","updatedAt":"2024-01-15T09:30:00Z"}],"limit":10}
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"External account not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/external/{code}/balances [get]
func (handler *BalanceHandler) GetBalancesExternalByCode(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_balances_external_by_code")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	code := c.Params("code")
	alias := cn.DefaultExternalAccountAliasPrefix + code

	logger.Infof("Initiating retrieval of balances by code")

	balances, err := handler.Query.GetAllBalancesByAlias(ctx, organizationID, ledgerID, alias)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve balances by code", err)

		logger.Errorf("Failed to retrieve balances by code, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved balances by code")

	if len(balances) == 0 {
		balances = []*mmodel.Balance{}
	}

	return http.OK(c, libPostgres.Pagination{
		Limit: 10,
		Items: balances,
	})
}

// CreateAdditionalBalance handles the creation of a new balance using the provided payload and context.
//
//	@ID				createAdditionalBalance
//	@Summary		Create an additional balance
//	@Description	Creates a new additional balance for an existing account. Additional balances allow tracking multiple balance keys or currencies within a single account.
//	@Tags			Balances
//	@Security		BearerAuth
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string							true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string							false	"Request ID for tracing"
//	@Param			organization_id	path		string							true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string							true	"Ledger ID in UUID format"
//	@Param			account_id		path		string							true	"Account ID in UUID format"
//	@Param			balance			body		mmodel.CreateAdditionalBalance	true	"Additional balance details including key and asset code"
//	@Success		201				{object}	mmodel.Balance					"Successfully created additional balance"
//	@Example		response	{"id":"b1234567-89ab-cdef-0123-456789abcdef","organizationId":"a1b2c3d4-e5f6-7890-abcd-1234567890ab","ledgerId":"b2c3d4e5-f6a1-7890-bcde-2345678901cd","accountId":"c3d4e5f6-a1b2-7890-cdef-3456789012de","alias":"@operating#bonus","key":"default","assetCode":"USD","available":"0.00","onHold":"0.00","version":1,"accountType":"deposit","allowSending":true,"allowReceiving":true,"createdAt":"2024-01-15T09:30:00Z","updatedAt":"2024-01-15T09:30:00Z"}
//	@Failure		400				{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Account not found"
//	@Failure		409				{object}	mmodel.Error	"Conflict: Balance with same key already exists"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/balances [post]
func (handler *BalanceHandler) CreateAdditionalBalance(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	accountID := c.Locals("account_id").(uuid.UUID)

	payload := p.(*mmodel.CreateAdditionalBalance)
	logger.Infof("Request to create a Balance with details: %#v", payload)

	ctx, span := tracer.Start(ctx, "handler.create_additional_balance")
	defer span.End()

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	balance, err := handler.Command.CreateAdditionalBalance(ctx, organizationID, ledgerID, accountID, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create additional balance on command", err)

		logger.Errorf("Failed to create additional balance, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully created additional balance")

	return http.Created(c, balance)
}
