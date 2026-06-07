// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/bson"

	// BalanceHandler struct contains a cqrs use case for managing balances.
	libLog "github.com/LerianStudio/lib-observability/log"
)

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
//	@Param			Authorization	header		string	false	"Bearer token authentication. Format: Bearer {access_token}. Only required when auth plugin is enabled."
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			limit			query		int		false	"Limit"			default(10)
//	@Param			start_date		query		string	false	"Start Date"	example	"2021-01-01"
//	@Param			end_date		query		string	false	"End Date"		example	"2021-01-01"
//	@Param			sort_order		query		string	false	"Sort Order"	Enums(asc,desc)
//	@Param			cursor			query		string	false	"Cursor"
//	@Success		200				{object}	http.Pagination{items=[]mmodel.Balance}
//	@Failure		400				{object}	mmodel.Error	"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/balances [Get]
func (handler *BalanceHandler) GetAllBalances(c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_balances")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		return http.WithError(c, err)
	}

	recordSafeQueryAttributes(span, headerParams)

	pagination := http.Pagination{
		Limit:     headerParams.Limit,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	headerParams.Metadata = &bson.M{}

	balances, cur, err := handler.Query.GetAllBalances(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve all Balances", err)

		return http.WithError(c, err)
	}

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
//	@Param			Authorization	header		string	false	"Bearer token authentication. Format: Bearer {access_token}. Only required when auth plugin is enabled."
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			account_id		path		string	true	"Account ID"
//	@Param			limit			query		int		false	"Limit"			default(10)
//	@Param			start_date		query		string	false	"Start Date"	example	"2021-01-01"
//	@Param			end_date		query		string	false	"End Date"		example	"2021-01-01"
//	@Param			sort_order		query		string	false	"Sort Order"	Enums(asc,desc)
//	@Param			cursor			query		string	false	"Cursor"
//	@Success		200				{object}	http.Pagination{items=[]mmodel.Balance}
//	@Failure		400				{object}	mmodel.Error	"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Account not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/balances [Get]
func (handler *BalanceHandler) GetAllBalancesByAccountID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_balances_by_account_id")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	accountID, err := http.GetUUIDFromLocals(c, "account_id")
	if err != nil {
		return http.WithError(c, err)
	}

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		return http.WithError(c, err)
	}

	recordSafeQueryAttributes(span, headerParams)

	pagination := http.Pagination{
		Limit:     headerParams.Limit,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	headerParams.Metadata = &bson.M{}

	balances, cur, err := handler.Query.GetAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve all Balances by account id", err)

		return http.WithError(c, err)
	}

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
//	@Param			Authorization	header		string	false	"Bearer token authentication. Format: Bearer {access_token}. Only required when auth plugin is enabled."
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

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_balance_by_id")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	balanceID, err := http.GetUUIDFromLocals(c, "balance_id")
	if err != nil {
		return http.WithError(c, err)
	}

	op, err := handler.Query.GetBalanceByID(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve balance by id", err)

		return http.WithError(c, err)
	}

	return http.OK(c, op)
}

// DeleteBalanceByID delete a balance by ID.
//
//	@Summary		Delete Balance by account
//	@Description	Delete a Balance with the input ID
//	@Tags			Balances
//	@Produce		json
//	@Param			Authorization	header		string			false	"Bearer token authentication. Format: Bearer {access_token}. Only required when auth plugin is enabled."
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

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_balance_by_id")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	balanceID, err := http.GetUUIDFromLocals(c, "balance_id")
	if err != nil {
		return http.WithError(c, err)
	}

	err = handler.Command.DeleteBalance(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete balance by id", err)

		return http.WithError(c, err)
	}

	return http.NoContent(c)
}

// UpdateBalance method that patch balance created before
//
//	@Summary		Update Balance
//	@Description	Update a Balance with the input payload
//	@Tags			Balances
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string					false	"Bearer token authentication. Format: Bearer {access_token}. Only required when auth plugin is enabled."
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

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_balance")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	balanceID, err := http.GetUUIDFromLocals(c, "balance_id")
	if err != nil {
		return http.WithError(c, err)
	}

	payload := p.(*mmodel.UpdateBalance)
	logSafePayload(ctx, logger, "Request to update a Balance", payload)

	recordSafePayloadAttributes(span, payload)

	balance, err := handler.Command.Update(ctx, organizationID, ledgerID, balanceID, *payload)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to update Balance on command", err)

		return http.WithError(c, err)
	}

	return http.OK(c, balance)
}

