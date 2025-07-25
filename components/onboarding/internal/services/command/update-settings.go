package command

import (
	"context"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"reflect"
	"time"
)

// UpdateSettings updates existing settings in the repository.
func (uc *UseCase) UpdateSettings(ctx context.Context, organizationID, ledgerID, applicationName string, usi *mmodel.UpdateSettingsInput) (*mmodel.Settings, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_settings")
	defer span.End()

	logger.Infof("Trying to update settings for org: %s, ledger: %s, app: %s", organizationID, ledgerID, applicationName)

	existingSettings, err := uc.SettingsRepo.Find(ctx, organizationID, ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to find existing settings", err)

		logger.Errorf("Error finding existing settings: %v", err)

		return nil, err
	}

	if existingSettings == nil {
		return nil, pkg.ValidateBusinessError(constant.ErrSettingsNotFound, reflect.TypeOf(mmodel.Settings{}).Name())
	}

	updatedSettings := &mmodel.Settings{
		OrganizationID:  organizationID,
		LedgerID:        ledgerID,
		ApplicationName: applicationName,
		Settings:        usi.Settings,
		Enabled:         usi.Enabled,
		UpdatedAt:       time.Now(),
	}

	err = libOpentelemetry.SetSpanAttributesFromStruct(&span, "settings_repository_input", updatedSettings)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert settings repository input to JSON string", err)

		return nil, err
	}

	err = uc.SettingsRepo.Update(ctx, updatedSettings)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update settings on repository", err)

		logger.Errorf("Error updating settings: %v", err)

		return nil, err
	}

	return updatedSettings, nil
}
