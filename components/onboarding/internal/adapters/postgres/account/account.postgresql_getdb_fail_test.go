// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package account

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

// stderrLogger is a minimal logger satisfying the interface without side effects.
type stderrLogger struct{}

func (stderrLogger) Info(_ ...any)                     {}
func (stderrLogger) Infof(_ string, _ ...any)          {}
func (stderrLogger) Infoln(_ ...any)                   {}
func (stderrLogger) Error(_ ...any)                    {}
func (stderrLogger) Errorf(_ string, _ ...any)         {}
func (stderrLogger) Errorln(_ ...any)                  {}
func (stderrLogger) Warn(_ ...any)                     {}
func (stderrLogger) Warnf(_ string, _ ...any)          {}
func (stderrLogger) Warnln(_ ...any)                   {}
func (stderrLogger) Debug(_ ...any)                    {}
func (stderrLogger) Debugf(_ string, _ ...any)         {}
func (stderrLogger) Debugln(_ ...any)                  {}
func (stderrLogger) Fatal(_ ...any)                    {}
func (stderrLogger) Fatalf(_ string, _ ...any)         {}
func (stderrLogger) Fatalln(_ ...any)                  {}
func (stderrLogger) WithFields(_ ...any) libLog.Logger { return stderrLogger{} }
func (stderrLogger) WithDefaultMessageTemplate(_ string) libLog.Logger {
	return stderrLogger{}
}
func (stderrLogger) Sync() error { return nil }

// repoWithBrokenConnection returns a repository whose connection will fail on
// any call to GetDB. This exercises the "failed to get database connection"
// error branch that sits at the top of every repository method.
func repoWithBrokenConnection(_ *testing.T) *AccountPostgreSQLRepository {
	return &AccountPostgreSQLRepository{
		tableName: "account",
		connection: &libPostgres.PostgresConnection{
			// No ConnectionDB, no migrations, and an invalid connection string
			// force the lazy Connect() path to fail fast on sql.Open.
			ConnectionStringPrimary: "not-a-valid-postgres-dsn",
			ConnectionStringReplica: "not-a-valid-postgres-dsn",
			Logger:                  stderrLogger{},
		},
	}
}

func TestAccountRepository_GetDBFailures(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		r := repoWithBrokenConnection(t)
		_, err := r.Create(ctx, &mmodel.Account{})
		require.Error(t, err)
	})

	t.Run("FindAll", func(t *testing.T) {
		t.Parallel()
		r := repoWithBrokenConnection(t)
		_, err := r.FindAll(ctx, uuid.New(), uuid.New(), nil, http.Pagination{Limit: 10, Page: 1, SortOrder: "asc"})
		require.Error(t, err)
	})

	t.Run("Find", func(t *testing.T) {
		t.Parallel()
		r := repoWithBrokenConnection(t)
		_, err := r.Find(ctx, uuid.New(), uuid.New(), nil, uuid.New())
		require.Error(t, err)
	})

	t.Run("FindWithDeleted", func(t *testing.T) {
		t.Parallel()
		r := repoWithBrokenConnection(t)
		_, err := r.FindWithDeleted(ctx, uuid.New(), uuid.New(), nil, uuid.New())
		require.Error(t, err)
	})

	t.Run("FindAlias", func(t *testing.T) {
		t.Parallel()
		r := repoWithBrokenConnection(t)
		_, err := r.FindAlias(ctx, uuid.New(), uuid.New(), nil, "alias")
		require.Error(t, err)
	})

	t.Run("FindByAlias", func(t *testing.T) {
		t.Parallel()
		r := repoWithBrokenConnection(t)
		_, err := r.FindByAlias(ctx, uuid.New(), uuid.New(), "alias")
		require.Error(t, err)
	})

	t.Run("ListByIDs", func(t *testing.T) {
		t.Parallel()
		r := repoWithBrokenConnection(t)
		_, err := r.ListByIDs(ctx, uuid.New(), uuid.New(), nil, []uuid.UUID{uuid.New()})
		require.Error(t, err)
	})

	t.Run("ListByAlias", func(t *testing.T) {
		t.Parallel()
		r := repoWithBrokenConnection(t)
		_, err := r.ListByAlias(ctx, uuid.New(), uuid.New(), uuid.New(), []string{"x"})
		require.Error(t, err)
	})

	t.Run("Update", func(t *testing.T) {
		t.Parallel()
		r := repoWithBrokenConnection(t)
		_, err := r.Update(ctx, uuid.New(), uuid.New(), nil, uuid.New(), &mmodel.Account{Name: "x"})
		require.Error(t, err)
	})

	t.Run("Delete", func(t *testing.T) {
		t.Parallel()
		r := repoWithBrokenConnection(t)
		err := r.Delete(ctx, uuid.New(), uuid.New(), nil, uuid.New())
		require.Error(t, err)
	})

	t.Run("ListAccountsByIDs", func(t *testing.T) {
		t.Parallel()
		r := repoWithBrokenConnection(t)
		_, err := r.ListAccountsByIDs(ctx, uuid.New(), uuid.New(), []uuid.UUID{uuid.New()})
		require.Error(t, err)
	})

	t.Run("ListAccountsByAlias", func(t *testing.T) {
		t.Parallel()
		r := repoWithBrokenConnection(t)
		_, err := r.ListAccountsByAlias(ctx, uuid.New(), uuid.New(), []string{"x"})
		require.Error(t, err)
	})

	t.Run("Count", func(t *testing.T) {
		t.Parallel()
		r := repoWithBrokenConnection(t)
		_, err := r.Count(ctx, uuid.New(), uuid.New())
		require.Error(t, err)
	})
}
