// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// TestEffectiveOperationAmount locks the overdraft audit-trail contract:
// the primary Operation record's `amount` must equal the ACTUAL balance
// movement when the overdraft engine redirects part of a debit/credit
// onto the companion balance. A companion Operation record with its own
// amount carries the redirected portion, so `sum(primary, companion)`
// still equals the user-requested amount and double-entry holds across
// the enriched record set.
//
// Regression trigger: a debit of 100 on a balance with Available=50 and
// overdraft enabled persisted `amount=100` on the primary DEBIT row while
// the default balance only moved 50 (the other 50 was accrued on the
// companion via a second DEBIT row also showing amount=50). sum(debits)
// then equaled 150 while sum(credits) was 100, which breaks every
// reconciliation report downstream.
func TestEffectiveOperationAmount(t *testing.T) {
	dec := decimal.NewFromInt

	cases := []struct {
		name            string
		requested       int64
		beforeAvailable int64
		afterAvailable  int64
		beforeOnHold    int64
		afterOnHold     int64
		expected        int64
	}{
		{
			name:            "normal debit — no overdraft, movement equals requested",
			requested:       100,
			beforeAvailable: 500,
			afterAvailable:  400,
			expected:        100,
		},
		{
			name:            "overdraft debit split — balance floored at 0, only actual movement recorded",
			requested:       100,
			beforeAvailable: 50,
			afterAvailable:  0,
			expected:        50,
		},
		{
			name:            "overdraft debit from already-at-zero — primary movement is 0, full redirect to companion",
			requested:       80,
			beforeAvailable: 0,
			afterAvailable:  0,
			expected:        0,
		},
		{
			name:            "credit repayment — destination receives only the remainder after overdraft is cleared",
			requested:       80,
			beforeAvailable: 0,
			afterAvailable:  30,
			expected:        30,
		},
		{
			name:            "full repayment with no remainder — primary movement is 0, full redirect to companion",
			requested:       40,
			beforeAvailable: 0,
			afterAvailable:  0,
			expected:        0,
		},
		{
			name:            "normal credit — no overdraft, movement equals requested",
			requested:       250,
			beforeAvailable: 100,
			afterAvailable:  350,
			expected:        250,
		},
		{
			name:            "direction=debit DEBIT companion op — liability grows, movement equals requested",
			requested:       50,
			beforeAvailable: 0,
			afterAvailable:  50,
			expected:        50,
		},
		{
			name:            "direction=debit CREDIT companion op — liability shrinks, movement equals requested",
			requested:       50,
			beforeAvailable: 50,
			afterAvailable:  0,
			expected:        50,
		},
		{
			name:            "pending entry — amount shifts from Available into OnHold, max-delta preserves full amount",
			requested:       100,
			beforeAvailable: 500,
			afterAvailable:  400,
			beforeOnHold:    0,
			afterOnHold:     100,
			expected:        100,
		},
		{
			name:            "movement exceeds request — clipped to requested amount to guard against ledger bug",
			requested:       100,
			beforeAvailable: 500,
			afterAvailable:  300,
			expected:        100,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			amt := mtransaction.Amount{Value: dec(tc.requested)}

			got := effectiveOperationAmount(
				amt,
				dec(tc.beforeAvailable), dec(tc.afterAvailable),
				dec(tc.beforeOnHold), dec(tc.afterOnHold),
			)

			assert.True(t, got.Equal(dec(tc.expected)),
				"expected effective amount %d, got %s (requested=%d, before=%d, after=%d)",
				tc.expected, got.String(), tc.requested, tc.beforeAvailable, tc.afterAvailable)
		})
	}
}
