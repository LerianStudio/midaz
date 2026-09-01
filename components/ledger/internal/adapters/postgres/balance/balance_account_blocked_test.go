// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance

import (
	"reflect"
	"testing"

	"github.com/google/uuid"
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

// TestRepository_CarriesUpdateAccountBlockedByAccountID pins the additive port
// contract the block/unblock command depends on. The command lives in another
// package and can only reach the projection through this method, so a signature
// drift here is a silent behaviour change one layer up. Asserted by reflection
// rather than by a compile-time var so a wrong parameter ORDER (organization,
// ledger, account) fails with a readable message instead of a type error.
func TestRepository_CarriesUpdateAccountBlockedByAccountID(t *testing.T) {
	t.Parallel()

	repoType := reflect.TypeOf((*Repository)(nil)).Elem()

	method, ok := repoType.MethodByName("UpdateAccountBlockedByAccountID")
	require.True(t, ok, "balance.Repository must expose UpdateAccountBlockedByAccountID")

	fn := method.Type

	require.Equal(t, 5, fn.NumIn(), "expected (ctx, organizationID, ledgerID, accountID, blocked)")
	assert.Equal(t, "context.Context", fn.In(0).String())
	assert.Equal(t, reflect.TypeOf(uuid.UUID{}), fn.In(1), "organizationID must be a uuid.UUID")
	assert.Equal(t, reflect.TypeOf(uuid.UUID{}), fn.In(2), "ledgerID must be a uuid.UUID")
	assert.Equal(t, reflect.TypeOf(uuid.UUID{}), fn.In(3), "accountID must be a uuid.UUID")
	assert.Equal(t, reflect.Bool, fn.In(4).Kind(), "the desired block state must be a plain bool")

	require.Equal(t, 1, fn.NumOut(), "the propagation reports only an error")
	assert.Equal(t, "error", fn.Out(0).String())
}

// TestUpdateAccountBlockedByAccountID_IsImplementedByTheAdapter keeps the
// concrete PostgreSQL adapter and the port from drifting apart, which is what
// would break the bootstrap wiring rather than any test.
func TestUpdateAccountBlockedByAccountID_IsImplementedByTheAdapter(t *testing.T) {
	t.Parallel()

	adapterType := reflect.TypeOf((*BalancePostgreSQLRepository)(nil))
	repoType := reflect.TypeOf((*Repository)(nil)).Elem()

	assert.True(t, adapterType.Implements(repoType),
		"*BalancePostgreSQLRepository must satisfy balance.Repository, including the new propagation method")

	_, ok := adapterType.MethodByName("UpdateAccountBlockedByAccountID")
	assert.True(t, ok, "the PostgreSQL adapter must implement UpdateAccountBlockedByAccountID")
}
