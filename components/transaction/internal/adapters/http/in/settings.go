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
//	@Description	Updates an existing setting's properties such as active status and description within the specified ledger
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

// DeleteSettingsByID is a method that deletes Setting information.
//
//	@Summary		Delete a setting
//	@Description	Deletes an existing setting identified by its UUID within the specified ledger
//	@Tags			Settings
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			id				path		string	true	"Setting ID in UUID format"
//	@Success		204				"Successfully deleted setting"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		404				{object}	mmodel.Error	"Setting not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/settings/{id} [delete]
func (handler *SettingsHandler) DeleteSettingsByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_settings_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating deletion of Setting with Setting ID: %s", id.String())

	if err := handler.Command.DeleteSettingsByID(ctx, organizationID, ledgerID, id); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to delete Setting on command", err)

		logger.Errorf("Failed to delete Setting with Setting ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully deleted Setting with Setting ID: %s", id.String())

	return http.NoContent(c)
}

// GetAllSettings is a method that retrieves all Settings from a ledger.
//
//	@Summary		Get all settings
//	@Description	Retrieves all settings from the specified ledger with cursor pagination support
//	@Tags			Settings
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			limit			query		int		false	"Maximum number of settings to return (default: 10)"
//	@Param			cursor			query		string	false	"Cursor for pagination"
//	@Param			sort_order		query		string	false	"Sort order: 'asc' or 'desc' (default: 'asc')"
//	@Param			start_date		query		string	false	"Start date for filtering (ISO 8601 format)"
//	@Param			end_date		query		string	false	"End date for filtering (ISO 8601 format)"
//	@Success		200				{object}	libPostgres.Pagination{items=[]mmodel.Settings,next_cursor=string,prev_cursor=string,limit=int,page=nil}
//	@Failure		400				{object}	mmodel.Error
//	@Failure		401				{object}	mmodel.Error
//	@Failure		404				{object}	mmodel.Error
//	@Failure		500				{object}	mmodel.Error
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/settings [get]
func (sh *SettingsHandler) GetAllSettings(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_settings")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	logger.Infof("Request to get all settings for ledger: %s", ledgerID)

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Initiating retrieval of all Settings with pagination: %#v", headerParams)

	settings, cur, err := sh.Query.GetAllSettings(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get all settings", err)

		logger.Errorf("Error getting all settings: %v", err)

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved %d settings", len(settings))

	pagination := libPostgres.Pagination{
		Limit:      headerParams.Limit,
		NextCursor: headerParams.Cursor,
		SortOrder:  headerParams.SortOrder,
		StartDate:  headerParams.StartDate,
		EndDate:    headerParams.EndDate,
	}

	pagination.SetItems(settings)
	pagination.SetCursor(cur.Next, cur.Prev)

	return http.OK(c, pagination)
}
