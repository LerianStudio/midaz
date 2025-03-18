package command

import (
	"context"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"go.opentelemetry.io/otel/attribute"

	"github.com/google/uuid"
)

// CreateSegment creates a new segment persists data in the repository.
func (uc *UseCase) CreateSegment(ctx context.Context, organizationID, ledgerID uuid.UUID, cpi *mmodel.CreateSegmentInput) (*mmodel.Segment, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	// Start time for duration measurement
	startTime := time.Now()

	ctx, span := tracer.Start(ctx, "command.create_segment")
	defer span.End()

	// Record operation metrics
	uc.recordOnboardingMetrics(ctx, "segment", "create",
		attribute.String("segment_name", cpi.Name),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()))

	logger.Infof("Trying to create segment: %v", cpi)

	var status mmodel.Status
	if cpi.Status.IsEmpty() || pkg.IsNilOrEmpty(&cpi.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cpi.Status
	}

	status.Description = cpi.Status.Description

	segment := &mmodel.Segment{
		ID:             pkg.GenerateUUIDv7().String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		Name:           cpi.Name,
		Status:         status,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	_, err := uc.SegmentRepo.FindByName(ctx, organizationID, ledgerID, cpi.Name)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to find segment by name", err)

		logger.Errorf("Error finding segment by name: %v", err)

		// Record error
		uc.recordOnboardingError(ctx, "segment", "find_error",
			attribute.String("segment_name", cpi.Name),
			attribute.String("error_detail", err.Error()))

		return nil, err
	}

	prod, err := uc.SegmentRepo.Create(ctx, segment)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create segment", err)

		logger.Errorf("Error creating segment: %v", err)

		// Record error
		uc.recordOnboardingError(ctx, "segment", "creation_error",
			attribute.String("segment_name", cpi.Name),
			attribute.String("error_detail", err.Error()))

		return nil, err
	}

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Segment{}).Name(), prod.ID, cpi.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create segment metadata", err)

		logger.Errorf("Error creating segment metadata: %v", err)

		// Record error
		uc.recordOnboardingError(ctx, "segment", "metadata_error",
			attribute.String("segment_id", prod.ID),
			attribute.String("error_detail", err.Error()))

		return nil, err
	}

	prod.Metadata = metadata

	// Record successful completion and duration
	uc.recordOnboardingDuration(ctx, startTime, "segment", "create", "success",
		attribute.String("segment_id", prod.ID),
		attribute.String("segment_name", prod.Name))

	return prod, nil
}
