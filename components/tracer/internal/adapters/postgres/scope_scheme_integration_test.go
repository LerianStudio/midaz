// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

var scopeSchemeCreatedAt = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

// legacyScopesJSON is a scopes column holding only transactionType, the shape
// stored before scopes carried the scheme key.
func legacyScopesJSON(accountID uuid.UUID, transactionType string) string {
	return fmt.Sprintf(`[{"accountId":%q,"transactionType":%q}]`, accountID.String(), transactionType)
}

// rewriteScopes replaces the stored scopes of a row as an older writer would
// have left them.
func rewriteScopes(t *testing.T, db *sql.DB, table string, id uuid.UUID, scopesJSON string) {
	t.Helper()

	_, err := db.Exec(`UPDATE `+table+` SET scopes = $1::jsonb WHERE id = $2`, scopesJSON, id)
	require.NoError(t, err)
}

func TestIntegration_LimitRepo_ScopeScheme(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	repo := NewLimitRepositoryWithConnection(&testutil.IntegrationDBAdapter{DB: db})
	ctx := context.Background()

	createLimit := func(t *testing.T, name string, accountSeed int64) *model.Limit {
		t.Helper()

		accountID := testutil.MustDeterministicUUID(accountSeed)
		boleto := model.TransactionType("BOLETO")

		lmt, err := model.NewLimit(name, model.LimitTypeDaily, decimal.RequireFromString("1000"), "BRL",
			[]model.Scope{{AccountID: &accountID, TransactionType: &boleto}}, nil, scopeSchemeCreatedAt)
		require.NoError(t, err)
		require.NoError(t, repo.CreateWithTx(ctx, db, lmt))

		t.Cleanup(func() {
			if _, err := db.Exec(`DELETE FROM limits WHERE id = $1`, lmt.ID); err != nil {
				t.Logf("cleanup: delete limit %s: %v", lmt.ID, err)
			}
		})

		return lmt
	}

	t.Run("a free-form scope scheme reads back on both fields", func(t *testing.T) {
		lmt := createLimit(t, "scope-scheme-limit-boleto", 9_028_400)

		got, err := repo.GetByID(ctx, lmt.ID)
		require.NoError(t, err)
		require.Len(t, got.Scopes, 1)
		require.NotNil(t, got.Scopes[0].TransactionType)
		assert.Equal(t, model.TransactionType("BOLETO"), *got.Scopes[0].TransactionType)
		assert.Equal(t, testutil.Ptr("BOLETO"), got.Scopes[0].Scheme)
	})

	t.Run("a stored scope with only transactionType gains its scheme", func(t *testing.T) {
		accountSeed := int64(9_028_410)
		lmt := createLimit(t, "scope-scheme-limit-legacy", accountSeed)
		rewriteScopes(t, db, "limits", lmt.ID, legacyScopesJSON(testutil.MustDeterministicUUID(accountSeed), "BOLETO"))

		got, err := repo.GetByID(ctx, lmt.ID)
		require.NoError(t, err)
		require.Len(t, got.Scopes, 1)
		assert.Equal(t, testutil.Ptr("BOLETO"), got.Scopes[0].Scheme, "GetByID fills the scheme")

		listed := listLimitByName(ctx, t, repo, lmt.Name)
		require.Len(t, listed.Scopes, 1)
		assert.Equal(t, testutil.Ptr("BOLETO"), listed.Scopes[0].Scheme, "List fills the scheme")
	})

	t.Run("list filters scopes by a free-form scheme", func(t *testing.T) {
		lmt := createLimit(t, "scope-scheme-limit-filter", 9_028_420)
		boleto := model.TransactionType("BOLETO")
		name := lmt.Name

		result, err := repo.List(ctx, &model.ListLimitsFilter{
			Name:        &name,
			ScopeFilter: &model.Scope{TransactionType: &boleto},
			Limit:       10,
		})
		require.NoError(t, err)
		require.Len(t, result.Limits, 1)
		assert.Equal(t, lmt.ID, result.Limits[0].ID)
	})
}

func TestIntegration_RuleRepo_ScopeScheme(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	repo := NewRepositoryWithConnection(&testutil.IntegrationDBAdapter{DB: db})
	ctx := context.Background()

	createRule := func(t *testing.T, name string, accountSeed int64, scheme model.TransactionType) *model.Rule {
		t.Helper()

		accountID := testutil.MustDeterministicUUID(accountSeed)

		rule, err := model.NewRule(name, "amount > 1", model.DecisionDeny,
			[]model.Scope{{AccountID: &accountID, TransactionType: &scheme}}, nil, scopeSchemeCreatedAt)
		require.NoError(t, err)

		created, err := repo.CreateWithTx(ctx, db, rule)
		require.NoError(t, err)

		t.Cleanup(func() {
			if _, err := db.Exec(`DELETE FROM rules WHERE id = $1`, created.ID); err != nil {
				t.Logf("cleanup: delete rule %s: %v", created.ID, err)
			}
		})

		return created
	}

	t.Run("list filtered by a free-form scheme returns the rule", func(t *testing.T) {
		boletoRule := createRule(t, "scope-scheme-rule-boleto", 9_028_500, "BOLETO")
		createRule(t, "scope-scheme-rule-pix", 9_028_501, model.TransactionTypePix)

		boleto := model.TransactionType("BOLETO")
		name := "scope-scheme-rule-"

		result, err := repo.List(ctx, &model.ListRulesFilter{
			Name:        &name,
			ScopeFilter: &model.Scope{TransactionType: &boleto},
			Limit:       10,
		})
		require.NoError(t, err)
		require.Len(t, result.Rules, 1)
		assert.Equal(t, boletoRule.ID, result.Rules[0].ID)
		require.Len(t, result.Rules[0].Scopes, 1)
		assert.Equal(t, testutil.Ptr("BOLETO"), result.Rules[0].Scopes[0].Scheme)
	})

	t.Run("a stored scope with only transactionType gains its scheme", func(t *testing.T) {
		accountSeed := int64(9_028_510)
		rule := createRule(t, "scope-scheme-rule-legacy", accountSeed, model.TransactionTypeWire)
		rewriteScopes(t, db, "rules", rule.ID, legacyScopesJSON(testutil.MustDeterministicUUID(accountSeed), "WIRE"))

		got, err := repo.GetByID(ctx, rule.ID)
		require.NoError(t, err)
		require.Len(t, got.Scopes, 1)
		assert.Equal(t, testutil.Ptr("WIRE"), got.Scopes[0].Scheme)
	})
}
