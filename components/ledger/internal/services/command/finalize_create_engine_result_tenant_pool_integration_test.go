//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	obsconst "github.com/LerianStudio/lib-observability/v4/constants"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/completion"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/tenantpool"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// finalizeTenantPoolSwapLogMessage is a copy of tenantpool.poolSwapLogMessage
// (unexported, in components/ledger/internal/adapters/postgres/tenantpool/tenantpool.go);
// keep the two in sync. The tests count it to prove the swap was observed exactly
// when expected.
const finalizeTenantPoolSwapLogMessage = "Tenant PostgreSQL pool changed during request; using the current pool"

// finalizeTenantPoolClosedMessage is the message database/sql returns for a call
// on a closed *sql.DB; its sentinel is unexported, so it is matched by text.
const finalizeTenantPoolClosedMessage = "sql: database is closed"

// finalizeTenantPoolResolver stands in for the tenant manager: the test decides
// which pool it answers with, or that it fails, at any point.
type finalizeTenantPoolResolver struct {
	mu    sync.Mutex
	db    dbresolver.DB
	err   error
	calls int
}

func (r *finalizeTenantPoolResolver) GetDB(_ context.Context, _ string) (dbresolver.DB, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls++

	return r.db, r.err
}

func (r *finalizeTenantPoolResolver) answer(db dbresolver.DB, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.db, r.err = db, err
}

func (r *finalizeTenantPoolResolver) resolutions() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.calls
}

// finalizeTenantPoolLogRecorder captures every log line emitted under the
// context. It satisfies libLog.Universal, which is all ContextWithLogger needs.
type finalizeTenantPoolLogRecorder struct {
	mu      sync.Mutex
	entries []finalizeTenantPoolLogEntry
}

type finalizeTenantPoolLogEntry struct {
	level  int
	msg    string
	fields map[string]any
}