// GetBalancesByAlias retrieves balances by Alias.
//
//	@Summary		Get Balances using Alias
//	@Description	Get Balances with alias
//	@Tags			Balances
//	@Produce		json
//	@Param			Authorization	header		string	false	"Bearer token authentication. Format: Bearer {access_token}. Only required when auth plugin is enabled."
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			alias			path		string	true	"Alias (e.g. @person1)"
//	@Success		200				{object}	http.Pagination{items=[]mmodel.Balance}
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Balance not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/alias/{alias}/balances [Get]
func (handler *BalanceHandler) GetBalancesByAlias(c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_balances_by_alias")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	alias := c.Params("alias")

	balances, err := handler.Query.GetAllBalancesByAlias(ctx, organizationID, ledgerID, alias)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve balances by alias", err)

		return http.WithError(c, err)
	}

	if len(balances) == 0 {
		balances = []*mmodel.Balance{}
	}

	return http.OK(c, http.Pagination{
		Limit: 10,
		Items: balances,
	})
}

// GetBalancesExternalByCode retrieves external balances by code.
//
//	@Summary		Get External balances using code
//	@Description	Get External balances with code
//	@Tags			Balances
//	@Produce		json
//	@Param			Authorization	header		string	false	"Bearer token authentication. Format: Bearer {access_token}. Only required when auth plugin is enabled."
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			code			path		string	true	"Code (e.g. BRL)"
//	@Success		200				{object}	http.Pagination{items=[]mmodel.Balance}
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Balance not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/external/{code}/balances [Get]
func (handler *BalanceHandler) GetBalancesExternalByCode(c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_balances_external_by_code")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	code := c.Params("code")
	alias := cn.DefaultExternalAccountAliasPrefix + code

	balances, err := handler.Query.GetAllBalancesByAlias(ctx, organizationID, ledgerID, alias)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve balances by code", err)

		return http.WithError(c, err)
	}

	if len(balances) == 0 {
		balances = []*mmodel.Balance{}
	}

	return http.OK(c, http.Pagination{
		Limit: 10,
		Items: balances,
	})
}

// CreateAdditionalBalance handles the creation of a new balance using the provided payload and context.
//
//	@Summary		Create Additional Balance
//	@Description	Create an Additional Balance with the input payload
//	@Tags			Balances
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string							false	"Bearer token authentication. Format: Bearer {access_token}. Only required when auth plugin is enabled."
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

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	accountID, err := http.GetUUIDFromLocals(c, "account_id")
	if err != nil {
		return http.WithError(c, err)
	}

	payload := p.(*mmodel.CreateAdditionalBalance)
	logSafePayload(ctx, logger, "Request to create a Balance", payload)

	ctx, span := tracer.Start(ctx, "handler.create_additional_balance")
	defer span.End()

	recordSafePayloadAttributes(span, payload)

	balance, err := handler.Command.CreateAdditionalBalance(ctx, organizationID, ledgerID, accountID, payload)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to create additional balance on command", err)

		return http.WithError(c, err)
	}

	return http.Created(c, balance)
}

