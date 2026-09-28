// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

func TestIntegrationLimitRepositoryStoresAndFiltersNativeAssetCode(t *testing.T) {
	db := completionDatabase(t)
	repo := NewLimitRepositoryWithConnection(&testutil.IntegrationDBAdapter{DB: db})
	account := testutil.MustDeterministicUUID(89601)
	at := testutil.FixedTime()
	limit := &model.Limit{
		ID: testutil.MustDeterministicUUID(89602), Name: "points daily cap", LimitType: model.LimitTypeDaily,
		MaxAmount: decimal.RequireFromString("1000"), Asset: "LERIANPOINTS", Scopes: []model.Scope{{AccountID: &account}},
		Status: model.LimitStatusDraft, CreatedAt: at, UpdatedAt: at,
	}
	require.NoError(t, repo.CreateWithTx(t.Context(), db, limit))

	stored, err := repo.GetByID(t.Context(), limit.ID)
	require.NoError(t, err)
	require.Equal(t, "LERIANPOINTS", stored.Asset)

	for _, tc := range []struct {
		asset string
		found bool
	}{{"LERIANPOINTS", true}, {"BTC", false}} {
		t.Run(tc.asset, func(t *testing.T) {
			asset := tc.asset
			result, err := repo.List(t.Context(), &model.ListLimitsFilter{Asset: &asset, Limit: 10})
			require.NoError(t, err)
			if !tc.found {
				require.Empty(t, result.Limits)
				return
			}
			require.Len(t, result.Limits, 1)
			require.Equal(t, limit.ID, result.Limits[0].ID)
			require.Equal(t, "LERIANPOINTS", result.Limits[0].Asset)
		})
	}
}

func TestIntegrationTransactionValidationRepositoryStoresNativeAssetCode(t *testing.T) {
	db := completionDatabase(t)
	repo := NewTransactionValidationRepositoryWithConnection(&testutil.IntegrationDBAdapter{DB: db})
	at := testutil.FixedTime()
	validation := &model.TransactionValidation{
		ID: testutil.MustDeterministicUUID(89611), RequestID: testutil.MustDeterministicUUID(89612),
		TransactionType: model.TransactionTypeCard, Amount: decimal.RequireFromString("25.50"), Asset: "LERIANPOINTS",
		TransactionTimestamp: at, Account: model.AccountContext{ID: testutil.MustDeterministicUUID(89613), Type: "checking", Status: "active"},
		Decision: model.DecisionAllow, Reason: "no limit applies", CreatedAt: at,
	}
	require.NoError(t, repo.Insert(t.Context(), validation))

	stored, err := repo.GetByID(t.Context(), validation.ID)
	require.NoError(t, err)
	require.Equal(t, "LERIANPOINTS", stored.Asset)
}
