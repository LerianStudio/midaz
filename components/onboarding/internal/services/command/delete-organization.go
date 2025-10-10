// Package command implements write operations (commands) for the onboarding service.
// This file contains the command for deleting an organization.
package command

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
	"github.com/google/uuid"
)

// DeleteOrganizationByID soft-deletes an organization from the repository.
//
// This use case performs a soft delete by setting the DeletedAt timestamp.
// The organization record is preserved for auditing but is excluded from normal
// queries. Child entities such as ledgers and accounts are not automatically deleted.
//
// Business Rules:
//   - The organization must exist and not have been previously deleted.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - id: The UUID of the organization to be deleted.
//
// Returns:
//   - error: An error if the organization is not found or if the deletion fails.
//
// FIXME: The OpenTelemetry span name "usecase.delete_organization_by_id" is
// inconsistent with other commands, which use the "command." prefix.
// It should be renamed to "command.delete_organization_by_id" for consistency.
func (uc *UseCase) DeleteOrganizationByID(ctx context.Context, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.delete_organization_by_id")
	defer span.End()

	logger.Infof("Remove organization for id: %s", id)

	if err := uc.OrganizationRepo.Delete(ctx, id); err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrOrganizationIDNotFound, reflect.TypeOf(mmodel.Organization{}).Name())

			logger.Warnf("Organization ID not found: %s", id.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete organization on repo by id", err)

			return err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete organization on repo by id", err)

		logger.Errorf("Error deleting organization: %v", err)

		return err
	}

	return nil
}
