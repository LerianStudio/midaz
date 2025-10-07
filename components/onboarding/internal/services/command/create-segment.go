// Package command implements write operations (commands) for the onboarding service.
// This file contains command implementation.

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

// CreateSegment creates a new segment and persists it to the repository.
//
// This method implements the create segment use case, which:
// 1. Sets default status to ACTIVE if not provided
// 2. Checks if a segment with the same name already exists (returns error if found)
// 3. Generates a UUIDv7 for the segment ID
// 4. Creates the segment in PostgreSQL
// 5. Creates associated metadata in MongoDB
// 6. Returns the complete segment with metadata
//
// Business Rules:
//   - Segment names must be unique within a ledger
//   - Status defaults to ACTIVE if not provided or empty
//   - Name is required (validated at HTTP layer)
//   - Organization and ledger must exist (validated by foreign key constraints)
//
// Segments are used to:
//   - Create logical divisions within a ledger (e.g., by product line, region, department)
//   - Categorize accounts for reporting and analysis
//   - Support multi-dimensional account organization
//
// Data Storage:
//   - Primary data: PostgreSQL (segments table)
//   - Metadata: MongoDB (flexible key-value storage)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization that owns this segment
//   - ledgerID: UUID of the ledger that contains this segment
//   - cpi: Create segment input with name, status, and metadata
//
// Returns:
//   - *mmodel.Segment: Created segment with metadata
//   - error: Business error if validation fails, database error if persistence fails
//
// Possible Errors:
//   - ErrDuplicateSegmentName: Segment with same name already exists in ledger
//   - ErrLedgerIDNotFound: Ledger doesn't exist
//   - ErrOrganizationIDNotFound: Organization doesn't exist
//   - Database errors: Connection failures, constraint violations
//
// Example:
//
//	input := &mmodel.CreateSegmentInput{
//	    Name:     "North America Region",
//	    Status:   mmodel.Status{Code: "ACTIVE"},
//	    Metadata: map[string]any{"region_code": "NA"},
//	}
//	segment, err := useCase.CreateSegment(ctx, orgID, ledgerID, input)
//
// OpenTelemetry:
//   - Creates span "command.create_segment"
//   - Records errors as span events
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
