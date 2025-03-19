package command

import (
	"context"
	"errors"
	"reflect"

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

	// Create a new organization operation with telemetry for delete
	op := uc.Telemetry.NewOrganizationOperation("delete", id.String())

	// Add important attributes for telemetry
	op.WithAttributes(
		attribute.String("organization_id", id.String()),
	)

	// Record system metric
	op.RecordSystemicMetric(ctx)

	// Start trace span for this operation
	ctx = op.StartTrace(ctx)

	defer func() {
		// End span will be done by op.End() at the end of the function
	}()

	logger.Infof("Remove organization for id: %s", id)

	if err := uc.OrganizationRepo.Delete(ctx, id); err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to delete organization on repo by id", err)

		logger.Errorf("Error deleting organization on repo by id: %v", err)

		// Record error
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "delete_error", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return pkg.ValidateBusinessError(constant.ErrOrganizationIDNotFound, reflect.TypeOf(mmodel.Organization{}).Name())
		}

		return err
	}

	// Mark operation as successful
	op.End(ctx, "success")

	return nil
}
