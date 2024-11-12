package command

import (
	"context"
	"github.com/LerianStudio/midaz/common/constant"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/common"
	gold "github.com/LerianStudio/midaz/common/gold/transaction/model"
	"github.com/LerianStudio/midaz/common/mgrpc/account"
	"github.com/LerianStudio/midaz/common/mlog"
	m "github.com/LerianStudio/midaz/components/transaction/internal/domain/metadata"
	o "github.com/LerianStudio/midaz/components/transaction/internal/domain/operation"
)

// CreateOperation creates a new operation based on transaction id and persisting data in the repository.
func (uc *UseCase) CreateOperation(ctx context.Context, accounts []*account.Account, transactionID string, dsl *gold.Transaction, validate gold.Responses, result chan []*o.Operation, err chan error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_operation")
	defer span.End()

	logger.Infof("Trying to create new oeprations")

	var operations []*o.Operation

	var fromTo []gold.FromTo
	fromTo = append(fromTo, dsl.Send.Source.From...)
	fromTo = append(fromTo, dsl.Distribute.To...)

	for _, acc := range accounts {
		for i := range fromTo {
			if fromTo[i].Account == acc.Id || fromTo[i].Account == acc.Alias {
				logger.Infof("Creating operation for account id: %s", acc.Id)

				balance := o.Balance{
					Available: &acc.Balance.Available,
					OnHold:    &acc.Balance.OnHold,
					Scale:     &acc.Balance.Scale,
				}

				amt, bat, er := gold.ValidateFromToOperation(fromTo[i], validate, acc)
				if er != nil {
					mopentelemetry.HandleSpanError(&span, "Failed to validate operation", er)

					err <- er
				}

				v := float64(amt.Value)
				s := float64(amt.Scale)

				amount := o.Amount{
					Amount: &v,
					Scale:  &s,
				}

				ba := float64(bat.Available)
				boh := float64(bat.OnHold)
				bs := float64(bat.Scale)

				balanceAfter := o.Balance{
					Available: &ba,
					OnHold:    &boh,
					Scale:     &bs,
				}

				description := fromTo[i].Description
				if common.IsNilOrEmpty(&fromTo[i].Description) {
					description = dsl.Description
				}

				var typeOperation string
				if fromTo[i].IsFrom {
					typeOperation = constant.DEBIT
				} else {
					typeOperation = constant.CREDIT
				}

				save := &o.Operation{
					ID:              common.GenerateUUIDv7().String(),
					TransactionID:   transactionID,
					Description:     description,
					Type:            typeOperation,
					AssetCode:       dsl.Send.Asset,
					ChartOfAccounts: fromTo[i].ChartOfAccounts,
					Amount:          amount,
					Balance:         balance,
					BalanceAfter:    balanceAfter,
					AccountID:       acc.Id,
					AccountAlias:    acc.Alias,
					PortfolioID:     &acc.PortfolioId,
					OrganizationID:  acc.OrganizationId,
					LedgerID:        acc.LedgerId,
					CreatedAt:       time.Now(),
					UpdatedAt:       time.Now(),
				}

				operation, er := uc.OperationRepo.Create(ctx, save)
				if er != nil {
					mopentelemetry.HandleSpanError(&span, "Failed to create operation", er)

					logger.Errorf("Error creating operation: %v", er)

					err <- er
				}

				er = uc.createMetadata(ctx, logger, fromTo[i].Metadata, operation)
				if er != nil {
					mopentelemetry.HandleSpanError(&span, "Failed to create metadata on operation", er)

					err <- er
				}

				operations = append(operations, operation)

				break
			}
		}
	}

	result <- operations
}

// createMetadata func that create metadata into operations
func (uc *UseCase) createMetadata(ctx context.Context, logger mlog.Logger, metadata map[string]any, operation *o.Operation) error {
	if metadata != nil {
		if err := common.CheckMetadataKeyAndValueLength(100, metadata); err != nil {
			return err
		}

		meta := m.Metadata{
			EntityID:   operation.ID,
			EntityName: reflect.TypeOf(o.Operation{}).Name(),
			Data:       metadata,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(o.Operation{}).Name(), &meta); err != nil {
			logger.Errorf("Error into creating operation metadata: %v", err)

			return err
		}

		operation.Metadata = metadata
	}

	return nil
}
