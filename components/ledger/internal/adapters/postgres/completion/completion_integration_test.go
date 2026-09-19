//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package completion

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/repository"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

type recoverySQLInfra struct {
	store        *Store
	db           *sql.DB
	transactions *countingTransactionRepository
	operations   *countingOperationRepository
}

type countingTransactionRepository struct {
	transactionRepository
	beginCount int
}

func (repo *countingTransactionRepository) BeginTx(ctx context.Context) (repository.DBTransaction, error) {
	repo.beginCount++
	return repo.transactionRepository.BeginTx(ctx)
}

type countingOperationRepository struct {
	operationRepository
	createBulkCount int
}

func (repo *countingOperationRepository) CreateBulkTx(ctx context.Context, tx repository.DBExecutor, rows []*operation.Operation) (*repository.BulkInsertResult, error) {
	repo.createBulkCount++
	return repo.operationRepository.CreateBulkTx(ctx, tx, rows)
}

func setupRecoverySQL(t *testing.T) recoverySQLInfra {
	t.Helper()
	cfg := pgtestutil.DefaultContainerConfig()
	cfg.Image = "postgres:17"
	container := pgtestutil.SetupContainerWithConfig(t, cfg)
	dsn := pgtestutil.BuildConnectionString(container.Host, container.Port, container.Config)
	migrations := pgtestutil.FindMigrationsPath(t, "transaction")
	client := pgtestutil.CreatePostgresClient(t, dsn, dsn, container.Config.DBName, migrations)
	transactions := &countingTransactionRepository{transactionRepository: transaction.NewTransactionPostgreSQLRepository(client, false)}
	operations := &countingOperationRepository{operationRepository: operation.NewOperationPostgreSQLRepository(client)}
	return recoverySQLInfra{
		store:        NewStore(transactions, operations),
		db:           container.DB,
		transactions: transactions,
		operations:   operations,
	}
}

