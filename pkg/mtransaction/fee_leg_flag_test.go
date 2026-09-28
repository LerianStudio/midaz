// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFeeLegFlagIsUnreachableFromAPayload pins the two independent reasons a payment payload
// cannot claim the ledger's fee mark for a movement the caller authored. The mark is written from
// Amount.FeeLeg, so a caller able to set that field would label its own money a ledger fee on the
// confirmation screen, which is the false positive the mark exists to remove.
//
// Barrier one: the field is off the wire, so a body naming it decodes to false.
// Barrier two: the amounts map the fee engine reads is built from named fields, so even a caller
// leg that somehow carried the flag would not carry it into the map.
func TestFeeLegFlagIsUnreachableFromAPayload(t *testing.T) {
	t.Parallel()

	t.Run("a body naming the flag decodes to false", func(t *testing.T) {
		t.Parallel()

		var amount Amount

		require.NoError(t, json.Unmarshal([]byte(`{"asset":"BRL","value":"100","feeLeg":true,"FeeLeg":true}`), &amount))
		assert.Equal(t, "BRL", amount.Asset, "the ordinary fields must still decode")
		assert.False(t, amount.FeeLeg, "a payment payload must not be able to set the ledger fee flag")
	})

	t.Run("a caller leg carrying the flag does not carry it into the amounts map", func(t *testing.T) {
		t.Parallel()

		value := decimal.NewFromInt(100)
		legs := []FromTo{{
			AccountAlias: "@payer",
			IsFrom:       true,
			Amount:       &Amount{Asset: "BRL", Value: value, FeeLeg: true},
		}}

		total := make(chan decimal.Decimal, 1)
		amounts := make(chan map[string]Amount, 1)
		aliases := make(chan []string, 1)
		routes := make(chan map[string]string, 1)

		CalculateTotal(legs, Transaction{Send: Send{Asset: "BRL", Value: value}}, "", total, amounts, aliases, routes)

		<-total
		built := <-amounts
		<-aliases
		<-routes

		entry, ok := built["@payer"]
		require.True(t, ok, "the caller's own movement must reach the amounts map")
		assert.False(t, entry.FeeLeg, "only the fee engine may set the flag on an entry of the amounts map")
	})
}
