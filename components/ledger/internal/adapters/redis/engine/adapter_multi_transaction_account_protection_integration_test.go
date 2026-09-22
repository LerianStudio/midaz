//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIntegration_AdapterExecute_MultiTransactionAccountClosingProtection(t *testing.T) {
	t.Run("protected cold seeds apply", func(t *testing.T) {
		ctx := context.Background()
		inspector, _, _ := newAdapterValkey(t)
		input, limits := multiTransactionAcceptanceExecution(t)
		adapter, err := newAdapterWithLimits(&integrationClientProvider{client: inspector}, limits)
		require.NoError(t, err)

		result, err := adapter.Execute(admitEngineSeeds(t, ctx, inspector, input.Execution), input)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Len(t, result.Final, 2)
	})

	t.Run("cold seeds without a batch admission refuse before writes", func(t *testing.T) {
		ctx := context.Background()
		inspector, _, _ := newAdapterValkey(t)
		input, limits := multiTransactionAcceptanceExecution(t)
		keys, err := resolveAdapterKeys(ctx, input.Execution)
		require.NoError(t, err)
		adapter, err := newAdapterWithLimits(&integrationClientProvider{client: inspector}, limits)
		require.NoError(t, err)
		before := captureAdapterState(t, inspector, keys)

		result, err := adapter.Execute(ctx, input)
		require.Nil(t, result)
		require.ErrorContains(t, err, `"code":"admission_not_confirmed"`)
		require.Equal(t, before, captureAdapterState(t, inspector, keys),
			"an unprotected multi-transaction seed must publish no batch state")
	})

	t.Run("closing account refuses the admitted batch before writes", func(t *testing.T) {
		ctx := context.Background()
		inspector, _, _ := newAdapterValkey(t)
		input, limits := multiTransactionAcceptanceExecution(t)
		admittedCtx := admitEngineSeeds(t, ctx, inspector, input.Execution)
		keys, err := resolveAdapterKeys(admittedCtx, input.Execution)
		require.NoError(t, err)

		accountID := input.Execution.Balances[0].AccountID
		protection, ok := keys.Accounts[accountID]
		require.True(t, ok)
		require.NoError(t, inspector.Set(ctx, protection.Closing, "closing-attempt-token", 0).Err())
		before := captureAdapterState(t, inspector, keys)

		adapter, err := newAdapterWithLimits(&integrationClientProvider{client: inspector}, limits)
		require.NoError(t, err)
		result, err := adapter.Execute(admittedCtx, input)
		require.Nil(t, result)
		require.ErrorContains(t, err, `"code":"account_closing_in_progress"`)
		require.Equal(t, before, captureAdapterState(t, inspector, keys),
			"a closing account must refuse before any transaction in the batch writes")
	})
}
