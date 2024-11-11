package command

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	cn "github.com/LerianStudio/midaz/common/constant"

	"github.com/LerianStudio/midaz/components/ledger/internal/app"
	o "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/organization"
	"github.com/google/uuid"
)

// DeleteOrganizationByID fetch a new organization from the repository
func (uc *UseCase) DeleteOrganizationByID(ctx context.Context, id uuid.UUID) error {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.delete_organization_by_id")
	defer span.End()

	logger.Infof("Remove organization for id: %s", id)

	if err := uc.OrganizationRepo.Delete(ctx, id); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to delete organization on repo by id", err)

		logger.Errorf("Error deleting organization on repo by id: %v", err)

		if errors.Is(err, app.ErrDatabaseItemNotFound) {
			return common.ValidateBusinessError(cn.ErrOrganizationIDNotFound, reflect.TypeOf(o.Organization{}).Name())
		}

		return err
	}

	return nil
}
