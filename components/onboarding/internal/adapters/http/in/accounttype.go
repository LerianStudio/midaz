package in

import (
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services/command"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services/query"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
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
