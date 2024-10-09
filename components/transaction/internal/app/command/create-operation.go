package command

import (
	"context"
	"reflect"
	"strconv"
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

				var amount o.Amount
				var balance o.Balance
				var balanceAfter o.Balance

				if ft.IsFrom {
					value, er := strconv.ParseFloat(validate.From[ft.Account].Value, 64)
					if er != nil {
						logger.Errorf("Error converting FROM value string to float64: %v", er)
						err <- er
					}

					scale, er := strconv.ParseFloat(validate.From[ft.Account].Scale, 64)
					if er != nil {
						logger.Errorf("Error converting FROM scale string to float64: %v", er)
						err <- er
					}

					amount = o.Amount{
						Amount: &value,
						Scale:  &scale,
					}

					balance = o.Balance{
						Available: &acc.Balance.Available,
						OnHold:    &acc.Balance.OnHold,
						Scale:     &acc.Balance.Scale,
					}

					ba := acc.Balance.Available - value
					if acc.Balance.Scale < scale {
						acc.Balance.Scale = scale
					}

					balanceAfter = o.Balance{
						Available: &ba,
						OnHold:    &acc.Balance.OnHold,
						Scale:     &acc.Balance.Scale,
					}

				} else {
					value, er := strconv.ParseFloat(validate.To[ft.Account].Value, 64)
					if er != nil {
						logger.Errorf("Error converting TO value string to float64: %v", er)
						err <- er
					}

					scale, er := strconv.ParseFloat(validate.To[ft.Account].Scale, 64)
					if er != nil {
						logger.Errorf("Error converting TO scale string to float64: %v", er)
						err <- er
					}

					amount = o.Amount{
						Amount: &value,
						Scale:  &scale,
					}

					balance = o.Balance{
						Available: &acc.Balance.Available,
						OnHold:    &acc.Balance.OnHold,
						Scale:     &acc.Balance.Scale,
					}

					baf := acc.Balance.Available + value
					if acc.Balance.Scale < scale {
						acc.Balance.Scale = scale
					}

					balanceAfter = o.Balance{
						Available: &baf,
						OnHold:    &acc.Balance.OnHold,
						Scale:     &acc.Balance.Scale,
					}
				}

				save := &o.Operation{
					ID:              uuid.New().String(),
					TransactionID:   transactionID,
					Description:     ft.Description,
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

					return
				}

				if ft.Metadata != nil {
					if er = common.CheckMetadataKeyAndValueLength(100, ft.Metadata); er != nil {
						err <- er

						return
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

						return
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
