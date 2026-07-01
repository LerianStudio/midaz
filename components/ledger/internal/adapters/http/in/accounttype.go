// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type AccountTypeHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// --- Transport-agnostic cores -------------------------------------------------
//
// The createAccountType/updateAccountType/... methods below own the span, the
// service call and the success log. They take primitive args (parsed UUIDs, the
// already-decoded payload, the query map) so BOTH transports feed them: the Fiber
// wrappers pull those from *fiber.Ctx (Locals + the WithBody-decoded payload +
// c.Queries) and the Huma handlers (accounttype_handler_huma.go) pull them from the
// request envelope. Every canonical Midaz error the cores return is rendered by the
// caller — http.WithError on the Fiber path, http.HumaProblem on the Huma path — so
// the code + HTTP status are identical across both transports.

// createAccountType owns the span + service call + success log for an already-decoded
// payload. Body decode+validation happens BEFORE this core: the Fiber path decodes via
// the WithBody decorator, the Huma path decodes via http.DecodeAndValidate(RawBody).
func (handler *AccountTypeHandler) createAccountType(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *mmodel.CreateAccountTypeInput) (*mmodel.AccountType, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_account_type")
	defer span.End()

	recordSafePayloadAttributes(span, payload)
	logSafePayload(ctx, logger, "Request to create an account type", payload)

	accountType, err := handler.Command.CreateAccountType(ctx, organizationID, ledgerID, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create account type", err)

		return nil, err
	}

	return accountType, nil
}

// getAccountTypeByID retrieves a single account type.
func (handler *AccountTypeHandler) getAccountTypeByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.AccountType, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_account_type_by_id")
	defer span.End()

	accountType, err := handler.Query.GetAccountTypeByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve Account Type on query", err)

		return nil, err
	}

	return accountType, nil
}

// updateAccountType owns the span + service call + success log for an already-decoded
// payload (see createAccountType for the decode split across transports).
func (handler *AccountTypeHandler) updateAccountType(ctx context.Context, organizationID, ledgerID, id uuid.UUID, payload *mmodel.UpdateAccountTypeInput) (*mmodel.AccountType, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_account_type")
	defer span.End()

	recordSafePayloadAttributes(span, payload)
	logSafePayload(ctx, logger, "Request to update account type", payload)

	accountType, err := handler.Command.UpdateAccountType(ctx, organizationID, ledgerID, id, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update account type", err)

		return nil, err
	}

	return accountType, nil
}

// deleteAccountType removes an account type.
func (handler *AccountTypeHandler) deleteAccountType(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_account_type_by_id")
	defer span.End()

	if err := handler.Command.DeleteAccountTypeByID(ctx, organizationID, ledgerID, id); err != nil {
		handleSpanByErrorClass(span, "Failed to delete Account Type on command", err)

		return err
	}

	return nil
}

// getAllAccountTypes binds the query map imperatively (http.ValidateParameters — the
// SAME binder the Fiber path used) so a bad query yields the canonical 400, then
// returns the assembled cursor-paginated envelope.
func (handler *AccountTypeHandler) getAllAccountTypes(ctx context.Context, organizationID, ledgerID uuid.UUID, queries map[string]string) (http.Pagination, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_account_types")
	defer span.End()

	headerParams, err := http.ValidateParameters(queries)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		return http.Pagination{}, err
	}

	recordSafeQueryAttributes(span, headerParams)

	pagination := http.Pagination{
		Limit:     headerParams.Limit,
		Page:      headerParams.Page,
		Cursor:    headerParams.Cursor,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	if headerParams.Metadata != nil {
		accountTypes, cur, err := handler.Query.GetAllMetadataAccountType(ctx, organizationID, ledgerID, *headerParams)
		if err != nil {
			handleSpanByErrorClass(span, "Failed to retrieve all Account Types on query", err)

			return http.Pagination{}, err
		}

		pagination.SetItems(accountTypes)
		pagination.SetCursor(cur.Next, cur.Prev)

		return pagination, nil
	}

	headerParams.Metadata = &bson.M{}

	accountTypes, cur, err := handler.Query.GetAllAccountType(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve Account Types on query", err)

		return http.Pagination{}, err
	}

	pagination.SetItems(accountTypes)
	pagination.SetCursor(cur.Next, cur.Prev)

	return pagination, nil
}

// Create an Account Type.
//
//	@Summary		Create Account Type
//	@Description	Endpoint to create a new Account Type.
//	@Tags			Account Types
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id	header		string							false	"Request ID for tracing"
//	@Param			organization_id	path		string							true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string							true	"Ledger ID in UUID format"
//	@Param			accountType		body		mmodel.CreateAccountTypeInput	true	"Account Type Input"
//	@Success		201				{object}	mmodel.AccountType				"Successfully created account type"
//	@Failure		400				{object}	mmodel.Error					"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error					"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error					"Forbidden access"
//	@Failure		409				{object}	mmodel.Error					"Conflict - account type key value already exists"
//	@Failure		500				{object}	mmodel.Error					"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/account-types [post]
func (handler *AccountTypeHandler) CreateAccountType(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	accountType, err := handler.createAccountType(ctx, organizationID, ledgerID, i.(*mmodel.CreateAccountTypeInput))
	if err != nil {
		return http.WithError(c, err)
	}

	return http.Created(c, accountType)
}

