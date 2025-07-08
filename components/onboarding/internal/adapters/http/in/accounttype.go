package in

import (
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/commons/postgres"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services/command"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services/query"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

type AccountTypeHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// Create an Account Type.
//
//	@Summary		Create Account Type
//	@Description	Endpoint to create a new Account Type.
//	@Tags			Account Types
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string							true	"Authorization Bearer Token with format: Bearer {token}"
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

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_account_type")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	payload := i.(*mmodel.CreateAccountTypeInput)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	logger.Infof("Request to create an account type with details: %#v", payload)

	accountType, err := handler.Command.CreateAccountType(ctx, organizationID, ledgerID, payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to create account type", err)

		return http.WithError(c, err)
	}

	logger.Infof("Successfully created account type")

	return http.Created(c, accountType)
}

// GetAccountTypeByID is a method that retrieves Account Type information by a given account type id.
//
//	@Summary		Retrieve a specific account type
//	@Description	Returns detailed information about an account type identified by its UUID within the specified ledger
//	@Tags			Account Types
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			id				path		string	true	"Account Type ID in UUID format"
//	@Success		200				{object}	mmodel.AccountType	"Successfully retrieved account type"
//	@Failure		401				{object}	mmodel.Error		"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error		"Forbidden access"
//	@Failure		404				{object}	mmodel.Error		"Account type, ledger, or organization not found"
//	@Failure		500				{object}	mmodel.Error		"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/account-types/{id} [get]
func (handler *AccountTypeHandler) GetAccountTypeByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_account_type_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating retrieval of Account Type with ID: %s", id.String())

	accountType, err := handler.Query.GetAccountTypeByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve Account Type on query", err)

		logger.Errorf("Failed to retrieve Account Type with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Account Type with ID: %s", id.String())

	return http.OK(c, accountType)
}

// Update an Account Type.
//
//	@Summary		Update Account Type
//	@Description	Endpoint to update an existing Account Type.
//	@Tags			Account Types
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string							true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string							false	"Request ID for tracing"
//	@Param			organization_id	path		string							true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string							true	"Ledger ID in UUID format"
//	@Param			id				path		string							true	"Account Type ID in UUID format"
//	@Param			accountType		body		mmodel.UpdateAccountTypeInput	true	"Account Type Update Input"
//	@Success		200				{object}	mmodel.AccountType				"Successfully updated account type"
//	@Failure		400				{object}	mmodel.Error					"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error					"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error					"Forbidden access"
//	@Failure		404				{object}	mmodel.Error					"Account type not found"
//	@Failure		500				{object}	mmodel.Error					"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/account-types/{id} [patch]
func (handler *AccountTypeHandler) UpdateAccountType(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_account_type")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	payload := i.(*mmodel.UpdateAccountTypeInput)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	logger.Infof("Request to update account type with ID: %s and details: %#v", id, payload)

	_, err = handler.Command.UpdateAccountType(ctx, organizationID, ledgerID, id, payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update account type", err)

		logger.Errorf("Failed to update account type with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	accountType, err := handler.Query.GetAccountTypeByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get updated account type", err)

		logger.Errorf("Failed to get updated account type with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully updated account type with ID: %s", id)

	return http.OK(c, accountType)
}

// DeleteAccountTypeByID is a method that deletes Account Type information.
//
//	@Summary		Delete an account type
//	@Description	Deletes an existing account type identified by its UUID within the specified ledger
//	@Tags			Account Types
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			id				path		string	true	"Account Type ID in UUID format"
//	@Success		204				"Successfully deleted account type"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		404				{object}	mmodel.Error	"Account type not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/account-types/{id} [delete]
func (handler *AccountTypeHandler) DeleteAccountTypeByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_account_type_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating deletion of Account Type with Account Type ID: %s", id.String())

	if err := handler.Command.DeleteAccountTypeByID(ctx, organizationID, ledgerID, id); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to delete Account Type on command", err)

		logger.Errorf("Failed to delete Account Type with Account Type ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully deleted Account Type with Account Type ID: %s", id.String())

	return http.NoContent(c)
}

// GetAllAccountTypes is a method that retrieves all Account Types.
//
//	@Summary		Get all account types
//	@Description	Returns a paginated list of all account types for the specified organization and ledger, optionally filtered by metadata
//	@Tags			Account Types
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			metadata		query		string	false	"JSON string to filter account types by metadata fields"
//	@Param			limit			query		int		false	"Limit of account types per page (default: 10, max: 100)"
//	@Param			page			query		int		false	"Page number for offset pagination (default: 1)"
//	@Param			cursor			query		string	false	"Cursor for cursor-based pagination"
//	@Param			sort_order		query		string	false	"Sort order (asc or desc, default: asc)"
//	@Param			start_date		query		string	false	"Start date for filtering (YYYY-MM-DD)"
//	@Param			end_date		query		string	false	"End date for filtering (YYYY-MM-DD)"
//	@Success		200				{object}	libPostgres.Pagination{items=[]mmodel.AccountType,next_cursor=string,prev_cursor=string,limit=int,page=nil}	"Successfully retrieved account types"
//	@Failure		400				{object}	mmodel.Error							"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error							"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error							"Forbidden access"
//	@Failure		404				{object}	mmodel.Error							"Organization, ledger, or account types not found"
//	@Failure		500				{object}	mmodel.Error							"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/account-types [get]
func (handler *AccountTypeHandler) GetAllAccountTypes(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_account_types")
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

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Account Types by metadata")

		accountTypes, cur, err := handler.Query.GetAllMetadataAccountType(ctx, organizationID, ledgerID, *headerParams)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to retrieve all Account Types on query", err)

			logger.Errorf("Failed to retrieve all Account Types, Error: %s", err.Error())

			return http.WithError(c, err)
		}

		logger.Infof("Successfully retrieved all Account Types by metadata")

		pagination.SetItems(accountTypes)
		pagination.SetCursor(cur.Next, cur.Prev)

		return http.OK(c, pagination)
	}

	logger.Infof("Initiating retrieval of Account Types")

	headerParams.Metadata = &bson.M{}

	accountTypes, cur, err := handler.Query.GetAllAccountType(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve Account Types on query", err)

		logger.Errorf("Failed to retrieve Account Types, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved %d Account Types", len(accountTypes))

	pagination.SetItems(accountTypes)
	pagination.SetCursor(cur.Next, cur.Prev)

	return http.OK(c, pagination)
}
