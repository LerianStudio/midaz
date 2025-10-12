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

// CreateOperationRoute creates a new operation route and persists it in the repository.
//
// Operation routes define reusable validation rules for transaction operations (sources/destinations).
// They enable enforcement of accounting policies by specifying which accounts can participate
// in specific types of operations.
//
// Use Cases:
// - Restrict cash-in operations to specific deposit accounts (by alias)
// - Limit expense posting to accounts of type "expense" (by account type)
// - Define standard entry patterns for common transaction flows
//
// Account Validation Rules:
// - ruleType="alias": Match exact account alias (e.g., "@cash_account")
// - ruleType="account_type": Match account types (e.g., ["expense", "liability"])
//
// OperationType values:
// - "source": Defines valid source (from) accounts for debits
// - "destination": Defines valid destination (to) accounts for credits
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: Organization UUID owning this route
//   - ledgerID: Ledger UUID containing this route
//   - payload: Operation route configuration including validation rules
//
// Returns:
//   - *mmodel.OperationRoute: The created operation route with generated ID
//   - error: Persistence or metadata validation errors
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
		if err := libCommons.CheckMetadataKeyAndValueLength(100, payload.Metadata); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to check metadata key and value length", err)

			return nil, err
		}

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
