// Package command implements write operations (commands) for the onboarding service.
// This file contains the command for creating a new segment.
package command

import (
	"context"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// CreateSegment creates a new segment in the repository.
//
// This use case is responsible for:
// 1. Ensuring the segment name is unique within the ledger.
// 2. Setting a default status of "ACTIVE" if none is provided.
// 3. Persisting the segment in the PostgreSQL database.
// 4. Storing any associated metadata in MongoDB.
// 5. Returning the newly created segment, including its metadata.
//
// Segments are used for logical divisions within a ledger, such as by product line or region.
//
// Business Rules:
//   - Segment names must be unique per ledger.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization that owns the segment.
//   - ledgerID: The UUID of the ledger where the segment will be created.
//   - cpi: The input data for creating the segment.
//
// Returns:
//   - *mmodel.Segment: The created segment, complete with its metadata.
//   - error: An error if the creation fails due to a business rule violation or database error.
func (uc *UseCase) CreateSegment(ctx context.Context, organizationID, ledgerID uuid.UUID, cpi *mmodel.CreateSegmentInput) (*mmodel.Segment, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_segment")
	defer span.End()

	logger.Infof("Trying to create segment: %v", cpi)

	var status mmodel.Status
	if cpi.Status.IsEmpty() || libCommons.IsNilOrEmpty(&cpi.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cpi.Status
	}

	status.Description = cpi.Status.Description

	segment := &mmodel.Segment{
		ID:             libCommons.GenerateUUIDv7().String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		Name:           cpi.Name,
		Status:         status,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// FIXME: This logic is incorrect. FindByName returns an error if the segment is *not* found.
	// The code should check if the error is `services.ErrDatabaseItemNotFound` and proceed in that case.
	// If the error is nil, it means a segment with the same name already exists, and an
	// `ErrDuplicateSegmentName` error should be returned. Any other error should be returned directly.
	_, err := uc.SegmentRepo.FindByName(ctx, organizationID, ledgerID, cpi.Name)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find segment by name", err)

		logger.Errorf("Error finding segment by name: %v", err)

		return nil, err
	}

	prod, err := uc.SegmentRepo.Create(ctx, segment)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create segment", err)

		logger.Errorf("Error creating segment: %v", err)

		return nil, err
	}

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Segment{}).Name(), prod.ID, cpi.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create segment metadata", err)

		logger.Errorf("Error creating segment metadata: %v", err)

		return nil, err
	}

	prod.Metadata = metadata

	return prod, nil
}
