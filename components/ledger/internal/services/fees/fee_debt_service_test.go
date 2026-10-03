// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"

	libPointers "github.com/LerianStudio/lib-commons/v7/commons/pointers"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/fee_debt"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

type debtorAccountsFake struct {
	accounts []*mmodel.Account
	err      error
	asked    []uuid.UUID
}

func (f *debtorAccountsFake) ListAccountsByIDs(_ context.Context, _, _ uuid.UUID, ids []uuid.UUID) ([]*mmodel.Account, error) {
	f.asked = append(f.asked, ids...)

	return f.accounts, f.err
}

func TestFeeDebtService_DebtorConfinement(t *testing.T) {
	org, ledger := uuid.New(), uuid.New()
	alice, bob := uuid.New(), uuid.New()

	t.Run("no confinement leaves the query as it was", func(t *testing.T) {
		svc := &FeeDebtService{}

		query, listsNothing, err := svc.confineDebtors(context.Background(), org, ledger, fee_debt.ListQuery{Limit: 5}, nil)
		require.NoError(t, err)
		assert.False(t, listsNothing)
		assert.Equal(t, fee_debt.ListQuery{Limit: 5}, query)
	})

	t.Run("the allowed accounts confine to their aliases", func(t *testing.T) {
		accounts := &debtorAccountsFake{accounts: []*mmodel.Account{
			{ID: alice.String(), Alias: libPointers.String("@alice")},
			{ID: bob.String(), Alias: libPointers.String("@bob")},
		}}
		svc := &FeeDebtService{Accounts: accounts}

		query, listsNothing, err := svc.confineDebtors(context.Background(), org, ledger, fee_debt.ListQuery{Limit: 5},
			http.ScopeConfinement{"accountId": {alice, bob}})
		require.NoError(t, err)
		assert.False(t, listsNothing)
		assert.True(t, query.ConfineDebtors)
		assert.ElementsMatch(t, []string{"@alice", "@bob"}, query.DebtorAliases)
		assert.ElementsMatch(t, []uuid.UUID{alice, bob}, accounts.asked)
	})

	t.Run("an empty allowed list, or allowed accounts that no longer exist, list nothing", func(t *testing.T) {
		svc := &FeeDebtService{Accounts: &debtorAccountsFake{}}

		for _, scope := range []http.ScopeConfinement{{"accountId": {}}, {"accountId": {alice}}} {
			_, listsNothing, err := svc.confineDebtors(context.Background(), org, ledger, fee_debt.ListQuery{}, scope)
			require.NoError(t, err)
			assert.True(t, listsNothing)
		}
	})

	t.Run("a confinement it cannot read is an error, never an unconfined list", func(t *testing.T) {
		boom := errors.New("replica down")
		svc := &FeeDebtService{Accounts: &debtorAccountsFake{err: boom}}

		_, _, err := svc.confineDebtors(context.Background(), org, ledger, fee_debt.ListQuery{}, http.ScopeConfinement{"accountId": {alice}})
		require.ErrorIs(t, err, boom)

		_, _, err = (&FeeDebtService{}).confineDebtors(context.Background(), org, ledger, fee_debt.ListQuery{}, http.ScopeConfinement{"accountId": {alice}})
		require.Error(t, err, "without an account reader the confinement cannot be applied")
	})

	t.Run("a dimension the listing cannot confine on lists nothing", func(t *testing.T) {
		svc := &FeeDebtService{Accounts: &debtorAccountsFake{accounts: []*mmodel.Account{{ID: alice.String(), Alias: libPointers.String("@alice")}}}}

		for _, scope := range []http.ScopeConfinement{{"portfolioId": {alice}}, {"accountId": {alice}, "portfolioId": {alice}}} {
			_, listsNothing, err := svc.confineDebtors(context.Background(), org, ledger, fee_debt.ListQuery{}, scope)
			require.NoError(t, err)
			assert.True(t, listsNothing)
		}
	})

	t.Run("a listing that lists nothing never reads the record", func(t *testing.T) {
		svc := &FeeDebtService{}

		debts, _, err := svc.ListFeeDebts(context.Background(), org, ledger, fee_debt.ListQuery{Limit: 5}, http.ScopeConfinement{"accountId": {}})
		require.NoError(t, err)
		assert.Empty(t, debts)
	})
}
