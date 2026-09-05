// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package recovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/repository"
)

type transactionRepository interface {
	BeginTx(context.Context) (repository.DBTransaction, error)
	CreateBulkTx(context.Context, repository.DBExecutor, []*transaction.Transaction) (*repository.BulkInsertResult, error)
}

type operationRepository interface {
	CreateBulkTx(context.Context, repository.DBExecutor, []*operation.Operation) (*repository.BulkInsertResult, error)
}

// Store persists a frozen execution in the existing transaction and operation
// tables. It does not execute balance accounting or perform backup cleanup.
type Store struct {
	transactions transactionRepository
	operations   operationRepository
}

var _ command.BalanceEngineRecoveryStore = (*Store)(nil)

// NewStore shares the existing transaction and operation repositories. Tenant
// connection selection is delegated to the transaction repository's BeginTx.
func NewStore(transactions transactionRepository, operations operationRepository) *Store {
	return &Store{transactions: transactions, operations: operations}
}

// Persist inserts or verifies all expected rows in a single SQL transaction.
// Any failure, including an uncertain commit, leaves recovery to the caller.
func (store *Store) Persist(ctx context.Context, record command.BalanceEnginePersistenceRecord) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := validateRecord(record); err != nil {
		return err
	}

	if store == nil || store.transactions == nil || store.operations == nil {
		return fmt.Errorf("recovery SQL repositories are not configured")
	}

	tx, err := store.transactions.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("begin recovery persistence: %w", err)
	}

	if tx == nil {
		return fmt.Errorf("begin recovery persistence returned a nil transaction")
	}

	committed := false
	defer func() {
		if !committed {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				err = errors.Join(err, fmt.Errorf("rollback recovery persistence: %w", rollbackErr))
			}
		}
	}()

	querier, ok := tx.(repository.DBQuerier)
	if !ok {
		return repository.ErrQueryContextNotSupported
	}

	allowInsert, err := store.persistTransaction(ctx, tx, querier, record)
	if err != nil {
		return err
	}

	if allowInsert {
		// Bulk repositories sort their input slices. Preserve frozen row ordering.
		operations := append([]*operation.Operation(nil), record.Transaction.Operations...)
		if _, err := store.operations.CreateBulkTx(ctx, tx, operations); err != nil {
			return fmt.Errorf("persist recovery operations: %w", err)
		}
	}

	for _, row := range record.Transaction.Operations {
		if err := verifyOperation(ctx, querier, row); err != nil {
			return err
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit recovery persistence: %w", err)
	}

	committed = true

	return nil
}

func validateRecord(record command.BalanceEnginePersistenceRecord) error {
	tran := record.Transaction
	if tran == nil || tran.Amount == nil || tran.Amount.IsNegative() || tran.AssetCode == "" || tran.CreatedAt.IsZero() || tran.DeletedAt != nil || len(tran.Operations) == 0 {
		return conflict("invalid frozen transaction")
	}

	if err := validateIDs(tran.ID, tran.OrganizationID, tran.LedgerID); err != nil {
		return err
	}

	if tran.ParentTransactionID != nil {
		if err := validateIDs(*tran.ParentTransactionID); err != nil {
			return err
		}

		if *tran.ParentTransactionID == tran.ID {
			return conflict("transaction cannot be its own parent")
		}
	}

	if err := validateLifecycle(record); err != nil {
		return err
	}

	seen := make(map[string]bool, len(tran.Operations))
	for _, row := range tran.Operations {
		if err := validateOperationIdentity(tran, row); err != nil {
			return err
		}

		if seen[row.ID] {
			return conflict("duplicate frozen operation identity")
		}

		seen[row.ID] = true
	}

	return nil
}

func validateLifecycle(record command.BalanceEnginePersistenceRecord) error {
	status := record.Transaction.Status.Code
	switch record.Action {
	case "direct", "revert":
		if record.ExpectedStatus == "" && status == constant.APPROVED {
			return nil
		}
	case "hold":
		if record.ExpectedStatus == "" && status == constant.PENDING && !record.Transaction.Body.IsEmpty() {
			return nil
		}
	case "commit":
		if record.ExpectedStatus == constant.PENDING && status == constant.APPROVED {
			return nil
		}
	case "cancel":
		if record.ExpectedStatus == constant.PENDING && status == constant.CANCELED {
			return nil
		}
	}

	return conflict("unsupported frozen lifecycle transition")
}

