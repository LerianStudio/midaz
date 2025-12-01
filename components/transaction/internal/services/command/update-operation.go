package command

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

// UpdateOperation updates an operation's description and metadata.
//
// Operations represent individual debit/credit entries within a transaction.
// This function allows modification of operation metadata after creation.
// Only non-financial properties can be updated - amounts, accounts, and
// balance changes are immutable for audit trail integrity.
//
// Update Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for observability
//
//	Step 2: Build Update Model
//	  - Map input description to Operation model
//	  - Only description field is updatable
//
//	Step 3: Repository Update
//	  - Call OperationRepo.Update with scoped IDs
//	  - Requires organizationID, ledgerID, transactionID, and operationID
//	  - Handle not-found scenarios with business error
//
//	Step 4: Metadata Update
//	  - Update associated metadata in MongoDB
//	  - Merge new metadata with existing data
//
// Immutable Fields:
//
// The following operation fields cannot be updated (audit trail requirement):
//   - Amount (value, scale)
//   - Balance before/after states
//   - Account and balance references
//   - Operation type (DEBIT/CREDIT)
//   - Asset code
//   - Timestamps
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - organizationID: Organization scope for multi-tenant isolation
//   - ledgerID: Ledger scope within the organization
//   - transactionID: Parent transaction containing this operation
//   - operationID: UUID of the operation to update
//   - uoi: Update payload with Description and optional Metadata
//
// Returns:
//   - *operation.Operation: Updated operation with refreshed metadata
//   - error: Business or infrastructure error
//
// Error Scenarios:
//   - ErrOperationIDNotFound: Operation with given ID does not exist
//   - Database errors: PostgreSQL or MongoDB unavailable
func (uc *UseCase) UpdateOperation(ctx context.Context, organizationID, ledgerID, transactionID, operationID uuid.UUID, uoi *operation.UpdateOperationInput) (*operation.Operation, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_operation")
	defer span.End()

	logger.Infof("Trying to update operation: %v", uoi)

	op := &operation.Operation{
		Description: uoi.Description,
	}

	operationUpdated, err := uc.OperationRepo.Update(ctx, organizationID, ledgerID, transactionID, operationID, op)
	if err != nil {
		logger.Errorf("Error updating op on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrOperationIDNotFound, reflect.TypeOf(operation.Operation{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update operation on repo by id", err)

			logger.Warnf("Error updating op on repo by id: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update operation on repo by id", err)

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(operation.Operation{}).Name(), operationID.String(), uoi.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update metadata on repo by id", err)

		return nil, err
	}

	operationUpdated.Metadata = metadataUpdated

	return operationUpdated, nil
}
