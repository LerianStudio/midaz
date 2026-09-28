// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouteJSON_LedgerIDPresentOnlyWhenTheRouteHasALedger(t *testing.T) {
	ledgerID := uuid.MustParse("01965ed9-7fa4-75b2-8872-fc9e8509ab0a")

	cases := []struct {
		name  string
		route any
	}{
		{name: "transaction route with ledger", route: TransactionRoute{ID: uuid.New(), LedgerID: &ledgerID}},
		{name: "operation route with ledger", route: OperationRoute{ID: uuid.New(), LedgerID: &ledgerID}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fields := marshalToFields(t, tc.route)

			require.Contains(t, fields, "ledgerId")
			assert.Equal(t, ledgerID.String(), fields["ledgerId"])
		})
	}

	withoutLedger := []struct {
		name  string
		route any
	}{
		{name: "transaction route without ledger", route: TransactionRoute{ID: uuid.New()}},
		{name: "operation route without ledger", route: OperationRoute{ID: uuid.New()}},
	}

	for _, tc := range withoutLedger {
		t.Run(tc.name, func(t *testing.T) {
			fields := marshalToFields(t, tc.route)

			assert.NotContains(t, fields, "ledgerId")
		})
	}
}

func marshalToFields(t *testing.T, v any) map[string]any {
	t.Helper()

	raw, err := json.Marshal(v)
	require.NoError(t, err)

	var fields map[string]any
	require.NoError(t, json.Unmarshal(raw, &fields))

	return fields
}
