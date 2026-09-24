// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestContextLimitRepositoryGuards(t *testing.T) {
	config := ContextLimitRepositoryConfig{MaxAccounts: 2, MaxLimits: 3, MaxScopes: 4, MaxScopeBytes: 4096, MaxTextBytes: 20}
	for _, change := range []func(*ContextLimitRepositoryConfig){
		func(c *ContextLimitRepositoryConfig) { c.MaxAccounts = 0 }, func(c *ContextLimitRepositoryConfig) { c.MaxLimits = 0 },
		func(c *ContextLimitRepositoryConfig) { c.MaxScopes = 0 }, func(c *ContextLimitRepositoryConfig) { c.MaxScopeBytes = 0 }, func(c *ContextLimitRepositoryConfig) { c.MaxTextBytes = 0 },
	} {
		bad := config
		change(&bad)
		_, err := NewContextLimitRepository(bad)
		require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
	}
	repo, err := NewContextLimitRepository(config)
	require.NoError(t, err)
	id := testutil.MustDeterministicUUID(82501)
	asset := tracercontract.AssetRef{Namespace: "ledger", ID: "asset", Code: "USD"}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, repo.BindAssetWithTx(ctx, nil, id, asset), context.Canceled)
	require.ErrorIs(t, repo.BindAssetWithTx(t.Context(), nil, id, asset), pgdb.ErrNilConnection)
	result, err := repo.ListCandidatesWithTx(t.Context(), nil, "ledger", []uuid.UUID{id})
	require.ErrorIs(t, err, pgdb.ErrNilConnection)
	require.Nil(t, result)
	tx := mocks.NewMockTx(gomock.NewController(t))
	require.ErrorIs(t, repo.BindAssetWithTx(t.Context(), tx, uuid.Nil, asset), constant.ErrInvalidRequestBody)
	require.ErrorIs(t, repo.BindAssetWithTx(t.Context(), tx, id, tracercontract.AssetRef{}), constant.ErrInvalidRequestBody)
	for _, ids := range [][]uuid.UUID{{uuid.Nil}, {id, id}, {id, testutil.MustDeterministicUUID(82502), testutil.MustDeterministicUUID(82503)}} {
		result, err := repo.ListCandidatesWithTx(t.Context(), tx, "ledger", ids)
		require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
		require.Nil(t, result)
	}
	for _, ns := range []string{"", " ledger", "ledger\x00", "namespace-is-far-too-long", string([]byte{0xff})} {
		result, err := repo.ListCandidatesWithTx(t.Context(), tx, ns, []uuid.UUID{id})
		require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
		require.Nil(t, result)
	}
	result, err = repo.ListCandidatesWithTx(t.Context(), tx, "ledger", nil)
	require.NoError(t, err)
	require.Empty(t, result)
	tx.EXPECT().QueryContext(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, context.DeadlineExceeded)
	result, err = repo.ListCandidatesWithTx(t.Context(), tx, "ledger", []uuid.UUID{id})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, result)
}

func TestContextLimitAssociationWriteResults(t *testing.T) {
	for _, tc := range []struct {
		name    string
		rows    int64
		execErr error
		want    error
	}{
		{name: "inserted", rows: 1},
		{name: "duplicate", want: constant.ErrLimitAssetReferenceConflict},
		{name: "unexpected count", rows: 2, want: constant.ErrInternalServer},
		{name: "deadline", execErr: context.DeadlineExceeded, want: context.DeadlineExceeded},
		{name: "wrong stored code or limit", execErr: &pgconn.PgError{Code: "23503", ConstraintName: "limit_asset_reference_limit_fk"}, want: constant.ErrContextLimitsUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, err := NewContextLimitRepository(ContextLimitRepositoryConfig{MaxAccounts: 2, MaxLimits: 3, MaxScopes: 4, MaxScopeBytes: 4096, MaxTextBytes: 20})
			require.NoError(t, err)
			tx := mocks.NewMockTx(gomock.NewController(t))
			tx.EXPECT().ExecContext(gomock.Any(), gomock.Any(), gomock.Any()).Return(sqlmock.NewResult(0, tc.rows), tc.execErr)
			err = repo.BindAssetWithTx(t.Context(), tx, testutil.MustDeterministicUUID(82504), tracercontract.AssetRef{Namespace: "ledger", ID: "asset", Code: "USD"})
			if tc.want == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.want)
			}
		})
	}
}
