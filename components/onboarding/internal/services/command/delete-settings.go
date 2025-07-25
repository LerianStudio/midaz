package command

import (
	"context"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"reflect"
)

// DeleteSettings removes settings from the repository.
func (uc *UseCase) DeleteSettings(ctx context.Context, organizationID, ledgerID, applicationName string) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_settings")
	defer span.End()

	logger.Infof("Trying to delete settings for org: %s, ledger: %s, app: %s", organizationID, ledgerID, applicationName)

	existingSettings, err := uc.SettingsRepo.FindAll(ctx, organizationID, ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to find existing settings", err)

		logger.Errorf("Error finding existing settings: %v", err)

		return err
	}

	if existingSettings == nil {
		return pkg.ValidateBusinessError(constant.ErrSettingsNotFound, reflect.TypeOf(mmodel.Settings{}).Name())
	}

	err = uc.SettingsRepo.Delete(ctx, organizationID, ledgerID, applicationName)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to delete settings on repository", err)

		logger.Errorf("Error deleting settings: %v", err)

		return err
	}

	logger.Infof("Successfully deleted settings for org: %s, ledger: %s, app: %s", organizationID, ledgerID, applicationName)

	return nil
}
