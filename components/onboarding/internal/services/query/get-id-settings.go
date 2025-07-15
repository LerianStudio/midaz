package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
)

// GetSettingsByID retrieves a setting by its ID.
// It returns the setting if found, otherwise it returns an error.
func (uc *UseCase) GetSettingsByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Settings, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_settings_by_id")
	defer span.End()

	logger.Infof("Retrieving setting for id: %s", id)

	settings, err := uc.SettingsRepo.FindByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		libCommons.NewLoggerFromContext(ctx).Errorf("Error getting setting on repo by id: %v", err)

		logger.Errorf("Error getting setting on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrSettingsNotFound, reflect.TypeOf(mmodel.Settings{}).Name())
		}

		return nil, err
	}

	logger.Infof("Successfully retrieved setting for id: %s", id)

	return settings, nil
}
