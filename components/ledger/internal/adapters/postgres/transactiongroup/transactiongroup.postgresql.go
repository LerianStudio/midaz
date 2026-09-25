// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transactiongroup

import (
	"context"
	"fmt"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/Masterminds/squirrel"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

var transactionGroupColumns = []string{
	"id", "organization_id", "ledger_id", "status", "asset_code", "intent", "created_at", "updated_at",
}

// TransactionGroupPostgreSQLRepository stores cross-ledger lifecycle groups.
type TransactionGroupPostgreSQLRepository struct {
	connection    *libPostgres.Client
	requireTenant bool
}

// NewTransactionGroupPostgreSQLRepository constructs a transaction-group repository.
func NewTransactionGroupPostgreSQLRepository(
	connection *libPostgres.Client,
	requireTenant ...bool,
) *TransactionGroupPostgreSQLRepository {
	repo := &TransactionGroupPostgreSQLRepository{connection: connection}
	if len(requireTenant) > 0 {
		repo.requireTenant = requireTenant[0]
	}

	return repo
}

func (r *TransactionGroupPostgreSQLRepository) getDB(ctx context.Context) (dbresolver.DB, error) {
	if db := tmcore.GetPGContext(ctx, constant.ModuleTransaction); db != nil {
		return db, nil
	}

	if db := tmcore.GetPGContext(ctx); db != nil {
		return db, nil
	}

	if r.requireTenant {
		return nil, fmt.Errorf("tenant postgres connection missing from context")
	}

	if r.connection == nil {
		return nil, fmt.Errorf("postgres connection not available")
	}

	return r.connection.Resolver(ctx)
}

// Create inserts a lifecycle group before its hold is published to the engine.
func (r *TransactionGroupPostgreSQLRepository) Create(ctx context.Context, group *TransactionGroup) error {
	db, err := r.getDB(ctx)
	if err != nil {
		return err
	}

	query, args, err := squirrel.Insert("transaction_group").
		Columns(transactionGroupColumns...).
		Values(group.ID, group.OrganizationID, group.LedgerID, group.Status, group.AssetCode, group.Intent, group.CreatedAt, group.UpdatedAt).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return err
	}

	_, err = db.ExecContext(ctx, query, args...)

	return err
}

// Find returns a group only from its primary API scope.
func (r *TransactionGroupPostgreSQLRepository) Find(
	ctx context.Context,
	organizationID, ledgerID, id uuid.UUID,
) (*TransactionGroup, error) {
	return r.find(ctx, squirrel.Eq{
		"id": id, "organization_id": organizationID, "ledger_id": ledgerID,
	})
}

// FindByID resolves a group across ledger scopes within the current tenant.
func (r *TransactionGroupPostgreSQLRepository) FindByID(ctx context.Context, id uuid.UUID) (*TransactionGroup, error) {
	return r.find(ctx, squirrel.Eq{"id": id})
}

func (r *TransactionGroupPostgreSQLRepository) find(
	ctx context.Context,
	where squirrel.Eq,
) (*TransactionGroup, error) {
	db, err := r.getDB(ctx)
	if err != nil {
		return nil, err
	}

	query, args, err := squirrel.Select(transactionGroupColumns...).
		From("transaction_group").
		Where(where).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, err
	}

	group := &TransactionGroup{}

	err = db.QueryRowContext(ctx, query, args...).Scan(
		&group.ID,
		&group.OrganizationID,
		&group.LedgerID,
		&group.Status,
		&group.AssetCode,
		&group.Intent,
		&group.CreatedAt,
		&group.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return group, nil
}

// ListByStatusOlderThan pages groups of one status by id for background
// reconciliation.
func (r *TransactionGroupPostgreSQLRepository) ListByStatusOlderThan(
	ctx context.Context,
	status string,
	before time.Time,
	afterID uuid.UUID,
	limit int,
) ([]*TransactionGroup, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("transaction group page limit must be positive, got %d", limit)
	}

	db, err := r.getDB(ctx)
	if err != nil {
		return nil, err
	}

	where := squirrel.And{
		squirrel.Eq{"status": status},
		squirrel.Lt{"created_at": before},
	}
	if afterID != uuid.Nil {
		where = append(where, squirrel.Gt{"id": afterID})
	}

	query, args, err := squirrel.Select(transactionGroupColumns...).
		From("transaction_group").
		Where(where).
		OrderBy("id ASC").
		Limit(uint64(limit)).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := make([]*TransactionGroup, 0, limit)

	for rows.Next() {
		group := &TransactionGroup{}
		if err := rows.Scan(
			&group.ID,
			&group.OrganizationID,
			&group.LedgerID,
			&group.Status,
			&group.AssetCode,
			&group.Intent,
			&group.CreatedAt,
			&group.UpdatedAt,
		); err != nil {
			return nil, err
		}

		groups = append(groups, group)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return groups, nil
}

// UpdateStatus applies a compare-and-swap lifecycle transition.
func (r *TransactionGroupPostgreSQLRepository) UpdateStatus(
	ctx context.Context,
	id uuid.UUID,
	from, to string,
) (bool, error) {
	db, err := r.getDB(ctx)
	if err != nil {
		return false, err
	}

	query, args, err := squirrel.Update("transaction_group").
		Set("status", to).
		Set("updated_at", squirrel.Expr("now()")).
		Where(squirrel.Eq{"id": id, "status": from}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return false, err
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	return rows == 1, nil
}

// DeleteIfMemberless removes an orphan intent. The membership check and the
// delete are one statement, so a member projected after the caller read an empty
// member set keeps its group.
func (r *TransactionGroupPostgreSQLRepository) DeleteIfMemberless(ctx context.Context, id uuid.UUID) (bool, error) {
	db, err := r.getDB(ctx)
	if err != nil {
		return false, err
	}

	query, args, err := squirrel.Delete("transaction_group").
		Where(squirrel.Eq{"id": id, "status": constant.PENDING}).
		Where("NOT EXISTS (SELECT 1 FROM transaction WHERE transaction.group_id = ?)", id).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return false, err
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	return rows == 1, nil
}

// Delete removes a pre-publication group intent after a confirmed hold refusal.
func (r *TransactionGroupPostgreSQLRepository) Delete(ctx context.Context, id uuid.UUID) error {
	db, err := r.getDB(ctx)
	if err != nil {
		return err
	}

	query, args, err := squirrel.Delete("transaction_group").
		Where(squirrel.Eq{"id": id}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return err
	}

	_, err = db.ExecContext(ctx, query, args...)

	return err
}
