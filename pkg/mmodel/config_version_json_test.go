// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The configuration versions must never reach JSON: Balance is also serialized
// into formats that the previous release decodes with unknown fields refused, and
// neither version belongs to the API contract. Changing only the version must
// therefore leave the encoded bytes identical.
func TestConfigVersion_NeverChangesTheJSONEncoding(t *testing.T) {
	t.Parallel()

	blocked := true
	created := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

	balance := Balance{
		ID:             "00000000-0000-0000-0000-000000000001",
		Alias:          "@a",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(10),
		AllowSending:   true,
		AllowReceiving: true,
		Settings:       NewDefaultBalanceSettings(),
		CreatedAt:      created,
	}
	versionedBalance := balance
	versionedBalance.ConfigVersion = 42

	account := Account{ID: "00000000-0000-0000-0000-000000000002", Name: "a", Blocked: &blocked, CreatedAt: created}
	versionedAccount := account
	versionedAccount.ConfigVersion = 42

	for name, pair := range map[string][2]any{
		"balance": {balance, versionedBalance},
		"account": {account, versionedAccount},
	} {
		plain, err := json.Marshal(pair[0])
		require.NoError(t, err, name)

		versioned, err := json.Marshal(pair[1])
		require.NoError(t, err, name)

		assert.JSONEq(t, string(plain), string(versioned), name)
		assert.NotContains(t, string(versioned), "42", name)
	}
}
