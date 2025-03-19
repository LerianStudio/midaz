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

// CreateSegment creates a new segment and persists data in the repository.
func (uc *UseCase) CreateSegment(ctx context.Context, organizationID, ledgerID uuid.UUID, cpi *mmodel.CreateSegmentInput) (*mmodel.Segment, error) {
	logger := pkg.NewLoggerFromContext(ctx)

	segmentID := pkg.GenerateUUIDv7().String()
	op := uc.Telemetry.NewSegmentOperation("create", segmentID)

	op.WithAttributes(
		attribute.String("segment_name", cpi.Name),
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
	)

	op.RecordSystemicMetric(ctx)
	ctx = op.StartTrace(ctx)

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
		ID:             segmentID,
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		Name:           cpi.Name,
		Status:         status,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	_, err := uc.SegmentRepo.FindByName(ctx, organizationID, ledgerID, cpi.Name)
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to find segment by name", err)
		logger.Errorf("Error finding segment by name: %v", err)
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "find_error", err)

		return nil, err
	}

	prod, err := uc.SegmentRepo.Create(ctx, segment)
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to create segment", err)
		logger.Errorf("Error creating segment: %v", err)
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "creation_error", err)

		return nil, err
	}

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Segment{}).Name(), prod.ID, cpi.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to create segment metadata", err)
		logger.Errorf("Error creating segment metadata: %v", err)
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "metadata_error", err)

		return nil, err
	}

	prod.Metadata = metadata

	op.End(ctx, "success")

	return prod, nil
}
