//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package account

import (
	"context"
	"slices"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

func TestIntegration_AccountRepository_ListHolderIDsInScope(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")
	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledger1 := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	ledger2 := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	holderA := uuid.Must(libCommons.GenerateUUIDv7())
	holderB := uuid.Must(libCommons.GenerateUUIDv7())
	holderGone := uuid.Must(libCommons.GenerateUUIDv7())

	owned := func(ledgerID uuid.UUID, alias string, holder uuid.UUID, deleted *time.Time) uuid.UUID {
		p := pgtestutil.DefaultAccountParams()
		p.Alias = alias
		p.HolderID = &holder
		p.DeletedAt = deleted

		return pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, p)
	}

	gone := time.Now()
	a1 := owned(ledger1, "@a1", holderA, nil)
	owned(ledger1, "@a2", holderA, nil)
	b2 := owned(ledger2, "@b2", holderB, nil)
	owned(ledger2, "@gone", holderGone, &gone)
	pgtestutil.CreateTestAccount(t, container.DB, orgID, ledger1, nil, "No holder", "@no-holder", "USD", nil)

	ctx := context.Background()

	tests := []struct {
		name  string
		scope http.ScopeConfinement
		want  []uuid.UUID
	}{
		{name: "every holder of a live account", want: []uuid.UUID{holderA, holderB}},
		{name: "holders with an account in the allowed ledgers", scope: http.ScopeConfinement{"ledgerId": {ledger2}}, want: []uuid.UUID{holderB}},
		{name: "holders of the allowed accounts", scope: http.ScopeConfinement{"accountId": {a1, b2}}, want: []uuid.UUID{holderA, holderB}},
		{name: "both dimensions intersect", scope: http.ScopeConfinement{"accountId": {a1, b2}, "ledgerId": {ledger1}}, want: []uuid.UUID{holderA}},
		{name: "an empty allowed list lists nothing", scope: http.ScopeConfinement{"ledgerId": {}}, want: []uuid.UUID{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.ListHolderIDs(ctx, orgID, tt.scope)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}

func TestIntegration_AccountRepository_ListLedgerIDsOfHolders(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")
	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	otherOrg := pgtestutil.CreateTestOrganization(t, container.DB)
	ledger1 := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	ledger2 := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	foreignLedger := pgtestutil.CreateTestLedger(t, container.DB, otherOrg)

	holderBoth := uuid.Must(libCommons.GenerateUUIDv7())
	holderOne := uuid.Must(libCommons.GenerateUUIDv7())
	holderGone := uuid.Must(libCommons.GenerateUUIDv7())
	holderNone := uuid.Must(libCommons.GenerateUUIDv7())

	owned := func(org, ledgerID uuid.UUID, alias string, holder uuid.UUID, deleted *time.Time) {
		p := pgtestutil.DefaultAccountParams()
		p.Alias = alias
		p.HolderID = &holder
		p.DeletedAt = deleted

		pgtestutil.CreateTestAccountWithParams(t, container.DB, org, ledgerID, p)
	}

	gone := time.Now()
	owned(orgID, ledger1, "@both-1", holderBoth, nil)
	owned(orgID, ledger1, "@both-1b", holderBoth, nil)
	owned(orgID, ledger2, "@both-2", holderBoth, nil)
	owned(orgID, ledger2, "@one", holderOne, nil)
	owned(orgID, ledger1, "@one-gone", holderOne, &gone)
	owned(orgID, ledger1, "@gone", holderGone, &gone)
	owned(otherOrg, foreignLedger, "@foreign", holderOne, nil)

	got, err := repo.ListLedgerIDsOfHolders(context.Background(), orgID, []uuid.UUID{holderBoth, holderOne, holderGone, holderNone})
	require.NoError(t, err)

	assert.ElementsMatch(t, []uuid.UUID{ledger1, ledger2}, got[holderBoth], "every ledger the holder has a live account in, each once")
	assert.Equal(t, []uuid.UUID{ledger2}, got[holderOne], "a deleted account and another organization's account add no ledger")
	assert.NotContains(t, got, holderGone, "a holder whose accounts are all deleted has no ledger")
	assert.NotContains(t, got, holderNone, "a holder without accounts has no ledger")

	inScope, err := repo.ListHolderIDs(context.Background(), orgID, http.ScopeConfinement{"ledgerId": {ledger2}})
	require.NoError(t, err)

	for _, holder := range []uuid.UUID{holderBoth, holderOne, holderGone, holderNone} {
		assert.Equal(t, slices.Contains(inScope, holder), slices.Contains(got[holder], ledger2),
			"holder %s: resolving its ledgers must agree with the list's inclusion rule", holder)
	}

	empty, err := repo.ListLedgerIDsOfHolders(context.Background(), orgID, nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestIntegration_AccountRepository_FindAllByHolderConfinedToTheScope(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")
	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledger1 := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	ledger2 := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	holder := uuid.Must(libCommons.GenerateUUIDv7())

	owned := func(ledgerID uuid.UUID, alias string) uuid.UUID {
		p := pgtestutil.DefaultAccountParams()
		p.Alias = alias
		p.HolderID = &holder

		return pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, p)
	}

	in1, in2 := owned(ledger1, "@in-1"), owned(ledger2, "@in-2")

	list := func(scope http.ScopeConfinement) []uuid.UUID {
		t.Helper()

		filter := holderListFilter(10, 1, "asc")
		filter.Scope = scope

		accounts, err := repo.FindAllByHolder(context.Background(), orgID, holder, nil, filter, mmodel.HolderOnV2)
		require.NoError(t, err)

		ids := make([]uuid.UUID, 0, len(accounts))
		for _, acc := range accounts {
			ids = append(ids, uuid.MustParse(acc.ID))
		}

		return ids
	}

	assert.ElementsMatch(t, []uuid.UUID{in1, in2}, list(nil), "no confinement lists the holder's accounts in every ledger")
	assert.ElementsMatch(t, []uuid.UUID{in2}, list(http.ScopeConfinement{"ledgerId": {ledger2}}), "only the accounts of the allowed ledgers")
	assert.Empty(t, list(http.ScopeConfinement{"ledgerId": {}}), "an empty allowed list lists nothing")
}
