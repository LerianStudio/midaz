// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operation

import (
	"context"
	"database/sql"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/stretchr/testify/require"
)

func TestOperationRecordedAtMapping(t *testing.T) {
	recordedAt := time.Date(2026, time.September, 21, 12, 34, 56, 123456000, time.UTC)

	model := &OperationPostgreSQLModel{}
	model.FromEntity(&Operation{RecordedAt: &recordedAt})
	require.True(t, model.RecordedAt.Valid)
	require.Equal(t, recordedAt, model.RecordedAt.Time)

	entity := model.ToEntity()
	require.NotNil(t, entity.RecordedAt)
	require.Equal(t, recordedAt, *entity.RecordedAt)

	model.FromEntity(&Operation{})
	require.False(t, model.RecordedAt.Valid)
	require.Nil(t, model.ToEntity().RecordedAt)
}

func TestOperationRepositoryRecordedAtFallbackAndPrecedence(t *testing.T) {
	fixed := time.Date(2026, time.September, 21, 13, 0, 0, 654321000, time.UTC)
	provided := fixed.Add(-time.Hour)

	for _, tc := range []struct {
		name     string
		provided *time.Time
		want     time.Time
	}{
		{name: "repository clock fills missing value", want: fixed},
		{name: "engine value takes precedence", provided: &provided, want: provided},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := &mockOperationDB{rowsAffected: 1}
			ctx := tmcore.ContextWithPG(context.Background(), db)
			repo := &OperationPostgreSQLRepository{
				tableName: "operation",
				clock:     func() time.Time { return fixed },
			}
			op := generateTestOperation("")
			op.RecordedAt = tc.provided

			created, err := repo.Create(ctx, op)
			require.NoError(t, err)
			require.NotNil(t, created.RecordedAt)
			require.Equal(t, tc.want, *created.RecordedAt)
		})
	}
}

func TestOperationRepositoryBulkUsesOneFallbackTimestamp(t *testing.T) {
	fixed := time.Date(2026, time.September, 21, 14, 0, 0, 123456000, time.UTC)
	clockCalls := 0
	db := &mockOperationDB{rowsAffected: 2}
	ctx := tmcore.ContextWithPG(context.Background(), db)
	repo := &OperationPostgreSQLRepository{
		tableName: "operation",
		clock: func() time.Time {
			clockCalls++
			return fixed
		},
	}

	_, err := repo.CreateBulkTx(ctx, db, generateTestOperations(2))
	require.NoError(t, err)
	require.Equal(t, 1, clockCalls)
	require.Len(t, db.queryArgs, 64)
	requireRecordedAtArg(t, fixed, db.queryArgs[31])
	requireRecordedAtArg(t, fixed, db.queryArgs[63])
}

func requireRecordedAtArg(t *testing.T, want time.Time, got any) {
	t.Helper()

	switch value := got.(type) {
	case sql.NullTime:
		require.True(t, value.Valid)
		require.Equal(t, want, value.Time)
	case time.Time:
		require.Equal(t, want, value)
	case *time.Time:
		require.NotNil(t, value)
		require.Equal(t, want, *value)
	default:
		t.Fatalf("unexpected recorded_at argument type %T", got)
	}
}
