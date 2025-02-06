package command

import (
	"context"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	goldModel "github.com/LerianStudio/midaz/pkg/gold/transaction/model"
	"github.com/LerianStudio/midaz/pkg/mlog"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
)

// CreateOperation creates a new operation based on transaction id and persisting data in the repository.
func (uc *UseCase) CreateOperation(ctx context.Context, balances []*mmodel.Balance, transactionID string, dsl *goldModel.Transaction, validate goldModel.Responses, result chan []*operation.Operation, err chan error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_operation")
	defer span.End()

	logger.Infof("Trying to create new oeprations")

	var operations []*operation.Operation

	var fromTo []goldModel.FromTo
	fromTo = append(fromTo, dsl.Send.Source.From...)
	fromTo = append(fromTo, dsl.Send.Distribute.To...)

	for _, blc := range balances {
		for i := range fromTo {
			if fromTo[i].Account == blc.ID || fromTo[i].Account == blc.Alias {
				logger.Infof("Creating operation for account id: %s", blc.ID)

				a := float64(blc.Available)
				oh := float64(blc.OnHold)
				s := float64(blc.Scale)

				balance := operation.Balance{
					Available: &a,
					OnHold:    &oh,
					Scale:     &s,
				}

				amt, bat, er := goldModel.ValidateFromToOperation(fromTo[i], validate, blc)
				if er != nil {
					mopentelemetry.HandleSpanError(&span, "Failed to validate operation", er)

					err <- er
				}

				amtv := float64(amt.Value)
				amts := float64(amt.Scale)

				amount := operation.Amount{
					Amount: &amtv,
					Scale:  &amts,
				}

				bata := float64(bat.Available)
				batoh := float64(bat.OnHold)
				bats := float64(bat.Scale)

				balanceAfter := operation.Balance{
					Available: &bata,
					OnHold:    &batoh,
					Scale:     &bats,
				}

				description := fromTo[i].Description
				if pkg.IsNilOrEmpty(&fromTo[i].Description) {
					description = dsl.Description
				}

				var typeOperation string
				if fromTo[i].IsFrom {
					typeOperation = constant.DEBIT
				} else {
					typeOperation = constant.CREDIT
				}

				save := &operation.Operation{
					ID:              pkg.GenerateUUIDv7().String(),
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
					mopentelemetry.HandleSpanError(&span, "Failed to create operation", er)

					logger.Errorf("Error creating operation: %v", er)

					err <- er
				}

				er = uc.createMetadata(ctx, logger, fromTo[i].Metadata, op)
				if er != nil {
					mopentelemetry.HandleSpanError(&span, "Failed to create metadata on operation", er)

					err <- er
				}

				operations = append(operations, op)

				break
			}
		}
	}

	result <- operations
}

// createMetadata func that create metadata into operations
func (uc *UseCase) createMetadata(ctx context.Context, logger mlog.Logger, metadata map[string]any, o *operation.Operation) error {
	if metadata != nil {
		if err := pkg.CheckMetadataKeyAndValueLength(100, metadata); err != nil {
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
