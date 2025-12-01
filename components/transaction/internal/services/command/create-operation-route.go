package command

import (
	"context"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// CreateOperationRoute creates a new operation route for transaction routing.
//
// Operation routes define how money moves in a single direction within a transaction.
// Each route specifies either a "source" (debit from account) or "destination"
// (credit to account) operation, along with rules for account selection.
//
// Operation Route Types:
//
//	Source (Debit):
//	  - Withdraws funds from an account
//	  - Decreases account balance
//	  - Must specify account selection rule
//
//	Destination (Credit):
//	  - Deposits funds to an account
//	  - Increases account balance
//	  - Must specify account selection rule
//
// Account Rules:
//
// The Account field contains rules for dynamic account selection:
//   - Static: Specific account alias (e.g., "@treasury")
//   - Dynamic: Query-based selection (e.g., "{{.source}}")
//   - Formula: Calculated distribution (e.g., "remaining")
//
// Creation Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for observability
//	  - Generate UUIDv7 for the new operation route
//
//	Step 2: Build Operation Route Model
//	  - Map input fields to OperationRoute model
//	  - Set timestamps (CreatedAt, UpdatedAt)
//
//	Step 3: Persist Operation Route
//	  - Store in PostgreSQL via repository
//	  - Handle unique constraint violations
//
//	Step 4: Create Metadata (Optional)
//	  - If metadata provided, store in MongoDB
//	  - Link metadata to operation route by entity ID
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - organizationID: Organization scope for multi-tenant isolation
//   - ledgerID: Ledger scope within the organization
//   - payload: Creation input with Title, Description, Code, OperationType, Account, and optional Metadata
//
// Returns:
//   - *mmodel.OperationRoute: Created operation route with generated ID
//   - error: Business or infrastructure error
//
// Error Scenarios:
//   - Database errors: PostgreSQL or MongoDB unavailable
//   - Unique constraint violation: Duplicate code within ledger
func (uc *UseCase) CreateOperationRoute(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *mmodel.CreateOperationRouteInput) (*mmodel.OperationRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_operation_route")
	defer span.End()

	now := time.Now()

	operationRoute := &mmodel.OperationRoute{
		ID:             libCommons.GenerateUUIDv7(),
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          payload.Title,
		Description:    payload.Description,
		Code:           payload.Code,
		OperationType:  payload.OperationType,
		Account:        payload.Account,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	createdOperationRoute, err := uc.OperationRouteRepo.Create(ctx, organizationID, ledgerID, operationRoute)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create operation route", err)

		logger.Errorf("Failed to create operation route: %v", err)

		return nil, err
	}

	if payload.Metadata != nil {
		meta := mongodb.Metadata{
			EntityID:   createdOperationRoute.ID.String(),
			EntityName: reflect.TypeOf(mmodel.OperationRoute{}).Name(),
			Data:       payload.Metadata,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		if err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(mmodel.OperationRoute{}).Name(), &meta); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create operation route metadata", err)

			logger.Errorf("Failed to create operation route metadata: %v", err)

			return nil, err
		}

		createdOperationRoute.Metadata = payload.Metadata
	}

	return createdOperationRoute, nil
}
