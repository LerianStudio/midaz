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

// UpdateSettings updates a setting by its ID.
// It returns the updated setting and an error if the operation fails.
func (uc *UseCase) UpdateSettings(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID, input *mmodel.UpdateSettingsInput) (*mmodel.Settings, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_settings")
	defer span.End()

	logger.Infof("Trying to update settings: %v", input)

	settings := &mmodel.Settings{
		Description: input.Description,
		Active:      input.Active,
	}

	settingsUpdated, err := uc.SettingsRepo.Update(ctx, organizationID, ledgerID, id, settings)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update settings on repo by id", err)

		logger.Errorf("Error updating settings on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrSettingsNotFound, reflect.TypeOf(mmodel.Settings{}).Name())
		}

		return nil, err
	}

	return settingsUpdated, nil
}
