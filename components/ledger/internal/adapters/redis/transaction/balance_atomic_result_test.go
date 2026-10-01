// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

func requestedBalances() map[string]*mmodel.Balance {
	return map[string]*mmodel.Balance{
		"0#@src#default": {ID: "b1", Alias: "0#@src#default", Key: "default", AssetCode: "BRL"},
		"1#@dst#default": {ID: "b2", Alias: "1#@dst#default", Key: "default", AssetCode: "BRL"},
	}
}

func TestDecodeBalanceAtomicResult_MapsRequestedBalances(t *testing.T) {
	result := `{"before":[{"ID":"b1","Alias":"0#@src#default","Available":"100","OnHold":"0","Version":1},` +
		`{"ID":"b2","Alias":"1#@dst#default","Available":"0","OnHold":"0","Version":4}],` +
		`"after":[{"ID":"b1","Alias":"0#@src#default","Available":"90","OnHold":"0","Version":2},` +
		`{"ID":"b2","Alias":"1#@dst#default","Available":"10","OnHold":"0","Version":5}]}`

	got, err := decodeBalanceAtomicResult(context.Background(), result, requestedBalances())

	require.NoError(t, err)
	require.Len(t, got.Before, 2)
	require.Len(t, got.After, 2)
	assert.True(t, decimal.NewFromInt(90).Equal(got.After[0].Available))
}

func TestDecodeBalanceAtomicResult_EmptyResultIsValid(t *testing.T) {
	for _, result := range []string{`{"before":[],"after":[]}`, `{"before":{},"after":{}}`} {
		got, err := decodeBalanceAtomicResult(context.Background(), result, requestedBalances())

		require.NoError(t, err, result)
		assert.Empty(t, got.Before)
		assert.Empty(t, got.After)
	}
}

// Once the script has answered, every failure to turn its answer into the
// requested balances means balances may have moved without a usable snapshot.
func TestDecodeBalanceAtomicResult_UnusableResultIsReported(t *testing.T) {
	tests := map[string]any{
		"balance outside the request": `{"before":[{"ID":"b9","Alias":"9#@other#default","Available":"100","OnHold":"0","Version":1}],` +
			`"after":[{"ID":"b9","Alias":"9#@other#default","Available":"90","OnHold":"0","Version":2}]}`,
		"undecodable balance": `{"before":[{"ID":"b1","Alias":"0#@src#default","Available":true,"OnHold":"0","Version":1}],` +
			`"after":[{"ID":"b1","Alias":"0#@src#default","Available":"90","OnHold":"0","Version":2}]}`,
		"malformed document":     `{"before":[`,
		"unexpected result type": int64(1),
	}

	for name, result := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := decodeBalanceAtomicResult(context.Background(), result, requestedBalances())

			var unusable *UnusableBalanceResultError
			assert.ErrorAs(t, err, &unusable)
		})
	}
}
