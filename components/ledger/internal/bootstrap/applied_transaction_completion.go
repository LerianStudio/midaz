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
// transactions and its tenant resolution between normal writes and recovery.
// It does not enable the engine.
func configureAppliedTransactionCompletion(consumer *RedisQueueConsumer, useCase *command.UseCase, multiTenantEnabled bool, mongoResolver recoveryMongoResolver) error {
	if consumer == nil || useCase == nil {
		return errors.New("applied transaction completion requires command and recovery owners")
	}

	delegate, err := command.NewTransactionCompletionServiceWithEvents(
		postgresCompletion.NewStore(useCase.TransactionRepo, useCase.OperationRepo),
		useCase.TransactionMetadataRepo,
		useCase,
	)
	if err != nil {
		return err
	}

	completer := &tenantAppliedTransactionCompleter{
		delegate: delegate, multiTenantEnabled: multiTenantEnabled, mongoResolver: mongoResolver,
	}
	useCase.AppliedTransactionCompleter = completer
	consumer.WithAppliedTransactionCompleter(completer)
	useCase.EngineRecoveryAcknowledger = consumer.newRecoveryRecordCompleter()

	return nil
}
