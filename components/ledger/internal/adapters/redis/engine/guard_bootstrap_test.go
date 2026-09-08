// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

var _ command.BalanceEngineGuardBootstrapper = (*Adapter)(nil)

type guardFailureHook struct {
	cause   error
	calls   int
	noRetry bool
}

type guardStaticProvider struct {
	client redis.UniversalClient
}

func (provider guardStaticProvider) GetClient(context.Context) (redis.UniversalClient, error) {
	return provider.client, nil
}

func (hook *guardFailureHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (hook *guardFailureHook) ProcessHook(redis.ProcessHook) redis.ProcessHook {
	return func(_ context.Context, cmd redis.Cmder) error {
		if strings.EqualFold(cmd.Name(), "hsetnx") {
			hook.calls++
			hook.noRetry = cmd.NoRetry()
		}

		return hook.cause
	}
}

func (hook *guardFailureHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func TestEnsureTransactionGuardRejectsInvalidInputBeforeProvider(t *testing.T) {
	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	transactionID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	limits := Limits{
		MaxTransactions: 1, MaxPostings: 1, MaxBalances: 1,
		MaxRecoveryBytes: 128, MaxRequestBytes: 128, MaxPreparedBytes: 8,
	}

	tests := []struct {
		name                     string
		organizationID, ledgerID uuid.UUID
		transactionID            uuid.UUID
		nextToken                string
	}{
		{name: "organization", ledgerID: ledgerID, transactionID: transactionID, nextToken: "PENDING"},
		{name: "ledger", organizationID: organizationID, transactionID: transactionID, nextToken: "PENDING"},
		{name: "transaction", organizationID: organizationID, ledgerID: ledgerID, nextToken: "PENDING"},
		{name: "empty token", organizationID: organizationID, ledgerID: ledgerID, transactionID: transactionID},
		{name: "oversized token", organizationID: organizationID, ledgerID: ledgerID, transactionID: transactionID, nextToken: strings.Repeat("x", limits.MaxPreparedBytes+1)},
		{name: "invalid UTF-8 token", organizationID: organizationID, ledgerID: ledgerID, transactionID: transactionID, nextToken: string([]byte{utf8.RuneSelf, 0xff})},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &countingProvider{}
			adapter, err := NewAdapter(provider, limits)
			require.NoError(t, err)

			err = adapter.EnsureTransactionGuard(context.Background(), test.organizationID, test.ledgerID, test.transactionID, test.nextToken)
			assertAdapterTechnical(t, err, "invalid_request", false)
			require.Zero(t, provider.calls)
		})
	}
}

func TestEnsureTransactionGuardRejectsCanceledContextBeforeProvider(t *testing.T) {
	provider := &countingProvider{}
	adapter, err := NewAdapter(provider, Limits{
		MaxTransactions: 1, MaxPostings: 1, MaxBalances: 1,
		MaxRecoveryBytes: 128, MaxRequestBytes: 128, MaxPreparedBytes: 128,
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = adapter.EnsureTransactionGuard(
		ctx,
		uuid.MustParse("11111111-1111-4111-8111-111111111111"),
		uuid.MustParse("22222222-2222-4222-8222-222222222222"),
		uuid.MustParse("33333333-3333-4333-8333-333333333333"),
		"PENDING",
	)
	assertAdapterTechnical(t, err, "context_canceled", false)
	require.Zero(t, provider.calls)
}

func TestEnsureTransactionGuardClassifiesCommandFailureWithoutRetry(t *testing.T) {
	cause := errors.New("connection dropped after write")
	hook := &guardFailureHook{cause: cause}
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: 3})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	client.AddHook(hook)
	adapter, err := NewAdapter(guardStaticProvider{client: client}, Limits{
		MaxTransactions: 1, MaxPostings: 1, MaxBalances: 1,
		MaxRecoveryBytes: 128, MaxRequestBytes: 128, MaxPreparedBytes: 128,
	})
	require.NoError(t, err)

	err = adapter.EnsureTransactionGuard(
		context.Background(),
		uuid.MustParse("11111111-1111-4111-8111-111111111111"),
		uuid.MustParse("22222222-2222-4222-8222-222222222222"),
		uuid.MustParse("33333333-3333-4333-8333-333333333333"),
		"PENDING",
	)
	assertAdapterTechnical(t, err, "transport", true)
	require.ErrorIs(t, err, cause)
	require.Equal(t, 1, hook.calls)
	require.True(t, hook.noRetry)
}