func recoverySQLRecord(t *testing.T) command.TransactionWriteSet {
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

func recoverySQLProjection(t *testing.T, db *sql.DB, transactionID string) [2]string {
	t.Helper()
	var projection [2]string
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT (to_jsonb(t)-'id')::text FROM transaction t WHERE id=$1`, transactionID).Scan(&projection[0]))
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT COALESCE(jsonb_agg(to_jsonb(o)-'id'-'transaction_id' ORDER BY id), '[]'::jsonb)::text FROM operation o WHERE transaction_id=$1`, transactionID).Scan(&projection[1]))
	return projection
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
		require.ErrorIs(t, infra.store.Persist(t.Context(), record), command.ErrTransactionCompletionConflict)
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
		require.ErrorIs(t, infra.store.Persist(t.Context(), record), command.ErrTransactionCompletionConflict)
		require.Equal(t, before, recoverySQLState(t, infra.db))
	})

	for _, action := range []string{"commit", "cancel"} {
		t.Run("pending to "+action, func(t *testing.T) {
			record := recoverySQLRecord(t)
			record.Action, record.Transaction.Status.Code = "hold", constant.PENDING
			record.Transaction.Body = mtransaction.Transaction{Pending: true, Send: mtransaction.Send{Asset: "USD", Value: *record.Transaction.Amount}}
			second := *record.Transaction.Operations[0]
			second.ID = uuid.NewSHA1(uuid.MustParse(record.Transaction.ID), []byte("hold:second")).String()
			record.Transaction.Operations = append(record.Transaction.Operations, &second)
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
			require.NoError(t, infra.store.Persist(t.Context(), pending))
			require.Equal(t, before, recoverySQLState(t, infra.db), "late hold must not insert rows or regress the terminal transaction")
			record.Transaction.Status.Code = other
			if action == "commit" {
				record.Action = "cancel"
			} else {
				record.Action = "commit"
			}
			require.ErrorIs(t, infra.store.Persist(t.Context(), record), command.ErrTransactionCompletionConflict)
			require.Equal(t, before, recoverySQLState(t, infra.db))

			_, err := infra.db.ExecContext(t.Context(), `UPDATE operation SET amount=amount+1 WHERE id=$1`, second.ID)
			require.NoError(t, err)
			corrupt := recoverySQLState(t, infra.db)
			require.ErrorIs(t, infra.store.Persist(t.Context(), pending), command.ErrTransactionCompletionConflict)
			require.Equal(t, corrupt, recoverySQLState(t, infra.db), "late hold must not overwrite a corrupted old row")

			_, err = infra.db.ExecContext(t.Context(), `DELETE FROM operation WHERE id=$1`, second.ID)
			require.NoError(t, err)
			missing := recoverySQLState(t, infra.db)
			require.ErrorIs(t, infra.store.Persist(t.Context(), pending), command.ErrTransactionCompletionConflict)
			require.Equal(t, missing, recoverySQLState(t, infra.db), "late hold must not recreate a missing old row")
		})
	}

	for _, scenario := range []struct {
		action, terminal string
	}{
		{action: "commit", terminal: constant.APPROVED},
		{action: "cancel", terminal: constant.CANCELED},
	} {
		t.Run("public outcome "+scenario.action, func(t *testing.T) {
			record := recoverySQLRecord(t)
			record.Action, record.Transaction.Status.Code = "hold", constant.PENDING
			record.Transaction.Body = mtransaction.Transaction{Pending: true, Send: mtransaction.Send{Asset: "USD", Value: *record.Transaction.Amount}}

			holdOutcome, err := infra.store.PersistWithOutcome(t.Context(), record)
			require.NoError(t, err)
			require.Equal(t, constant.PENDING, holdOutcome.TransactionStatus)
			require.Equal(t, command.TransactionLifecyclePhaseCreated, holdOutcome.LifecyclePhase)

			pending := record
			pendingTransaction := *record.Transaction
			pending.Transaction = &pendingTransaction
			record.Action, record.ExpectedStatus, record.Transaction.Status.Code = scenario.action, constant.PENDING, scenario.terminal
			record.Transaction.UpdatedAt = record.Transaction.CreatedAt.Add(time.Second)
			row := *record.Transaction.Operations[0]
			row.ID = uuid.NewSHA1(uuid.MustParse(record.Transaction.ID), []byte("outcome:"+scenario.action)).String()
			row.CreatedAt, row.UpdatedAt = record.Transaction.UpdatedAt, record.Transaction.UpdatedAt
			record.Transaction.Operations = []*operation.Operation{&row}

			terminalOutcome, err := infra.store.PersistWithOutcome(t.Context(), record)
			require.NoError(t, err)
			require.Equal(t, scenario.terminal, terminalOutcome.TransactionStatus)
			require.Equal(t, command.TransactionLifecyclePhaseUpdated, terminalOutcome.LifecyclePhase)
			beforeReplay := recoverySQLState(t, infra.db)

			replayOutcome, err := infra.store.PersistWithOutcome(t.Context(), pending)
			require.NoError(t, err)
			require.Equal(t, scenario.terminal, replayOutcome.TransactionStatus)
			require.Equal(t, command.TransactionLifecyclePhaseNoop, replayOutcome.LifecyclePhase)
			require.Equal(t, beforeReplay, recoverySQLState(t, infra.db), "late hold must report the terminal status without changing persisted rows")
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
			require.ErrorIs(t, infra.store.Persist(t.Context(), record), command.ErrTransactionCompletionConflict)
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
		require.ErrorIs(t, infra.store.Persist(t.Context(), record), command.ErrTransactionCompletionConflict)
		require.Equal(t, before, recoverySQLState(t, infra.db))
	})
}

