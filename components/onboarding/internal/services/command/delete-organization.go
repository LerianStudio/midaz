package command

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"go.opentelemetry.io/otel/attribute"

	"github.com/google/uuid"
)

// DeleteOrganizationByID fetch a new organization from the repository
func (uc *UseCase) DeleteOrganizationByID(ctx context.Context, id uuid.UUID) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	// Start time for duration measurement
	startTime := time.Now()

	ctx, span := tracer.Start(ctx, "usecase.delete_organization_by_id")
	defer span.End()

	// Record operation metrics
	uc.recordOnboardingMetrics(ctx, "organization", "delete",
		attribute.String("organization_id", id.String()))

	logger.Infof("Remove organization for id: %s", id)

	if err := uc.OrganizationRepo.Delete(ctx, id); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to delete organization on repo by id", err)

		logger.Errorf("Error deleting organization on repo by id: %v", err)

		// Record error
		uc.recordOnboardingError(ctx, "organization", "delete_error",
			attribute.String("organization_id", id.String()),
			attribute.String("error_detail", err.Error()))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return pkg.ValidateBusinessError(constant.ErrOrganizationIDNotFound, reflect.TypeOf(mmodel.Organization{}).Name())
		}

		return err
	}

	// Record successful completion and duration
	uc.recordOnboardingDuration(ctx, startTime, "organization", "delete", "success",
		attribute.String("organization_id", id.String()))

	return nil
}
