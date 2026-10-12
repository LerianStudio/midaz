// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"errors"
	"fmt"

	postgresCompletion "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/completion"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

// configureAppliedTransactionCompletion shares completion of already-applied
// transactions and its tenant resolution between normal writes and recovery, so
// feeDebt records the same fee-debt changes on both and factRecorder records
// the same transaction facts on both. It does not enable the engine. The Store
// refuses a nil recorder: a disabled System Facts producer hands the no-op one.
func configureAppliedTransactionCompletion(consumer *RedisQueueConsumer, useCase *command.UseCase, multiTenantEnabled bool, mongoResolver recoveryMongoResolver, feeDebt command.FeeDebtRecorder, factRecorder command.TransactionFactRecorder, maxRowsPerInsert ...int) error {
	if consumer == nil || useCase == nil {
		return errors.New("applied transaction completion requires command and recovery owners")
	}

	store, err := postgresCompletion.NewStore(useCase.TransactionRepo, useCase.OperationRepo, factRecorder, maxRowsPerInsert...)
	if err != nil {
		return fmt.Errorf("applied transaction completion: %w", err)
	}

	delegate, err := command.NewTransactionCompletionServiceWithEvents(store, useCase.TransactionMetadataRepo, useCase)
	if err != nil {
		return err
	}

	delegate.WithFeeDebtRecorder(feeDebt)
	useCase.FeeDebts = feeDebt

	completer := &tenantAppliedTransactionCompleter{
		delegate: delegate, multiTenantEnabled: multiTenantEnabled, mongoResolver: mongoResolver,
	}
	useCase.AppliedTransactionCompleter = completer
	consumer.WithAppliedTransactionCompleter(completer)
	consumer.tenantMongo = mongoResolver
	useCase.EngineRecoveryAcknowledger = consumer.newRecoveryRecordCompleter()

	return nil
}
