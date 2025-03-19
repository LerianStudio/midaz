package command

import (
	"context"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libLog "github.com/LerianStudio/lib-commons/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libTransaction "github.com/LerianStudio/lib-commons/commons/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"reflect"
	"time"
)

// CreateOperation creates a new operation based on transaction id and persisting data in the repository.
func (uc *UseCase) CreateOperation(ctx context.Context, balances []*mmodel.Balance, transactionID string, dsl *libTransaction.Transaction, validate libTransaction.Responses, result chan []*operation.Operation, err chan error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_operation")
	defer span.End()

	logger.Infof("Trying to create new operations")

	var operations []*operation.Operation

	var fromTo []libTransaction.FromTo
	fromTo = append(fromTo, dsl.Send.Source.From...)
	fromTo = append(fromTo, dsl.Send.Distribute.To...)

	for _, blc := range balances {
		for i := range fromTo {
			if fromTo[i].Account == blc.ID || fromTo[i].Account == blc.Alias {
				logger.Infof("Creating operation for account id: %s", blc.ID)

				balance := operation.Balance{
					Available: &blc.Available,
					OnHold:    &blc.OnHold,
					Scale:     &blc.Scale,
				}

				amt, bat, er := libTransaction.ValidateFromToOperation(fromTo[i], validate, blc.ConvertToLibBalance())
				if er != nil {
					libOpentelemetry.HandleSpanError(&span, "Failed to validate operation", er)

					err <- er
				}

				amount := operation.Amount{
					Amount: &amt.Value,
					Scale:  &amt.Scale,
				}

				balanceAfter := operation.Balance{
					Available: &bat.Available,
					OnHold:    &bat.OnHold,
					Scale:     &bat.Scale,
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
					libOpentelemetry.HandleSpanError(&span, "Failed to create operation", er)

					logger.Errorf("Error creating operation: %v", er)

					err <- er
				}

				er = uc.CreateMetadata(ctx, logger, fromTo[i].Metadata, op)
				if er != nil {
					libOpentelemetry.HandleSpanError(&span, "Failed to create metadata on operation", er)

					err <- er
				}

				operations = append(operations, op)

				break
			}
		}
	}

	result <- operations
}

// CreateMetadata func that create metadata into operations
func (uc *UseCase) CreateMetadata(ctx context.Context, logger libLog.Logger, metadata map[string]any, o *operation.Operation) error {
	if metadata != nil {
		if err := libCommons.CheckMetadataKeyAndValueLength(100, metadata); err != nil {
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
