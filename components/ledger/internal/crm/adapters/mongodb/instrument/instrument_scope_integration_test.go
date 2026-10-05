//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package instrument

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
)

func TestIntegration_InstrumentRepo_FindAll_ConfinedToTheScope(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	organizationID := "org-scope-" + uuid.New().String()[:8]
	repo := createRepository(t, container, organizationID)
	ctx := context.Background()

	ledger1, ledger2 := uuid.New(), uuid.New()
	account1, account2, account3 := uuid.New(), uuid.New(), uuid.New()
	holder1, holder2 := uuid.New(), uuid.New()

	seed := func(ledgerID, accountID, holderID uuid.UUID, document string) uuid.UUID {
		params := mongotestutil.DefaultInstrumentParams()
		params.LedgerID = ledgerID.String()
		params.AccountID = accountID.String()
		params.Document = document

		created, err := repo.Create(ctx, organizationID, mongotestutil.CreateTestInstrument(t, holderID, params))
		require.NoError(t, err)

		return *created.ID
	}

	i1 := seed(ledger1, account1, holder1, "22222222201")
	i2 := seed(ledger1, account2, holder2, "22222222202")
	i3 := seed(ledger2, account3, holder2, "22222222203")

	tests := []struct {
		name  string
		scope http.ScopeConfinement
		want  []uuid.UUID
	}{
		{name: "no confinement lists every instrument", want: []uuid.UUID{i1, i2, i3}},
		{name: "allowed ledger", scope: http.ScopeConfinement{"ledgerId": {ledger1}}, want: []uuid.UUID{i1, i2}},
		{name: "allowed accounts", scope: http.ScopeConfinement{"accountId": {account2, account3}}, want: []uuid.UUID{i2, i3}},
		{name: "both dimensions intersect", scope: http.ScopeConfinement{"ledgerId": {ledger1}, "accountId": {account2, account3}}, want: []uuid.UUID{i2}},
		{name: "allowed holders", scope: http.ScopeConfinement{"holderId": {holder2}}, want: []uuid.UUID{i2, i3}},
		{name: "holder and ledger intersect", scope: http.ScopeConfinement{"holderId": {holder2}, "ledgerId": {ledger1}}, want: []uuid.UUID{i2}},
		{name: "an empty allowed list lists nothing", scope: http.ScopeConfinement{"accountId": {}}, want: []uuid.UUID{}},
		{name: "an empty allowed holder list lists nothing", scope: http.ScopeConfinement{"holderId": {}}, want: []uuid.UUID{}},
		{name: "a dimension instruments cannot be confined on lists nothing", scope: http.ScopeConfinement{"portfolioId": {uuid.New()}}, want: []uuid.UUID{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instruments, err := repo.FindAll(ctx, organizationID, uuid.Nil, http.QueryHeader{Limit: 10, Page: 1, Scope: tt.scope}, false)
			require.NoError(t, err)

			got := make([]uuid.UUID, 0, len(instruments))
			for _, i := range instruments {
				got = append(got, *i.ID)
			}

			assert.ElementsMatch(t, tt.want, got)
		})
	}
}

func TestIntegration_InstrumentRepo_LedgerIDsByIDs(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	organizationID := "org-ledgers-" + uuid.New().String()[:8]
	repo := createRepository(t, container, organizationID)
	ctx := context.Background()

	holder, otherHolder := uuid.New(), uuid.New()
	ledger1, ledger2 := uuid.New(), uuid.New()

	seed := func(holderID uuid.UUID, ledgerID, document string) uuid.UUID {
		params := mongotestutil.DefaultInstrumentParams()
		params.LedgerID = ledgerID
		params.AccountID = uuid.NewString()
		params.Document = document

		created, err := repo.Create(ctx, organizationID, mongotestutil.CreateTestInstrument(t, holderID, params))
		require.NoError(t, err)

		return *created.ID
	}

	in1 := seed(holder, ledger1.String(), "33333333301")
	in2 := seed(holder, ledger2.String(), "33333333302")
	deleted := seed(holder, ledger1.String(), "33333333303")
	foreign := seed(otherHolder, ledger2.String(), "33333333304")
	noLedger := seed(holder, "", "33333333305")
	unknown := uuid.New()

	require.NoError(t, repo.Delete(ctx, organizationID, holder, deleted, false))

	got, err := repo.LedgerIDsByIDs(ctx, organizationID, holder, []uuid.UUID{in1, in2, deleted, foreign, noLedger, unknown})
	require.NoError(t, err)

	assert.Equal(t, map[uuid.UUID]string{
		in1:     ledger1.String(),
		in2:     ledger2.String(),
		deleted: ledger1.String(),
	}, got, "every instrument of the holder answers its own ledger, deleted ones too; one naming no ledger, another holder's and an unknown one are absent")

	none, err := repo.LedgerIDsByIDs(ctx, organizationID, holder, nil)
	require.NoError(t, err)
	assert.Empty(t, none)
}
