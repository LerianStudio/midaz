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

type engineProviderStub struct{}

func (*engineProviderStub) GetClient(context.Context) (redis.UniversalClient, error) {
	return nil, nil
}

type appliedTransactionCompleterStub struct{}

func (*appliedTransactionCompleterStub) Complete(context.Context, *command.TransactionCompletionRecord) (command.TransactionCompletionResult, error) {
	return command.TransactionCompletionResult{}, nil
}

func TestConfigureEngineWiresDefaultAdapter(t *testing.T) {
	useCase := &command.UseCase{AppliedTransactionCompleter: &appliedTransactionCompleterStub{}}

	require.NoError(t, configureEngine(useCase, &engineProviderStub{}))
	assert.IsType(t, &redisengine.Adapter{}, useCase.Engine)
}

func TestConfigureEngineRequiresAppliedTransactionCompleter(t *testing.T) {
	err := configureEngine(&command.UseCase{}, &engineProviderStub{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "applied transaction completer")
}

func TestConfigureEngineRequiresCommandOwner(t *testing.T) {
	err := configureEngine(nil, &engineProviderStub{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command owner")
}

func TestConfigureEngineRequiresProvider(t *testing.T) {
	useCase := &command.UseCase{AppliedTransactionCompleter: &appliedTransactionCompleterStub{}}

	err := configureEngine(useCase, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider")
	assert.Nil(t, useCase.Engine)
}
