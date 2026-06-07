// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
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
//	@Param			Authorization	header		string						false	"Bearer token authentication. Format: Bearer {access_token}. Only required when auth plugin is enabled."
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

	logger, tracer, _, metricFactory := libObservability.NewTrackingFromContext(ctx)

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	payload := i.(*mmodel.CreateAccountInput)
	logSafePayload(ctx, logger, "Request to create an account", payload)

	ctx, span := tracer.Start(ctx, "handler.create_account")
	defer span.End()

	recordSafePayloadAttributes(span, payload)

	token := c.Get("Authorization")

	account, err := handler.Command.CreateAccount(ctx, organizationID, ledgerID, payload, token)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to create Account on command", err)

		return http.WithError(c, err)
	}

	if err := metricFactory.RecordAccountCreated(
		ctx,
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
	); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to record account created metric", err)
	}

	return http.Created(c, account)
}

// GetAllAccounts is a method that retrieves all Accounts.
//
//	@Summary		List all accounts
//	@Description	Returns a paginated list of accounts within the specified ledger, optionally filtered by metadata, date range, and other criteria
//	@Tags			Accounts
//	@Produce		json
//	@Param			Authorization	header		string																false	"Bearer token authentication. Format: Bearer {access_token}. Only required when auth plugin is enabled."
//	@Param			X-Request-Id	header		string																false	"Request ID for tracing"
//	@Param			organization_id	path		string																true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string																true	"Ledger ID in UUID format"
//	@Param			metadata		query		string																false	"JSON string to filter accounts by metadata fields"
//	@Param			limit			query		int																	false	"Maximum number of records to return per page"	default(10)	minimum(1)	maximum(100)
//	@Param			page			query		int																	false	"Page number for pagination"					default(1)	minimum(1)
//	@Param			start_date		query		string																false	"Filter accounts created on or after this date (format: YYYY-MM-DD)"
//	@Param			end_date		query		string																false	"Filter accounts created on or before this date (format: YYYY-MM-DD)"
//	@Param			sort_order		query		string																false	"Sort direction for results based on creation date"	Enums(asc,desc)
//	@Param			portfolio_id	query		string																false	"Filter accounts by portfolio ID (UUID format). If both portfolio_id and segment_id are provided, both filters are applied (AND semantics)."	format(uuid)
//	@Param			segment_id		query		string																false	"Filter accounts by segment ID (UUID format). If both portfolio_id and segment_id are provided, both filters are applied (AND semantics)."	format(uuid)
//	@Param			status			query		string																false	"Filter accounts by status"
//	@Param			type			query		string																false	"Filter accounts by type (e.g., deposit, savings, external)"
//	@Param			asset_code		query		string																false	"Filter accounts by asset code (e.g., USD, BRL, EUR)"
//	@Param			entity_id		query		string																false	"Filter accounts by entity ID"
//	@Param			blocked			query		boolean																false	"Filter accounts by blocked status"				Enums(true,false)
//	@Param			parent_account_id	query	string																false	"Filter accounts by parent account ID (UUID format)"	format(uuid)
//	@Param			name				query	string																false	"Filter accounts by name (case-insensitive, prefix match)"	maxLength(256)
//	@Param			alias				query	string																false	"Filter accounts by alias (case-insensitive, prefix match)"	maxLength(256)
//	@Success		200				{object}	http.Pagination{items=[]mmodel.Account}	"Successfully retrieved accounts list"
//	@Failure		400				{object}	mmodel.Error														"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error														"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error														"Forbidden access"
//	@Failure		404				{object}	mmodel.Error														"Organization or ledger not found"
//	@Failure		500				{object}	mmodel.Error														"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts [get]
func (handler *AccountHandler) GetAllAccounts(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_accounts")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	var (
		portfolioID *uuid.UUID
		segmentID   *uuid.UUID
	)

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		return http.WithError(c, err)
	}

	if headerParams.Status != nil && !isValidStatus(*headerParams.Status, accountAllowedStatuses) {
		err := pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityAccount, "status")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters: invalid account status", err)

		logger.Log(ctx, libLog.LevelWarn, "Failed to validate account status query parameter", libLog.String("status", *headerParams.Status), libLog.Err(err))

		return http.WithError(c, err)
	}

	recordSafeQueryAttributes(span, headerParams)

	pagination := http.Pagination{
		Limit:     headerParams.Limit,
		Page:      headerParams.Page,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	if !libCommons.IsNilOrEmpty(&headerParams.PortfolioID) {
		parsedID := uuid.MustParse(headerParams.PortfolioID)
		portfolioID = &parsedID
	}

	if !libCommons.IsNilOrEmpty(&headerParams.SegmentID) {
		parsedID := uuid.MustParse(headerParams.SegmentID)
		segmentID = &parsedID
	}

	if headerParams.Metadata != nil {
		accounts, err := handler.Query.GetAllMetadataAccounts(ctx, organizationID, ledgerID, portfolioID, segmentID, *headerParams)
		if err != nil {
			handleSpanByErrorClass(span, "Failed to retrieve all Accounts on query", err)

			return http.WithError(c, err)
		}

		pagination.SetItems(accounts)

		return http.OK(c, pagination)
	}

	headerParams.Metadata = &bson.M{}

	accounts, err := handler.Query.GetAllAccount(ctx, organizationID, ledgerID, portfolioID, segmentID, *headerParams)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve all Accounts on query", err)

		return http.WithError(c, err)
	}

	pagination.SetItems(accounts)

	return http.OK(c, pagination)
}

