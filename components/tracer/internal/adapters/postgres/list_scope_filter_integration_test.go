//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

func TestTransactionValidationRepository_ListConfinedToTheScope_Integration(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	repo := NewTransactionValidationRepositoryWithConnection(&testutil.IntegrationDBAdapter{DB: db})

	accountA, accountB := uuid.New(), uuid.New()
	segment, merchant := uuid.New(), uuid.New()
	at := time.Now().UTC().Add(-time.Hour)

	seed := func(account uuid.UUID, segmentJSON, merchantJSON any) uuid.UUID {
		t.Helper()

		id := uuid.New()
		_, err := db.Exec(`
			INSERT INTO transaction_validations
				(id, request_id, transaction_type, amount, asset, transaction_timestamp,
				 account, segment, merchant, decision, matched_rule_ids, evaluated_rule_ids,
				 processing_time_ms, reason, created_at)
			VALUES ($1,$2,'CARD',10,'USD',$3,$4,$5,$6,'ALLOW','{}','{}',1,'allowed',$3)`,
			id, uuid.New(), at, fmt.Sprintf(`{"accountId":%q,"type":"CHECKING"}`, account.String()), segmentJSON, merchantJSON)
		require.NoError(t, err)

		return id
	}

	a := seed(accountA, fmt.Sprintf(`{"segmentId":%q}`, segment.String()), nil)
	b := seed(accountB, nil, fmt.Sprintf(`{"merchantId":%q}`, merchant.String()))

	tests := []struct {
		name  string
		scope http.ScopeConfinement
		want  []uuid.UUID
	}{
		{name: "allowed account", scope: http.ScopeConfinement{"accountId": {accountA}}, want: []uuid.UUID{a}},
		{name: "allowed segment", scope: http.ScopeConfinement{"segmentId": {segment}}, want: []uuid.UUID{a}},
		{name: "allowed merchant", scope: http.ScopeConfinement{"merchantId": {merchant}}, want: []uuid.UUID{b}},
		{name: "an empty allowed list lists nothing", scope: http.ScopeConfinement{"accountId": {}}, want: []uuid.UUID{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.List(context.Background(), &model.TransactionValidationFilters{
				StartDate: at.Add(-time.Minute), EndDate: at.Add(time.Minute), Limit: 100, Scope: tt.scope,
			})
			require.NoError(t, err)

			got := make([]uuid.UUID, 0, len(result.TransactionValidations))
			for _, v := range result.TransactionValidations {
				got = append(got, v.ID)
			}

			assert.ElementsMatch(t, tt.want, got)
		})
	}
}

func TestRuleAndLimitRepositories_ListConfinedToTheScope_Integration(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	ctx := context.Background()
	prefix := "scope-list-" + uuid.NewString()[:8]
	now := time.Now().UTC()

	rules := NewRepositoryWithConnection(&testutil.IntegrationDBAdapter{DB: db})
	limits := NewLimitRepositoryWithConnection(&testutil.IntegrationDBAdapter{DB: db})

	newRule := func(name string) uuid.UUID {
		t.Helper()

		rule, err := model.NewRule(prefix+name, "amount > 0", model.DecisionDeny, nil, nil, now)
		require.NoError(t, err)

		created, err := rules.CreateWithTx(ctx, db, rule)
		require.NoError(t, err)

		t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM rules WHERE id = $1`, created.ID) })

		return created.ID
	}

	newLimit := func(name string) uuid.UUID {
		t.Helper()

		account := uuid.New()
		lmt, err := model.NewLimit(prefix+name, model.LimitTypeDaily, decimal.RequireFromString("1000"), "BRL",
			[]model.Scope{{AccountID: &account}}, nil, now)
		require.NoError(t, err)
		require.NoError(t, limits.CreateWithTx(ctx, db, lmt))

		t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM limits WHERE id = $1`, lmt.ID) })

		return lmt.ID
	}

	ruleA, ruleB := newRule("-a"), newRule("-b")
	limitA, limitB := newLimit("-a"), newLimit("-b")

	name := prefix

	tests := []struct {
		name       string
		scope      http.ScopeConfinement
		wantRules  []uuid.UUID
		wantLimits []uuid.UUID
	}{
		{name: "no confinement lists every one", wantRules: []uuid.UUID{ruleA, ruleB}, wantLimits: []uuid.UUID{limitA, limitB}},
		{name: "the allowed ones only", scope: http.ScopeConfinement{"ruleId": {ruleA}, "limitId": {limitB}}, wantRules: []uuid.UUID{ruleA}, wantLimits: []uuid.UUID{limitB}},
		{name: "an empty allowed list lists nothing", scope: http.ScopeConfinement{"ruleId": {}, "limitId": {}}, wantRules: []uuid.UUID{}, wantLimits: []uuid.UUID{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ruleScope, limitScope http.ScopeConfinement
			if tt.scope != nil {
				ruleScope = http.ScopeConfinement{"ruleId": tt.scope["ruleId"]}
				limitScope = http.ScopeConfinement{"limitId": tt.scope["limitId"]}
			}

			gotRules, err := rules.List(ctx, &model.ListRulesFilter{Name: &name, Limit: 100, SortBy: "created_at", SortOrder: "DESC", Scope: ruleScope})
			require.NoError(t, err)

			ruleIDs := make([]uuid.UUID, 0, len(gotRules.Rules))
			for _, r := range gotRules.Rules {
				ruleIDs = append(ruleIDs, r.ID)
			}

			assert.ElementsMatch(t, tt.wantRules, ruleIDs)

			gotLimits, err := limits.List(ctx, &model.ListLimitsFilter{Name: &name, Limit: 100, Scope: limitScope})
			require.NoError(t, err)

			limitIDs := make([]uuid.UUID, 0, len(gotLimits.Limits))
			for _, l := range gotLimits.Limits {
				limitIDs = append(limitIDs, l.ID)
			}

			assert.ElementsMatch(t, tt.wantLimits, limitIDs)
		})
	}

	t.Run("a confinement on a dimension the list does not carry lists nothing", func(t *testing.T) {
		got, err := rules.List(ctx, &model.ListRulesFilter{Name: &name, Limit: 100, SortBy: "created_at", SortOrder: "DESC",
			Scope: http.ScopeConfinement{"limitId": {limitA}}})
		require.NoError(t, err)
		assert.Empty(t, got.Rules)
	})
}
