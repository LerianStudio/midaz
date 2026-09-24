//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracercontext

import (
	"context"
	"database/sql"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	traceradapter "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

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
	loader, err := traceradapter.NewOfficialContextLoader(repo, "ledger", testBounds())
	require.NoError(t, err)
	facts, err := loader.AccountAssets(ctx, orgID, ledgerID, []uuid.UUID{accountID})
	require.NoError(t, err)
	require.Len(t, facts, 1)
	require.Equal(t, "BTC", facts[0].Asset.Code)
	otherResolver := dbresolver.New(dbresolver.WithPrimaryDBs(other.DB), dbresolver.WithReplicaDBs(primary.DB))
	otherCtx := tmcore.ContextWithPG(t.Context(), otherResolver, constant.ModuleOnboarding)
	facts, err = loader.AccountAssets(otherCtx, orgID, ledgerID, []uuid.UUID{accountID})
	require.NoError(t, err)
	require.Len(t, facts, 1)
	require.Equal(t, "POINTS", facts[0].Asset.Code)
	entries := []traceradapter.PreparedEntry{
		{AccountID: accountID, Direction: tracercontract.Debit, Amount: decimal.RequireFromString("10.125"), AssetCode: "BTC"},
		{External: true, Direction: tracercontract.Credit, Amount: decimal.RequireFromString("10.125"), AssetCode: "BTC"},
	}
	projected, err := loader.EvaluationContext(ctx, orgID, ledgerID, entries)
	require.NoError(t, err)
	require.Len(t, projected.Entries, 2)
	require.Equal(t, tracercontract.Amount("10.125"), projected.Entries[0].Amount)
	require.Equal(t, assetID.String(), projected.Entries[1].Asset.ID)
	for _, scope := range [][2]uuid.UUID{{ledgerID, ledgerID}, {orgID, orgID}} {
		_, _, err := repo.Read(ctx, scope[0], scope[1], []uuid.UUID{accountID}, nil)
		require.ErrorIs(t, err, constant.ErrTracerFactsUnavailable)
	}
	_, _, err = repo.Read(context.Background(), orgID, ledgerID, []uuid.UUID{accountID}, nil)
	require.Error(t, err)

	t.Run("missing and ambiguous assets", func(t *testing.T) {
		_, err := primary.DB.ExecContext(t.Context(), `UPDATE asset SET deleted_at='2026-01-02T00:00:00Z' WHERE id=$1`, assetID)
		require.NoError(t, err)
		_, _, err = repo.Read(ctx, orgID, ledgerID, []uuid.UUID{accountID}, nil)
		require.ErrorIs(t, err, constant.ErrTracerFactsUnavailable)
		_, err = primary.DB.ExecContext(t.Context(), `UPDATE asset SET deleted_at=NULL WHERE id=$1`, assetID)
		require.NoError(t, err)
		duplicateID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440005")
		_, err = primary.DB.ExecContext(t.Context(), `INSERT INTO asset (id,organization_id,ledger_id,type,code,status,created_at) VALUES ($1,$2,$3,'crypto','BTC','ACTIVE','2026-01-01T00:00:00Z')`, duplicateID, orgID, ledgerID)
		require.NoError(t, err)
		_, _, err = repo.Read(ctx, orgID, ledgerID, []uuid.UUID{accountID}, nil)
		require.ErrorIs(t, err, constant.ErrTracerFactsUnavailable)
		_, err = primary.DB.ExecContext(t.Context(), `DELETE FROM asset WHERE id=$1`, duplicateID)
		require.NoError(t, err)
	})
	t.Run("snapshot across account and asset reads", func(t *testing.T) {
		tx, err := resolver.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		accounts, err := repo.readAccounts(ctx, tx, orgID, ledgerID, []uuid.UUID{accountID})
		require.NoError(t, err)
		require.Len(t, accounts, 1)
		_, err = primary.DB.ExecContext(t.Context(), `UPDATE asset SET code='CHANGED' WHERE id=$1`, assetID)
		require.NoError(t, err)
		assets, err := repo.readAssets(ctx, tx, orgID, ledgerID, map[string]struct{}{"BTC": {}})
		require.NoError(t, err)
		require.Len(t, assets, 1)
		require.Equal(t, "BTC", assets[0].Code)
		require.NoError(t, tx.Commit())
		_, _, err = repo.Read(ctx, orgID, ledgerID, []uuid.UUID{accountID}, nil)
		require.ErrorIs(t, err, constant.ErrTracerFactsUnavailable)
	})
}
