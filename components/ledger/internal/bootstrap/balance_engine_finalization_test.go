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

func TestConfigureBalanceEngineFinalizationSharesTenantAwareCompletion(t *testing.T) {
	uc := &command.UseCase{}
	consumer := &RedisQueueConsumer{}
	resolver := &recoveryMongoStub{}
	require.NoError(t, configureBalanceEngineFinalization(consumer, uc, true, resolver))

	finalizer, ok := consumer.recoveryFinalizer.(*tenantRecoveryFinalizer)
	require.True(t, ok)
	assert.Same(t, finalizer, uc.BalanceEngineFinalizer)
	assert.Same(t, resolver, finalizer.mongoResolver)
	assert.True(t, finalizer.multiTenantEnabled)
	assert.IsType(t, &command.BalanceEngineFinalizer{}, finalizer.delegate)
	assert.Nil(t, uc.BalanceEngine)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	outcome, err := uc.BalanceEngineFinalizer.FinalizeWithOutcome(ctx, &command.BalanceEngineRecoveryEnvelope{})
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, command.BalanceEngineFinalizationResult{}, outcome)
	assert.ErrorIs(t, consumer.recoveryFinalizer.Finalize(ctx, &command.BalanceEngineRecoveryEnvelope{}), context.Canceled)
}

func TestConfigureBalanceEngineFinalizationRejectsMissingOwners(t *testing.T) {
	require.Error(t, configureBalanceEngineFinalization(nil, &command.UseCase{}, false, nil))
	require.Error(t, configureBalanceEngineFinalization(&RedisQueueConsumer{}, nil, false, nil))
}
