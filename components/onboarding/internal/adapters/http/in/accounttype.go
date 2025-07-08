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