// GetAccountTypeByID is a method that retrieves Account Type information by a given account type id.
//
//	@Summary		Retrieve a specific account type
//	@Description	Returns detailed information about an account type identified by its UUID within the specified ledger
//	@Tags			Account Types
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id	header		string				false	"Request ID for tracing"
//	@Param			organization_id	path		string				true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string				true	"Ledger ID in UUID format"
//	@Param			account_type_id				path		string				true	"Account Type ID in UUID format"
//	@Success		200				{object}	mmodel.AccountType	"Successfully retrieved account type"
//	@Failure		401				{object}	mmodel.Error		"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error		"Forbidden access"
//	@Failure		404				{object}	mmodel.Error		"Account type, ledger, or organization not found"
//	@Failure		500				{object}	mmodel.Error		"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/account-types/{account_type_id} [get]
func (handler *AccountTypeHandler) GetAccountTypeByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

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

	accountType, err := handler.getAccountTypeByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, accountType)
}

// Update an Account Type.
//
//	@Summary		Update Account Type
//	@Description	Endpoint to update an existing Account Type.
//	@Tags			Account Types
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id	header		string							false	"Request ID for tracing"
//	@Param			organization_id	path		string							true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string							true	"Ledger ID in UUID format"
//	@Param			account_type_id				path		string							true	"Account Type ID in UUID format"
//	@Param			accountType		body		mmodel.UpdateAccountTypeInput	true	"Account Type Update Input"
//	@Success		200				{object}	mmodel.AccountType				"Successfully updated account type"
//	@Failure		400				{object}	mmodel.Error					"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error					"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error					"Forbidden access"
//	@Failure		404				{object}	mmodel.Error					"Account type not found"
//	@Failure		500				{object}	mmodel.Error					"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/account-types/{account_type_id} [patch]
func (handler *AccountTypeHandler) UpdateAccountType(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

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

	accountType, err := handler.updateAccountType(ctx, organizationID, ledgerID, id, i.(*mmodel.UpdateAccountTypeInput))
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, accountType)
}

// DeleteAccountTypeByID is a method that deletes Account Type information.
//
//	@Summary		Delete an account type
//	@Description	Deletes an existing account type identified by its UUID within the specified ledger
//	@Tags			Account Types
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id	header	string	false	"Request ID for tracing"
//	@Param			organization_id	path	string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path	string	true	"Ledger ID in UUID format"
//	@Param			account_type_id				path	string	true	"Account Type ID in UUID format"
//	@Success		204				"Successfully deleted account type"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		404				{object}	mmodel.Error	"Account type not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/account-types/{account_type_id} [delete]
func (handler *AccountTypeHandler) DeleteAccountTypeByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

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

	if err := handler.deleteAccountType(ctx, organizationID, ledgerID, id); err != nil {
		return http.WithError(c, err)
	}

	return http.NoContent(c)
}

// GetAllAccountTypes is a method that retrieves all Account Types.
//
//	@Summary		Get all account types
//	@Description	Returns a paginated list of all account types for the specified organization and ledger, optionally filtered by metadata
//	@Tags			Account Types
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Request-Id	header		string																										false	"Request ID for tracing"
//	@Param			organization_id	path		string																										true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string																										true	"Ledger ID in UUID format"
//	@Param			metadata		query		string																										false	"JSON string to filter account types by metadata fields"
//	@Param			key_value		query		string																										false	"Filter account types by key value"
//	@Param			limit			query		int																											false	"Limit of account types per page (default: 10, max: 100)"
//	@Param			page			query		int																											false	"Page number for offset pagination (default: 1)"
//	@Param			cursor			query		string																										false	"Cursor for cursor-based pagination"
//	@Param			sort_order		query		string																										false	"Sort order (asc or desc, default: asc)"
//	@Param			start_date		query		string																										false	"Start date for filtering (YYYY-MM-DD)"
//	@Param			end_date		query		string																										false	"End date for filtering (YYYY-MM-DD)"
//	@Success		200				{object}	http.Pagination{items=[]mmodel.AccountType}	"Successfully retrieved account types"
//	@Failure		400				{object}	mmodel.Error																								"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error																								"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error																								"Forbidden access"
//	@Failure		404				{object}	mmodel.Error																								"Organization, ledger, or account types not found"
//	@Failure		500				{object}	mmodel.Error																								"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/account-types [get]
func (handler *AccountTypeHandler) GetAllAccountTypes(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	pagination, err := handler.getAllAccountTypes(ctx, organizationID, ledgerID, c.Queries())
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, pagination)
}
