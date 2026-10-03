//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package account

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

func scopeListFilter(scope http.ScopeConfinement) http.QueryHeader {
	return http.QueryHeader{
		Limit:     100,
		Page:      1,
		SortOrder: "asc",
		StartDate: time.Now().Add(-24 * time.Hour),
		EndDate:   time.Now().Add(24 * time.Hour),
		Scope:     scope,
	}
}

func accountIDs(accounts []*mmodel.Account) []string {
	ids := make([]string, 0, len(accounts))
	for _, acc := range accounts {
		ids = append(ids, acc.ID)
	}

	return ids
}

func TestIntegration_AccountRepository_ScopeConfinesTheListAndTheCount(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "onboarding")
	repo := createRepository(t, container)

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	portfolioID := pgtestutil.CreateTestPortfolio(t, container.DB, orgID, ledgerID)

	inPortfolio := pgtestutil.DefaultAccountParams()
	inPortfolio.Alias = "@scope-in-portfolio"
	inPortfolio.PortfolioID = &portfolioID

	a := pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "A", "@scope-a", "USD", nil)
	b := pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil, "B", "@scope-b", "USD", nil)
	p := pgtestutil.CreateTestAccountWithParams(t, container.DB, orgID, ledgerID, inPortfolio)

	ctx := context.Background()

	tests := []struct {
		name  string
		scope http.ScopeConfinement
		want  []string
	}{
		{name: "no confinement lists every account", want: []string{a.String(), b.String(), p.String()}},
		{name: "allowed accounts", scope: http.ScopeConfinement{"accountId": {a, p}}, want: []string{a.String(), p.String()}},
		{name: "allowed portfolio", scope: http.ScopeConfinement{"portfolioId": {portfolioID}}, want: []string{p.String()}},
		{name: "two dimensions intersect", scope: http.ScopeConfinement{"accountId": {a, p}, "portfolioId": {portfolioID}}, want: []string{p.String()}},
		{name: "an empty allowed list lists nothing", scope: http.ScopeConfinement{"accountId": {}}, want: []string{}},
		{name: "an allowed account of nothing here lists nothing", scope: http.ScopeConfinement{"accountId": {uuid.New()}}, want: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accounts, err := repo.FindAll(ctx, orgID, ledgerID, nil, nil, scopeListFilter(tt.scope), mmodel.HolderOnV2)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.want, accountIDs(accounts))

			count, err := repo.Count(ctx, orgID, ledgerID, tt.scope)
			require.NoError(t, err)
			assert.Equal(t, int64(len(tt.want)), count, "the count must count the same set the list lists")
		})
	}
}
