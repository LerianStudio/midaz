// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"reflect"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/mongo"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
)

// CreateTransaction creates a new transaction persisting data in the repository.
func (uc *UseCase) CreateTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, t *pkgTransaction.Transaction) (*transaction.Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_transaction")
	defer span.End()

	logger.Infof("Trying to create new transaction")

	// Mirror the query-path guard in get-balances.go:227. During an in-progress
	// shard migration for any alias referenced by this transaction, writes must
	// block briefly so the persisted rows don't race with migration cleanup and
	// land on soon-to-be-deleted source keys.
	if err := uc.waitForMigrationUnlock(ctx, organizationID, ledgerID, collectTransactionAliases(t)); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed due to in-progress shard migration", err)

		logger.Errorf("Failed due to in-progress shard migration: %v", err)

		return nil, err
	}

	description := constant.APPROVED
	status := transaction.Status{
		Code:        description,
		Description: &description,
	}

	var parentTransactionID *string

	if transactionID != uuid.Nil {
		value := transactionID.String()
		parentTransactionID = &value
	}

	save := &transaction.Transaction{
		ID:                       libCommons.GenerateUUIDv7().String(),
		ParentTransactionID:      parentTransactionID,
		OrganizationID:           organizationID.String(),
		LedgerID:                 ledgerID.String(),
		Description:              t.Description,
		Status:                   status,
		Amount:                   &t.Send.Value,
		AssetCode:                t.Send.Asset,
		ChartOfAccountsGroupName: t.ChartOfAccountsGroupName,
		Body:                     *t,
		CreatedAt:                time.Now().UTC(),
		UpdatedAt:                time.Now().UTC(),
	}

	tran, err := uc.TransactionRepo.Create(ctx, save)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create transaction on repo", err)

		logger.Errorf("Error creating t: %v", err)

		return nil, err
	}

	if tran == nil { //nolint:nestif
		if transactionUUID, parseErr := uuid.Parse(save.ID); parseErr == nil {
			existing, findErr := uc.TransactionRepo.Find(ctx, organizationID, ledgerID, transactionUUID)
			if findErr == nil && existing != nil {
				tran = existing
			} else {
				logger.Warnf("Transaction repo returned nil transaction without error and fallback lookup failed (id=%s): %v", save.ID, findErr)
			}
		}

		if tran == nil {
			logger.Warnf("Transaction repo returned nil transaction without error (id=%s); using request payload as canonical transaction", save.ID)
			tran = save
		}
	}

	if t.Metadata != nil {
		meta := mongodb.Metadata{
			EntityID:   tran.ID,
			EntityName: reflect.TypeOf(transaction.Transaction{}).Name(),
			Data:       t.Metadata,
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		}

		if err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(transaction.Transaction{}).Name(), &meta); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				tran.Metadata = t.Metadata
				return tran, nil
			}

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create transaction metadata", err)

			logger.Errorf("Error into creating transactiont metadata: %v", err)

			return nil, err
		}

		tran.Metadata = t.Metadata
	}

	return tran, nil
}
