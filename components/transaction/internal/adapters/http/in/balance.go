package in

import (
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mlog"
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
//	@Summary		Get all balances
//	@Description	Get all balances
//	@Tags			Balances
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			limit			query		int		false	"Limit"			default(10)
//	@Param			start_date		query		string	false	"Start Date"	example	"2021-01-01"
//	@Param			end_date		query		string	false	"End Date"		example	"2021-01-01"
//	@Param			sort_order		query		string	false	"Sort Order"	enum(asc,desc)
//	@Param			cursor			query		string	false	"Cursor"
//	@Success		200				{object}	libPostgres.Pagination{items=[]mmodel.Balance,next_cursor=string,prev_cursor=string,limit=int}
//	@Failure		400				{object}	mmodel.Error	"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/balances [Get]
func (handler *BalanceHandler) GetAllBalances(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_balances")
	defer span.End()

	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")

	mlog.EnrichBalance(c, organizationID, ledgerID, uuid.Nil, uuid.Nil)
	mlog.SetHandler(c, "get_all_balances")

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
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

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	logger.Infof("Successfully retrieved all Balances")

	pagination.SetItems(balances)
	pagination.SetCursor(cur.Next, cur.Prev)

	if err := http.OK(c, pagination); err != nil {
		return err
	}

	return nil
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
//	@Param			start_date		query		string	false	"Start Date"	example	"2021-01-01"
//	@Param			end_date		query		string	false	"End Date"		example	"2021-01-01"
//	@Param			sort_order		query		string	false	"Sort Order"	enum(asc,desc)
//	@Param			cursor			query		string	false	"Cursor"
//	@Success		200				{object}	libPostgres.Pagination{items=[]mmodel.Balance,next_cursor=string,prev_cursor=string,limit=int}
//	@Failure		400				{object}	mmodel.Error	"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Account not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/balances [Get]
func (handler *BalanceHandler) GetAllBalancesByAccountID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_balances_by_account_id")
	defer span.End()

	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
	accountID := http.LocalUUID(c, "account_id")

	mlog.EnrichBalance(c, organizationID, ledgerID, accountID, uuid.Nil)
	mlog.SetHandler(c, "get_all_balances_by_account_id")

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
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

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	logger.Infof("Successfully retrieved all Balances by account id")

	pagination.SetItems(balances)
	pagination.SetCursor(cur.Next, cur.Prev)

	if err := http.OK(c, pagination); err != nil {
		return err
	}

	return nil
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
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Balance not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/balances/{balance_id} [Get]
func (handler *BalanceHandler) GetBalanceByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_balance_by_id")
	defer span.End()

	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
	balanceID := http.LocalUUID(c, "balance_id")

	mlog.EnrichBalance(c, organizationID, ledgerID, uuid.Nil, balanceID)
	mlog.SetHandler(c, "get_balance_by_id")

	logger.Infof("Initiating retrieval of balance by id")

	op, err := handler.Query.GetBalanceByID(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve balance by id", err)

		logger.Errorf("Failed to retrieve balance by id, Error: %s", err.Error())

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	logger.Infof("Successfully retrieved balance by id")

	if err := http.OK(c, op); err != nil {
		return err
	}

	return nil
}

// DeleteBalanceByID delete a balance by ID.
//
//	@Summary		Delete Balance by account
//	@Description	Delete a Balance with the input ID
//	@Tags			Balances
//	@Produce		json
//	@Param			Authorization	header		string			true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string			false	"Request ID"
//	@Param			organization_id	path		string			true	"Organization ID"
//	@Param			ledger_id		path		string			true	"Ledger ID"
//	@Param			balance_id		path		string			true	"Balance ID"
//	@Success		204				"Balance successfully deleted"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Balance not found"
//	@Failure		409				{object}	mmodel.Error	"Conflict: Cannot delete balance with active operations"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/balances/{balance_id} [Delete]
func (handler *BalanceHandler) DeleteBalanceByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_balance_by_id")
	defer span.End()

	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
	balanceID := http.LocalUUID(c, "balance_id")

	mlog.EnrichBalance(c, organizationID, ledgerID, uuid.Nil, balanceID)
	mlog.SetHandler(c, "delete_balance_by_id")

	logger.Infof("Initiating delete balance by id")

	err := handler.Command.DeleteBalance(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete balance by id", err)

		logger.Errorf("Failed to delete balance by id, Error: %s", err.Error())

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	logger.Infof("Successfully delete balance by id")

	if err := http.NoContent(c); err != nil {
		return err
	}

	return nil
}

// UpdateBalance method that patch balance created before
//
//	@Summary		Update Balance
//	@Description	Update a Balance with the input payload
//	@Tags			Balances
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string					true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string					false	"Request ID"
//	@Param			organization_id	path		string					true	"Organization ID"
//	@Param			ledger_id		path		string					true	"Ledger ID"
//	@Param			balance_id		path		string					true	"Balance ID"
//	@Param			balance			body		mmodel.UpdateBalance	true	"Balance Input"
//	@Success		200				{object}	mmodel.Balance
//	@Failure		400				{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Balance not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/balances/{balance_id} [Patch]
func (handler *BalanceHandler) UpdateBalance(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_balance")
	defer span.End()

	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
	balanceID := http.LocalUUID(c, "balance_id")

	mlog.EnrichBalance(c, organizationID, ledgerID, uuid.Nil, balanceID)
	mlog.SetHandler(c, "update_balance")

	logger.Infof("Initiating update of Balance with Organization ID: %s, Ledger ID: %s, and ID: %s", organizationID.String(), ledgerID.String(), balanceID.String())

	payload := http.Payload[*mmodel.UpdateBalance](c, p)
	logger.Infof("Request to update a Balance with details: %#v", payload)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)
	}

	// Get the updated balance directly from the primary database using RETURNING clause,
	// avoiding stale reads from replicas due to replication lag
	balance, err := handler.Command.Update(ctx, organizationID, ledgerID, balanceID, *payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update Balance on command", err)

		logger.Errorf("Failed to update Balance with ID: %s, Error: %s", balanceID, err.Error())

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	logger.Infof("Successfully updated Balance with Organization ID: %s, Ledger ID: %s, and ID: %s", organizationID, ledgerID, balanceID)

	if err := http.OK(c, balance); err != nil {
		return err
	}

	return nil
}

// GetBalancesByAlias retrieves balances by Alias.
//
//	@Summary		Get Balances using Alias
//	@Description	Get Balances with alias
//	@Tags			Balances
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			alias			path		string	true	"Alias (e.g. @person1)"
//	@Param			limit			query		int		false	"Limit"	default(10)
//	@Success		200				{object}	libPostgres.Pagination{items=[]mmodel.Balance,next_cursor=string,prev_cursor=string,limit=int}
//	@Failure		400				{object}	mmodel.Error	"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Balance not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/alias/{alias}/balances [Get]
func (handler *BalanceHandler) GetBalancesByAlias(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_balances_by_alias")
	defer span.End()

	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
	alias := http.LocalString(c, "alias")

	mlog.EnrichBalance(c, organizationID, ledgerID, uuid.Nil, uuid.Nil)
	mlog.SetHandler(c, "get_balances_by_alias")

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

	logger.Infof("Initiating retrieval of balances by alias")

	balances, err := handler.Query.GetAllBalancesByAlias(ctx, organizationID, ledgerID, alias)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve balances by alias", err)

		logger.Errorf("Failed to retrieve balances by alias, Error: %s", err.Error())

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	logger.Infof("Successfully retrieved balances by alias")

	if len(balances) == 0 {
		balances = []*mmodel.Balance{}
	}

	if err := http.OK(c, libPostgres.Pagination{
		Limit: headerParams.Limit,
		Items: balances,
	}); err != nil {
		return err
	}

	return nil
}

// GetBalancesExternalByCode retrieves external balances by code.
//
//	@Summary		Get External balances using code
//	@Description	Get External balances with code
//	@Tags			Balances
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			code			path		string	true	"Code (e.g. BRL)"
//	@Param			limit			query		int		false	"Limit"	default(10)
//	@Success		200				{object}	libPostgres.Pagination{items=[]mmodel.Balance,next_cursor=string,prev_cursor=string,limit=int}
//	@Failure		400				{object}	mmodel.Error	"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Balance not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/external/{code}/balances [Get]
func (handler *BalanceHandler) GetBalancesExternalByCode(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_balances_external_by_code")
	defer span.End()

	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
	code := http.LocalString(c, "code")
	alias := cn.DefaultExternalAccountAliasPrefix + code

	mlog.EnrichBalance(c, organizationID, ledgerID, uuid.Nil, uuid.Nil)
	mlog.SetHandler(c, "get_balances_external_by_code")

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

	logger.Infof("Initiating retrieval of balances by code")

	balances, err := handler.Query.GetAllBalancesByAlias(ctx, organizationID, ledgerID, alias)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve balances by code", err)

		logger.Errorf("Failed to retrieve balances by code, Error: %s", err.Error())

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	logger.Infof("Successfully retrieved balances by code")

	if len(balances) == 0 {
		balances = []*mmodel.Balance{}
	}

	if err := http.OK(c, libPostgres.Pagination{
		Limit: headerParams.Limit,
		Items: balances,
	}); err != nil {
		return err
	}

	return nil
}

// CreateAdditionalBalance handles the creation of a new balance using the provided payload and context.
//
//	@Summary		Create Additional Balance
//	@Description	Create an Additional Balance with the input payload
//	@Tags			Balances
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string							true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string							false	"Request ID"
//	@Param			organization_id	path		string							true	"Organization ID"
//	@Param			ledger_id		path		string							true	"Ledger ID"
//	@Param			account_id		path		string							true	"Account ID"
//	@Param			balance			body		mmodel.CreateAdditionalBalance	true	"Balance Input"
//	@Success		201				{object}	mmodel.Balance
//	@Failure		400				{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Balance not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/balances [Post]
func (handler *BalanceHandler) CreateAdditionalBalance(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	organizationID := http.LocalUUID(c, "organization_id")
	ledgerID := http.LocalUUID(c, "ledger_id")
	accountID := http.LocalUUID(c, "account_id")

	mlog.EnrichBalance(c, organizationID, ledgerID, accountID, uuid.Nil)
	mlog.SetHandler(c, "create_additional_balance")

	payload := http.Payload[*mmodel.CreateAdditionalBalance](c, p)
	logger.Infof("Request to create a Balance with details: %#v", payload)

	ctx, span := tracer.Start(ctx, "handler.create_additional_balance")
	defer span.End()

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to convert payload to JSON string", err)

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	balance, err := handler.Command.CreateAdditionalBalance(ctx, organizationID, ledgerID, accountID, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create additional balance on command", err)

		logger.Errorf("Failed to create additional balance, Error: %s", err.Error())

		if httpErr := http.WithError(c, err); httpErr != nil {
			return httpErr
		}

		return nil
	}

	logger.Infof("Successfully created additional balance")

	if err := http.Created(c, balance); err != nil {
		return err
	}

	return nil
}
