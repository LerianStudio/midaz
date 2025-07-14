package command

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
)

// DeleteSettingsByID deletes a setting by its ID.
// It returns an error if the operation fails or if the setting is not found.
func (uc *UseCase) DeleteSettingsByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_settings_by_id")
	defer span.End()

	logger.Infof("Initiating deletion of Setting with Setting ID: %s", id.String())

	if err := uc.SettingsRepo.Delete(ctx, organizationID, ledgerID, id); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to delete Setting on repo", err)

		logger.Errorf("Failed to delete Setting with Setting ID: %s, Error: %s", id.String(), err.Error())

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return pkg.ValidateBusinessError(constant.ErrSettingsNotFound, reflect.TypeOf(mmodel.Settings{}).Name())
		}

		return err
	}

	logger.Infof("Successfully deleted Setting with Setting ID: %s", id.String())

	return nil
}
