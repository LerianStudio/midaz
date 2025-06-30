package in

import (
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type SettingsHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// Create a Setting.
//
//	@Summary		Create Setting
//	@Description	Endpoint to create a new Setting.
//	@Tags			Settings
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string							true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string							false	"Request ID for tracing"
//	@Param			organization_id	path		string							true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string							true	"Ledger ID in UUID format"
//	@Param			settings		body		mmodel.CreateSettingsInput		true	"Settings Input"
//	@Success		201				{object}	mmodel.Settings					"Successfully created setting"
//	@Failure		400				{object}	mmodel.Error					"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error					"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error					"Forbidden access"
//	@Failure		409				{object}	mmodel.Error					"Conflict - setting key already exists"
//	@Failure		500				{object}	mmodel.Error					"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/settings [post]
func (handler *SettingsHandler) CreateSettings(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_settings")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	payload := i.(*mmodel.CreateSettingsInput)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	logger.Infof("Request to create a setting with details: %#v", payload)

	settings, err := handler.Command.CreateSettings(ctx, organizationID, ledgerID, payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to create settings", err)

		return http.WithError(c, err)
	}

	logger.Infof("Successfully created setting")

	return http.Created(c, settings)
}

// GetSettingsByID retrieves a setting by its ID.
//
//	@Summary		Get Setting by ID
//	@Description	Retrieve a specific setting by its ID within an organization and ledger
//	@Tags			Settings
//	@Produce		json
//	@Param			Authorization	header		string				true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string				false	"Request ID for tracing"
//	@Param			organization_id	path		string				true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string				true	"Ledger ID in UUID format"
//	@Param			id				path		string				true	"Setting ID in UUID format"
//	@Success		200				{object}	mmodel.Settings		"Successfully retrieved setting"
//	@Failure		400				{object}	mmodel.Error		"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error		"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error		"Forbidden access"
//	@Failure		404				{object}	mmodel.Error		"Setting not found"
//	@Failure		500				{object}	mmodel.Error		"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/settings/{id} [get]
func (handler *SettingsHandler) GetSettingsByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_settings_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Request to get setting with id: %s", id)

	settings, err := handler.Query.GetSettingsByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get settings by id", err)

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved setting with id: %s", id)

	return http.OK(c, settings)
}

// UpdateSettings is a method that updates Setting information.
//
//	@Summary		Update a setting
//	@Description	Updates an existing setting's properties such as value and description within the specified ledger
//	@Tags			Settings
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string							true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string							false	"Request ID for tracing"
//	@Param			organization_id	path		string							true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string							true	"Ledger ID in UUID format"
//	@Param			id				path		string							true	"Setting ID in UUID format"
//	@Param			settings		body		mmodel.UpdateSettingsInput		true	"Settings Input"
//	@Success		200				{object}	mmodel.Settings					"Successfully updated setting"
//	@Failure		400				{object}	mmodel.Error					"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error					"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error					"Forbidden access"
//	@Failure		404				{object}	mmodel.Error					"Setting not found"
//	@Failure		409				{object}	mmodel.Error					"Conflict: Setting with the same key already exists"
//	@Failure		500				{object}	mmodel.Error					"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/settings/{id} [patch]
func (handler *SettingsHandler) UpdateSettings(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_settings")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating update of Setting with Setting ID: %s", id.String())

	payload := i.(*mmodel.UpdateSettingsInput)
	logger.Infof("Request to update a Setting with details: %#v", payload)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	_, err = handler.Command.UpdateSettings(ctx, organizationID, ledgerID, id, payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update Setting on command", err)

		logger.Errorf("Failed to update Setting with Setting ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	settings, err := handler.Query.GetSettingsByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve Setting on query", err)

		logger.Errorf("Failed to retrieve Setting with Setting ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully updated Setting with Setting ID: %s", id.String())

	return http.OK(c, settings)
}
