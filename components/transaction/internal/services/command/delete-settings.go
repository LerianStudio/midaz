package command

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
)

// DeleteSettingsByID is a method that deletes Setting information.
func (uc *UseCase) DeleteSettingsByID(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_settings_by_id")
	defer span.End()

	logger.Infof("Remove setting for id: %s", id.String())

	if err := uc.SettingsRepo.Delete(ctx, organizationID, ledgerID, id); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to delete setting on repo by id", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			logger.Errorf("Setting ID not found: %s", id.String())

			return pkg.ValidateBusinessError(constant.ErrSettingsNotFound, reflect.TypeOf(mmodel.Settings{}).Name())
		}

		logger.Errorf("Error deleting setting: %v", err)

		return err
	}

	logger.Infof("Successfully deleted setting with key: %s", id.String())

	return nil
}
