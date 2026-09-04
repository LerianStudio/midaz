//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package recovery

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

type recoverySQLInfra struct {
	store *Store
	db    *sql.DB
}

func setupRecoverySQL(t *testing.T) recoverySQLInfra {
	t.Helper()
	cfg := pgtestutil.DefaultContainerConfig()
	cfg.Image = "postgres:17"
	container := pgtestutil.SetupContainerWithConfig(t, cfg)
	dsn := pgtestutil.BuildConnectionString(container.Host, container.Port, container.Config)
	migrations := pgtestutil.FindMigrationsPath(t, "transaction")
	client := pgtestutil.CreatePostgresClient(t, dsn, dsn, container.Config.DBName, migrations)
	return recoverySQLInfra{
		store: NewStore(transaction.NewTransactionPostgreSQLRepository(client), operation.NewOperationPostgreSQLRepository(client)),
		db:    container.DB,
	}
}

func recoverySQLRecord(t *testing.T) command.BalanceEnginePersistenceRecord {
	t.Helper()
	record := frozenStoreRecord()
	namespace := uuid.MustParse("f5acb658-b8bf-51f8-a208-d4eb90cbcbf3")
	record.Transaction.ID = uuid.NewSHA1(namespace, []byte(t.Name()+":transaction")).String()
	record.Transaction.Operations[0].TransactionID = record.Transaction.ID
	record.Transaction.Operations[0].ID = uuid.NewSHA1(namespace, []byte(t.Name()+":operation")).String()
	return record
}

func recoverySQLState(t *testing.T, db *sql.DB) [2]string {
	t.Helper()
	var state [2]string
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT COALESCE(jsonb_agg(to_jsonb(t) ORDER BY id), '[]'::jsonb)::text FROM transaction t`).Scan(&state[0]))
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT COALESCE(jsonb_agg(to_jsonb(o) ORDER BY id), '[]'::jsonb)::text FROM operation o`).Scan(&state[1]))
	return state
}