func validateOperationIdentity(tran *transaction.Transaction, row *operation.Operation) error {
	if row == nil || row.TransactionID != tran.ID || row.OrganizationID != tran.OrganizationID || row.LedgerID != tran.LedgerID || row.AssetCode != tran.AssetCode {
		return conflict("operation scope disagrees with transaction")
	}

	if row.Amount.Value == nil || row.Amount.Value.IsNegative() || row.Balance.Available == nil || row.Balance.OnHold == nil || row.Balance.Version == nil || row.BalanceAfter.Available == nil || row.BalanceAfter.OnHold == nil || row.BalanceAfter.Version == nil || row.DeletedAt != nil {
		return conflict("incomplete frozen operation")
	}

	return validateIDs(row.ID, row.AccountID, row.BalanceID)
}

func validateIDs(values ...string) error {
	for _, value := range values {
		parsed, err := uuid.Parse(value)
		if err != nil || parsed == uuid.Nil || parsed.String() != value {
			return conflict("invalid canonical persistence identity")
		}
	}

	return nil
}

func (store *Store) persistTransaction(ctx context.Context, tx repository.DBTransaction, querier repository.DBQuerier, record command.BalanceEnginePersistenceRecord) (bool, error) {
	status, exists, err := verifyTransaction(ctx, querier, record)
	if err != nil {
		return false, err
	}

	created := false

	if !exists {
		if record.ExpectedStatus != "" {
			return false, conflict("pending transaction has not been persisted")
		}

		inserted, err := store.transactions.CreateBulkTx(ctx, tx, []*transaction.Transaction{record.Transaction})
		if err != nil {
			return false, fmt.Errorf("persist recovery transaction: %w", err)
		}

		created, err = confirmedTransactionInsert(inserted, record.Transaction.ID)
		if err != nil {
			return false, err
		}

		status, exists, err = verifyTransaction(ctx, querier, record)
		if err != nil {
			return false, err
		}

		if !exists {
			return false, conflict("transaction missing after insertion")
		}
	}

	if status == record.Transaction.Status.Code {
		return created, nil
	}

	if record.Action == "hold" && terminalTransactionStatus(status) {
		// The locked transaction already matches the frozen hold identity and
		// body. Verify every old operation below, without inserting or regressing
		// the later status, before confirming this earlier execution persisted.
		return false, nil
	}

	if record.ExpectedStatus == "" || status != record.ExpectedStatus {
		return false, conflict("persisted lifecycle does not match expected state")
	}

	result, err := tx.ExecContext(ctx, `UPDATE transaction SET status = $1, status_description = $2, updated_at = $3
WHERE id = $4 AND organization_id = $5 AND ledger_id = $6 AND status = $7 AND deleted_at IS NULL`,
		record.Transaction.Status.Code, record.Transaction.Status.Description, record.Transaction.UpdatedAt,
		record.Transaction.ID, record.Transaction.OrganizationID, record.Transaction.LedgerID, record.ExpectedStatus)
	if err != nil {
		return false, fmt.Errorf("update recovery transaction status: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("confirm recovery status update: %w", err)
	}

	if count != 1 {
		return false, conflict("recovery status update did not affect one transaction")
	}

	return true, nil
}

func terminalTransactionStatus(status string) bool {
	return status == constant.APPROVED || status == constant.CANCELED
}

func confirmedTransactionInsert(inserted *repository.BulkInsertResult, transactionID string) (bool, error) {
	if inserted == nil || inserted.Inserted < 0 || inserted.Inserted > 1 {
		return false, conflict("transaction insertion outcome is not confirmed")
	}

	created := inserted.Inserted == 1
	if created && (len(inserted.InsertedIDs) != 1 || inserted.InsertedIDs[0] != transactionID) {
		return false, conflict("inserted transaction identity is not confirmed")
	}

	return created, nil
}

func verifyTransaction(ctx context.Context, querier repository.DBQuerier, record command.BalanceEnginePersistenceRecord) (string, bool, error) {
	tran := record.Transaction
	columns := []string{"organization_id", "ledger_id", "parent_transaction_id", "amount", "asset_code", "chart_of_accounts_group_name", "created_at", "route", "route_id", "fees_skipped", "tracer_skipped"}
	legacyRoute := nullableText(tran.Route) //nolint:staticcheck // Recovery verifies the persisted legacy column without changing it.
	args := []any{tran.ID, tran.OrganizationID, tran.LedgerID, tran.ParentTransactionID, tran.Amount, tran.AssetCode, tran.ChartOfAccountsGroupName, tran.CreatedAt, legacyRoute, tran.RouteID, tran.FeesSkipped, tran.TracerSkipped}
	predicate := equalityPredicate(columns, 2)

	if record.Action == "hold" {
		body, err := json.Marshal(tran.Body)
		if err != nil {
			return "", false, fmt.Errorf("encode frozen pending transaction: %w", err)
		}

		args = append(args, string(body))
		predicate += fmt.Sprintf(" AND body::jsonb IS NOT DISTINCT FROM $%d::jsonb", len(args))
	}

	query := "SELECT status, deleted_at IS NULL, (" + predicate + ") FROM transaction WHERE id = $1 FOR UPDATE"

	rows, err := querier.QueryContext(ctx, query, args...)
	if err != nil {
		return "", false, fmt.Errorf("read recovery transaction: %w", err)
	}

	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return "", false, fmt.Errorf("read recovery transaction row: %w", err)
		}

		return "", false, nil
	}

	var (
		status          string
		active, matches bool
	)

	if err := rows.Scan(&status, &active, &matches); err != nil {
		return "", false, fmt.Errorf("decode recovery transaction: %w", err)
	}

	if err := finishSingleRow(rows); err != nil {
		return "", false, err
	}

	if !active || !matches {
		return "", false, conflict("persisted transaction differs from immutable intent")
	}

	return status, true, nil
}

