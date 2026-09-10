// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

func TestConfigureTransactionCompletionSharesTenantAwareCompletion(t *testing.T) {
	uc := &command.UseCase{}
	consumer := &RedisQueueConsumer{}
	resolver := &recoveryMongoStub{}
	require.NoError(t, configureTransactionCompletion(consumer, uc, true, resolver))

	finalizer, ok := consumer.transactionCompleter.(*tenantTransactionCompleter)
	require.True(t, ok)
	assert.Same(t, finalizer, uc.TransactionCompleter)
	assert.Same(t, resolver, finalizer.mongoResolver)
	assert.True(t, finalizer.multiTenantEnabled)
	assert.IsType(t, &command.TransactionCompletionService{}, finalizer.delegate)
	assert.Nil(t, uc.BalanceEngine)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	outcome, err := uc.TransactionCompleter.Complete(ctx, &command.TransactionCompletionRecord{})
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, command.TransactionCompletionResult{}, outcome)
	assert.ErrorIs(t, completionError(consumer.transactionCompleter.Complete(ctx, &command.TransactionCompletionRecord{})), context.Canceled)
}

func TestConfigureTransactionCompletionRejectsMissingOwners(t *testing.T) {
	require.Error(t, configureTransactionCompletion(nil, &command.UseCase{}, false, nil))
	require.Error(t, configureTransactionCompletion(&RedisQueueConsumer{}, nil, false, nil))
}
