// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// TestBalanceColumnList_CarriesAccountBlocked pins the column to the position
// every row scan in this adapter reads it from. balanceColumnList drives both
// the SELECT projections and the INSERT/UPDATE RETURNING clauses, so its order
// IS the scan order: a column added anywhere but next to its sibling allow
// flags would shift every field after it.
func TestBalanceColumnList_CarriesAccountBlocked(t *testing.T) {
	t.Parallel()

	idx := -1

	for i, column := range balanceColumnList {
		if column == "account_blocked" {
			idx = i

			break
		}
	}

	require.NotEqual(t, -1, idx, "balanceColumnList must project account_blocked")
	require.Greater(t, idx, 0)
	assert.Equal(t, "allow_receiving", balanceColumnList[idx-1],
		"account_blocked must sit immediately after allow_receiving, matching every row scan")
}

// TestBalancePostgreSQLModel_AccountBlockedRoundTrip locks the entity/model
// translation in both directions. The flag is a read-model projection of the
// owning account, so it has to survive FromEntity -> ToEntity untouched.
func TestBalancePostgreSQLModel_AccountBlockedRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		blocked bool
	}{
		{name: "blocked account", blocked: true},
		{name: "unblocked account", blocked: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			entity := &mmodel.Balance{
				ID:             "01930000-0000-7000-8000-000000000001",
				OrganizationID: "01930000-0000-7000-8000-000000000002",
				LedgerID:       "01930000-0000-7000-8000-000000000003",
				AccountID:      "01930000-0000-7000-8000-000000000004",
				Alias:          "@blocked",
				Key:            "default",
				AssetCode:      "USD",
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
				AccountBlocked: tc.blocked,
			}

			var model BalancePostgreSQLModel
			model.FromEntity(entity)

			assert.Equal(t, tc.blocked, model.AccountBlocked,
				"FromEntity must carry the account block projection into the SQL model")

			got := model.ToEntity()
			require.NotNil(t, got)
			assert.Equal(t, tc.blocked, got.AccountBlocked,
				"ToEntity must carry the account block projection back to the domain model")
		})
	}
}
