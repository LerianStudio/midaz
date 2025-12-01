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

// CreateSegment creates a new segment within a ledger.
//
// Segments are logical groupings of accounts within a ledger, used for
// organizational and reporting purposes. Examples include "retail", "corporate",
// "treasury", or any business-defined categorization.
//
// Creation Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span "command.create_segment"
//
//	Step 2: Status Resolution
//	  - If input status is empty or has no code: Default to "ACTIVE"
//	  - Otherwise: Use provided status code
//	  - Apply status description from input (optional)
//
//	Step 3: Segment Model Construction
//	  - Generate UUIDv7 for new segment ID (time-ordered)
//	  - Set organization and ledger references
//	  - Apply name and resolved status
//	  - Set CreatedAt and UpdatedAt timestamps to current time
//
//	Step 4: Uniqueness Validation
//	  - Check if segment with same name already exists in ledger
//	  - FindByName returns error if name is available (inverse logic)
//	  - If name exists: Error is returned by repository
//
//	Step 5: Segment Persistence
//	  - Create segment in PostgreSQL via SegmentRepo.Create
//	  - If creation fails: Return error with span event
//
//	Step 6: Metadata Creation
//	  - Create metadata document in MongoDB via CreateMetadata
//	  - If metadata creation fails: Return error (segment already created)
//
//	Step 7: Response Assembly
//	  - Attach metadata to segment entity
//	  - Return complete segment with generated ID
//
// Default Values:
//
//   - Status code: "ACTIVE" if not provided
//   - ID: Auto-generated UUIDv7
//   - Timestamps: Current time
//
// Parameters:
//   - ctx: Request context with tracing and tenant information
//   - organizationID: UUID of the owning organization (tenant scope)
//   - ledgerID: UUID of the ledger to contain the segment
//   - cpi: Creation input with name, optional status, and metadata
//
// Returns:
//   - *mmodel.Segment: Created segment with generated ID and metadata
//   - error: Business or infrastructure error
//
// Error Scenarios:
//   - Segment name already exists in ledger
//   - Database connection failure
//   - MongoDB metadata creation failure
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
