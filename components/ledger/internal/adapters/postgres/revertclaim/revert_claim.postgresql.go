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
	"strings"
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
	StateArmed                  State = "ARMED"
	StateRecovering             State = "RECOVERING"
	StateMutated                State = "MUTATED"
	StateCompleted              State = "COMPLETED"
	StateReconciliationRequired State = "RECONCILIATION_REQUIRED"
)

type Claim struct {
	OrganizationID       uuid.UUID
	LedgerID             uuid.UUID
	OriginTransactionID  uuid.UUID
	ReverseTransactionID uuid.UUID
	LegacyFenceKey       *string
	LegacyFenceOwner     *string
	RolloutMode          *string
	RolloutToken         *string
	RedisGeneration      *string
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
	Claim(ctx context.Context, organizationID, ledgerID, originID, reverseID uuid.UUID,
		legacyFenceKey, legacyFenceOwner, rolloutMode, rolloutToken, redisGeneration *string) (*Claim, bool, error)
	Get(ctx context.Context, organizationID, ledgerID, originID uuid.UUID) (*Claim, error)
	GetByReverseID(ctx context.Context, organizationID, ledgerID, reverseID uuid.UUID) (*Claim, error)
	Arm(ctx context.Context, organizationID, ledgerID, originID, reverseID uuid.UUID, attemptOwner string) error
	Transition(ctx context.Context, organizationID, ledgerID, originID, reverseID uuid.UUID, state State, failureReason *string) error
	BeginPreMutationRecovery(ctx context.Context, organizationID, ledgerID, originID, reverseID uuid.UUID) (bool, error)
	Release(ctx context.Context, organizationID, ledgerID, originID, reverseID uuid.UUID) (bool, error)
	ReleaseRejectedArm(ctx context.Context, organizationID, ledgerID, originID, reverseID uuid.UUID) (bool, error)
}

type PostgreSQLRepository struct {
	connection                  *libPostgres.Client
	requireTenant               bool
	commitRolloutInitialization func(dbresolver.Tx) error
	commitClaimArm              func(dbresolver.Tx) error
}

func NewPostgreSQLRepository(connection *libPostgres.Client, requireTenant ...bool) *PostgreSQLRepository {
	r := &PostgreSQLRepository{connection: connection}
	if len(requireTenant) > 0 {
		r.requireTenant = requireTenant[0]
	}

	return r
}