func TestIntegrationEngineWriteBehindCausalCompletion(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL")
	}
	infra := setupRecoverySQL(t)

	for _, scenario := range []struct {
		action, terminal, conflicting string
	}{
		{action: "commit", terminal: constant.APPROVED, conflicting: constant.CANCELED},
		{action: "cancel", terminal: constant.CANCELED, conflicting: constant.APPROVED},
	} {
		t.Run(scenario.action+" before hold", func(t *testing.T) {
			pending := recoverySQLRecord(t)
			pending.Action, pending.Transaction.Status.Code = "hold", constant.PENDING
			pending.Transaction.Body = mtransaction.Transaction{Pending: true, Send: mtransaction.Send{Asset: "USD", Value: *pending.Transaction.Amount}}

			terminal := pending
			terminalTransaction := *pending.Transaction
			terminal.Transaction = &terminalTransaction
			terminal.Action, terminal.ExpectedStatus, terminal.Transaction.Status.Code = scenario.action, constant.PENDING, scenario.terminal
			terminal.Transaction.UpdatedAt = terminal.Transaction.CreatedAt.Add(time.Second)
			row := *pending.Transaction.Operations[0]
			row.ID = uuid.NewSHA1(uuid.MustParse(pending.Transaction.ID), []byte("causal:"+scenario.action)).String()
			row.CreatedAt, row.UpdatedAt = terminal.Transaction.UpdatedAt, terminal.Transaction.UpdatedAt
			terminal.Transaction.Operations = []*operation.Operation{&row}

			before := recoverySQLState(t, infra.db)
			require.ErrorIs(t, infra.store.Persist(t.Context(), terminal), command.ErrTransactionCompletionConflict)
			require.Equal(t, before, recoverySQLState(t, infra.db), "a terminal execution cannot synthesize a missing hold")

			require.NoError(t, infra.store.Persist(t.Context(), pending))
			require.NoError(t, infra.store.Persist(t.Context(), terminal))
			durable := recoverySQLState(t, infra.db)
			require.NoError(t, infra.store.Persist(t.Context(), terminal))
			require.NoError(t, infra.store.Persist(t.Context(), pending))
			require.Equal(t, durable, recoverySQLState(t, infra.db), "duplicates and a late hold must converge without regression")

			conflicting := terminal
			conflictingTransaction := *terminal.Transaction
			conflicting.Transaction = &conflictingTransaction
			conflicting.Transaction.Status.Code = scenario.conflicting
			if scenario.action == "commit" {
				conflicting.Action = "cancel"
			} else {
				conflicting.Action = "commit"
			}
			require.ErrorIs(t, infra.store.Persist(t.Context(), conflicting), command.ErrTransactionCompletionConflict)
			require.Equal(t, durable, recoverySQLState(t, infra.db), "a real terminal conflict must not alter durable state")
		})
	}

	t.Run("revert before origin", func(t *testing.T) {
		origin := recoverySQLRecord(t)
		revert := origin
		revertTransaction := *origin.Transaction
		revert.Transaction = &revertTransaction
		revert.Transaction.ID = uuid.NewSHA1(uuid.MustParse(origin.Transaction.ID), []byte("revert")).String()
		revert.Transaction.ParentTransactionID = &origin.Transaction.ID
		revert.Transaction.CreatedAt = origin.Transaction.CreatedAt.Add(time.Second)
		revert.Transaction.UpdatedAt = revert.Transaction.CreatedAt
		revert.Action = "revert"
		revertRow := *origin.Transaction.Operations[0]
		revertRow.ID = uuid.NewSHA1(uuid.MustParse(revert.Transaction.ID), []byte("operation")).String()
		revertRow.TransactionID = revert.Transaction.ID
		revertRow.CreatedAt, revertRow.UpdatedAt = revert.Transaction.CreatedAt, revert.Transaction.UpdatedAt
		revert.Transaction.Operations = []*operation.Operation{&revertRow}

		before := recoverySQLState(t, infra.db)
		require.Error(t, infra.store.Persist(t.Context(), revert), "the parent foreign key must reject a revert whose origin is absent")
		require.Equal(t, before, recoverySQLState(t, infra.db))

		require.NoError(t, infra.store.Persist(t.Context(), origin))
		require.NoError(t, infra.store.Persist(t.Context(), revert))
		durable := recoverySQLState(t, infra.db)
		require.NoError(t, infra.store.Persist(t.Context(), revert))
		require.NoError(t, infra.store.Persist(t.Context(), origin))
		require.Equal(t, durable, recoverySQLState(t, infra.db), "origin and revert duplicates must converge")

		conflicting := revert
		conflictingTransaction := *revert.Transaction
		conflicting.Transaction = &conflictingTransaction
		changedAmount := conflicting.Transaction.Amount.Add(decimal.NewFromInt(1))
		conflicting.Transaction.Amount = &changedAmount
		require.ErrorIs(t, infra.store.Persist(t.Context(), conflicting), command.ErrTransactionCompletionConflict)
		require.Equal(t, durable, recoverySQLState(t, infra.db), "immutable revert conflict must not alter durable state")
	})
}

