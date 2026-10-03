//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"context"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

func TestIntegration_TransactionListAndCountConfinedToTheScope(t *testing.T) {
	infra := setupIntegrationInfra(t)
	ctx := context.Background()

	a := uuid.Must(libCommons.GenerateUUIDv7())
	b := uuid.Must(libCommons.GenerateUUIDv7())
	c := uuid.Must(libCommons.GenerateUUIDv7())
	dropped := uuid.Must(libCommons.GenerateUUIDv7())

	ab := pgtestutil.CreateTestTransactionWithStatus(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, constant.APPROVED, decimal.NewFromInt(10), "USD")
	accountRefsLeg(t, infra, ab, a, constant.DEBIT)
	accountRefsLeg(t, infra, ab, b, constant.CREDIT)

	bc := pgtestutil.CreateTestTransactionWithStatus(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, constant.APPROVED, decimal.NewFromInt(10), "USD")
	accountRefsLeg(t, infra, bc, b, constant.DEBIT)
	accountRefsLeg(t, infra, bc, c, constant.CREDIT)

	deletedLeg := pgtestutil.CreateTestTransactionWithStatus(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, constant.APPROVED, decimal.NewFromInt(10), "USD")
	op := accountRefsLeg(t, infra, deletedLeg, dropped, constant.DEBIT)
	_, err := infra.pgContainer.DB.Exec(`UPDATE operation SET deleted_at = now() WHERE id = $1`, op)
	require.NoError(t, err)

	start, end := time.Now().Add(-24*time.Hour), time.Now().Add(24*time.Hour)

	tests := []struct {
		name  string
		scope http.ScopeConfinement
		want  []uuid.UUID
	}{
		{name: "no confinement lists every transaction", want: []uuid.UUID{ab, bc, deletedLeg}},
		{name: "a transaction with any leg in the allowed accounts", scope: http.ScopeConfinement{"accountId": {a}}, want: []uuid.UUID{ab}},
		{name: "an account on both", scope: http.ScopeConfinement{"accountId": {b}}, want: []uuid.UUID{ab, bc}},
		{name: "a deleted leg names no account", scope: http.ScopeConfinement{"accountId": {dropped}}, want: []uuid.UUID{}},
		{name: "an empty allowed list lists nothing", scope: http.ScopeConfinement{"accountId": {}}, want: []uuid.UUID{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trans, _, err := infra.repo.FindOrListAllWithOperations(ctx, infra.orgID, infra.ledgerID, nil,
				http.Pagination{Limit: 100, SortOrder: "asc", StartDate: start, EndDate: end, Scope: tt.scope})
			require.NoError(t, err)

			got := make([]uuid.UUID, 0, len(trans))
			for _, tr := range trans {
				got = append(got, uuid.MustParse(tr.ID))
			}

			assert.ElementsMatch(t, tt.want, got)

			count, err := infra.repo.CountByFilters(ctx, infra.orgID, infra.ledgerID, CountFilter{StartDate: start, EndDate: end, Scope: tt.scope})
			require.NoError(t, err)
			assert.Equal(t, int64(len(tt.want)), count, "the count must count the same set the list lists")
		})
	}
}

func TestIntegration_TransactionListShowsAPendingHoldToItsDestination(t *testing.T) {
	infra := setupIntegrationInfra(t)
	ctx := context.Background()

	source := uuid.Must(libCommons.GenerateUUIDv7())
	destination := uuid.Must(libCommons.GenerateUUIDv7())

	legs := `{"send":{"asset":"USD","value":"10","source":{"from":[{"accountAlias":"@source"}]},"distribute":{"to":[{"accountAlias":"0#@destination#default"}]}}}`

	pending := pgtestutil.CreateTestTransactionWithStatus(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, constant.PENDING, decimal.NewFromInt(10), "USD")
	accountRefsLeg(t, infra, pending, source, constant.ONHOLD)

	settled := pgtestutil.CreateTestTransactionWithStatus(t, infra.pgContainer.DB, infra.orgID, infra.ledgerID, constant.APPROVED, decimal.NewFromInt(10), "USD")
	accountRefsLeg(t, infra, settled, source, constant.DEBIT)

	for _, id := range []uuid.UUID{pending, settled} {
		_, err := infra.pgContainer.DB.Exec(`UPDATE "transaction" SET body = $1 WHERE id = $2`, legs, id)
		require.NoError(t, err)
	}

	start, end := time.Now().Add(-24*time.Hour), time.Now().Add(24*time.Hour)

	tests := []struct {
		name    string
		scope   http.ScopeConfinement
		aliases []string
		want    []uuid.UUID
	}{
		{name: "the destination of a pending hold sees it", scope: http.ScopeConfinement{"accountId": {destination}}, aliases: []string{"@destination"}, want: []uuid.UUID{pending}},
		{name: "a body leg of a settled transaction is not a confinement by itself", scope: http.ScopeConfinement{"accountId": {destination}}, aliases: []string{"@destination"}, want: []uuid.UUID{pending}},
		{name: "an alias is matched whole", scope: http.ScopeConfinement{"accountId": {destination}}, aliases: []string{"@dest"}, want: []uuid.UUID{}},
		{name: "the source sees both", scope: http.ScopeConfinement{"accountId": {source}}, aliases: []string{"@source"}, want: []uuid.UUID{pending, settled}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trans, _, err := infra.repo.FindOrListAllWithOperations(ctx, infra.orgID, infra.ledgerID, nil,
				http.Pagination{Limit: 100, SortOrder: "asc", StartDate: start, EndDate: end, Scope: tt.scope, ScopeAccountAliases: tt.aliases})
			require.NoError(t, err)

			got := make([]uuid.UUID, 0, len(trans))
			for _, tr := range trans {
				got = append(got, uuid.MustParse(tr.ID))
			}

			assert.ElementsMatch(t, tt.want, got)

			count, err := infra.repo.CountByFilters(ctx, infra.orgID, infra.ledgerID, CountFilter{StartDate: start, EndDate: end, Scope: tt.scope, ScopeAccountAliases: tt.aliases})
			require.NoError(t, err)
			assert.Equal(t, int64(len(tt.want)), count)
		})
	}
}