// GetAccountByID is a method that retrieves Account information by a given account id.
//
//	@Summary		Retrieve a specific account
//	@Description	Returns detailed information about an account identified by its UUID within the specified ledger
//	@Tags			Accounts
//	@Produce		json
//	@Param			Authorization	header		string			false	"Bearer token authentication. Format: Bearer {access_token}. Only required when auth plugin is enabled."
//	@Param			X-Request-Id	header		string			false	"Request ID for tracing"
//	@Param			organization_id	path		string			true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string			true	"Ledger ID in UUID format"
//	@Param			account_id				path		string			true	"Account ID in UUID format"
//	@Success		200				{object}	mmodel.Account	"Successfully retrieved account"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Account, ledger, or organization not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id} [get]
func (handler *AccountHandler) GetAccountByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_account_by_id")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	account, err := handler.Query.GetAccountByID(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve Account on query", err)

		return http.WithError(c, err)
	}

	return http.OK(c, account)
}

// GetAccountExternalByCode is a method that retrieves External Account information by a given asset code.
//
//	@Summary		Retrieve an account by external code
//	@Description	Returns detailed information about an account identified by its external code within the specified ledger
//	@Tags			Accounts
//	@Produce		json
//	@Param			Authorization	header		string			false	"Bearer token authentication. Format: Bearer {access_token}. Only required when auth plugin is enabled."
//	@Param			X-Request-Id	header		string			false	"Request ID for tracing"
//	@Param			organization_id	path		string			true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string			true	"Ledger ID in UUID format"
//	@Param			code			path		string			true	"Account External Code (e.g. BRL)"
//	@Success		200				{object}	mmodel.Account	"Successfully retrieved account"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Account with the specified external code, ledger, or organization not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/external/{code} [get]
func (handler *AccountHandler) GetAccountExternalByCode(c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_account_external_by_code")
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

	alias := constant.DefaultExternalAccountAliasPrefix + code

	account, err := handler.Query.GetAccountByAlias(ctx, organizationID, ledgerID, nil, alias)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve Account on query", err)

		return http.WithError(c, err)
	}

	return http.OK(c, account)
}

// GetAccountByAlias is a method that retrieves Account information by a given account alias.
//
//	@Summary		Retrieve an account by alias
//	@Description	Returns detailed information about an account identified by its alias within the specified ledger
//	@Tags			Accounts
//	@Produce		json
//	@Param			Authorization	header		string			false	"Bearer token authentication. Format: Bearer {access_token}. Only required when auth plugin is enabled."
//	@Param			X-Request-Id	header		string			false	"Request ID for tracing"
//	@Param			organization_id	path		string			true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string			true	"Ledger ID in UUID format"
//	@Param			alias			path		string			true	"Account alias (e.g. @person1)"
//	@Success		200				{object}	mmodel.Account	"Successfully retrieved account"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Account with the specified alias, ledger, or organization not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/alias/{alias} [get]
func (handler *AccountHandler) GetAccountByAlias(c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_account_by_alias")
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

	account, err := handler.Query.GetAccountByAlias(ctx, organizationID, ledgerID, nil, alias)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve Account on query", err)

		return http.WithError(c, err)
	}

	return http.OK(c, account)
}

