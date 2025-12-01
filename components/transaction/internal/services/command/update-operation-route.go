// Package command provides write operations (commands) for the transaction service.
//
// This package implements the command side of CQRS for financial transaction processing:
//   - Operation route management (create, update, delete)
//   - Transaction route management with operation route relationships
//   - Balance operations (create, update, sync, delete)
//   - Transaction processing (create, update, async execution)
//   - Idempotency handling for transaction safety
//   - Event publishing for transaction audit and notifications
//
// Command Operations:
//
// Operation Routes define how money moves in a transaction (source/destination):
//   - CreateOperationRoute: Define new debit/credit operation patterns
//   - UpdateOperationRoute: Modify existing operation route properties
//   - DeleteOperationRoute: Remove unused operation routes (with link validation)
//
// Transaction Routes combine operation routes into complete transaction templates:
//   - CreateTransactionRoute: Build transaction templates from operation routes
//   - UpdateTransactionRoute: Modify transaction templates and relationships
//   - DeleteTransactionRoute: Remove transaction templates (cascading relationship cleanup)
//
// Balance Management handles account balance lifecycle:
//   - CreateBalance: Initialize balances for new accounts
//   - CreateAdditionalBalance: Add secondary balances to accounts
//   - SyncBalance: Synchronize Redis cache with PostgreSQL
//   - DeleteAllBalancesByAccountID: Safe balance removal with validation
//
// Transaction Processing executes financial transactions:
//   - CreateBalanceTransactionOperationsAsync: Process transactions via queue
//   - TransactionExecute: Route transactions to sync/async processing
//   - UpdateTransaction: Modify transaction metadata
//
// Thread Safety:
//
// All command operations are designed to be called concurrently. Each operation:
//   - Uses its own database transaction where needed
//   - Propagates context for cancellation and tracing
//   - Logs operations with structured fields for debugging
//
// Related Packages:
//   - query: Read operations (CQRS query side)
//   - adapters/postgres: Database repositories
//   - adapters/mongodb: Metadata storage
//   - adapters/redis: Caching and idempotency
//   - adapters/rabbitmq: Event publishing
package command

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// UpdateOperationRoute updates an existing operation route with new properties.
//
// Operation routes define how money flows in a transaction (debit from source,
// credit to destination). This function allows modifying the route's title,
// description, code, and account rules without changing its fundamental type.
//
// Update Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for observability
//
//	Step 2: Build Update Model
//	  - Map input fields to OperationRoute model
//	  - Only non-nil fields will be updated
//
//	Step 3: Repository Update
//	  - Call OperationRouteRepo.Update with scoped IDs
//	  - Handle not-found scenarios with business error
//	  - Return updated operation route
//
//	Step 4: Metadata Update
//	  - Update associated metadata in MongoDB
//	  - Merge new metadata with existing data
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - organizationID: Organization scope for multi-tenant isolation
//   - ledgerID: Ledger scope within the organization
//   - id: UUID of the operation route to update
//   - input: Update payload with optional fields (Title, Description, Code, Account, Metadata)
//
// Returns:
//   - *mmodel.OperationRoute: Updated operation route with refreshed metadata
//   - error: Business or infrastructure error
//
// Error Scenarios:
//   - ErrOperationRouteNotFound: Operation route with given ID does not exist
//   - Database connection errors: PostgreSQL unavailable
//   - Metadata update errors: MongoDB unavailable
func (uc *UseCase) UpdateOperationRoute(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID, input *mmodel.UpdateOperationRouteInput) (*mmodel.OperationRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_operation_route")
	defer span.End()

	logger.Infof("Trying to update operation route: %v", input)

	operationRoute := &mmodel.OperationRoute{
		Title:       input.Title,
		Description: input.Description,
		Code:        input.Code,
		Account:     input.Account,
	}

	operationRouteUpdated, err := uc.OperationRouteRepo.Update(ctx, organizationID, ledgerID, id, operationRoute)
	if err != nil {
		logger.Errorf("Error updating operation route on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update operation route on repo by id", err)

			logger.Warnf("Error updating operation route on repo by id: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update operation route on repo by id", err)

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.OperationRoute{}).Name(), id.String(), input.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update metadata on repo by id", err)

		logger.Errorf("Error updating metadata on repo by id: %v", err)

		return nil, err
	}

	operationRouteUpdated.Metadata = metadataUpdated

	return operationRouteUpdated, nil
}
