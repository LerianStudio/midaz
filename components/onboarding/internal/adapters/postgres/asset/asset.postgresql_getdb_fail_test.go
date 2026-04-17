// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package asset

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
)

type stderrLogger struct{}

func (stderrLogger) Info(_ ...any)                                     {}
func (stderrLogger) Infof(_ string, _ ...any)                          {}
func (stderrLogger) Infoln(_ ...any)                                   {}
func (stderrLogger) Error(_ ...any)                                    {}
func (stderrLogger) Errorf(_ string, _ ...any)                         {}
func (stderrLogger) Errorln(_ ...any)                                  {}
func (stderrLogger) Warn(_ ...any)                                     {}
func (stderrLogger) Warnf(_ string, _ ...any)                          {}
func (stderrLogger) Warnln(_ ...any)                                   {}
func (stderrLogger) Debug(_ ...any)                                    {}
func (stderrLogger) Debugf(_ string, _ ...any)                         {}
func (stderrLogger) Debugln(_ ...any)                                  {}
func (stderrLogger) Fatal(_ ...any)                                    {}
func (stderrLogger) Fatalf(_ string, _ ...any)                         {}
func (stderrLogger) Fatalln(_ ...any)                                  {}
func (stderrLogger) WithFields(_ ...any) libLog.Logger                 { return stderrLogger{} }
func (stderrLogger) WithDefaultMessageTemplate(_ string) libLog.Logger { return stderrLogger{} }
func (stderrLogger) Sync() error                                       { return nil }

func brokenRepo() *AssetPostgreSQLRepository {
	return &AssetPostgreSQLRepository{
		tableName: "asset",
		connection: &libPostgres.PostgresConnection{
			ConnectionStringPrimary: "bad-dsn",
			ConnectionStringReplica: "bad-dsn",
			Logger:                  stderrLogger{},
		},
	}
}

func TestAssetRepository_GetDBFailures(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("Create", func(t *testing.T) {
		t.Parallel()

		_, err := brokenRepo().Create(ctx, &mmodel.Asset{})
		require.Error(t, err)
	})
	t.Run("FindByNameOrCode", func(t *testing.T) {
		t.Parallel()

		_, err := brokenRepo().FindByNameOrCode(ctx, uuid.New(), uuid.New(), "n", "c")
		require.Error(t, err)
	})
	t.Run("FindAll", func(t *testing.T) {
		t.Parallel()

		_, err := brokenRepo().FindAll(ctx, uuid.New(), uuid.New(), http.Pagination{Limit: 1, Page: 1, SortOrder: "asc"})
		require.Error(t, err)
	})
	t.Run("ListByIDs", func(t *testing.T) {
		t.Parallel()

		_, err := brokenRepo().ListByIDs(ctx, uuid.New(), uuid.New(), []uuid.UUID{uuid.New()})
		require.Error(t, err)
	})
	t.Run("Find", func(t *testing.T) {
		t.Parallel()

		_, err := brokenRepo().Find(ctx, uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})
	t.Run("Update", func(t *testing.T) {
		t.Parallel()

		_, err := brokenRepo().Update(ctx, uuid.New(), uuid.New(), uuid.New(), &mmodel.Asset{Name: "x"})
		require.Error(t, err)
	})
	t.Run("Delete", func(t *testing.T) {
		t.Parallel()

		err := brokenRepo().Delete(ctx, uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})
	t.Run("Count", func(t *testing.T) {
		t.Parallel()

		_, err := brokenRepo().Count(ctx, uuid.New(), uuid.New())
		require.Error(t, err)
	})
}