func verifyOperation(ctx context.Context, querier repository.DBQuerier, row *operation.Operation) error {
	columns := []string{"transaction_id", "description", "type", "asset_code", "amount", "available_balance", "on_hold_balance", "available_balance_after", "on_hold_balance_after", "status", "status_description", "account_id", "account_alias", "balance_id", "chart_of_accounts", "organization_id", "ledger_id", "created_at", "updated_at", "deleted_at", "route", "balance_affected", "balance_key", "balance_version_before", "balance_version_after", "direction", "route_id", "route_code", "route_description"}

	balanceKey := row.BalanceKey
	if balanceKey == "" {
		balanceKey = constant.DefaultBalanceKey
	}

	legacyRoute := nullableText(row.Route) //nolint:staticcheck // Recovery verifies the persisted legacy column without changing it.
	args := make([]any, 0, 31)
	args = append(
		args,
		row.ID, row.TransactionID, row.Description, row.Type, row.AssetCode, row.Amount.Value, row.Balance.Available, row.Balance.OnHold, row.BalanceAfter.Available, row.BalanceAfter.OnHold, row.Status.Code, row.Status.Description,
		row.AccountID, row.AccountAlias, row.BalanceID, row.ChartOfAccounts, row.OrganizationID, row.LedgerID, row.CreatedAt, row.UpdatedAt, row.DeletedAt, legacyRoute, row.BalanceAffected, balanceKey,
		row.Balance.Version, row.BalanceAfter.Version, row.Direction, row.RouteID, row.RouteCode, row.RouteDescription,
	)

	snapshot, err := json.Marshal(row.Snapshot)
	if err != nil {
		return fmt.Errorf("encode frozen operation snapshot: %w", err)
	}

	args = append(args, string(snapshot))
	predicate := equalityPredicate(columns, 2) + fmt.Sprintf(" AND snapshot IS NOT DISTINCT FROM $%d::jsonb", len(args))

	rows, err := querier.QueryContext(ctx, "SELECT ("+predicate+") FROM operation WHERE id = $1 FOR SHARE", args...)
	if err != nil {
		return fmt.Errorf("read recovery operation: %w", err)
	}

	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return fmt.Errorf("read recovery operation row: %w", err)
		}

		return conflict("expected operation missing after insertion")
	}

	var matches bool
	if err := rows.Scan(&matches); err != nil {
		return fmt.Errorf("decode recovery operation: %w", err)
	}

	if err := finishSingleRow(rows); err != nil {
		return err
	}

	if !matches {
		return conflict("persisted operation differs from frozen row")
	}

	return nil
}

type scannedRows interface {
	Next() bool
	Err() error
	Close() error
}

func finishSingleRow(rows scannedRows) error {
	if rows.Next() {
		return conflict("multiple rows share one persistence identity")
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("finish recovery verification: %w", err)
	}

	if err := rows.Close(); err != nil {
		return fmt.Errorf("close recovery verification: %w", err)
	}

	return nil
}

func equalityPredicate(columns []string, firstParameter int) string {
	comparisons := make([]string, 0, len(columns))
	for index, column := range columns {
		comparisons = append(comparisons, fmt.Sprintf("%s IS NOT DISTINCT FROM $%d", column, index+firstParameter))
	}

	return strings.Join(comparisons, " AND ")
}

func nullableText(value string) *string {
	if value == "" {
		return nil
	}

	return &value
}

func conflict(reason string) error {
	return fmt.Errorf("%w: %s", command.ErrBalanceEnginePersistenceConflict, reason)
}
