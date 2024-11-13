package command

import (
	"context"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"github.com/google/uuid"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/constant"
	gold "github.com/LerianStudio/midaz/common/gold/transaction/model"
	m "github.com/LerianStudio/midaz/components/transaction/internal/domain/metadata"
	t "github.com/LerianStudio/midaz/components/transaction/internal/domain/transaction"
)

// CreateTransaction creates a new transaction persisting data in the repository.
func (uc *UseCase) CreateTransaction(ctx context.Context, organizationID, ledgerID uuid.UUID, transaction *gold.Transaction) (*t.Transaction, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_transaction")
	defer span.End()

	logger.Infof("Trying to create new transaction")

	description := constant.CREATED
	status := t.Status{
		Code:        description,
		Description: &description,
	}

	amount := float64(transaction.Send.Value)
	scale := float64(transaction.Send.Scale)

	save := &t.Transaction{
		ID:                       common.GenerateUUIDv7().String(),
		ParentTransactionID:      nil,
		OrganizationID:           organizationID.String(),
		LedgerID:                 ledgerID.String(),
		Description:              transaction.Description,
		Template:                 transaction.ChartOfAccountsGroupName,
		Status:                   status,
		Amount:                   &amount,
		AmountScale:              &scale,
		AssetCode:                transaction.Send.Asset,
		ChartOfAccountsGroupName: transaction.ChartOfAccountsGroupName,
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
	}

	tran, err := uc.TransactionRepo.Create(ctx, save)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create transaction on repo", err)

		logger.Errorf("Error creating transaction: %v", err)

		return nil, err
	}

	if transaction.Metadata != nil {
		if err := common.CheckMetadataKeyAndValueLength(100, transaction.Metadata); err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to check metadata key and value length", err)

			return nil, err
		}

		meta := m.Metadata{
			EntityID:   tran.ID,
			EntityName: reflect.TypeOf(t.Transaction{}).Name(),
			Data:       transaction.Metadata,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(t.Transaction{}).Name(), &meta); err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to create transaction metadata", err)

			logger.Errorf("Error into creating transaction metadata: %v", err)

			return nil, err
		}

		tran.Metadata = transaction.Metadata
	}

	return tran, nil
}
