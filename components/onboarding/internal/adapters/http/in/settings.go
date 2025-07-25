package in

import (
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services/command"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services/query"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/gofiber/fiber/v2"
)

// SettingsHandler struct contains settings use cases for managing settings related operations.
type SettingsHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateSettings is a method that creates Settings information.
//
//	@Summary		Create new settings
//	@Description	Creates new settings for an organization, ledger, and application combination
//	@Tags			Settings
//	@Accept			json
//	@Produce		json
//	@Param			Authorization		header		string						true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id		header		string						false	"Request ID for tracing"
//	@Param			organizationID		path		string						true	"Organization ID"
//	@Param			ledgerID			path		string						true	"Ledger ID"
//	@Param			applicationName		path		string						true	"Application Name"
//	@Param			settings			body		mmodel.CreateSettingsInput	true	"Settings details"
//	@Success		201					{object}	mmodel.Settings				"Successfully created settings"
//	@Failure		400					{object}	mmodel.Error				"Invalid input, validation errors"
//	@Failure		401					{object}	mmodel.Error				"Unauthorized access"
//	@Failure		403					{object}	mmodel.Error				"Forbidden access"
//	@Failure		500					{object}	mmodel.Error				"Internal server error"
//	@Router			/v1/organizations/{organizationID}/ledgers/{ledgerID}/settings/{applicationName} [post]
func (handler *SettingsHandler) CreateSettings(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_settings")
	defer span.End()

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")

	payload := p.(*mmodel.CreateSettingsInput)
	logger.Infof("Request to create settings with details: %#v", payload)

	logger.Infof("Initiating create settings process with organizationID: %s, ledgerID: %s, applicationName: %s", organizationID, ledgerID, payload.ApplicationName)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	settings, err := handler.Command.CreateSettings(ctx, organizationID, ledgerID, payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to create settings", err)

		logger.Errorf("Failed to create settings with Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully created settings")

	return http.Created(c, settings)
}

// GetSettings is a method that retrieves Settings information.
//
//	@Summary		Get settings
//	@Description	Retrieves settings for an organization, ledger, and application combination
//	@Tags			Settings
//	@Accept			json
//	@Produce		json
//	@Param			Authorization		header		string		true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id		header		string		false	"Request ID for tracing"
//	@Param			organizationID		path		string		true	"Organization ID"
//	@Param			ledgerID			path		string		true	"Ledger ID"
//	@Param			applicationName		path		string		true	"Application Name"
//	@Success		200					{object}	mmodel.Settings	"Successfully retrieved settings"
//	@Failure		401					{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403					{object}	mmodel.Error	"Forbidden access"
//	@Failure		404					{object}	mmodel.Error	"Settings not found"
//	@Failure		500					{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organizationID}/ledgers/{ledgerID}/settings/{applicationName} [get]
func (handler *SettingsHandler) GetSettings(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_settings")
	defer span.End()

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")

	logger.Infof("Retrieving settings with organizationID: %s, ledgerID: %s", organizationID, ledgerID)

	settings, err := handler.Query.GetSettings(ctx, organizationID, ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get settings", err)

		logger.Errorf("Failed to get settings with Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved settings")

	return http.OK(c, settings)
}

// UpdateSettings is a method that updates Settings information.
//
//	@Summary		Update settings
//	@Description	Updates existing settings for an organization, ledger, and application combination
//	@Tags			Settings
//	@Accept			json
//	@Produce		json
//	@Param			Authorization		header		string						true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id		header		string						false	"Request ID for tracing"
//	@Param			organizationID		path		string						true	"Organization ID"
//	@Param			ledgerID			path		string						true	"Ledger ID"
//	@Param			applicationName		path		string						true	"Application Name"
//	@Param			settings			body		mmodel.UpdateSettingsInput	true	"Settings details"
//	@Success		200					{object}	mmodel.Settings				"Successfully updated settings"
//	@Failure		400					{object}	mmodel.Error				"Invalid input, validation errors"
//	@Failure		401					{object}	mmodel.Error				"Unauthorized access"
//	@Failure		403					{object}	mmodel.Error				"Forbidden access"
//	@Failure		404					{object}	mmodel.Error				"Settings not found"
//	@Failure		500					{object}	mmodel.Error				"Internal server error"
//	@Router			/v1/organizations/{organizationID}/ledgers/{ledgerID}/settings/{applicationName} [patch]
func (handler *SettingsHandler) UpdateSettings(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_settings")
	defer span.End()

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")
	applicationName := c.Params("application_name")

	logger.Infof("Initiating update settings process with organizationID: %s, ledgerID: %s, applicationName: %s", organizationID, ledgerID, applicationName)

	payload := p.(*mmodel.UpdateSettingsInput)
	logger.Infof("Request to update settings with details: %#v", payload)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	settings, err := handler.Command.UpdateSettings(ctx, organizationID, ledgerID, applicationName, payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update settings", err)

		logger.Errorf("Failed to update settings with Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully updated settings")

	return http.OK(c, settings)
}

// DeleteSettings is a method that deletes Settings information.
//
//	@Summary		Delete settings
//	@Description	Deletes existing settings for an organization, ledger, and application combination
//	@Tags			Settings
//	@Accept			json
//	@Produce		json
//	@Param			Authorization		header		string		true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id		header		string		false	"Request ID for tracing"
//	@Param			organizationID		path		string		true	"Organization ID"
//	@Param			ledgerID			path		string		true	"Ledger ID"
//	@Param			applicationName		path		string		true	"Application Name"
//	@Success		204					{object}	nil			"Successfully deleted settings"
//	@Failure		401					{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403					{object}	mmodel.Error	"Forbidden access"
//	@Failure		404					{object}	mmodel.Error	"Settings not found"
//	@Failure		500					{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organizationID}/ledgers/{ledgerID}/settings/{applicationName} [delete]
func (handler *SettingsHandler) DeleteSettings(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_settings")
	defer span.End()

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")
	applicationName := c.Params("application_name")

	logger.Infof("Initiating delete settings process with organizationID: %s, ledgerID: %s, applicationName: %s", organizationID, ledgerID, applicationName)

	err := handler.Command.DeleteSettings(ctx, organizationID, ledgerID, applicationName)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to delete settings", err)

		logger.Errorf("Failed to delete settings with Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully deleted settings")

	return http.NoContent(c)
}
