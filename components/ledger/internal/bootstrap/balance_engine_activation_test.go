// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	redisengine "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/engine"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

type balanceEngineProviderStub struct{}

func (*balanceEngineProviderStub) GetClient(context.Context) (redis.UniversalClient, error) {
	return nil, nil
}

type balanceTransactionCompleterStub struct{}

func (*balanceTransactionCompleterStub) Complete(context.Context, *command.TransactionCompletionRecord) (command.TransactionCompletionResult, error) {
	return command.TransactionCompletionResult{}, nil
}

func TestConfigureBalanceEngineDisabledLeavesLegacyPath(t *testing.T) {
	useCase := &command.UseCase{}
	require.NoError(t, configureBalanceEngine(useCase, nil, &Config{}))
	assert.Nil(t, useCase.BalanceEngine)
}

func TestConfigureBalanceEngineEnabledWiresAdapter(t *testing.T) {
	useCase := &command.UseCase{TransactionCompleter: &balanceTransactionCompleterStub{}}
	cfg := validBalanceEngineConfig()

	require.NoError(t, configureBalanceEngine(useCase, &balanceEngineProviderStub{}, cfg))
	assert.IsType(t, &redisengine.Adapter{}, useCase.BalanceEngine)
}

func TestConfigureBalanceEngineEnabledRequiresTransactionCompleter(t *testing.T) {
	cfg := validBalanceEngineConfig()
	err := configureBalanceEngine(&command.UseCase{}, &balanceEngineProviderStub{}, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transaction completer")
}

func TestConfigureBalanceEngineEnabledRequiresPositiveLimits(t *testing.T) {
	useCase := &command.UseCase{TransactionCompleter: &balanceTransactionCompleterStub{}}
	cfg := validBalanceEngineConfig()
	cfg.BalanceEngineMaxPreparedBytes = 0

	err := configureBalanceEngine(useCase, &balanceEngineProviderStub{}, cfg)
	require.Error(t, err)
	assert.Nil(t, useCase.BalanceEngine)
}

func TestBalanceEngineConfigLoadsFromEnvironment(t *testing.T) {
	t.Setenv("BALANCE_ENGINE_ENABLED", "true")
	t.Setenv("BALANCE_ENGINE_MAX_TRANSACTIONS", "1")
	t.Setenv("BALANCE_ENGINE_MAX_POSTINGS", "10000")
	t.Setenv("BALANCE_ENGINE_MAX_BALANCES", "20000")
	t.Setenv("BALANCE_ENGINE_MAX_RECOVERY_BYTES", "33554432")
	t.Setenv("BALANCE_ENGINE_MAX_REQUEST_BYTES", "67108864")
	t.Setenv("BALANCE_ENGINE_MAX_PREPARED_BYTES", "67108864")

	cfg := &Config{}
	require.NoError(t, libCommons.SetConfigFromEnvVars(cfg))
	assert.Equal(t, validBalanceEngineConfig(), cfgWithOnlyBalanceEngineFields(cfg))
}

func validBalanceEngineConfig() *Config {
	return &Config{
		BalanceEngineEnabled:              true,
		BalanceEngineMaxTransactions:      1,
		BalanceEngineMaxPostings:          10_000,
		BalanceEngineMaxBalances:          20_000,
		TransactionCompletionMaxPlanBytes: 32 * 1024 * 1024,
		BalanceEngineMaxRequestBytes:      64 * 1024 * 1024,
		BalanceEngineMaxPreparedBytes:     64 * 1024 * 1024,
	}
}

func cfgWithOnlyBalanceEngineFields(cfg *Config) *Config {
	return &Config{
		BalanceEngineEnabled:              cfg.BalanceEngineEnabled,
		BalanceEngineMaxTransactions:      cfg.BalanceEngineMaxTransactions,
		BalanceEngineMaxPostings:          cfg.BalanceEngineMaxPostings,
		BalanceEngineMaxBalances:          cfg.BalanceEngineMaxBalances,
		TransactionCompletionMaxPlanBytes: cfg.TransactionCompletionMaxPlanBytes,
		BalanceEngineMaxRequestBytes:      cfg.BalanceEngineMaxRequestBytes,
		BalanceEngineMaxPreparedBytes:     cfg.BalanceEngineMaxPreparedBytes,
	}
}
