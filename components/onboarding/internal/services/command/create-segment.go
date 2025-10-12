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

// CreateSegment creates a new segment and persists it in the repository.
//
// Segments represent logical divisions within a ledger, such as business areas, product lines,
// geographic regions, customer categories, or cost centers. They enable dimensional analysis
// and reporting by allowing accounts to be categorized and filtered along segment boundaries.
//
// The function performs the following steps:
// 1. Validates and normalizes the segment status (defaults to ACTIVE)
// 2. Generates a UUIDv7 for time-ordered identification
// 3. Checks for name uniqueness within the ledger
// 4. Persists the segment to PostgreSQL
// 5. Stores custom metadata in MongoDB if provided
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: The UUID of the organization owning this segment
//   - ledgerID: The UUID of the ledger containing this segment
//   - cpi: The segment creation input containing all required fields
//
// Returns:
//   - *mmodel.Segment: The created segment with generated ID and metadata
//   - error: Business validation or persistence errors (e.g., duplicate name)
func (uc *UseCase) CreateSegment(ctx context.Context, organizationID, ledgerID uuid.UUID, cpi *mmodel.CreateSegmentInput) (*mmodel.Segment, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_segment")
	defer span.End()

	logger.Infof("Trying to create segment: %v", cpi)

	// Step 1: Determine segment status, defaulting to ACTIVE if not specified
	var status mmodel.Status
	if cpi.Status.IsEmpty() || libCommons.IsNilOrEmpty(&cpi.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cpi.Status
	}

	status.Description = cpi.Status.Description

	// Step 2: Construct segment entity with generated UUIDv7 for time-ordered IDs
	segment := &mmodel.Segment{
		ID:             libCommons.GenerateUUIDv7().String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		Name:           cpi.Name,
		Status:         status,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Step 3: Verify segment name uniqueness within the ledger.
	// Duplicate segment names can cause confusion in dimensional analysis and reporting.
	_, err := uc.SegmentRepo.FindByName(ctx, organizationID, ledgerID, cpi.Name)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find segment by name", err)

		logger.Errorf("Error finding segment by name: %v", err)

		return nil, err
	}

	// Step 4: Persist the segment to PostgreSQL
	prod, err := uc.SegmentRepo.Create(ctx, segment)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create segment", err)

		logger.Errorf("Error creating segment: %v", err)

		return nil, err
	}

	// Step 5: Store custom metadata in MongoDB if provided.
	// Metadata enables flexible extension (e.g., region codes, cost center IDs, manager info)
	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Segment{}).Name(), prod.ID, cpi.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create segment metadata", err)

		logger.Errorf("Error creating segment metadata: %v", err)

		return nil, err
	}

	prod.Metadata = metadata

	return prod, nil
}
