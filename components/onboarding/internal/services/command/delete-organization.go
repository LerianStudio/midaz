package command

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// DeleteOrganizationByID fetch a new organization from the repository
func (uc *UseCase) DeleteOrganizationByID(ctx context.Context, id uuid.UUID) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.delete_organization_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
	)

	logger.Infof("Remove organization for id: %s", id)

	if err := uc.OrganizationRepo.Delete(ctx, id); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to delete organization on repo by id", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			logger.Errorf("Organization ID not found: %s", id.String())
			return pkg.ValidateBusinessError(constant.ErrOrganizationIDNotFound, reflect.TypeOf(mmodel.Organization{}).Name())
		}

		logger.Errorf("Error deleting organization: %v", err)

		return err
	}

	return nil
}
