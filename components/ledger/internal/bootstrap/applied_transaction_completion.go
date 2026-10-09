// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"errors"

	postgresCompletion "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/completion"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

// configureAppliedTransactionCompletion shares completion of already-applied
// transactions and its tenant resolution between normal writes and recovery, so
// feeDebt records the same fee-debt changes on both and factRecorder records
// the same transaction facts on both. It does not enable the engine. The
// recorder is required: a disabled System Facts producer hands the no-op one.
func configureAppliedTransactionCompletion(consumer *RedisQueueConsumer, useCase *command.UseCase, multiTenantEnabled bool, mongoResolver recoveryMongoResolver, feeDebt command.FeeDebtRecorder, factRecorder command.TransactionFactRecorder, maxRowsPerInsert ...int) error {
	if consumer == nil || useCase == nil {
		return errors.New("applied transaction completion requires command and recovery owners")
	}

	if factRecorder == nil {
		return errors.New("applied transaction completion requires a transaction fact recorder")
	}

	delegate, err := command.NewTransactionCompletionServiceWithEvents(
		newAppliedTransactionStore(useCase, factRecorder, maxRowsPerInsert...),
		useCase.TransactionMetadataRepo,
		useCase,
	)
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

// newAppliedTransactionStore builds the completion Store over the use case's
// repositories with factRecorder attached, so every created or transitioned
// transaction row is handed to it inside the SQL transaction that commits it.
func newAppliedTransactionStore(useCase *command.UseCase, factRecorder command.TransactionFactRecorder, maxRowsPerInsert ...int) *postgresCompletion.Store {
	return postgresCompletion.NewStore(useCase.TransactionRepo, useCase.OperationRepo, maxRowsPerInsert...).
		WithTransactionFactRecorder(factRecorder)
}
