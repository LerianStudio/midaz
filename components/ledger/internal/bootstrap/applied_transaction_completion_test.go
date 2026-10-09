// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

func TestConfigureAppliedTransactionCompletionSharesTenantAwareCompleter(t *testing.T) {
	uc := &command.UseCase{}
	consumer := &RedisQueueConsumer{}
	resolver := &recoveryMongoStub{}
	require.NoError(t, configureAppliedTransactionCompletion(consumer, uc, true, resolver, nil, fakeTransactionFactRecorder{}))

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
	require.Error(t, configureAppliedTransactionCompletion(nil, &command.UseCase{}, false, nil, nil, fakeTransactionFactRecorder{}))
	require.Error(t, configureAppliedTransactionCompletion(&RedisQueueConsumer{}, nil, false, nil, nil, fakeTransactionFactRecorder{}))
}

// fakeTransactionFactRecorder is a transaction fact recorder that never records.
type fakeTransactionFactRecorder struct{}

func (fakeTransactionFactRecorder) RecordTransactionCommitted(context.Context, *sql.Tx, command.TransactionWriteSet) error {
	return nil
}

// The Store exposes no accessor for its recorder and a behavioural check needs
// Postgres (the completion integration tests), so the wiring is pinned on the
// private field: this is the only way to fail when the WithTransactionFactRecorder
// call is dropped.
func TestNewAppliedTransactionStore_CarriesTheTransactionFactRecorder(t *testing.T) {
	t.Parallel()

	store := newAppliedTransactionStore(&command.UseCase{}, fakeTransactionFactRecorder{})
	require.NotNil(t, store)

	recorder := reflect.ValueOf(store).Elem().FieldByName("factRecorder")
	require.True(t, recorder.IsValid(), "the completion Store keeps its fact recorder in factRecorder")
	assert.False(t, recorder.IsNil(), "the Store must carry the fact recorder the wiring hands it")
}

func TestConfigureAppliedTransactionCompletionRejectsMissingFactRecorder(t *testing.T) {
	t.Parallel()

	err := configureAppliedTransactionCompletion(&RedisQueueConsumer{}, &command.UseCase{}, false, nil, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fact recorder")
}