func TestIntegrationEngineWriteBehindBulk(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL")
	}
	infra := setupRecoverySQL(t)

	t.Run("correlates terminal before hold with one group commit", func(t *testing.T) {
		pending := recoverySQLRecord(t)
		pending.Action, pending.Transaction.Status.Code = "hold", constant.PENDING
		pending.Transaction.Body = mtransaction.Transaction{Pending: true, Send: mtransaction.Send{Asset: "USD", Value: *pending.Transaction.Amount}}

		terminal := pending
		terminalTransaction := *pending.Transaction
		terminal.Transaction = &terminalTransaction
		terminal.Action, terminal.ExpectedStatus, terminal.Transaction.Status.Code = "commit", constant.PENDING, constant.APPROVED
		terminal.Transaction.UpdatedAt = terminal.Transaction.CreatedAt.Add(time.Second)
		terminalRow := *pending.Transaction.Operations[0]
		terminalRow.ID = uuid.NewSHA1(uuid.MustParse(pending.Transaction.ID), []byte("bulk:commit")).String()
		terminalRow.CreatedAt, terminalRow.UpdatedAt = terminal.Transaction.UpdatedAt, terminal.Transaction.UpdatedAt
		terminal.Transaction.Operations = []*operation.Operation{&terminalRow}

		beginCount := infra.transactions.beginCount
		outcomes, err := infra.store.PersistBulkWithOutcome(t.Context(), []command.TransactionWriteSet{terminal, pending})
		require.NoError(t, err)
		require.Equal(t, beginCount+1, infra.transactions.beginCount, "two messages in one scope must share one SQL transaction")
		require.Equal(t, []command.TransactionPersistenceOutcome{
			{TransactionStatus: constant.APPROVED, LifecyclePhase: command.TransactionLifecyclePhaseUpdated},
			{TransactionStatus: constant.APPROVED, LifecyclePhase: command.TransactionLifecyclePhaseCreated},
		}, outcomes, "outcomes must remain correlated to input order after causal reordering")

		durable := recoverySQLState(t, infra.db)
		outcomes, err = infra.store.PersistBulkWithOutcome(t.Context(), []command.TransactionWriteSet{terminal, pending})
		require.NoError(t, err)
		require.Equal(t, command.TransactionLifecyclePhaseNoop, outcomes[0].LifecyclePhase)
		require.Equal(t, command.TransactionLifecyclePhaseNoop, outcomes[1].LifecyclePhase)
		require.Equal(t, durable, recoverySQLState(t, infra.db), "bulk duplicate must only verify durable rows")
	})

	t.Run("orders revert after origin in the same group", func(t *testing.T) {
		origin := recoverySQLRecord(t)
		origin.Transaction.ID = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
		origin.Transaction.Operations[0].TransactionID = origin.Transaction.ID
		origin.Transaction.Operations[0].ID = "eeeeeeee-eeee-5eee-8eee-eeeeeeeeeeee"

		revert := recoverySQLRecord(t)
		revert.Transaction.ID = "11111111-1111-4111-8111-111111111112"
		revert.Transaction.ParentTransactionID = &origin.Transaction.ID
		revert.Transaction.CreatedAt = origin.Transaction.CreatedAt.Add(time.Second)
		revert.Transaction.UpdatedAt = revert.Transaction.CreatedAt
		revert.Transaction.Operations[0].TransactionID = revert.Transaction.ID
		revert.Transaction.Operations[0].ID = "11111111-1111-5111-8111-111111111112"
		revert.Transaction.Operations[0].CreatedAt = revert.Transaction.CreatedAt
		revert.Transaction.Operations[0].UpdatedAt = revert.Transaction.UpdatedAt
		revert.Action = "revert"

		outcomes, err := infra.store.PersistBulkWithOutcome(t.Context(), []command.TransactionWriteSet{revert, origin})
		require.NoError(t, err, "the origin dependency must override lexical transaction order")
		require.Len(t, outcomes, 2)
		require.Equal(t, constant.APPROVED, outcomes[0].TransactionStatus)
		require.Equal(t, constant.APPROVED, outcomes[1].TransactionStatus)
	})

	t.Run("chunks operation rows at configured limit", func(t *testing.T) {
		record := recoverySQLRecord(t)
		for index := 1; index < 3; index++ {
			row := *record.Transaction.Operations[0]
			row.ID = uuid.NewSHA1(uuid.MustParse(record.Transaction.ID), []byte("bulk-row:"+string(rune('0'+index)))).String()
			record.Transaction.Operations = append(record.Transaction.Operations, &row)
		}
		limited := NewStore(infra.transactions, infra.operations, 1)
		bulkCalls := infra.operations.createBulkCount
		_, err := limited.PersistBulkWithOutcome(t.Context(), []command.TransactionWriteSet{record})
		require.NoError(t, err)
		require.Equal(t, bulkCalls+3, infra.operations.createBulkCount)
	})

	t.Run("rolls back the complete scope group on conflict", func(t *testing.T) {
		existing := recoverySQLRecord(t)
		require.NoError(t, infra.store.Persist(t.Context(), existing))
		before := recoverySQLState(t, infra.db)

		newRecord := recoverySQLRecord(t)
		newRecord.Transaction.ID = "00000000-0000-4000-8000-000000000001"
		newRecord.Transaction.Operations[0].TransactionID = newRecord.Transaction.ID
		newRecord.Transaction.Operations[0].ID = "00000000-0000-5000-8000-000000000002"

		conflicting := recoverySQLRecord(t)
		changedAmount := conflicting.Transaction.Amount.Add(decimal.NewFromInt(1))
		conflicting.Transaction.Amount = &changedAmount

		_, err := infra.store.PersistBulkWithOutcome(t.Context(), []command.TransactionWriteSet{newRecord, conflicting})
		require.ErrorIs(t, err, command.ErrTransactionCompletionConflict)
		require.Equal(t, before, recoverySQLState(t, infra.db), "an earlier insert in the failed group must be rolled back")
	})

	t.Run("deduplicates equivalent units without losing correlation", func(t *testing.T) {
		record := recoverySQLRecord(t)
		outcomes, err := infra.store.PersistBulkWithOutcome(t.Context(), []command.TransactionWriteSet{record, record})
		require.NoError(t, err)
		require.Equal(t, command.TransactionLifecyclePhaseCreated, outcomes[0].LifecyclePhase)
		require.Equal(t, command.TransactionLifecyclePhaseNoop, outcomes[1].LifecyclePhase)
	})

	t.Run("matches individual persistence", func(t *testing.T) {
		individual := recoverySQLRecord(t)
		bulk := recoverySQLRecord(t)
		bulk.Transaction.ID = uuid.NewSHA1(uuid.MustParse(individual.Transaction.ID), []byte("bulk-equivalent")).String()
		bulk.Transaction.Operations[0].TransactionID = bulk.Transaction.ID
		bulk.Transaction.Operations[0].ID = uuid.NewSHA1(uuid.MustParse(individual.Transaction.Operations[0].ID), []byte("bulk-equivalent")).String()

		require.NoError(t, infra.store.Persist(t.Context(), individual))
		_, err := infra.store.PersistBulkWithOutcome(t.Context(), []command.TransactionWriteSet{bulk})
		require.NoError(t, err)
		require.Equal(
			t,
			recoverySQLProjection(t, infra.db, individual.Transaction.ID),
			recoverySQLProjection(t, infra.db, bulk.Transaction.ID),
		)
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
