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
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/google/uuid"
)

// DeleteOrganizationByID fetch a new organization from the repository
func (uc *UseCase) DeleteOrganizationByID(ctx context.Context, id uuid.UUID) error {
	logger := pkg.NewLoggerFromContext(ctx)

	op := uc.Telemetry.NewOrganizationOperation("delete", id.String())

	op.WithAttributes(
		attribute.String("organization_id", id.String()),
	)

	op.RecordSystemicMetric(ctx)
	ctx = op.StartTrace(ctx)

	logger.Infof("Remove organization for id: %s", id)

	// Get the organization before deletion to record its status and hierarchy info
	originalOrg, err := uc.OrganizationRepo.Find(ctx, id)
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			mopentelemetry.HandleSpanError(&op.span, "Organization not found", err)
			logger.Errorf("Organization not found: %v", err)
			op.WithAttribute("error_detail", "not_found")
			op.RecordError(ctx, "not_found_error", err)
			return pkg.ValidateBusinessError(constant.ErrOrganizationIDNotFound, reflect.TypeOf(mmodel.Organization{}).Name())
		}

		mopentelemetry.HandleSpanError(&op.span, "Failed to fetch organization before deletion", err)
		logger.Errorf("Error fetching organization before deletion: %v", err)
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "fetch_error", err)
		return err
	}

	// Record metrics about the organization being deleted
	// Create meter for organization lifecycle metrics
	meter := otel.Meter("business.organization")

	// Create counter for organizations deleted
	orgDeletionCounter, _ := meter.Int64Counter(
		mopentelemetry.GetMetricName("business", "organization", "deletion", "count"),
		metric.WithDescription("Count of organization deletions"),
		metric.WithUnit("{organization}"),
	)

	// Get current time attributes for time-based analysis
	now := time.Now()
	year, week := now.ISOWeek()

	// Prepare attributes for the deletion event
	attrs := []attribute.KeyValue{
		attribute.String("organization_id", id.String()),
		attribute.Int("year", year),
		attribute.Int("month", int(now.Month())),
		attribute.Int("week", week),
		attribute.Int("day", now.Day()),
		attribute.Int("hour", now.Hour()),
	}

	// Add status if available
	if originalOrg.Status.Code != "" {
		attrs = append(attrs, attribute.String("status", originalOrg.Status.Code))
	}

	// Calculate and add hierarchy depth
	hierarchyDepth, err := uc.CalculateOrganizationHierarchyDepth(ctx, id.String())
	if err != nil {
		// If there's an error, default to depth 1 if no parent, depth 2 if it has a parent
		hierarchyDepth = 1
		if originalOrg.ParentOrganizationID != nil {
			hierarchyDepth = 2
			attrs = append(attrs, attribute.String("parent_organization_id", *originalOrg.ParentOrganizationID))
		}
	} else {
		// Add parent ID if this is not a root organization
		if originalOrg.ParentOrganizationID != nil {
			attrs = append(attrs, attribute.String("parent_organization_id", *originalOrg.ParentOrganizationID))
		}
	}

	attrs = append(attrs, attribute.Int("hierarchy_depth", hierarchyDepth))

	// Record the deletion with contextual information
	orgDeletionCounter.Add(ctx, 1, metric.WithAttributes(attrs...))

	if err := uc.OrganizationRepo.Delete(ctx, id); err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to delete organization on repo by id", err)
		logger.Errorf("Error deleting organization on repo by id: %v", err)
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "delete_error", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return pkg.ValidateBusinessError(constant.ErrOrganizationIDNotFound, reflect.TypeOf(mmodel.Organization{}).Name())
		}

		return err
	}

	op.End(ctx, "success")

	return nil
}