// BeginRolloutInitialization atomically creates or reads the deployment-wide
// birth certificate for the financial Redis dataset. These methods
// deliberately use the configured transaction primary rather than a tenant
// connection: one rollout generation fences every tenant served by this
// deployment.
func (r *PostgreSQLRepository) BeginRolloutInitialization(
	ctx context.Context,
	redisGeneration, initializationRequestID uuid.UUID,
) (prepared, created bool, err error) {
	db, err := r.getRolloutDB(ctx)
	if err != nil {
		return false, false, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, false, fmt.Errorf("begin revert rollout initialization: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `
		INSERT INTO transaction_revert_rollout_initialization (
			singleton, redis_generation, initialization_request_id, state
		) VALUES (TRUE, $1, $2, 'PREPARING')
		ON CONFLICT (singleton) DO NOTHING`, redisGeneration, initializationRequestID)
	if err != nil {
		return false, false, fmt.Errorf("insert revert rollout initialization: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, false, fmt.Errorf("read revert rollout initialization insert result: %w", err)
	}

	storedGeneration, storedRequestID, state, err := readRolloutInitializationRow(tx.QueryRowContext(ctx, `
		SELECT redis_generation, initialization_request_id, state
		FROM transaction_revert_rollout_initialization
		WHERE singleton = TRUE`))
	if err != nil {
		return false, false, fmt.Errorf("read revert rollout initialization: %w", err)
	}

	if storedGeneration != redisGeneration {
		return false, false, fmt.Errorf("revert rollout dataset generation differs from its birth certificate")
	}

	if storedRequestID != initializationRequestID {
		return false, false, fmt.Errorf("revert rollout initialization request differs from its birth certificate")
	}

	if state != "PREPARING" && state != "PREPARED" {
		return false, false, fmt.Errorf("invalid revert rollout initialization state %q", state)
	}

	if err := r.commitRolloutTx(tx); err != nil {
		committedPrepared, reconcileErr := r.readExactRolloutInitialization(ctx, redisGeneration,
			initializationRequestID)
		if reconcileErr == nil {
			return committedPrepared, rows == 1, nil
		}

		return false, false, errors.Join(fmt.Errorf("commit revert rollout initialization: %w", err), reconcileErr)
	}

	return state == "PREPARED", rows == 1, nil
}

// CompleteRolloutInitialization promotes the exact PREPARING birth
// certificate only after Redis contains the exact generation and prepared
// marker. Retrying the same identity is idempotent; any other identity fails.
func (r *PostgreSQLRepository) CompleteRolloutInitialization(
	ctx context.Context,
	redisGeneration, initializationRequestID uuid.UUID,
) error {
	db, err := r.getRolloutDB(ctx)
	if err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin revert rollout completion: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `
		UPDATE transaction_revert_rollout_initialization
		SET state = 'PREPARED', updated_at = NOW()
		WHERE singleton = TRUE
		  AND redis_generation = $1
		  AND initialization_request_id = $2
		  AND state = 'PREPARING'`, redisGeneration, initializationRequestID)
	if err != nil {
		return fmt.Errorf("complete revert rollout initialization: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read revert rollout completion result: %w", err)
	}

	if rows == 0 {
		storedGeneration, storedRequestID, state, readErr := readRolloutInitializationRow(tx.QueryRowContext(ctx, `
			SELECT redis_generation, initialization_request_id, state
			FROM transaction_revert_rollout_initialization
			WHERE singleton = TRUE`))
		if readErr != nil {
			return fmt.Errorf("read revert rollout completion identity: %w", readErr)
		}

		if storedGeneration != redisGeneration {
			return fmt.Errorf("revert rollout dataset generation differs from its birth certificate")
		}

		if storedRequestID != initializationRequestID {
			return fmt.Errorf("revert rollout initialization request differs from its birth certificate")
		}

		if state != "PREPARED" {
			return fmt.Errorf("revert rollout initialization is not prepared")
		}
	}

	if err := r.commitRolloutTx(tx); err != nil {
		prepared, reconcileErr := r.readExactRolloutInitialization(ctx, redisGeneration, initializationRequestID)
		if reconcileErr == nil && prepared {
			return nil
		}

		if reconcileErr == nil {
			reconcileErr = fmt.Errorf("revert rollout initialization remains preparing")
		}

		return errors.Join(fmt.Errorf("commit revert rollout completion: %w", err), reconcileErr)
	}

	return nil
}

// ValidatePreparedRollout reads the deployment control row from PostgreSQL
// primary. A replica must never decide startup/readiness for the money path.
func (r *PostgreSQLRepository) ValidatePreparedRollout(ctx context.Context, redisGeneration uuid.UUID) error {
	db, err := r.getRolloutDB(ctx)
	if err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("begin primary revert rollout validation: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	storedGeneration, _, state, err := readRolloutInitializationRow(tx.QueryRowContext(ctx, `
		SELECT redis_generation, initialization_request_id, state
		FROM transaction_revert_rollout_initialization
		WHERE singleton = TRUE`))
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("revert rollout birth certificate is missing")
	}

	if err != nil {
		return fmt.Errorf("read revert rollout birth certificate: %w", err)
	}

	if storedGeneration != redisGeneration {
		return fmt.Errorf("revert rollout dataset generation differs from its birth certificate")
	}

	if state != "PREPARED" {
		return fmt.Errorf("revert rollout birth certificate is not prepared")
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit primary revert rollout validation: %w", err)
	}

	return nil
}

// InspectRolloutInitialization reads the global birth certificate from the
// deployment primary without requiring a configured generation. Phase-zero
// pods with an empty target use it on every readiness/admission decision: only
// true row absence means the released legacy algorithm is still admissible.
func (r *PostgreSQLRepository) InspectRolloutInitialization(
	ctx context.Context,
) (exists bool, redisGeneration uuid.UUID, state string, err error) {
	db, err := r.getRolloutDB(ctx)
	if err != nil {
		return false, uuid.Nil, "", err
	}

	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return false, uuid.Nil, "", fmt.Errorf("begin primary revert rollout inspection: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	storedGeneration, _, storedState, err := readRolloutInitializationRow(tx.QueryRowContext(ctx, `
		SELECT redis_generation, initialization_request_id, state
		FROM transaction_revert_rollout_initialization
		WHERE singleton = TRUE`))
	if errors.Is(err, sql.ErrNoRows) {
		if err := tx.Commit(); err != nil {
			return false, uuid.Nil, "", fmt.Errorf("commit empty primary revert rollout inspection: %w", err)
		}

		return false, uuid.Nil, "", nil
	}

	if err != nil {
		return false, uuid.Nil, "", fmt.Errorf("inspect revert rollout birth certificate: %w", err)
	}

	if storedState != "PREPARING" && storedState != "PREPARED" {
		return false, uuid.Nil, "", fmt.Errorf("invalid revert rollout initialization state %q", storedState)
	}

	if err := tx.Commit(); err != nil {
		return false, uuid.Nil, "", fmt.Errorf("commit primary revert rollout inspection: %w", err)
	}

	return true, storedGeneration, storedState, nil
}

func (r *PostgreSQLRepository) getRolloutDB(ctx context.Context) (dbresolver.DB, error) {
	if r == nil || r.connection == nil {
		return nil, fmt.Errorf("deployment transaction postgres connection not available")
	}

	return r.connection.Resolver(ctx)
}

func (r *PostgreSQLRepository) commitRolloutTx(tx dbresolver.Tx) error {
	if r.commitRolloutInitialization != nil {
		return r.commitRolloutInitialization(tx)
	}

	return tx.Commit()
}

func (r *PostgreSQLRepository) readExactRolloutInitialization(
	ctx context.Context,
	redisGeneration, initializationRequestID uuid.UUID,
) (bool, error) {
	db, err := r.getRolloutDB(ctx)
	if err != nil {
		return false, err
	}

	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return false, fmt.Errorf("begin primary revert rollout reconciliation: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	storedGeneration, storedRequestID, state, err := readRolloutInitializationRow(tx.QueryRowContext(ctx, `
		SELECT redis_generation, initialization_request_id, state
		FROM transaction_revert_rollout_initialization
		WHERE singleton = TRUE`))
	if errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("revert rollout birth certificate is missing")
	}

	if err != nil {
		return false, fmt.Errorf("read revert rollout birth certificate: %w", err)
	}

	if storedGeneration != redisGeneration {
		return false, fmt.Errorf("revert rollout dataset generation differs from its birth certificate")
	}

	if storedRequestID != initializationRequestID {
		return false, fmt.Errorf("revert rollout initialization request differs from its birth certificate")
	}

	if state != "PREPARING" && state != "PREPARED" {
		return false, fmt.Errorf("invalid revert rollout initialization state %q", state)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit primary revert rollout reconciliation: %w", err)
	}

	return state == "PREPARED", nil
}

func readRolloutInitializationRow(row interface{ Scan(...any) error }) (uuid.UUID, uuid.UUID, string, error) {
	var (
		redisGeneration         uuid.UUID
		initializationRequestID uuid.UUID
		state                   string
	)
	if err := row.Scan(&redisGeneration, &initializationRequestID, &state); err != nil {
		return uuid.Nil, uuid.Nil, "", err
	}

	return redisGeneration, initializationRequestID, state, nil
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
		&claim.LegacyFenceKey,
		&claim.LegacyFenceOwner,
		&claim.RolloutMode,
		&claim.RolloutToken,
		&claim.RedisGeneration,
		&claim.State,
		&claim.FailureReason,
		&claim.CreatedAt,
		&claim.UpdatedAt,
	); err != nil {
		return nil, err
	}

	return claim, nil
}

//nolint:gocyclo // Claim CAS handles every persisted claim state transition; refactor candidate.
func (r *PostgreSQLRepository) Claim(
	ctx context.Context,
	organizationID, ledgerID, originID, reverseID uuid.UUID,
	legacyFenceKey, legacyFenceOwner, rolloutMode, rolloutToken, redisGeneration *string,
) (*Claim, bool, error) {
	if legacyFenceKey != nil && strings.TrimSpace(*legacyFenceKey) == "" {
		return nil, false, fmt.Errorf("legacy fence key cannot be empty")
	}

	if legacyFenceOwner != nil && (legacyFenceKey == nil || *legacyFenceOwner != reverseID.String()) {
		return nil, false, fmt.Errorf("legacy fence owner requires an exact fence key")
	}

	if (rolloutMode == nil) != (rolloutToken == nil) {
		return nil, false, fmt.Errorf("revert rollout mode and token must be provided together")
	}

	if rolloutMode != nil {
		if *rolloutMode != "legacy" && *rolloutMode != "bridge" {
			return nil, false, fmt.Errorf("revert rollout mode must be legacy or bridge")
		}

		if strings.TrimSpace(*rolloutToken) == "" {
			return nil, false, fmt.Errorf("revert rollout token cannot be empty")
		}

		if redisGeneration == nil {
			return nil, false, fmt.Errorf("revert rollout requires a financial Redis generation")
		}
	}

	if redisGeneration != nil && strings.TrimSpace(*redisGeneration) == "" {
		return nil, false, fmt.Errorf("financial Redis generation cannot be empty")
	}

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
			organization_id, ledger_id, origin_transaction_id, reverse_transaction_id,
			legacy_fence_key, legacy_fence_owner, rollout_mode, rollout_token, redis_generation
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (organization_id, ledger_id, origin_transaction_id) DO NOTHING`,
		organizationID, ledgerID, originID, reverseID, legacyFenceKey, legacyFenceOwner, rolloutMode, rolloutToken,
		redisGeneration,
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
		       reverse_transaction_id, legacy_fence_key, legacy_fence_owner, rollout_mode, rollout_token, redis_generation,
		       state, failure_reason, created_at, updated_at
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
		       reverse_transaction_id, legacy_fence_key, legacy_fence_owner, rollout_mode, rollout_token, redis_generation,
		       state, failure_reason, created_at, updated_at
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

func (r *PostgreSQLRepository) GetByReverseID(ctx context.Context, organizationID, ledgerID, reverseID uuid.UUID) (*Claim, error) {
	db, err := r.getDB(ctx)
	if err != nil {
		return nil, err
	}

	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin primary revert claim reverse-id read: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	claim, err := scanClaim(tx.QueryRowContext(ctx, `
		SELECT organization_id, ledger_id, origin_transaction_id,
		       reverse_transaction_id, legacy_fence_key, legacy_fence_owner, rollout_mode, rollout_token, redis_generation,
		       state, failure_reason, created_at, updated_at
		FROM transaction_revert_claim
		WHERE organization_id = $1 AND ledger_id = $2 AND reverse_transaction_id = $3`,
		organizationID, ledgerID, reverseID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("read revert claim by reverse id: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit primary revert claim reverse-id read: %w", err)
	}

	return claim, nil
}

// Arm is the durable handoff between recoverable preparation and the first
// balance command. Only the reserved reverse can promote CLAIMED to ARMED, and
// a lost commit response is accepted only when a primary reread proves that
// exact identity in ARMED. No later state authorizes another balance attempt.
func (r *PostgreSQLRepository) Arm(
	ctx context.Context,
	organizationID, ledgerID, originID, reverseID uuid.UUID,
	attemptOwner string,
) error {
	if attemptOwner != reverseID.String() {
		return fmt.Errorf("revert claim arm requires the reserved reverse as attempt owner")
	}

	db, err := r.getDB(ctx)
	if err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin revert claim arm: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `
		UPDATE transaction_revert_claim
		SET state = 'ARMED', failure_reason = NULL, updated_at = NOW()
		WHERE organization_id = $1 AND ledger_id = $2
		  AND origin_transaction_id = $3 AND reverse_transaction_id = $4
		  AND state = 'CLAIMED'`,
		organizationID, ledgerID, originID, reverseID,
	)
	if err != nil {
		return fmt.Errorf("arm revert claim: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read revert claim arm result: %w", err)
	}

	if rows == 0 {
		var state State
		if err := tx.QueryRowContext(ctx, `
			SELECT state
			FROM transaction_revert_claim
			WHERE organization_id = $1 AND ledger_id = $2
			  AND origin_transaction_id = $3 AND reverse_transaction_id = $4`,
			organizationID, ledgerID, originID, reverseID,
		).Scan(&state); err != nil {
			return fmt.Errorf("read revert claim arm identity: %w", err)
		}

		if state != StateArmed {
			return fmt.Errorf("revert claim is %s, not armed", state)
		}
	}

	if err := r.commitArmTx(tx); err != nil {
		claim, readErr := r.Get(ctx, organizationID, ledgerID, originID)
		if readErr == nil && claim != nil && claim.ReverseTransactionID == reverseID && claim.State == StateArmed {
			return nil
		}

		if readErr == nil {
			readErr = fmt.Errorf("primary does not prove the exact armed revert claim")
		}

		return errors.Join(fmt.Errorf("commit revert claim arm: %w", err), readErr)
	}

	return nil
}

func (r *PostgreSQLRepository) commitArmTx(tx dbresolver.Tx) error {
	if r.commitClaimArm != nil {
		return r.commitClaimArm(tx)
	}

	return tx.Commit()
}

func (r *PostgreSQLRepository) Transition(ctx context.Context, organizationID, ledgerID, originID, reverseID uuid.UUID, state State, failureReason *string) error {
	db, err := r.getDB(ctx)
	if err != nil {
		return err
	}

	result, err := db.ExecContext(ctx, `
		UPDATE transaction_revert_claim
		SET state = CASE
		      WHEN state = 'COMPLETED' AND $5 = 'RECONCILIATION_REQUIRED' THEN state
		      ELSE $5
		    END,
		    failure_reason = CASE
		      WHEN state = 'COMPLETED' AND $5 = 'RECONCILIATION_REQUIRED' THEN failure_reason
		      ELSE $6
		    END,
		    updated_at = NOW()
		WHERE organization_id = $1 AND ledger_id = $2
		  AND origin_transaction_id = $3 AND reverse_transaction_id = $4
		  AND (
		    ($5 = 'MUTATED' AND state IN ('ARMED', 'MUTATED'))
		    OR ($5 = 'COMPLETED' AND state IN ('CLAIMED', 'ARMED', 'MUTATED', 'RECONCILIATION_REQUIRED', 'COMPLETED'))
		    OR $5 = 'RECONCILIATION_REQUIRED'
		  )`,
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
		return fmt.Errorf("revert claim not transitional")
	}

	return nil
}

// preMutationRecoveryLeaseSeconds is how long a RECOVERING claim fences other
// recovery candidates before its lease can be stolen.
const preMutationRecoveryLeaseSeconds = 30

// BeginPreMutationRecovery elects exactly one recovery owner while the durable
// claim still fences every competing bridge/final request. Redis cleanup is
// performed only by that owner, and the PostgreSQL claim is released last.
func (r *PostgreSQLRepository) BeginPreMutationRecovery(ctx context.Context, organizationID, ledgerID, originID, reverseID uuid.UUID) (bool, error) {
	db, err := r.getDB(ctx)
	if err != nil {
		return false, err
	}

	result, err := db.ExecContext(ctx, `
		UPDATE transaction_revert_claim
		SET state = 'RECOVERING', failure_reason = 'pre_movement_recovery', updated_at = NOW()
		WHERE organization_id = $1 AND ledger_id = $2
		  AND origin_transaction_id = $3 AND reverse_transaction_id = $4
		  AND (
		    state = 'CLAIMED'
		    OR (state = 'RECOVERING' AND updated_at <= NOW() - make_interval(secs => $5))
		  )`,
		organizationID, ledgerID, originID, reverseID, preMutationRecoveryLeaseSeconds,
	)
	if err != nil {
		return false, fmt.Errorf("begin pre-mutation revert recovery: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read pre-mutation revert recovery result: %w", err)
	}

	return rows == 1, nil
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
		  AND state IN ('CLAIMED', 'RECOVERING')`,
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

// ReleaseRejectedArm is separate from pre-arm release so an ARMED claim can
// disappear only on the live path that received a definitive atomic rejection
// and already removed the exact seed and owned Redis fences. Crash recovery
// never calls this method: missing post-arm evidence is reconciliation.
func (r *PostgreSQLRepository) ReleaseRejectedArm(
	ctx context.Context,
	organizationID, ledgerID, originID, reverseID uuid.UUID,
) (bool, error) {
	db, err := r.getDB(ctx)
	if err != nil {
		return false, err
	}

	result, err := db.ExecContext(ctx, `
		DELETE FROM transaction_revert_claim
		WHERE organization_id = $1 AND ledger_id = $2
		  AND origin_transaction_id = $3 AND reverse_transaction_id = $4
		  AND state = 'ARMED'`,
		organizationID, ledgerID, originID, reverseID,
	)
	if err != nil {
		return false, fmt.Errorf("release definitively rejected armed revert claim: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read definitively rejected armed revert claim release result: %w", err)
	}

	return rows == 1, nil
}
