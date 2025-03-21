package in

import (
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/commons/postgres"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
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
//	@Summary		Get all balances
//	@Description	Get all balances
//	@Tags			Balances
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			limit			query		int		false	"Limit"			default(10)
//	@Param			start_date		query		string	false	"Start Date"	example "2021-01-01"
//	@Param			end_date		query		string	false	"End Date"		example "2021-01-01"
//	@Param			sort_order		query		string	false	"Sort Order"		enum(asc,desc)
//	@Param			cursor			query		string	false	"Cursor"
//	@Success		200				{object}	libPostgres.Pagination{items=[]mmodel.Balance, next_cursor=string, prev_cursor=string,limit=int}
//	@Router			/v1/organizations/:organization_id/ledgers/:ledger_id/balances [get]
func (handler *BalanceHandler) GetAllBalances(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_balances")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	pagination := libPostgres.Pagination{
		Limit:      headerParams.Limit,
		NextCursor: headerParams.Cursor,
		SortOrder:  headerParams.SortOrder,
		StartDate:  headerParams.StartDate,
		EndDate:    headerParams.EndDate,
	}

	logger.Infof("Initiating retrieval of all Balances")

	headerParams.Metadata = &bson.M{}

	err = libOpentelemetry.SetSpanAttributesFromStruct(&span, "headerParams", headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert headerParams to JSON string", err)

		return http.WithError(c, err)
	}

	balances, cur, err := handler.Query.GetAllBalances(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve all Balances", err)

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
//	@Summary		Get all balances by account id
//	@Description	Get all balances by account id
//	@Tags			Balances
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			account_id		path		string	true	"Account ID"
//	@Param			limit			query		int		false	"Limit"			default(10)
//	@Param			start_date		query		string	false	"Start Date"	example "2021-01-01"
//	@Param			end_date		query		string	false	"End Date"		example "2021-01-01"
//	@Param			sort_order		query		string	false	"Sort Order"		enum(asc,desc)
//	@Param			cursor			query		string	false	"Cursor"
//	@Success		200				{object}	libPostgres.Pagination{items=[]mmodel.Balance, next_cursor=string, prev_cursor=string,limit=int}
//	@Router			/v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:account_id/balances [get]
func (handler *BalanceHandler) GetAllBalancesByAccountID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_balances_by_account_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	accountID := c.Locals("account_id").(uuid.UUID)

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	pagination := libPostgres.Pagination{
		Limit:      headerParams.Limit,
		NextCursor: headerParams.Cursor,
		SortOrder:  headerParams.SortOrder,
		StartDate:  headerParams.StartDate,
		EndDate:    headerParams.EndDate,
	}

	logger.Infof("Initiating retrieval of all Balances by account id")

	headerParams.Metadata = &bson.M{}

	err = libOpentelemetry.SetSpanAttributesFromStruct(&span, "headerParams", headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert headerParams to JSON string", err)

		return http.WithError(c, err)
	}

	balances, cur, err := handler.Query.GetAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve all Balances by account id", err)

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
//	@Summary		Get Balance by id
//	@Description	Get a Balance with the input ID
//	@Tags			Balances
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			balance_id		path		string	true	"Balance ID"
//	@Success		200				{object}	mmodel.Balance
//	@Router			/v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id [get]
func (handler *BalanceHandler) GetBalanceByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_balance_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	balanceID := c.Locals("balance_id").(uuid.UUID)

	logger.Infof("Initiating retrieval of balance by id")

	op, err := handler.Query.GetBalanceByID(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve balance by id", err)

		logger.Errorf("Failed to retrieve balance by id, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved balance by id")

	return http.OK(c, op)
}

// DeleteBalanceByID delete a balance by ID.
//
//	@Summary		Delete Balance by account
//	@Description	Delete a Balance with the input ID
//	@Tags			Balances
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id		header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			balance_id		path		string	true	"Balance ID"
//	@Success		200				{object}	mmodel.Balance
//	@Router			/v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id [delete]
func (handler *BalanceHandler) DeleteBalanceByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_balance_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	balanceID := c.Locals("balance_id").(uuid.UUID)

	logger.Infof("Initiating delete balance by id")

	err := handler.Command.DeleteBalance(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to delete balance by id", err)

		logger.Errorf("Failed to delete balance by id, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully delete balance by id")

	return http.NoContent(c)
}

// UpdateBalance method that patch balance created before
//
//	@Summary		Update Balance
//	@Description	Update a Balance with the input payload
//	@Tags			Balances
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string							true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string							false	"Request ID"
//	@Param			organization_id	path		string							true	"Organization ID"
//	@Param			ledger_id		path		string							true	"Ledger ID"
//	@Param			balance_id		path		string							true	"Balance ID"
//	@Param			balance		    body		mmodel.UpdateBalance			true	"Balance Input"
//	@Success		200				{object}	mmodel.Balance
//	@Router			/v1/organizations/:organization_id/ledgers/:ledger_id/balances/:balance_id [patch]
func (handler *BalanceHandler) UpdateBalance(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_balance")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	balanceID := c.Locals("balance_id").(uuid.UUID)

	logger.Infof("Initiating update of Balance with Organization ID: %s, Ledger ID: %s, and ID: %s", organizationID.String(), ledgerID.String(), balanceID.String())

	payload := p.(*mmodel.UpdateBalance)
	logger.Infof("Request to update a Balance with details: %#v", payload)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	err = handler.Command.Update(ctx, organizationID, ledgerID, balanceID, *payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update Balance on command", err)

		logger.Errorf("Failed to update Balance with ID: %s, Error: %s", balanceID, err.Error())

		return http.WithError(c, err)
	}

	op, err := handler.Query.GetBalanceByID(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve Balance on query", err)

		logger.Errorf("Failed to retrieve Balance with ID: %s, Error: %s", balanceID, err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully updated Balance with Organization ID: %s, Ledger ID: %s, and ID: %s", organizationID, ledgerID, balanceID)

	return http.OK(c, op)
}
