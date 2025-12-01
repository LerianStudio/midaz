package query

import (
	"context"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/google/uuid"
)

// GetOperationByID retrieves a specific operation within a transaction.
//
// Operations are the atomic units of a transaction, representing individual
// debit or credit movements. This method retrieves a single operation by its
// ID within the scope of a specific transaction.
//
// Query Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span "query.get_operation_by_id"
//
//	Step 2: Operation Retrieval
//	  - Query OperationRepo.Find with organization, ledger, transaction, and operation IDs
//	  - If retrieval fails: Return error with span event
//	  - Note: Returns nil operation (not error) if not found
//
//	Step 3: Metadata Enrichment
//	  - If operation found: Query MongoDB for associated metadata
//	  - If metadata retrieval fails: Return error with span event
//	  - If metadata exists: Attach to operation entity
//
//	Step 4: Response
//	  - Return enriched operation with metadata (or nil if not found)
//
// Operation Structure:
//
// Each operation represents a single ledger movement:
//   - AccountID: Account being debited or credited
//   - Type: "debit" or "credit"
//   - Amount: Value of the movement
//   - AssetCode: Currency/asset of the movement
//   - BalanceBefore/BalanceAfter: Account balance snapshots
//
// Parameters:
//   - ctx: Request context with tracing and tenant information
//   - organizationID: UUID of the owning organization (tenant scope)
//   - ledgerID: UUID of the ledger containing the operation
//   - transactionID: UUID of the parent transaction
//   - operationID: UUID of the operation to retrieve
//
// Returns:
//   - *operation.Operation: Operation with metadata if found
//   - error: Infrastructure error (nil operation is valid for not found)
//
// Error Scenarios:
//   - Database connection failure
//   - MongoDB metadata retrieval failure
func (uc *UseCase) GetOperationByID(ctx context.Context, organizationID, ledgerID, transactionID, operationID uuid.UUID) (*operation.Operation, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_operation_by_id")
	defer span.End()

	logger.Infof("Trying to get operation")

	o, err := uc.OperationRepo.Find(ctx, organizationID, ledgerID, transactionID, operationID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get operation on repo by id", err)

		logger.Errorf("Error getting operation: %v", err)

		return nil, err
	}

	if o != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(operation.Operation{}).Name(), operationID.String())
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb operation", err)

			logger.Errorf("Error get metadata on mongodb operation: %v", err)

			return nil, err
		}

		if metadata != nil {
			o.Metadata = metadata.Data
		}
	}

	return o, nil
}
