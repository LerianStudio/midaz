package query

import (
	"context"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"reflect"
)

// CountOrganizations returns the total count of organizations
func (uc *UseCase) CountOrganizations(ctx context.Context) (int64, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.count_organizations")
	defer span.End()

	logger.Infof("Counting organizations")

	count, err := uc.OrganizationRepo.Count(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to count organizations on repo", err)
		logger.Errorf("Error counting organizations on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return 0, pkg.ValidateBusinessError(constant.ErrNoOrganizationsFound, reflect.TypeOf(mmodel.Organization{}).Name())
		}

		return 0, err
	}

	return count, nil
}
