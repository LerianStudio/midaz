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
	m "github.com/LerianStudio/midaz/components/transaction/internal/domain/metadata"
	o "github.com/LerianStudio/midaz/components/transaction/internal/domain/operation"
	t "github.com/LerianStudio/midaz/components/transaction/internal/domain/transaction"
	"github.com/google/uuid"
)

// CreateOperation creates a new operation based on transaction id and persisting data in the repository.
func (uc *UseCase) CreateOperation(ctx context.Context, accounts []*account.Account, transaction *t.Transaction, dsl *gold.Transaction, result chan []*o.Operation, err chan error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to create new oeprations")

	var operations []*o.Operation

	var fromTo []gold.FromTo
	fromTo = append(fromTo, dsl.Send.Source.From...)
	fromTo = append(fromTo, dsl.Distribute.To...)

	for _, acc := range accounts {
		for _, ft := range fromTo {
			if ft.Account == acc.Id || ft.Account == acc.Alias {
				logger.Infof("Creating operation for acc %s", acc.Id)

				amount, err, done := validateAndChangeBalance(ft, logger, err, dsl)
				if done {
					return
				}

				balance := o.Balance{
					Available: &acc.Balance.Available,
					OnHold:    &acc.Balance.OnHold,
					Scale:     &acc.Balance.Scale,
				}

				balanceAfter := o.Balance{
					Available: &acc.Balance.Available,
					OnHold:    &acc.Balance.OnHold,
					Scale:     &acc.Balance.Scale,
				}

				save := &o.Operation{
					ID:              uuid.New().String(),
					TransactionID:   transaction.ID,
					Description:     transaction.Description,
					Type:            acc.Type,
					InstrumentCode:  dsl.Send.Asset,
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

func validateAndChangeBalance(ft gold.FromTo, logger mlog.Logger, err chan error, dsl *gold.Transaction) (o.Amount, chan error, bool) {
	var amount o.Amount

	value := ft.Amount.Value

	if !common.IsNilOrEmpty(&value) {
		a, er := strconv.ParseFloat(ft.Amount.Value, 64)
		if er != nil {
			logger.Errorf("Error converting amount string to float64: %v", er)
			err <- er

			return o.Amount{}, nil, true
		}

		s, er := strconv.ParseFloat(ft.Amount.Scale, 64)
		if er != nil {
			logger.Errorf("Error converting scale string to float64: %v", er)
			err <- er

			return o.Amount{}, nil, true
		}

		amount = o.Amount{
			Amount: &a,
			Scale:  &s,
		}
	} else {
		a, er := strconv.ParseFloat(dsl.Send.Value, 64)
		if er != nil {
			logger.Errorf("Error converting amount string to float64: %v", er)
			err <- er

			return o.Amount{}, nil, true
		}

		s, er := strconv.ParseFloat(dsl.Send.Scale, 64)
		if er != nil {
			logger.Errorf("Error converting scale string to float64: %v", er)
			err <- er

			return o.Amount{}, nil, true
		}

		amount = o.Amount{
			Amount: &a,
			Scale:  &s,
		}
	}

	return amount, err, false
}
