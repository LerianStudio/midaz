package query

import (
	"context"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"reflect"
)

// GetSettings retrieves settings from the repository.
func (uc *UseCase) GetSettings(ctx context.Context, organizationID, ledgerID string) ([]*mmodel.Settings, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_settings")
	defer span.End()

	logger.Infof("Retrieving settings for org: %s, ledger: %s", organizationID, ledgerID)

	settings, err := uc.SettingsRepo.Find(ctx, organizationID, ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get settings on repository", err)

		logger.Errorf("Error getting settings: %v", err)

		return nil, err
	}

	if settings == nil {
		return nil, pkg.ValidateBusinessError(constant.ErrSettingsNotFound, reflect.TypeOf(mmodel.Settings{}).Name())
	}

	return settings, nil
}