func (r *finalizeTenantPoolLogRecorder) Log(_ context.Context, level int, msg string, fields ...any) {
	entry := finalizeTenantPoolLogEntry{level: level, msg: msg, fields: make(map[string]any, len(fields))}

	for _, f := range fields {
		if field, ok := f.(libLog.Field); ok {
			entry.fields[field.Key] = field.Value
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.entries = append(r.entries, entry)
}

// swapWarnings counts the pool-swap warnings logged for tenantID and the
// transaction module.
func (r *finalizeTenantPoolLogRecorder) swapWarnings(tenantID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	count := 0

	for _, e := range r.entries {
		if e.msg == finalizeTenantPoolSwapLogMessage && e.level == libLog.LevelWarn &&
			e.fields[obsconst.AttrKeyTenantID] == tenantID && e.fields["module"] == constant.ModuleTransaction {
			count++
		}
	}

	return count
}

// openFinalizeTenantPool opens an independent pool on the migrated database,
// built like the tenant manager builds one: a dbresolver.DB over its own *sql.DB.
func openFinalizeTenantPool(t *testing.T, dsn string) (dbresolver.DB, *sql.DB) {
	t.Helper()

	sqlDB, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	// Close is idempotent, so a handle the test already closed is safe here.
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, sqlDB.PingContext(context.Background()))

	return dbresolver.New(dbresolver.WithPrimaryDBs(sqlDB), dbresolver.WithReplicaDBs(sqlDB)), sqlDB
}

// TestIntegration_FinalizeCreateEngineResult_TenantPool covers the pool layer of
// "Projeção da transação após o engine usa o pool atual". The production
// completer behind finalizeCreateEngineResult (TransactionCompletionService over
// the PostgreSQL completion store) runs with multi-tenant repositories that read
// their pool from the context, where the request pinned the lazy tenantpool
// adapter. The engine is modelled as already applied: the envelope it would have
// recorded is the input, and the pool changes after it and before Complete.
func TestIntegration_FinalizeCreateEngineResult_TenantPool(t *testing.T) {
	const tenantID = "finalize-tenant-pool"

	pg := pgtestutil.SetupMigratedContainer(t, "transaction")
	mongoContainer := mongotestutil.SetupReusableContainer(t)

	store := completion.NewStore(
		transaction.NewTransactionPostgreSQLRepository(nil, false, true),
		operation.NewOperationPostgreSQLRepository(nil, true),
	)
	completer := command.NewTransactionCompletionService(store, mongodb.NewMetadataMongoDBRepository(nil, true))

	requestContext := func(resolver tenantpool.Resolver, pinned dbresolver.DB) (context.Context, *finalizeTenantPoolLogRecorder) {
		logs := &finalizeTenantPoolLogRecorder{}
		ctx := tmcore.ContextWithTenantID(context.Background(), tenantID)
		ctx = libObservability.ContextWithLogger(ctx, logs)
		ctx = tmcore.ContextWithPG(ctx, tenantpool.New(resolver, tenantID, constant.ModuleTransaction, pinned), constant.ModuleTransaction)
		ctx = tmcore.ContextWithMB(ctx, mongoContainer.Database, constant.ModuleTransaction)

		return ctx, logs
	}

	t.Run("Pool trocado entre o engine e a projeção", func(t *testing.T) {
		poolA, sqlA := openFinalizeTenantPool(t, pg.DSN)
		poolB, _ := openFinalizeTenantPool(t, pg.DSN)
		resolver := &finalizeTenantPoolResolver{db: poolA}
		ctx, logs := requestContext(resolver, poolA)
		envelope := finalizerIntegrationEnvelope(t, t.Name(), tenantID, false, false, false)

		// Before the swap the request runs on the pool it pinned and nothing is logged.
		require.NoError(t, tmcore.GetPGContext(ctx, constant.ModuleTransaction).PingContext(ctx))
		require.Equal(t, 1, resolver.resolutions())
		require.Zero(t, logs.swapWarnings(tenantID), "no swap may be reported while the pinned pool is current")

		// The engine has applied; the tenant manager now closes the pinned pool
		// and answers with a new one before the projection runs.
		require.NoError(t, sqlA.Close())
		require.ErrorContains(t, poolA.PingContext(context.Background()), finalizeTenantPoolClosedMessage)
		resolver.answer(poolB, nil)

		require.NoError(t, completionError(completer.Complete(ctx, envelope)))

		assertFinalizerSQLCounts(t, pg.DB, envelope.TransactionID, 1, 1)
		assertFinalizerMetadataCount(t, mongoContainer.Database, constant.EntityTransaction, envelope.TransactionID.String(), 1)
		assert.Greater(t, resolver.resolutions(), 1, "the projection re-resolves the pool")
		assert.GreaterOrEqual(t, logs.swapWarnings(tenantID), 1, "the projection reports it ran on a pool other than the pinned one")
	})

	t.Run("Re-resolução falha após o engine", func(t *testing.T) {
		pinned, pinnedSQL := openFinalizeTenantPool(t, pg.DSN)
		resolver := &finalizeTenantPoolResolver{err: errors.New("tenant manager unavailable")}
		ctx, logs := requestContext(resolver, pinned)
		envelope := finalizerIntegrationEnvelope(t, t.Name(), tenantID, false, false, false)

		// The engine has applied; the pinned pool is closed and re-resolution fails.
		require.NoError(t, pinnedSQL.Close())

		err := completionError(completer.Complete(ctx, envelope))

		require.Error(t, err)
		assert.ErrorContains(t, err, finalizeTenantPoolClosedMessage, "the projection reports the real closed-pool error")
		assertFinalizerSQLCounts(t, pg.DB, envelope.TransactionID, 0, 0)
		assertFinalizerMetadataCount(t, mongoContainer.Database, constant.EntityTransaction, envelope.TransactionID.String(), 0)
		assert.GreaterOrEqual(t, resolver.resolutions(), 1, "the projection tried to re-resolve the pool")
		assert.Zero(t, logs.swapWarnings(tenantID), "a failed re-resolution falls back to the pin and reports no swap")
	})
}
