// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"testing"

	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFeeHarness_Sanity_NoPackageSucceeds proves the harness itself is sound:
// with NO fee package seeded, applyFees is a no-op and a plain transfer creates
// and balances exactly like the existing non-fee integration suite. This
// isolates the proof-suite failures to the fee seam (when a package IS present),
// not to the harness wiring.
func TestFeeHarness_Sanity_NoPackageSucceeds(t *testing.T) {
	h := setupFeeHarness(t)
	app := h.newApp()

	h.seedBalance(t, "@payer", "USD", decimal.NewFromInt(1000), "deposit")
	h.seedBalance(t, "@receiver", "USD", decimal.Zero, "deposit")

	// No package seeded -> fee engine finds no package -> applyFees is a no-op.
	body := `{
		"description": "no-fee transfer through the fee harness",
		"pending": false,
		"send": {
			"asset": "USD",
			"value": "100",
			"source": { "from": [{"accountAlias": "@payer", "amount": {"asset": "USD", "value": "100"}}] },
			"distribute": { "to": [{"accountAlias": "@receiver", "amount": {"asset": "USD", "value": "100"}}] }
		}
	}`

	resp := h.createJSON(t, app, body, nil)
	require.Equalf(t, 201, resp.status, "no-fee create through the harness must succeed: %s", string(resp.rawBody))

	txID := mustTxID(t, resp)
	require.Equal(t, cn.APPROVED, dbTxStatus(t, h.db, txID))

	legs := loadLegs(t, h.db, txID)
	require.Len(t, legs, 2, "a plain transfer must persist exactly 2 operations")
	requireBalanced(t, legs, "no-fee transfer")
	assert.True(t, dbTxAmount(t, h.db, txID).Equal(decimal.NewFromInt(100)))
}
