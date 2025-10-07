// Package query implements read operations (queries) for the onboarding service.
// This file contains query implementation.

package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// CountOrganizations returns the total count of active organizations for pagination.
//
// Counts total organizations in PostgreSQL. Excludes soft-deleted organizations.
// Used for X-Total-Count header and pagination metadata.
//
// Returns: Total count of active organizations, or error if query fails
// OpenTelemetry: Creates span "query.count_organizations"
func (uc *UseCase) CountOrganizations(ctx context.Context) (int64, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.count_organizations")
	defer span.End()

	logger.Infof("Counting organizations")

	count, err := uc.OrganizationRepo.Count(ctx)
	if err != nil {
		logger.Errorf("Error counting organizations on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrNoOrganizationsFound, reflect.TypeOf(mmodel.Organization{}).Name())

			logger.Warn("No organizations found")

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to count organizations on repo", err)

			return 0, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to count organizations on repo", err)

		return 0, err
	}

	return count, nil
}
