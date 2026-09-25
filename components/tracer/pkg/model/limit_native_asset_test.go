// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestLimitNativeAssetCodes(t *testing.T) {
	for _, code := range []string{"BTC", "POINTS", "wBTC", "US1", "X", "token/v1", strings.Repeat("x", 256)} {
		t.Run(code, func(t *testing.T) {
			limit, err := NewLimit("Native", LimitTypeDaily, decimal.RequireFromString("10.125"), code, []Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(89001))}}, nil, testutil.FixedTime())
			require.NoError(t, err)
			require.Equal(t, code, limit.Asset)
		})
	}
	for _, code := range []string{"", " BTC", "BTC ", "x\x00y", string([]byte{0xff}), strings.Repeat("é", 129)} {
		t.Run("invalid"+code, func(t *testing.T) {
			_, err := NewLimit("Native", LimitTypeDaily, decimal.RequireFromString("10.125"), code, []Scope{{AccountID: testutil.UUIDPtr(testutil.MustDeterministicUUID(89001))}}, nil, testutil.FixedTime())
			require.Error(t, err)
		})
	}
}
