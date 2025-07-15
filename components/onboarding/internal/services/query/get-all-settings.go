package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libHTTP "github.com/LerianStudio/lib-commons/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/google/uuid"
)

// GetAllSettings fetch all Settings from the repository
func (uc *UseCase) GetAllSettings(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Settings, libHTTP.CursorPagination, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_settings")
	defer span.End()

	logger.Infof("Retrieving settings")

	settings, cur, err := uc.SettingsRepo.FindAll(ctx, organizationID, ledgerID, filter.ToCursorPagination())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get settings on repo", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, libHTTP.CursorPagination{}, pkg.ValidateBusinessError(constant.ErrSettingsNotFound, reflect.TypeOf(mmodel.Settings{}).Name())
		}

		logger.Errorf("Error getting settings on repo: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	logger.Infof("Successfully retrieved %d settings", len(settings))

	return settings, cur, nil
}
