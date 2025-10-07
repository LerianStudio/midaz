// Package command implements write operations (commands) for the onboarding service.
// This file contains the DeleteOrganizationByID command implementation.
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
// This method implements the delete organization use case, which performs a soft delete
// by setting the DeletedAt timestamp. The organization record remains in the database
// but is excluded from normal queries.
//
// Soft Deletion:
//   - Sets DeletedAt timestamp to current time
//   - Organization remains in database for audit purposes
//   - Excluded from list and get operations (via WHERE deleted_at IS NULL)
//   - Can be used for historical reporting
//   - Cannot be undeleted (no restore operation)
//
// Cascade Behavior:
//   - Child entities (ledgers, accounts, etc.) are NOT automatically deleted
//   - Clients should delete child entities first if desired
//   - Foreign key constraints prevent deletion if child entities reference this organization
//
// Business Rules:
//   - Organization must exist
//   - Organization must not have active child entities (enforced by constraints)
//   - Soft delete is NOT idempotent (deleting already deleted org returns error)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - id: UUID of the organization to delete
//
// Returns:
//   - error: Business error if organization not found, database error if deletion fails
//
// Possible Errors:
//   - ErrOrganizationIDNotFound: Organization doesn't exist or already deleted
//   - Database errors: Foreign key violations, connection failures
//
// Example:
//
//	err := useCase.DeleteOrganizationByID(ctx, orgID)
//	if err != nil {
//	    return err
//	}
//	// Organization is soft-deleted (DeletedAt set)
//
// OpenTelemetry:
//   - Creates span "usecase.delete_organization_by_id"
//   - Records errors as span events
//
// Note: The span name uses "usecase" prefix instead of "command" (inconsistent with other methods)
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
