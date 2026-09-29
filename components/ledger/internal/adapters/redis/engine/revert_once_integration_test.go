//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

func TestIntegrationEngineRevertsAnOriginOnce(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}
	container := redistestutil.SetupReusableContainer(t)
	f := newIntegrationFixture(t, container.Client)
	f.seed(t, 0, f.input.Execution.Balances[0])
	ctx := context.Background()

	// revert points the fixture at a new execution of transaction txID reverting parent.
	revert := func(executionID, txID, parent uuid.UUID) {
		f.input.Execution.ExecutionID = executionID
		f.input.Execution.Transactions[0].ID = txID
		f.input.Guards[0].TransactionID, f.input.Guards[0].NextToken = txID, txID.String()
		f.input.CompletionPlans[0].TransactionID = txID
		f.input.CompletionPlans[0].Payload = json.RawMessage(`{"action":"revert","parentTransactionId":"` + parent.String() + `"}`)
	}
	markedBy := func(origin uuid.UUID) string {
		value, err := container.Client.HGet(ctx, f.resolved.Guards, origin.String()+":reverted").Result()
		require.NoError(t, err)
		return value
	}

	origin := uuid.MustParse("7d0c6f3e-6a0b-4f59-9d0e-2f3f8f7a1c01")
	firstExecution, first := uuid.MustParse("a1f2c3d4-0000-4000-8000-000000000001"), uuid.MustParse("a1f2c3d4-0000-4000-8000-0000000000f1")
	revert(firstExecution, first, origin)
	raw, err := f.run(t)
	require.NoError(t, err)
	require.Equal(t, first.String(), markedBy(origin))

	before := f.capture(t)
	replay, err := f.run(t)
	require.NoError(t, err)
	require.Equal(t, raw, replay, "the first revert still replays")
	require.Equal(t, before, f.capture(t))

	revert(uuid.MustParse("a1f2c3d4-0000-4000-8000-000000000002"), uuid.MustParse("a1f2c3d4-0000-4000-8000-0000000000f2"), origin)
	before = f.capture(t)
	_, err = f.run(t)
	require.ErrorContains(t, err, `"code":"transaction_already_reverted"`)
	require.Equal(t, before, f.capture(t), "a second revert of the origin writes nothing")
}
