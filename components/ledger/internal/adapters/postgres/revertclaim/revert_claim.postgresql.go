// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package revertclaim owns the durable, origin-scoped claim that fences a
// transaction reversal before any balance mutation.
package revertclaim

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v6/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

type State string

const (
	StateClaimed                State = "CLAIMED"
	StateMutated                State = "MUTATED"
	StateCompleted              State = "COMPLETED"
	StateReconciliationRequired State = "RECONCILIATION_REQUIRED"
)

type Claim struct {
	OrganizationID       uuid.UUID
	LedgerID             uuid.UUID
	OriginTransactionID  uuid.UUID
	ReverseTransactionID uuid.UUID
	State                State
	FailureReason        *string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// Repository stores the reversal fence on PostgreSQL primary. Claim is an
// insert-or-read operation: exactly one caller reserves the reverse ID for an
// origin, while every loser receives that same durable reservation.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 --destination=revert_claim.postgresql_mock.go --package=revertclaim . Repository
type Repository interface {
	Claim(ctx context.Context, organizationID, ledgerID, originID, reverseID uuid.UUID) (*Claim, bool, error)
	Get(ctx context.Context, organizationID, ledgerID, originID uuid.UUID) (*Claim, error)
	Transition(ctx context.Context, organizationID, ledgerID, originID, reverseID uuid.UUID, state State, failureReason *string) error
	Release(ctx context.Context, organizationID, ledgerID, originID, reverseID uuid.UUID) (bool, error)
}

type PostgreSQLRepository struct {
	connection    *libPostgres.Client
	requireTenant bool
}

func NewPostgreSQLRepository(connection *libPostgres.Client, requireTenant ...bool) *PostgreSQLRepository {
	r := &PostgreSQLRepository{connection: connection}
	if len(requireTenant) > 0 {
		r.requireTenant = requireTenant[0]
	}

	return r
}

func (r *PostgreSQLRepository) getDB(ctx context.Context) (dbresolver.DB, error) {
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

func scanClaim(row interface{ Scan(...any) error }) (*Claim, error) {
	claim := &Claim{}
	if err := row.Scan(
		&claim.OrganizationID,
		&claim.LedgerID,
		&claim.OriginTransactionID,
		&claim.ReverseTransactionID,
		&claim.State,
		&claim.FailureReason,
		&claim.CreatedAt,
		&claim.UpdatedAt,
	); err != nil {
		return nil, err
	}

	return claim, nil
}

func (r *PostgreSQLRepository) Claim(ctx context.Context, organizationID, ledgerID, originID, reverseID uuid.UUID) (*Claim, bool, error) {
	db, err := r.getDB(ctx)
	if err != nil {
		return nil, false, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("begin revert claim transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `
		INSERT INTO transaction_revert_claim (
			organization_id, ledger_id, origin_transaction_id, reverse_transaction_id
		) VALUES ($1, $2, $3, $4)
		ON CONFLICT (organization_id, ledger_id, origin_transaction_id) DO NOTHING`,
		organizationID, ledgerID, originID, reverseID,
	)
	if err != nil {
		return nil, false, fmt.Errorf("insert revert claim: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return nil, false, fmt.Errorf("read revert claim insert result: %w", err)
	}

	claim, err := scanClaim(tx.QueryRowContext(ctx, `
		SELECT organization_id, ledger_id, origin_transaction_id,
		       reverse_transaction_id, state, failure_reason, created_at, updated_at
		FROM transaction_revert_claim
		WHERE organization_id = $1 AND ledger_id = $2 AND origin_transaction_id = $3`,
		organizationID, ledgerID, originID,
	))
	if err != nil {
		return nil, false, fmt.Errorf("read revert claim: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit revert claim: %w", err)
	}

	return claim, rows == 1, nil
}

func (r *PostgreSQLRepository) Get(ctx context.Context, organizationID, ledgerID, originID uuid.UUID) (*Claim, error) {
	db, err := r.getDB(ctx)
	if err != nil {
		return nil, err
	}

	// BeginTx always routes to the primary in dbresolver. Revert eligibility and
	// replay must never be decided from a lagging replica.
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin primary revert claim read: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	claim, err := scanClaim(tx.QueryRowContext(ctx, `
		SELECT organization_id, ledger_id, origin_transaction_id,
		       reverse_transaction_id, state, failure_reason, created_at, updated_at
		FROM transaction_revert_claim
		WHERE organization_id = $1 AND ledger_id = $2 AND origin_transaction_id = $3`,
		organizationID, ledgerID, originID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read revert claim: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit primary revert claim read: %w", err)
	}

	return claim, nil
}

func (r *PostgreSQLRepository) Transition(ctx context.Context, organizationID, ledgerID, originID, reverseID uuid.UUID, state State, failureReason *string) error {
	db, err := r.getDB(ctx)
	if err != nil {
		return err
	}

	result, err := db.ExecContext(ctx, `
		UPDATE transaction_revert_claim
		SET state = $5, failure_reason = $6, updated_at = NOW()
		WHERE organization_id = $1 AND ledger_id = $2
		  AND origin_transaction_id = $3 AND reverse_transaction_id = $4
		  AND (state <> 'COMPLETED' OR $5 = 'COMPLETED')`,
		organizationID, ledgerID, originID, reverseID, state, failureReason,
	)
	if err != nil {
		return fmt.Errorf("transition revert claim: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read revert claim transition result: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("revert claim not transitionable")
	}

	return nil
}

func (r *PostgreSQLRepository) Release(ctx context.Context, organizationID, ledgerID, originID, reverseID uuid.UUID) (bool, error) {
	db, err := r.getDB(ctx)
	if err != nil {
		return false, err
	}

	result, err := db.ExecContext(ctx, `
		DELETE FROM transaction_revert_claim
		WHERE organization_id = $1 AND ledger_id = $2
		  AND origin_transaction_id = $3 AND reverse_transaction_id = $4
		  AND state = 'CLAIMED'`,
		organizationID, ledgerID, originID, reverseID,
	)
	if err != nil {
		return false, fmt.Errorf("release revert claim: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read revert claim release result: %w", err)
	}

	return rows == 1, nil
}
