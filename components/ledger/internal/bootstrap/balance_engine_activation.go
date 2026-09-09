// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"errors"
	"fmt"

	redisengine "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/engine"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

func configureBalanceEngine(useCase *command.UseCase, provider redisengine.RedisClientProvider, cfg *Config) error {
	if useCase == nil || cfg == nil {
		return errors.New("balance engine activation requires command and configuration owners")
	}

	if !cfg.BalanceEngineEnabled {
		return nil
	}

	if useCase.BalanceEngineFinalizer == nil {
		return errors.New("balance engine activation requires a durable finalizer")
	}

	adapter, err := redisengine.NewAdapter(provider, redisengine.Limits{
		MaxTransactions:  cfg.BalanceEngineMaxTransactions,
		MaxPostings:      cfg.BalanceEngineMaxPostings,
		MaxBalances:      cfg.BalanceEngineMaxBalances,
		MaxRecoveryBytes: cfg.BalanceEngineMaxRecoveryBytes,
		MaxRequestBytes:  cfg.BalanceEngineMaxRequestBytes,
		MaxPreparedBytes: cfg.BalanceEngineMaxPreparedBytes,
	})
	if err != nil {
		return fmt.Errorf("initialize balance engine adapter: %w", err)
	}

	useCase.BalanceEngine = adapter

	return nil
}
