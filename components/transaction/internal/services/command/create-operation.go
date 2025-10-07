// Package command implements write operations (commands) for the transaction service.
// This file contains command implementation.

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

// CreateOperation creates operations for a transaction based on DSL send/distribute specifications.
//
// This method implements the operation creation use case, which:
// 1. Extracts source (from) and destination (to) specifications from DSL
// 2. Matches specifications to balances by account alias or ID
// 3. Validates each operation (amount, balance availability)
// 4. Determines operation type (DEBIT for source, CREDIT for destination)
// 5. Calculates balance after operation
// 6. Creates operation records in PostgreSQL
// 7. Creates metadata for each operation
// 8. Returns operations via channel (async pattern)
//
// Operation Types:
//   - DEBIT: Source operations (money leaving account)
//   - CREDIT: Destination operations (money entering account)
//
// The method uses channels for async communication:
//   - result channel: Returns created operations
//   - err channel: Returns any errors encountered
//
// Business Rules:
//   - Each from/to specification creates one operation
//   - Operations must balance (total debits = total credits)
//   - Balance validation ensures sufficient funds
//   - Description defaults to transaction description if not specified
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - balances: Array of account balances involved in transaction
//   - transactionID: UUID of the parent transaction
//   - dsl: lib-commons Transaction with send/distribute specifications
//   - validate: Validation responses from lib-commons
//   - result: Channel to return created operations
//   - err: Channel to return errors
//
// OpenTelemetry: Creates span "command.create_operation"
func (uc *UseCase) CreateOperation(ctx context.Context, balances []*mmodel.Balance, transactionID string, dsl *libTransaction.Transaction, validate libTransaction.Responses, result chan []*operation.Operation, err chan error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_operation")
	defer span.End()

	logger.Infof("Trying to create new operations")

	var operations []*operation.Operation

	var fromTo []libTransaction.FromTo

	fromTo = append(fromTo, dsl.Send.Source.From...)
	fromTo = append(fromTo, dsl.Send.Distribute.To...)

	for _, blc := range balances {
		for i := range fromTo {
			if fromTo[i].AccountAlias == blc.ID || fromTo[i].AccountAlias == blc.Alias {
				logger.Infof("Creating operation for account id: %s", blc.ID)

				balance := operation.Balance{
					Available: &blc.Available,
					OnHold:    &blc.OnHold,
				}

				amt, bat, er := libTransaction.ValidateFromToOperation(fromTo[i], validate, blc.ConvertToLibBalance())
				if er != nil {
					libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate operation", er)

					err <- er
				}

				amount := operation.Amount{
					Value: &amt.Value,
				}

				balanceAfter := operation.Balance{
					Available: &bat.Available,
					OnHold:    &bat.OnHold,
				}

				description := fromTo[i].Description
				if libCommons.IsNilOrEmpty(&fromTo[i].Description) {
					description = dsl.Description
				}

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

// CreateMetadata creates metadata for an operation in MongoDB.
//
// This helper function validates and persists operation metadata. It:
// 1. Validates metadata key and value lengths (max 100 chars per key)
// 2. Creates metadata document in MongoDB
// 3. Attaches metadata to the operation object
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - logger: Logger instance for error reporting
//   - metadata: Key-value metadata map (nil if no metadata)
//   - o: Operation to attach metadata to
//
// Returns:
//   - error: nil on success, validation or creation error
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
