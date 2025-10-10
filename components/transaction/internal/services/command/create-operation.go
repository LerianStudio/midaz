// Package command implements write operations (commands) for the transaction service.
// This file contains the command for creating transaction operations.
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

// CreateOperation creates the operations for a transaction based on the parsed DSL.
//
// This function is responsible for generating the DEBIT and CREDIT operations that
// correspond to the 'from' and 'to' specifications in a transaction's DSL.
// It validates each operation, calculates the resulting balance, and persists
// the operation records and their metadata. The results are returned through channels.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - balances: A slice of the account balances involved in the transaction.
//   - transactionID: The UUID of the parent transaction.
//   - dsl: The parsed DSL containing the transaction's send and distribute specifications.
//   - validate: The validation responses from the transaction validation step.
//   - result: A channel to send the created operations through.
//   - err: A channel to send any errors that occur during processing.
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
// This helper function validates and persists metadata for a given operation.
// It checks the length of metadata keys and values before creating the metadata
// document in MongoDB and then attaches the metadata to the operation object.
//
// Parameters:
//   - ctx: The context for tracing and logging.
//   - logger: The logger instance for error reporting.
//   - metadata: A map of key-value pairs to be stored.
//   - o: The operation to which the metadata will be attached.
//
// Returns:
//   - error: An error if the validation or persistence fails.
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