// UpdateAccount is a method that updates Account information.
//
//	@Summary		Update an account
//	@Description	Updates an existing account's properties such as name, status, portfolio, segment, and metadata within the specified ledger
//	@Tags			Accounts
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string						false	"Bearer token authentication. Format: Bearer {access_token}. Only required when auth plugin is enabled."
//	@Param			X-Request-Id	header		string						false	"Request ID for tracing"
//	@Param			organization_id	path		string						true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string						true	"Ledger ID in UUID format"
//	@Param			account_id				path		string						true	"Account ID in UUID format"
//	@Param			account			body		mmodel.UpdateAccountInput	true	"Account properties to update including name, status, portfolio, segment, and optional metadata"
//	@Success		200				{object}	mmodel.Account				"Successfully updated account"
//	@Failure		400				{object}	mmodel.Error				"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error				"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error				"Forbidden access"
//	@Failure		404				{object}	mmodel.Error				"Account, ledger, organization, portfolio, or segment not found"
//	@Failure		409				{object}	mmodel.Error				"Conflict: Account with the same name already exists"
//	@Failure		500				{object}	mmodel.Error				"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id} [patch]
func (handler *AccountHandler) UpdateAccount(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_account")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	payload := i.(*mmodel.UpdateAccountInput)
	logSafePayload(ctx, logger, "Request to update account", payload)

	recordSafePayloadAttributes(span, payload)

	if _, err := handler.Command.UpdateAccount(ctx, organizationID, ledgerID, nil, id, payload); err != nil {
		handleSpanByErrorClass(span, "Failed to update Account on command", err)

		return http.WithError(c, err)
	}

	account, err := handler.Query.GetAccountByID(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve Account on query", err)

		return http.WithError(c, err)
	}

	return http.OK(c, account)
}

// DeleteAccountByID is a method that removes Account information by a given account id.
//
//	@Summary		Delete an account
//	@Description	Permanently removes an account from the specified ledger. This operation cannot be undone.
//	@Tags			Accounts
//	@Param			Authorization	header		string			false	"Bearer token authentication. Format: Bearer {access_token}. Only required when auth plugin is enabled."
//	@Param			X-Request-Id	header		string			false	"Request ID for tracing"
//	@Param			organization_id	path		string			true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string			true	"Ledger ID in UUID format"
//	@Param			account_id				path		string			true	"Account ID in UUID format"
//	@Success		204				"Account successfully deleted"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Account, ledger, or organization not found"
//	@Failure		409				{object}	mmodel.Error	"Conflict: Account cannot be deleted due to existing dependencies"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id} [delete]
func (handler *AccountHandler) DeleteAccountByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_account_by_id")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	token := c.Get("Authorization")

	if err := handler.Command.DeleteAccountByID(ctx, organizationID, ledgerID, nil, id, token); err != nil {
		handleSpanByErrorClass(span, "Failed to remove Account on command", err)

		return http.WithError(c, err)
	}

	return http.NoContent(c)
}

// CountAccounts is a method that counts all accounts for a given organization and ledger, with an optional portfolio ID.
//
//	@Summary		Count accounts
//	@Description	Returns the total count of accounts for the specified organization, ledger, and optional portfolio
//	@Tags			Accounts
//	@Param			Authorization	header	string	false	"Bearer token authentication. Format: Bearer {access_token}. Only required when auth plugin is enabled."
//	@Param			X-Request-Id	header	string	false	"Request ID for tracing"
//	@Param			organization_id	path	string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path	string	true	"Ledger ID in UUID format"
//	@Success		204				"Successfully retrieved accounts count"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Organization or ledger not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/metrics/count [head]
func (handler *AccountHandler) CountAccounts(c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.count_accounts")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	count, err := handler.Query.CountAccounts(ctx, organizationID, ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to count accounts", err)

		return http.WithError(c, err)
	}

	c.Set(constant.XTotalCount, fmt.Sprintf("%d", count))
	c.Set(constant.ContentLength, "0")

	return http.NoContent(c)
}