func TestIntegrationRecoverySQLStore(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL")
	}
	infra := setupRecoverySQL(t)

	t.Run("atomic creation and exact replay", func(t *testing.T) {
		record := recoverySQLRecord(t)
		require.NoError(t, infra.store.Persist(t.Context(), record))
		before := recoverySQLState(t, infra.db)
		require.NoError(t, infra.store.Persist(t.Context(), record))
		require.Equal(t, before, recoverySQLState(t, infra.db))

		var amount, available, after string
		var versionBefore, versionAfter int64
		var feesSkipped, tracerSkipped bool
		require.NoError(t, infra.db.QueryRowContext(t.Context(), `SELECT amount::text, fees_skipped, tracer_skipped FROM transaction WHERE id=$1`, record.Transaction.ID).
			Scan(&amount, &feesSkipped, &tracerSkipped))
		require.True(t, decimal.RequireFromString(amount).Equal(*record.Transaction.Amount))
		require.True(t, feesSkipped)
		require.True(t, tracerSkipped)
		require.NoError(t, infra.db.QueryRowContext(t.Context(), `SELECT available_balance::text, available_balance_after::text, balance_version_before, balance_version_after FROM operation WHERE id=$1`, record.Transaction.Operations[0].ID).
			Scan(&available, &after, &versionBefore, &versionAfter))
		require.True(t, decimal.RequireFromString(available).Equal(*record.Transaction.Operations[0].Balance.Available))
		require.True(t, decimal.RequireFromString(after).Equal(*record.Transaction.Operations[0].BalanceAfter.Available))
		require.Equal(t, int64(9007199254740993), versionBefore)
		require.Equal(t, int64(9007199254740994), versionAfter)
	})

	t.Run("second operation conflict rolls back earlier writes", func(t *testing.T) {
		existing := recoverySQLRecord(t)
		require.NoError(t, infra.store.Persist(t.Context(), existing))
		before := recoverySQLState(t, infra.db)
		record := recoverySQLRecord(t)
		record.Transaction.ID = "11111111-aaaa-4111-8111-111111111111"
		first := record.Transaction.Operations[0]
		first.TransactionID = record.Transaction.ID
		first.ID = "11111111-bbbb-5111-8111-111111111111"
		second := *first
		second.ID = existing.Transaction.Operations[0].ID
		record.Transaction.Operations = append(record.Transaction.Operations, &second)
		require.ErrorIs(t, infra.store.Persist(t.Context(), record), command.ErrBalanceEnginePersistenceConflict)
		require.Equal(t, before, recoverySQLState(t, infra.db))
	})

	for _, zeroPrimary := range []bool{false, true} {
		name := "large decimal amount"
		if zeroPrimary {
			name = "zero primary amount with recorded debt"
		}
		t.Run(name, func(t *testing.T) {
			record := recoverySQLRecord(t)
			amount := decimal.RequireFromString("123456789012345678901234567890.00000000000000000001")
			before, after := amount.Mul(decimal.NewFromInt(2)), amount
			record.Transaction.Amount = &amount
			row := record.Transaction.Operations[0]
			row.Amount.Value = &amount
			row.Balance.Available, row.BalanceAfter.Available = &before, &after
			if zeroPrimary {
				zero := decimal.Zero
				row.Amount.Value = &zero
				row.BalanceAfter.Available = &before
				row.Snapshot.OverdraftUsedBefore = amount.String()
				row.Snapshot.OverdraftUsedAfter = "0"
			}
			require.NoError(t, infra.store.Persist(t.Context(), record))
			stored := recoverySQLState(t, infra.db)
			require.NoError(t, infra.store.Persist(t.Context(), record))
			require.Equal(t, stored, recoverySQLState(t, infra.db))
			var storedTransaction, storedAmount, storedBefore, storedAfter string
			require.NoError(t, infra.db.QueryRowContext(t.Context(), `SELECT t.amount::text, o.amount::text, o.available_balance::text, o.available_balance_after::text FROM transaction t JOIN operation o ON o.transaction_id=t.id WHERE o.id=$1`, row.ID).
				Scan(&storedTransaction, &storedAmount, &storedBefore, &storedAfter))
			require.True(t, decimal.RequireFromString(storedTransaction).Equal(amount))
			require.True(t, decimal.RequireFromString(storedAmount).Equal(*row.Amount.Value))
			require.True(t, decimal.RequireFromString(storedBefore).Equal(*row.Balance.Available))
			require.True(t, decimal.RequireFromString(storedAfter).Equal(*row.BalanceAfter.Available))
		})
	}

	t.Run("same target with different operation cannot insert", func(t *testing.T) {
		record := recoverySQLRecord(t)
		require.NoError(t, infra.store.Persist(t.Context(), record))
		before := recoverySQLState(t, infra.db)
		record.Transaction.Operations[0].ID = "77777777-7777-5777-8777-777777777777"
		require.ErrorIs(t, infra.store.Persist(t.Context(), record), command.ErrBalanceEnginePersistenceConflict)
		require.Equal(t, before, recoverySQLState(t, infra.db))
	})

	for _, action := range []string{"commit", "cancel"} {
		t.Run("pending to "+action, func(t *testing.T) {
			record := recoverySQLRecord(t)
			record.Action, record.Transaction.Status.Code = "hold", constant.PENDING
			record.Transaction.Body = mtransaction.Transaction{Pending: true, Send: mtransaction.Send{Asset: "USD", Value: *record.Transaction.Amount}}
			require.NoError(t, infra.store.Persist(t.Context(), record))
			pending := record
			pendingTransaction := *record.Transaction
			pending.Transaction = &pendingTransaction
			terminal := constant.APPROVED
			other := constant.CANCELED
			if action == "cancel" {
				terminal, other = other, terminal
			}
			record.Action, record.ExpectedStatus, record.Transaction.Status.Code = action, constant.PENDING, terminal
			record.Transaction.UpdatedAt = record.Transaction.CreatedAt.Add(time.Second)
			row := *record.Transaction.Operations[0]
			row.ID = uuid.NewSHA1(uuid.MustParse(record.Transaction.ID), []byte(action)).String()
			row.CreatedAt, row.UpdatedAt = record.Transaction.UpdatedAt, record.Transaction.UpdatedAt
			record.Transaction.Operations = []*operation.Operation{&row}
			require.NoError(t, infra.store.Persist(t.Context(), record))
			before := recoverySQLState(t, infra.db)
			require.NoError(t, infra.store.Persist(t.Context(), record))
			require.ErrorIs(t, infra.store.Persist(t.Context(), pending), command.ErrBalanceEnginePersistenceConflict)
			record.Transaction.Status.Code = other
			if action == "commit" {
				record.Action = "cancel"
			} else {
				record.Action = "commit"
			}
			require.ErrorIs(t, infra.store.Persist(t.Context(), record), command.ErrBalanceEnginePersistenceConflict)
			require.Equal(t, before, recoverySQLState(t, infra.db))
		})
	}

	for _, mutation := range []string{"organization", "ledger", "account", "amount", "snapshot"} {
		t.Run("reject changed "+mutation, func(t *testing.T) {
			record := recoverySQLRecord(t)
			require.NoError(t, infra.store.Persist(t.Context(), record))
			before := recoverySQLState(t, infra.db)
			changedID := "99999999-9999-4999-8999-999999999999"
			switch mutation {
			case "organization":
				record.Transaction.OrganizationID, record.Transaction.Operations[0].OrganizationID = changedID, changedID
			case "ledger":
				record.Transaction.LedgerID, record.Transaction.Operations[0].LedgerID = changedID, changedID
			case "account":
				record.Transaction.Operations[0].AccountID = changedID
			case "amount":
				amount := decimal.NewFromInt(1)
				record.Transaction.Operations[0].Amount.Value = &amount
			case "snapshot":
				record.Transaction.Operations[0].Snapshot.OverdraftUsedAfter = "1"
			}
			require.ErrorIs(t, infra.store.Persist(t.Context(), record), command.ErrBalanceEnginePersistenceConflict)
			require.Equal(t, before, recoverySQLState(t, infra.db))
		})
	}

	t.Run("nullable fields and JSONB comparison", func(t *testing.T) {
		record := recoverySQLRecord(t)
		empty := ""
		row := record.Transaction.Operations[0]
		row.RouteCode, row.RouteDescription = &empty, &empty
		require.NoError(t, infra.store.Persist(t.Context(), record))
		snapshot, err := json.Marshal(row.Snapshot)
		require.NoError(t, err)
		_, err = infra.db.ExecContext(t.Context(), `UPDATE operation SET snapshot=$1::jsonb WHERE id=$2`, " \n "+string(snapshot), row.ID)
		require.NoError(t, err)
		require.NoError(t, infra.store.Persist(t.Context(), record))
		before := recoverySQLState(t, infra.db)
		row.RouteCode = nil
		require.ErrorIs(t, infra.store.Persist(t.Context(), record), command.ErrBalanceEnginePersistenceConflict)
		require.Equal(t, before, recoverySQLState(t, infra.db))
	})
}

func TestIntegrationRecoverySQLTenantContext(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL")
	}
	static := setupRecoverySQL(t)
	tenant := setupRecoverySQL(t)
	resolver := dbresolver.New(dbresolver.WithPrimaryDBs(tenant.db), dbresolver.WithReplicaDBs(tenant.db))
	t.Cleanup(func() { _ = resolver.Close() })
	ctx := tmcore.ContextWithPG(context.Background(), resolver)
	record := recoverySQLRecord(t)
	staticBefore := recoverySQLState(t, static.db)
	require.NoError(t, static.store.Persist(ctx, record))
	require.Equal(t, staticBefore, recoverySQLState(t, static.db))
	var count int
	require.NoError(t, tenant.db.QueryRowContext(t.Context(), `SELECT count(*) FROM operation WHERE id=$1`, record.Transaction.Operations[0].ID).Scan(&count))
	require.Equal(t, 1, count)
	tenantBefore := recoverySQLState(t, tenant.db)
	require.NoError(t, static.store.Persist(ctx, record))
	require.Equal(t, tenantBefore, recoverySQLState(t, tenant.db))
}
