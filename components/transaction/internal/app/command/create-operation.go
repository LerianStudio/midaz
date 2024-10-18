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
		for _, ft := range fromTo {
			if ft.Account == acc.Id || ft.Account == acc.Alias {
				logger.Infof("Creating operation for account id: %s", acc.Id)

				balance := o.Balance{
					Available: &acc.Balance.Available,
					OnHold:    &acc.Balance.OnHold,
					Scale:     &acc.Balance.Scale,
				}

				amount, balanceAfter, er := v.ValidateFromToOperation(ft, validate, acc)
				if er != nil {
					err <- er
				}

				description := ft.Description
				if common.IsNilOrEmpty(&ft.Description) {
					description = dsl.Description
				}

				save := &o.Operation{
					ID:              uuid.New().String(),
					TransactionID:   transactionID,
					Description:     description,
					Type:            acc.Type,
					AssetCode:       dsl.Send.Asset,
					ChartOfAccounts: ft.ChartOfAccounts,
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

				if ft.Metadata != nil {
					if er = common.CheckMetadataKeyAndValueLength(100, ft.Metadata); er != nil {
						err <- er
					}

					meta := m.Metadata{
						EntityID:   operation.ID,
						EntityName: reflect.TypeOf(o.Operation{}).Name(),
						Data:       ft.Metadata,
						CreatedAt:  time.Now(),
						UpdatedAt:  time.Now(),
					}

					if er = uc.MetadataRepo.Create(ctx, reflect.TypeOf(o.Operation{}).Name(), &meta); er != nil {
						logger.Errorf("Error into creating operation metadata: %v", er)

						err <- er
					}

					operation.Metadata = ft.Metadata
				}

				operations = append(operations, operation)

				break
			}
		}
	}

	result <- operations
}
