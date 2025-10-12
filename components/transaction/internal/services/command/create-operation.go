package command

import (
	"context"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// CreateOperation creates operation records for a transaction's double-entry accounting.
//
// Operations represent the individual debits and credits that make up a transaction,
// implementing double-entry bookkeeping. Each transaction produces 2+ operations:
// - Source operations (DEBIT): Money leaving accounts
// - Destination operations (CREDIT): Money entering accounts
//
// This function correlates the parsed Gold DSL with actual account balances and creates
// operation records that capture before/after balance states for complete audit trails.
//
// Double-Entry Accounting:
// - Every transaction must balance (total debits = total credits)
// - Each operation links to a balance and records the mutation
// - Balance snapshots (before/after) enable transaction reconstruction
//
// The function uses channels for concurrent/async result handling.
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - balances: Current balance states for all accounts in the transaction
//   - transactionID: The parent transaction UUID this operation belongs to
//   - dsl: Parsed Gold DSL transaction with source/destination details
//   - validate: Validated amounts per account alias
//   - result: Channel to send created operations back to caller
//   - err: Channel to send any errors back to caller
func (uc *UseCase) CreateOperation(ctx context.Context, balances []*mmodel.Balance, transactionID string, dsl *libTransaction.Transaction, validate libTransaction.Responses, result chan []*operation.Operation, err chan error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_operation")
	defer span.End()

	logger.Infof("Trying to create new operations")

	var operations []*operation.Operation

	// Step 1: Merge source (from) and destination (to) into unified list
	var fromTo []libTransaction.FromTo

	fromTo = append(fromTo, dsl.Send.Source.From...)
	fromTo = append(fromTo, dsl.Send.Distribute.To...)

	// Step 2: For each balance, find matching DSL entry and create operation record
	for _, blc := range balances {
		for i := range fromTo {
			// Match balance to DSL entry by ID or alias
			if fromTo[i].AccountAlias == blc.ID || fromTo[i].AccountAlias == blc.Alias {
				logger.Infof("Creating operation for account id: %s", blc.ID)

				// Capture balance state BEFORE the operation
				balance := operation.Balance{
					Available: &blc.Available,
					OnHold:    &blc.OnHold,
				}

				// Validate and calculate balance state AFTER the operation
				amt, bat, er := libTransaction.ValidateFromToOperation(fromTo[i], validate, blc.ConvertToLibBalance())
				if er != nil {
					libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate operation", er)

					err <- er
				}

				amount := operation.Amount{
					Value: &amt.Value,
				}

				// Capture balance state AFTER the operation for audit trail
				balanceAfter := operation.Balance{
					Available: &bat.Available,
					OnHold:    &bat.OnHold,
				}

				// Use operation-specific description if provided, otherwise inherit from transaction
				description := fromTo[i].Description
				if libCommons.IsNilOrEmpty(&fromTo[i].Description) {
					description = dsl.Description
				}

				// Determine operation type: source accounts are DEBIT, destination are CREDIT
				var typeOperation string
				if fromTo[i].IsFrom {
					typeOperation = constant.DEBIT
				} else {
					typeOperation = constant.CREDIT
				}

				save := &operation.Operation{
					ID:              libCommons.GenerateUUIDv7().String(),
					TransactionID:   transactionID,
					Description:     description,
					Type:            typeOperation,
					AssetCode:       dsl.Send.Asset,
					ChartOfAccounts: fromTo[i].ChartOfAccounts,
					Amount:          amount,
					Balance:         balance,
					BalanceAfter:    balanceAfter,
					BalanceID:       blc.ID,
					AccountID:       blc.AccountID,
					AccountAlias:    blc.Alias,
					OrganizationID:  blc.OrganizationID,
					LedgerID:        blc.LedgerID,
					CreatedAt:       time.Now(),
					UpdatedAt:       time.Now(),
				}

				op, er := uc.OperationRepo.Create(ctx, save)
				if er != nil {
					libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create operation", er)

					logger.Errorf("Error creating operation: %v", er)

					err <- er
				}

				er = uc.CreateMetadata(ctx, logger, fromTo[i].Metadata, op)
				if er != nil {
					libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create metadata on operation", er)

					logger.Errorf("Failed to create metadata on operation: %v", er)
					logger.Errorf("Returning error: %v", er)

					err <- er
				}

				operations = append(operations, op)

				break
			}
		}
	}

	result <- operations
}

// CreateMetadata stores custom metadata for an operation in MongoDB.
//
// Operations can have their own metadata separate from the parent transaction,
// allowing fine-grained attribution and tracking per debit/credit entry.
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - logger: Structured logger for operation tracking
//   - metadata: Custom key-value pairs to store (can be nil)
//   - o: The operation entity to attach metadata to
//
// Returns:
//   - error: Metadata validation or persistence errors
func (uc *UseCase) CreateMetadata(ctx context.Context, logger libLog.Logger, metadata map[string]any, o *operation.Operation) error {
	if metadata != nil {
		if err := libCommons.CheckMetadataKeyAndValueLength(100, metadata); err != nil {
			logger.Errorf("Error checking metadata key and value length: %v", err)

			return err
		}

		meta := mongodb.Metadata{
			EntityID:   o.ID,
			EntityName: reflect.TypeOf(operation.Operation{}).Name(),
			Data:       metadata,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(operation.Operation{}).Name(), &meta); err != nil {
			logger.Errorf("Error into creating operation metadata: %v", err)

			return err
		}

		o.Metadata = metadata
	}

	return nil
}
