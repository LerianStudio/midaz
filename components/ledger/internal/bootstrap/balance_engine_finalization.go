// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"errors"

	postgresRecovery "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/recovery"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

// configureBalanceEngineFinalization shares durable completion and its tenant
// resolution between normal writes and recovery. It does not enable the engine.
func configureBalanceEngineFinalization(consumer *RedisQueueConsumer, useCase *command.UseCase, multiTenantEnabled bool, mongoResolver recoveryMongoResolver) error {
	if consumer == nil || useCase == nil {
		return errors.New("balance engine finalization requires command and recovery owners")
	}

	delegate, err := command.NewBalanceEngineFinalizerWithEvents(
		postgresRecovery.NewStore(useCase.TransactionRepo, useCase.OperationRepo),
		useCase.TransactionMetadataRepo,
		useCase,
	)
	if err != nil {
		return err
	}

	finalizer := &tenantRecoveryFinalizer{
		delegate: delegate, multiTenantEnabled: multiTenantEnabled, mongoResolver: mongoResolver,
	}
	useCase.BalanceEngineFinalizer = finalizer
	consumer.WithBalanceEngineFinalizer(finalizer)

	return nil
}
