package command

import (
	"context"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/common"
	gold "github.com/LerianStudio/midaz/common/gold/transaction/model"
	"github.com/LerianStudio/midaz/common/mgrpc/account"
	"github.com/LerianStudio/midaz/common/mlog"
	v "github.com/LerianStudio/midaz/components/transaction/internal/domain/account"
	m "github.com/LerianStudio/midaz/components/transaction/internal/domain/metadata"
	o "github.com/LerianStudio/midaz/components/transaction/internal/domain/operation"
	"github.com/google/uuid"
)

// CreateOperation creates a new operation based on transaction id and persisting data in the repository.
func (uc *UseCase) CreateOperation(ctx context.Context, accounts []*account.Account, transactionID string, dsl *gold.Transaction, validate v.Responses, result chan []*o.Operation, err chan error) {
	logger := mlog.NewLoggerFromContext(ctx)
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

				amount, balanceAfter, er := v.ValidateFromToOperation(fromTo[i], validate, acc)
				if er != nil {
					err <- er
				}

				description := fromTo[i].Description
				if common.IsNilOrEmpty(&fromTo[i].Description) {
					description = dsl.Description
				}

				save := &o.Operation{
					ID:              uuid.New().String(),
					TransactionID:   transactionID,
					Description:     description,
					Type:            acc.Type,
					AssetCode:       dsl.Send.Asset,
					ChartOfAccounts: fromTo[i].ChartOfAccounts,
					Amount:          amount,
					Balance:         balance,
					BalanceAfter:    balanceAfter,
					AccountID:       acc.Id,
					AccountAlias:    acc.Alias,
					PortfolioID:     acc.PortfolioId,
					OrganizationID:  acc.OrganizationId,
					LedgerID:        acc.LedgerId,
					CreatedAt:       time.Now(),
					UpdatedAt:       time.Now(),
				}

				operation, er := uc.OperationRepo.Create(ctx, save)
				if er != nil {
					logger.Errorf("Error creating operation: %v", er)

					err <- er
				}

				er = uc.createMetadata(ctx, logger, fromTo[i].Metadata, operation)
				if er != nil {
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
