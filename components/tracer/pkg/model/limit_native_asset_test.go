// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestLimitNativeAssetCodes(t *testing.T) {
	t.Parallel()

	scopes := func() []Scope {
		return []Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(89001))}}
	}

	for _, code := range []string{"BTC", "USD", "X", "LERIANPOINTS", strings.Repeat("A", 100)} {
		t.Run("accepts_"+code, func(t *testing.T) {
			t.Parallel()

			limit, err := NewLimit("Native", LimitTypeDaily, decimal.RequireFromString("10.125"), code, scopes(), nil, testutil.FixedTime())
			require.NoError(t, err)
			require.Equal(t, code, limit.Asset)
			require.NoError(t, ValidateLimitAssetCode(code))
		})
	}

	invalid := map[string]string{
		"empty":        "",
		"lowercase":    "usd",
		"mixed_case":   "wBTC",
		"digit":        "US1",
		"hyphen":       "US-D",
		"slash":        "token/v1",
		"leading_ws":   " BTC",
		"trailing_ws":  "BTC ",
		"nul":          "B\x00C",
		"invalid_utf8": string([]byte{0xff}),
		"too_long":     strings.Repeat("A", 101),
	}

	for name, code := range invalid {
		t.Run("rejects_"+name, func(t *testing.T) {
			t.Parallel()

			_, err := NewLimit("Native", LimitTypeDaily, decimal.RequireFromString("10.125"), code, scopes(), nil, testutil.FixedTime())
			require.ErrorIs(t, err, constant.ErrLimitInvalidCurrency)
			require.ErrorIs(t, ValidateLimitAssetCode(code), constant.ErrLimitInvalidCurrency)
		})
	}
}