// GetBalanceAtTimestamp retrieves a balance at a specific point in time.
//
//	@Summary		Get Balance history at date
//	@Description	Get the historical state of a Balance at a specific point in time (yyyy-mm-dd hh:mm:ss format)
//	@Tags			Balances
//	@Produce		json
//	@Param			Authorization	header		string	false	"Bearer token authentication. Format: Bearer {access_token}. Only required when auth plugin is enabled."
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			balance_id		path		string	true	"Balance ID"
//	@Param			date			query		string	true	"Point in time (format: yyyy-mm-dd hh:mm:ss, e.g. 2024-01-15 10:30:00)"
//	@Success		200				{object}	mmodel.BalanceHistory
//	@Failure		400				{object}	mmodel.Error	"Invalid date format or date in the future"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Balance not found or no data available at date"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/balances/{balance_id}/history [Get]
func (handler *BalanceHandler) GetBalanceAtTimestamp(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_balance_at_timestamp")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get organization_id from path", err)

		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get ledger_id from path", err)

		return http.WithError(c, err)
	}

	balanceID, err := http.GetUUIDFromLocals(c, "balance_id")
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get balance_id from path", err)

		return http.WithError(c, err)
	}

	// Parse date from query parameter - requires yyyy-mm-dd hh:mm:ss format
	dateStr := c.Query("date")
	if dateStr == "" {
		err := pkg.ValidateBusinessError(cn.ErrMissingRequiredQueryParameter, "Balance", "date")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Missing date parameter", err)
		logger.Log(ctx, libLog.LevelWarn, "Missing date query parameter")

		return http.WithError(c, err)
	}

	date, hasTime, err := libCommons.ParseDateTime(dateStr, false)
	if err != nil {
		validationErr := pkg.ValidateBusinessError(cn.ErrInvalidDatetimeFormat, "Balance", "date", "yyyy-mm-dd hh:mm:ss")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid date format", validationErr)

		return http.WithError(c, validationErr)
	}

	// Time component is required
	if !hasTime {
		validationErr := pkg.ValidateBusinessError(cn.ErrInvalidDatetimeFormat, "Balance", "date", "yyyy-mm-dd hh:mm:ss")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Time component is required", validationErr)

		return http.WithError(c, validationErr)
	}

	balance, err := handler.Query.GetBalanceAtTimestamp(ctx, organizationID, ledgerID, balanceID, date)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to retrieve balance at date", err)

		return http.WithError(c, err)
	}

	// Convert to history response (without permission flags)
	return http.OK(c, balance.ToHistoryResponse())
}

// GetAccountBalancesAtTimestamp retrieves all balances for an account at a specific point in time.
//
//	@Summary		Get Account Balances history at date
//	@Description	Get the historical state of all Balances for an account at a specific point in time (yyyy-mm-dd hh:mm:ss format)
//	@Tags			Balances
//	@Produce		json
//	@Param			Authorization	header		string	false	"Bearer token authentication. Format: Bearer {access_token}. Only required when auth plugin is enabled."
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			account_id		path		string	true	"Account ID"
//	@Param			date			query		string	true	"Point in time (format: yyyy-mm-dd hh:mm:ss, e.g. 2024-01-15 10:30:00)"
//	@Success		200				{object}	[]mmodel.BalanceHistory
//	@Failure		400				{object}	mmodel.Error	"Invalid date format or date in the future"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Account not found or no data available at date"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/balances/history [Get]
func (handler *BalanceHandler) GetAccountBalancesAtTimestamp(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_account_balances_at_timestamp")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get organization_id from path", err)

		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get ledger_id from path", err)

		return http.WithError(c, err)
	}

	accountID, err := http.GetUUIDFromLocals(c, "account_id")
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get account_id from path", err)

		return http.WithError(c, err)
	}

	// Parse date from query parameter - requires yyyy-mm-dd hh:mm:ss format
	dateStr := c.Query("date")
	if dateStr == "" {
		err := pkg.ValidateBusinessError(cn.ErrMissingRequiredQueryParameter, "Balance", "date")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Missing date parameter", err)
		logger.Log(ctx, libLog.LevelWarn, "Missing date query parameter")

		return http.WithError(c, err)
	}

	date, hasTime, err := libCommons.ParseDateTime(dateStr, false)
	if err != nil {
		validationErr := pkg.ValidateBusinessError(cn.ErrInvalidDatetimeFormat, "Balance", "date", "yyyy-mm-dd hh:mm:ss")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid date format", validationErr)

		return http.WithError(c, validationErr)
	}

	// Time component is required
	if !hasTime {
		validationErr := pkg.ValidateBusinessError(cn.ErrInvalidDatetimeFormat, "Balance", "date", "yyyy-mm-dd hh:mm:ss")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Time component is required", validationErr)

		return http.WithError(c, validationErr)
	}

	balances, err := handler.Query.GetAccountBalancesAtTimestamp(ctx, organizationID, ledgerID, accountID, date)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to retrieve account balances at date", err)

		return http.WithError(c, err)
	}

	// Check if we have any balances to return
	if len(balances) == 0 {
		// No balances existed at that time
		err := pkg.ValidateBusinessError(cn.ErrNoBalanceDataAtTimestamp, date.Format("2006-01-02 15:04:05"))
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "No balance data available for the specified timestamp", err)

		return http.WithError(c, err)
	}

	// Convert to history responses (without permission flags)
	historyBalances := make([]*mmodel.BalanceHistory, len(balances))
	for i := range balances {
		historyBalances[i] = balances[i].ToHistoryResponse()
	}

	return http.OK(c, historyBalances)
}
