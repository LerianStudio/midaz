//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package account

import (
	"context"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
