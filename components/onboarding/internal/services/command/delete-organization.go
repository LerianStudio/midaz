// Package command provides CQRS command handlers for the onboarding component.
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

// DeleteOrganizationByID removes an organization from the repository.
//
// This function performs a soft delete of an organization. The deletion
// operation is subject to referential integrity constraints - organizations
// with active ledgers, accounts, or child organizations may be restricted
// from deletion depending on the repository implementation.
//
// # Deletion Constraints
//
// Before deletion, consider:
//   - Child organizations (if any) should be reassigned or deleted
//   - Ledgers owned by this organization should be deleted
//   - Active balances should be zeroed and accounts closed
//   - Associated metadata will need cleanup
//
// # Soft Delete
//
// Organizations are typically soft-deleted (marked as deleted) rather than
// physically removed. This preserves:
//   - Audit trail and historical records
//   - References from transactions and operations
//   - Compliance requirements for data retention
//
// # Process
//
//  1. Extract logger and tracer from context for observability
//  2. Start tracing span "usecase.delete_organization_by_id"
//  3. Call repository Delete method
//  4. Handle not found error (ErrOrganizationIDNotFound)
//  5. Handle other database errors
//  6. Return success or error
//
// # Parameters
//
//   - ctx: Request context containing tenant info, tracing, and cancellation
//   - id: The UUID of the organization to delete
//
// # Returns
//
//   - error: nil on success, or error if:
//   - Organization not found (ErrOrganizationIDNotFound)
//   - Referential integrity violation
//   - Database operation fails
//
// # Error Scenarios
//
//   - ErrOrganizationIDNotFound: No organization with given ID
//   - Referential integrity violation (has dependent entities)
//   - Database connection failure
//   - Context cancellation/timeout
//
// # Observability
//
// Creates tracing span "usecase.delete_organization_by_id" with error events.
// Logs organization ID at info level, warnings for not found, errors for failures.
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
