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

func TestConfigureAppliedTransactionCompletionSharesTenantAwareCompleter(t *testing.T) {
	uc := &command.UseCase{}
	consumer := &RedisQueueConsumer{}
	resolver := &recoveryMongoStub{}
	require.NoError(t, configureAppliedTransactionCompletion(consumer, uc, true, resolver))

	completer, ok := consumer.appliedTransactionCompleter.(*tenantAppliedTransactionCompleter)
	require.True(t, ok)
	assert.Same(t, completer, uc.AppliedTransactionCompleter)
	assert.IsType(t, &recoveryRecordCompleter{}, uc.EngineRecoveryAcknowledger)
	assert.Same(t, resolver, completer.mongoResolver)
	assert.True(t, completer.multiTenantEnabled)
	assert.IsType(t, &command.TransactionCompletionService{}, completer.delegate)
	assert.Nil(t, uc.Engine)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	outcome, err := uc.AppliedTransactionCompleter.Complete(ctx, &command.TransactionCompletionRecord{})
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, command.TransactionCompletionResult{}, outcome)
	assert.ErrorIs(t, completionError(consumer.appliedTransactionCompleter.Complete(ctx, &command.TransactionCompletionRecord{})), context.Canceled)
}

func TestConfigureAppliedTransactionCompletionRejectsMissingOwners(t *testing.T) {
	require.Error(t, configureAppliedTransactionCompletion(nil, &command.UseCase{}, false, nil))
	require.Error(t, configureAppliedTransactionCompletion(&RedisQueueConsumer{}, nil, false, nil))
}
