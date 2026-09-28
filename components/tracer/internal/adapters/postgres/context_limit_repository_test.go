// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestContextLimitRepositoryGuards(t *testing.T) {
	config := ContextLimitRepositoryConfig{MaxAccounts: 2, MaxLimits: 3, MaxScopes: 4, MaxScopeBytes: 4096}
	for _, change := range []func(*ContextLimitRepositoryConfig){
		func(c *ContextLimitRepositoryConfig) { c.MaxAccounts = 0 }, func(c *ContextLimitRepositoryConfig) { c.MaxLimits = 0 },
		func(c *ContextLimitRepositoryConfig) { c.MaxScopes = 0 }, func(c *ContextLimitRepositoryConfig) { c.MaxScopeBytes = 0 },
	} {
		bad := config
		change(&bad)
		_, err := NewContextLimitRepository(bad)
		require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
	}
	repo, err := NewContextLimitRepository(config)
	require.NoError(t, err)
	id := testutil.MustDeterministicUUID(82501)
	btc := []string{"BTC"}
	result, err := repo.ListCandidatesWithTx(t.Context(), nil, btc, []uuid.UUID{id})
	require.ErrorIs(t, err, pgdb.ErrNilConnection)
	require.Nil(t, result)
	tx := mocks.NewMockTx(gomock.NewController(t))
	for _, ids := range [][]uuid.UUID{{uuid.Nil}, {id, id}, {id, testutil.MustDeterministicUUID(82502), testutil.MustDeterministicUUID(82503)}} {
		result, err := repo.ListCandidatesWithTx(t.Context(), tx, btc, ids)
		require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
		require.Nil(t, result)
	}
	two := []uuid.UUID{id, testutil.MustDeterministicUUID(82502)}
	for _, tc := range []struct {
		assets []string
		ids    []uuid.UUID
	}{
		{[]string{""}, []uuid.UUID{id}},
		{[]string{"btc"}, []uuid.UUID{id}},
		{[]string{" BTC"}, []uuid.UUID{id}},
		{[]string{strings.Repeat("A", 101)}, []uuid.UUID{id}},
		{[]string{"BTC", "BTC"}, two},
		{[]string{"BTC", "XBT"}, []uuid.UUID{id}},
		{btc, nil},
	} {
		result, err := repo.ListCandidatesWithTx(t.Context(), tx, tc.assets, tc.ids)
		require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
		require.Nil(t, result)
	}
	for _, ids := range [][]uuid.UUID{nil, {id}} {
		result, err = repo.ListCandidatesWithTx(t.Context(), tx, nil, ids)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Empty(t, result)
	}
	tx.EXPECT().QueryContext(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, context.DeadlineExceeded)
	result, err = repo.ListCandidatesWithTx(t.Context(), tx, btc, []uuid.UUID{id})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, result)
}
