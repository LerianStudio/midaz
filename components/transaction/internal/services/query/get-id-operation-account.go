package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/google/uuid"
)

// GetOperationByAccount retrieves a specific operation for an account.
//
// This method queries an operation by its ID within the context of a specific
// account, ensuring the operation belongs to the specified account. This is
// useful for account-centric views where operations need to be filtered by
// account ownership.
//
// Query Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span "query.get_operation_by_account"
//
//	Step 2: Operation Retrieval
//	  - Query OperationRepo.FindByAccount with organization, ledger, account, and operation IDs
//	  - If operation not found: Return ErrNoOperationsFound business error
//	  - If other error: Return wrapped error with span event
//
//	Step 3: Metadata Enrichment
//	  - If operation found: Query MongoDB for associated metadata
//	  - If metadata retrieval fails: Return error
//	  - If metadata exists: Attach to operation entity
//
//	Step 4: Response
//	  - Return enriched operation with metadata
//
// Account Scoping:
//
// Unlike GetOperationByID which queries by transaction scope, this method
// queries by account scope. This ensures the operation is relevant to the
// specified account (either as source or destination).
//
// Parameters:
//   - ctx: Request context with tracing and tenant information
//   - organizationID: UUID of the owning organization (tenant scope)
//   - ledgerID: UUID of the ledger containing the operation
//   - accountID: UUID of the account to scope the operation query
//   - operationID: UUID of the operation to retrieve
//
// Returns:
//   - *operation.Operation: Operation with metadata if found
//   - error: Business or infrastructure error
//
// Error Scenarios:
//   - ErrNoOperationsFound: Operation not found for account
//   - Database connection failure
//   - MongoDB metadata retrieval failure
func (uc *UseCase) GetOperationByAccount(ctx context.Context, organizationID, ledgerID, accountID, operationID uuid.UUID) (*operation.Operation, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_operation_by_account")
	defer span.End()

	logger.Infof("Retrieving operation by account")

	op, err := uc.OperationRepo.FindByAccount(ctx, organizationID, ledgerID, accountID, operationID)
	if err != nil {
		logger.Errorf("Error getting operation on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoOperationsFound, reflect.TypeOf(operation.Operation{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operation on repo by account", err)

			logger.Warnf("Error getting operation on repo: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operation on repo by account", err)

		return nil, err
	}

	if op != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(operation.Operation{}).Name(), operationID.String())
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb operation", err)

			logger.Errorf("Error get metadata on mongodb operation: %v", err)

			return nil, err
		}

		if metadata != nil {
			op.Metadata = metadata.Data
		}
	}

	return op, nil
}
