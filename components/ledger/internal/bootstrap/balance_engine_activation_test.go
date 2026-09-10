// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"testing"

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

type appliedTransactionCompleterStub struct{}

func (*appliedTransactionCompleterStub) Complete(context.Context, *command.TransactionCompletionRecord) (command.TransactionCompletionResult, error) {
	return command.TransactionCompletionResult{}, nil
}

func TestConfigureBalanceEngineWiresDefaultAdapter(t *testing.T) {
	useCase := &command.UseCase{AppliedTransactionCompleter: &appliedTransactionCompleterStub{}}

	require.NoError(t, configureBalanceEngine(useCase, &balanceEngineProviderStub{}))
	assert.IsType(t, &redisengine.Adapter{}, useCase.BalanceEngine)
}

func TestConfigureBalanceEngineRequiresAppliedTransactionCompleter(t *testing.T) {
	err := configureBalanceEngine(&command.UseCase{}, &balanceEngineProviderStub{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "applied transaction completer")
}

func TestConfigureBalanceEngineRequiresCommandOwner(t *testing.T) {
	err := configureBalanceEngine(nil, &balanceEngineProviderStub{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command owner")
}

func TestConfigureBalanceEngineRequiresProvider(t *testing.T) {
	useCase := &command.UseCase{AppliedTransactionCompleter: &appliedTransactionCompleterStub{}}

	err := configureBalanceEngine(useCase, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider")
	assert.Nil(t, useCase.BalanceEngine)
}
