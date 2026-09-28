//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracercontext

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	traceradapter "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

var assetID = uuid.MustParse("550e8400-e29b-41d4-a716-446655440004")

func seedOfficialRecords(t *testing.T, db *sql.DB, code string) {
	t.Helper()
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO organization (id,legal_name,legal_document,address,status,created_at) VALUES ($1,'Test','doc','{}','ACTIVE','2026-01-01T00:00:00Z')`, []any{orgID}},
		{`INSERT INTO ledger (id,organization_id,name,status,created_at) VALUES ($1,$2,'Test','ACTIVE','2026-01-01T00:00:00Z')`, []any{ledgerID, orgID}},
		{`INSERT INTO asset (id,organization_id,ledger_id,type,code,status,created_at) VALUES ($1,$2,$3,'crypto',$4,'ACTIVE','2026-01-01T00:00:00Z')`, []any{assetID, orgID, ledgerID, code}},
		{`INSERT INTO account (id,organization_id,ledger_id,asset_code,status,alias,type,blocked,created_at) VALUES ($1,$2,$3,$4,'ACTIVE','@test','deposit',false,'2026-01-01T00:00:00Z')`, []any{accountID, orgID, ledgerID, code}},
	}
	for _, statement := range statements {
		_, err := db.ExecContext(t.Context(), statement.query, statement.args...)
		require.NoError(t, err)
	}
}

func officialEntries(code string) []traceradapter.PreparedEntry {
	return []traceradapter.PreparedEntry{
		{AccountID: accountID, Direction: tracercontract.Debit, Amount: decimal.RequireFromString("10.125"), AssetCode: code},
		{External: true, Direction: tracercontract.Credit, Amount: decimal.RequireFromString("10.125"), AssetCode: code},
	}
}

func TestOfficialRecordsPrimaryAndTenants(t *testing.T) {
	primary := pgtestutil.SetupMigratedContainer(t, "onboarding")
	other := pgtestutil.SetupMigratedContainer(t, "onboarding")
	seedOfficialRecords(t, primary.DB, "BTC")
	seedOfficialRecords(t, other.DB, "POINTS")
	// A replica deliberately contains different facts for the same IDs.
	resolver := dbresolver.New(dbresolver.WithPrimaryDBs(primary.DB), dbresolver.WithReplicaDBs(other.DB))
	ctx := tmcore.ContextWithPG(t.Context(), resolver)
	repo, err := NewRepository(nil, testBounds(), true)
	require.NoError(t, err)
	loader, err := traceradapter.NewOfficialContextLoader(repo, testBounds())
	require.NoError(t, err)
	projected, err := loader.EvaluationContext(ctx, orgID, ledgerID, officialEntries("BTC"))
	require.NoError(t, err)
	require.Len(t, projected.Accounts, 1)
	require.Len(t, projected.Entries, 2)
	require.Equal(t, "BTC", projected.Accounts[0].Asset)
	require.Equal(t, tracercontract.Amount("10.125"), projected.Entries[0].Amount)
	require.Equal(t, "BTC", projected.Entries[1].Asset)
	otherResolver := dbresolver.New(dbresolver.WithPrimaryDBs(other.DB), dbresolver.WithReplicaDBs(primary.DB))
	otherCtx := tmcore.ContextWithPG(t.Context(), otherResolver, constant.ModuleOnboarding)
	projected, err = loader.EvaluationContext(otherCtx, orgID, ledgerID, officialEntries("POINTS"))
	require.NoError(t, err)
	require.Equal(t, "POINTS", projected.Accounts[0].Asset)
	_, err = loader.EvaluationContext(otherCtx, orgID, ledgerID, officialEntries("BTC"))
	require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
	for _, scope := range [][2]uuid.UUID{{ledgerID, ledgerID}, {orgID, orgID}} {
		_, err := repo.Read(ctx, scope[0], scope[1], []uuid.UUID{accountID})
		require.ErrorIs(t, err, constant.ErrTracerFactsUnavailable)
	}
	_, err = repo.Read(context.Background(), orgID, ledgerID, []uuid.UUID{accountID})
	require.Error(t, err)

	t.Run("missing and deleted accounts", func(t *testing.T) {
		_, err := repo.Read(ctx, orgID, ledgerID, []uuid.UUID{accountID, assetID})
		require.ErrorIs(t, err, constant.ErrTracerFactsUnavailable)
		_, err = primary.DB.ExecContext(t.Context(), `UPDATE account SET deleted_at='2026-01-02T00:00:00Z' WHERE id=$1`, accountID)
		require.NoError(t, err)
		_, err = repo.Read(ctx, orgID, ledgerID, []uuid.UUID{accountID})
		require.ErrorIs(t, err, constant.ErrTracerFactsUnavailable)
		_, err = primary.DB.ExecContext(t.Context(), `UPDATE account SET deleted_at=NULL WHERE id=$1`, accountID)
		require.NoError(t, err)
	})
	t.Run("snapshot across account reads", func(t *testing.T) {
		tx, err := resolver.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		accounts, err := repo.readAccounts(ctx, tx, orgID, ledgerID, []uuid.UUID{accountID})
		require.NoError(t, err)
		require.Len(t, accounts, 1)
		_, err = primary.DB.ExecContext(t.Context(), `UPDATE account SET blocked=true WHERE id=$1`, accountID)
		require.NoError(t, err)
		accounts, err = repo.readAccounts(ctx, tx, orgID, ledgerID, []uuid.UUID{accountID})
		require.NoError(t, err)
		require.False(t, *accounts[0].Blocked)
		require.NoError(t, tx.Commit())
		accounts, err = repo.Read(ctx, orgID, ledgerID, []uuid.UUID{accountID})
		require.NoError(t, err)
		require.True(t, *accounts[0].Blocked)
	})
}

func TestOfficialRecordsBoundAssetCodeByCharacters(t *testing.T) {
	primary := pgtestutil.SetupMigratedContainer(t, "onboarding")
	seedOfficialRecords(t, primary.DB, "usdt")
	ctx := tmcore.ContextWithPG(t.Context(), dbresolver.New(dbresolver.WithPrimaryDBs(primary.DB)))
	// The generic text bound is smaller than a 100-character multi-byte code.
	bounds := testBounds()
	bounds.MaxTextBytes = 64
	repo, err := NewRepository(nil, bounds, true)
	require.NoError(t, err)
	loader, err := traceradapter.NewOfficialContextLoader(repo, bounds)
	require.NoError(t, err)

	for _, tc := range []struct {
		name  string
		code  string
		valid bool
	}{
		{"legacy lowercase stored code", "usdt", true},
		{"legacy mixed stored code", "USDC2", true},
		{"hundred multi-byte characters", strings.Repeat("É", utils.MaxAssetCodeLength), true},
		{"over hundred characters", strings.Repeat("A", utils.MaxAssetCodeLength+1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := primary.DB.ExecContext(t.Context(), `UPDATE account SET asset_code=$1 WHERE id=$2`, tc.code, accountID)
			require.NoError(t, err)

			accounts, err := repo.Read(ctx, orgID, ledgerID, []uuid.UUID{accountID})
			if !tc.valid {
				// The query withholds an over-bound code as NULL, which fails closed at Scan.
				require.ErrorContains(t, err, "scan official account")
				require.ErrorContains(t, err, "converting NULL to string is unsupported")
				require.Nil(t, accounts)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.code, accounts[0].AssetCode)

			projected, err := loader.EvaluationContext(ctx, orgID, ledgerID, officialEntries(tc.code))
			require.NoError(t, err)
			require.Equal(t, tc.code, projected.Accounts[0].Asset)
			require.Equal(t, tc.code, projected.Entries[0].Asset)
		})
	}
}
