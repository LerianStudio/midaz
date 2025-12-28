package command

import (
	"context"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"go.opentelemetry.io/otel/trace"
)

// CreateOperation creates a new operation based on transaction id and persisting data in the repository.
func (uc *UseCase) CreateOperation(ctx context.Context, balances []*mmodel.Balance, transactionID string, dsl *pkgTransaction.Transaction, validate pkgTransaction.Responses, result chan []*operation.Operation, err chan error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	// Validate channels before any operation - writing to nil channel panics
	assert.NotNil(result, "result channel must not be nil",
		"transaction_id", transactionID)
	assert.NotNil(err, "error channel must not be nil",
		"transaction_id", transactionID)

	ctx, span := tracer.Start(ctx, "command.create_operation")
	defer span.End()

	logger.Infof("Trying to create new operations")

	var operations []*operation.Operation

	var fromTo []pkgTransaction.FromTo

	fromTo = append(fromTo, dsl.Send.Source.From...)
	fromTo = append(fromTo, dsl.Send.Distribute.To...)

	for _, blc := range balances {
		for i := range fromTo {
			if !uc.isMatchingAccount(fromTo[i], blc) {
				continue
			}

			logger.Infof("Creating operation for account id: %s", blc.ID)

			op, er := uc.createOperationForBalance(ctx, logger, &span, blc, fromTo[i], transactionID, dsl, validate)
			if er != nil {
				err <- er
				break
			}

			operations = append(operations, op)

			break
		}
	}

	result <- operations
}

// CreateMetadata func that create metadata into operations
func (uc *UseCase) CreateMetadata(ctx context.Context, logger libLog.Logger, metadata map[string]any, o *operation.Operation) error {
	if metadata != nil {
		meta := mongodb.Metadata{
			EntityID:   o.ID,
			EntityName: reflect.TypeOf(operation.Operation{}).Name(),
			Data:       metadata,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(operation.Operation{}).Name(), &meta); err != nil {
			logger.Errorf("Error into creating operation metadata: %v", err)

			return pkg.ValidateInternalError(err, reflect.TypeOf(operation.Operation{}).Name())
		}

		o.Metadata = metadata
	}

	return nil
}

// isMatchingAccount checks if the fromTo account matches the balance
func (uc *UseCase) isMatchingAccount(ft pkgTransaction.FromTo, blc *mmodel.Balance) bool {
	return ft.AccountAlias == blc.ID || ft.AccountAlias == blc.Alias
}

// createOperationForBalance creates an operation for a specific balance
func (uc *UseCase) createOperationForBalance(
	ctx context.Context,
	logger libLog.Logger,
	span *trace.Span,
	blc *mmodel.Balance,
	ft pkgTransaction.FromTo,
	transactionID string,
	dsl *pkgTransaction.Transaction,
	validate pkgTransaction.Responses,
) (*operation.Operation, error) {
	balance := operation.Balance{
		Available: &blc.Available,
		OnHold:    &blc.OnHold,
	}

	amt, bat, err := pkgTransaction.ValidateFromToOperation(ft, validate, blc.ToTransactionBalance())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate operation", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(operation.Operation{}).Name())
	}

	amount := operation.Amount{
		Value: &amt.Value,
	}

	balanceAfter := operation.Balance{
		Available: &bat.Available,
		OnHold:    &bat.OnHold,
	}

	description := ft.Description
	if libCommons.IsNilOrEmpty(&ft.Description) {
		description = dsl.Description
	}

	var typeOperation string
	if ft.IsFrom {
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
		ChartOfAccounts: ft.ChartOfAccounts,
		Amount:          amount,
		Balance:         balance,
		BalanceAfter:    balanceAfter,
		BalanceID:       blc.ID,
		AccountID:       blc.AccountID,
		AccountAlias:    blc.Alias,
		OrganizationID:  blc.OrganizationID,
		LedgerID:        blc.LedgerID,
		// BalanceAffected: true because this code path (async queue processing) is ONLY
		// used for balance-affecting transactions. Annotation transactions (status='NOTED')
		// use the HTTP handler path which sets BalanceAffected based on isAnnotation param.
		BalanceAffected: true,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	op, err := uc.OperationRepo.Create(ctx, save)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create operation", err)
		logger.Errorf("Error creating operation: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(operation.Operation{}).Name())
	}

	err = uc.CreateMetadata(ctx, logger, ft.Metadata, op)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create metadata on operation", err)
		logger.Errorf("Failed to create metadata on operation: %v", err)

		return nil, err
	}

	return op, nil
}
