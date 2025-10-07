// Package command implements write operations (commands) for the transaction service.
// This file contains command implementation.

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

// CreateOperationRoute creates a new operation route and persists it to the repository.
//
// This method implements the create operation route use case, which:
// 1. Generates UUIDv7 for the operation route ID
// 2. Creates the operation route in PostgreSQL
// 3. Creates associated metadata in MongoDB
// 4. Returns the complete operation route with metadata
//
// Business Rules:
//   - Operation type must be either "source" or "destination"
//   - Title and description are required
//   - Code is optional (for programmatic reference)
//   - Account rules define which accounts match this route
//
// Operation Routes:
//   - Define account selection rules for transaction routing
//   - Specify operation type (source or destination)
//   - Support account matching by alias or account_type
//   - Enable automated account selection in transactions
//
// Account Rules:
//   - rule_type: "alias" or "account_type"
//   - valid_if: Matching criteria (exact alias or account type value)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - payload: Operation route input with title, description, type, account rules
//
// Returns:
//   - *mmodel.OperationRoute: Created operation route with metadata
//   - error: Business error if creation fails
//
// OpenTelemetry: Creates span "command.create_operation_route"
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
