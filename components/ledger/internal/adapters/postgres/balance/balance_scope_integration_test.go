//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance

import (
	"context"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

func TestIntegration_BalanceRepository_ListAllConfinedToTheScope(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "transaction")
	repo := createRepository(t, container)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	a, b := createTestAccountID(), createTestAccountID()

	params := pgtestutil.DefaultBalanceParams()
	params.Alias = "@scope-a"
	balanceA := pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, a, params)
	params.Alias = "@scope-b"
	balanceB := pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, b, params)

	ctx := context.Background()

	tests := []struct {
		name  string
		scope http.ScopeConfinement
		want  []string
	}{
		{name: "no confinement lists every balance", want: []string{balanceA.String(), balanceB.String()}},
		{name: "allowed account", scope: http.ScopeConfinement{"accountId": {a}}, want: []string{balanceA.String()}},
		{name: "an empty allowed list lists nothing", scope: http.ScopeConfinement{"accountId": {}}, want: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			balances, _, err := repo.ListAll(ctx, orgID, ledgerID, http.Pagination{Limit: 100, SortOrder: "asc", Scope: tt.scope})
			require.NoError(t, err)

			got := make([]string, 0, len(balances))
			for _, b := range balances {
				got = append(got, b.ID)
			}

			assert.ElementsMatch(t, tt.want, got)
		})
	}
}
