// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package accountregistration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v4/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/Masterminds/squirrel"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// tableName is the PostgreSQL table backing the AccountRegistration entity.
const tableName = "account_registration"

// columnList is the canonical column order used by SELECT and INSERT statements.
// Keeping it in one place prevents drift between the two; Scan() order must mirror it.
var columnList = []string{
	"id",
	"organization_id",
	"ledger_id",
	"holder_id",
	"idempotency_key",
	"request_hash",
	"account_id",
	"crm_alias_id",
	"status",
	"failure_code",
	"failure_message",
	"retry_count",
	"next_retry_at",
	"claimed_by",
	"claimed_at",
	"last_recovered_at",
	"created_at",
	"updated_at",
	"completed_at",
}

// Repository is the persistence boundary for the AccountRegistration saga state. It is
// intentionally small: the saga orchestrator (Phase 4) is the only caller that knows
// when each transition happens. Multi-statement operations (like claiming a row for
// recovery) are left to the orchestrator and executed via the Postgres connection
// resolved from context.
//
// UpsertByIdempotencyKey is the saga entry point: the saga hashes the canonical request
// body and calls this method to either create a new registration or look up an existing
// one for the same idempotency key. If the stored hash matches, the registration is
// returned for replay; if the stored hash differs, ErrAccountRegistrationIdempotencyConflict
// is returned so the caller can fail the request.
type Repository interface {
	// UpsertByIdempotencyKey inserts a new registration when none exists for the tuple
	// (OrganizationID, LedgerID, IdempotencyKey). If one already exists with the same
	// RequestHash, it is returned for replay and wasCreated is false. If one exists with
	// a different RequestHash, an ErrAccountRegistrationIdempotencyConflict business
	// error is returned.
	//
	// The caller is responsible for populating reg.ID, reg.CreatedAt, reg.UpdatedAt, and
	// reg.Status before calling; this method does not defaulting. Per project rule 16,
	// the caller should capture time.Now().UTC() once and reuse it for both timestamps.
	UpsertByIdempotencyKey(ctx context.Context, reg *mmodel.AccountRegistration) (registration *mmodel.AccountRegistration, wasCreated bool, err error)

	// FindByID returns the registration identified by (organizationID, ledgerID, id), or
	// ErrAccountRegistrationNotFound if none exists.
	FindByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.AccountRegistration, error)

	// UpdateStatus transitions the registration to the given status, bumping updated_at.
	// Additional column mutations can be applied via the variadic StatusMutator callbacks
	// (for example, setting failure_code and failure_message when transitioning to
	// FAILED_TERMINAL).
	UpdateStatus(ctx context.Context, id uuid.UUID, status mmodel.AccountRegistrationStatus, mutators ...StatusMutator) error

	// AttachAccount sets account_id on the registration and refreshes updated_at.
	AttachAccount(ctx context.Context, id, accountID uuid.UUID) error

	// AttachCRMAlias sets crm_alias_id on the registration and refreshes updated_at.
	AttachCRMAlias(ctx context.Context, id, aliasID uuid.UUID) error

	// MarkCompleted sets status=COMPLETED, completed_at, and updated_at in a single
	// statement.
	MarkCompleted(ctx context.Context, id uuid.UUID, completedAt time.Time) error

	// MarkFailed sets status, failure_code, failure_message, and updated_at in a single
	// statement. Status must be one of FAILED_RETRYABLE, FAILED_TERMINAL, or
	// COMPENSATED.
	MarkFailed(ctx context.Context, id uuid.UUID, status mmodel.AccountRegistrationStatus, code, message string) error
}

// StatusMutator is a callback that applies additional column updates when transitioning
// an AccountRegistration to a new status. It is invoked against the squirrel.UpdateBuilder
// immediately after status and updated_at have been set, so the callback can add Set()
// calls for columns specific to the target transition (for example, setting next_retry_at
// when transitioning to FAILED_RETRYABLE).
type StatusMutator func(builder squirrel.UpdateBuilder) squirrel.UpdateBuilder

// AccountRegistrationPostgreSQLRepository is the Postgres-backed implementation of
// Repository. It honors the multi-tenant DB resolution pattern from the sibling
// /postgres/account package: in multi-tenant mode the middleware injects a tenant-scoped
// dbresolver.DB into context; in single-tenant mode the static connection is used.
type AccountRegistrationPostgreSQLRepository struct {
	connection    *libPostgres.Client
	requireTenant bool
}

// NewAccountRegistrationPostgreSQLRepository constructs the repository.
// Passing requireTenant=true makes the repository refuse to fall back to the static
// connection when no tenant context is available — use that in multi-tenant deployments
// to fail-fast on missing tenant context.
func NewAccountRegistrationPostgreSQLRepository(pc *libPostgres.Client, requireTenant ...bool) *AccountRegistrationPostgreSQLRepository {
	r := &AccountRegistrationPostgreSQLRepository{
		connection: pc,
	}

	if len(requireTenant) > 0 {
		r.requireTenant = requireTenant[0]
	}

	return r
}

// UpsertByIdempotencyKey implements the saga entry point.
//
// The method uses ON CONFLICT DO NOTHING on the (organization_id, ledger_id, idempotency_key)
// unique index. If the insert takes effect (RowsAffected == 1), the provided registration
// is returned as-created. If it does not (row exists), a follow-up SELECT resolves the
// existing row and compares hashes: same hash → replay; different hash → conflict.
func (r *AccountRegistrationPostgreSQLRepository) UpsertByIdempotencyKey(ctx context.Context, reg *mmodel.AccountRegistration) (*mmodel.AccountRegistration, bool, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.account_registration.upsert_by_idempotency_key")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get database connection",
			libLog.Err(err))

		return nil, false, err
	}

	record := &AccountRegistrationPostgreSQLModel{}
	record.FromEntity(reg)

	builder := squirrel.Insert(tableName).
		Columns(columnList...).
		Values(
			record.ID,
			record.OrganizationID,
			record.LedgerID,
			record.HolderID,
			record.IdempotencyKey,
			record.RequestHash,
			record.AccountID,
			record.CRMAliasID,
			record.Status,
			record.FailureCode,
			record.FailureMessage,
			record.RetryCount,
			record.NextRetryAt,
			record.ClaimedBy,
			record.ClaimedAt,
			record.LastRecoveredAt,
			record.CreatedAt,
			record.UpdatedAt,
			record.CompletedAt,
		).
		Suffix("ON CONFLICT (organization_id, ledger_id, idempotency_key) DO NOTHING").
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := builder.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build insert query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to build insert query",
			libLog.Err(err))

		return nil, false, err
	}

	_, spanExec := tracer.Start(ctx, "postgres.account_registration.upsert.exec")

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute upsert", err)
		spanExec.End()

		logger.Log(ctx, libLog.LevelError, "Failed to execute account_registration upsert",
			libLog.Err(err))

		return nil, false, fmt.Errorf("account_registration upsert: %w", err)
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to read rows affected", err)

		logger.Log(ctx, libLog.LevelError, "Failed to read rows affected on account_registration upsert",
			libLog.Err(err))

		return nil, false, fmt.Errorf("account_registration upsert rows_affected: %w", err)
	}

	if rowsAffected == 1 {
		return record.ToEntity(), true, nil
	}

	// No rows inserted — an existing row already owns the idempotency key. Load it and
	// compare the request hash to decide replay vs conflict.
	existing, err := r.findByIdempotencyKey(ctx, reg.OrganizationID, reg.LedgerID, reg.IdempotencyKey)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to load existing registration for idempotency replay", err)

		return nil, false, err
	}

	if existing.RequestHash != reg.RequestHash {
		businessErr := pkg.ValidateBusinessError(constant.ErrAccountRegistrationIdempotencyConflict, constant.EntityAccountRegistration)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Idempotency key reused with different request body", businessErr)

		logger.Log(ctx, libLog.LevelWarn, "Idempotency key reused with different request body",
			libLog.String("idempotency_key", reg.IdempotencyKey),
			libLog.String("organization_id", reg.OrganizationID.String()),
			libLog.String("ledger_id", reg.LedgerID.String()))

		return nil, false, businessErr
	}

	return existing, false, nil
}

// FindByID implements Repository.
func (r *AccountRegistrationPostgreSQLRepository) FindByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.AccountRegistration, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.account_registration.find_by_id")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get database connection",
			libLog.Err(err))

		return nil, err
	}

	builder := squirrel.Select(columnList...).
		From(tableName).
		Where(squirrel.Eq{
			"organization_id": organizationID,
			"ledger_id":       ledgerID,
			"id":              id,
		}).
		Limit(1).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := builder.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build find query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to build find query",
			libLog.Err(err))

		return nil, err
	}

	reg, err := r.scanOne(ctx, db, query, args)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			businessErr := pkg.ValidateBusinessError(constant.ErrAccountRegistrationNotFound, constant.EntityAccountRegistration)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Account registration not found", businessErr)

			logger.Log(ctx, libLog.LevelWarn, "Account registration not found",
				libLog.String("organization_id", organizationID.String()),
				libLog.String("ledger_id", ledgerID.String()),
				libLog.String("id", id.String()))

			return nil, businessErr
		}

		libOpentelemetry.HandleSpanError(span, "Failed to scan account registration", err)

		logger.Log(ctx, libLog.LevelError, "Failed to scan account registration",
			libLog.Err(err))

		return nil, fmt.Errorf("account_registration find_by_id: %w", err)
	}

	return reg, nil
}

// UpdateStatus implements Repository. The StatusMutator callbacks run AFTER the status
// and updated_at assignments, so they can freely override them (for example, to set
// next_retry_at on a transition to FAILED_RETRYABLE).
func (r *AccountRegistrationPostgreSQLRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status mmodel.AccountRegistrationStatus, mutators ...StatusMutator) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.account_registration.update_status")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get database connection",
			libLog.Err(err))

		return err
	}

	builder := squirrel.Update(tableName).
		Set("status", string(status)).
		Set("updated_at", time.Now().UTC()).
		Where(squirrel.Eq{"id": id}).
		PlaceholderFormat(squirrel.Dollar)

	for _, mutate := range mutators {
		if mutate != nil {
			builder = mutate(builder)
		}
	}

	return r.execUpdate(ctx, span, db, builder, "update_status", id)
}

// AttachAccount implements Repository.
func (r *AccountRegistrationPostgreSQLRepository) AttachAccount(ctx context.Context, id, accountID uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.account_registration.attach_account")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get database connection",
			libLog.Err(err))

		return err
	}

	builder := squirrel.Update(tableName).
		Set("account_id", accountID).
		Set("updated_at", time.Now().UTC()).
		Where(squirrel.Eq{"id": id}).
		PlaceholderFormat(squirrel.Dollar)

	return r.execUpdate(ctx, span, db, builder, "attach_account", id)
}

// AttachCRMAlias implements Repository.
func (r *AccountRegistrationPostgreSQLRepository) AttachCRMAlias(ctx context.Context, id, aliasID uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.account_registration.attach_crm_alias")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get database connection",
			libLog.Err(err))

		return err
	}

	builder := squirrel.Update(tableName).
		Set("crm_alias_id", aliasID).
		Set("updated_at", time.Now().UTC()).
		Where(squirrel.Eq{"id": id}).
		PlaceholderFormat(squirrel.Dollar)

	return r.execUpdate(ctx, span, db, builder, "attach_crm_alias", id)
}

// MarkCompleted implements Repository.
func (r *AccountRegistrationPostgreSQLRepository) MarkCompleted(ctx context.Context, id uuid.UUID, completedAt time.Time) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.account_registration.mark_completed")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get database connection",
			libLog.Err(err))

		return err
	}

	builder := squirrel.Update(tableName).
		Set("status", string(mmodel.AccountRegistrationCompleted)).
		Set("completed_at", completedAt.UTC()).
		Set("updated_at", completedAt.UTC()).
		Where(squirrel.Eq{"id": id}).
		PlaceholderFormat(squirrel.Dollar)

	return r.execUpdate(ctx, span, db, builder, "mark_completed", id)
}

// MarkFailed implements Repository.
func (r *AccountRegistrationPostgreSQLRepository) MarkFailed(ctx context.Context, id uuid.UUID, status mmodel.AccountRegistrationStatus, code, message string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.account_registration.mark_failed")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get database connection",
			libLog.Err(err))

		return err
	}

	builder := squirrel.Update(tableName).
		Set("status", string(status)).
		Set("failure_code", code).
		Set("failure_message", message).
		Set("updated_at", time.Now().UTC()).
		Where(squirrel.Eq{"id": id}).
		PlaceholderFormat(squirrel.Dollar)

	return r.execUpdate(ctx, span, db, builder, "mark_failed", id)
}

// getDB resolves the Postgres connection for the current request. See the sibling
// postgres/account getDB() for the full multi-tenant contract.
func (r *AccountRegistrationPostgreSQLRepository) getDB(ctx context.Context) (dbresolver.DB, error) {
	if db := tmcore.GetPGContext(ctx, constant.ModuleOnboarding); db != nil {
		return db, nil
	}

	if db := tmcore.GetPGContext(ctx); db != nil {
		return db, nil
	}

	if r.requireTenant {
		return nil, errors.New("accountregistration: tenant postgres connection missing from context")
	}

	if r.connection == nil {
		return nil, errors.New("accountregistration: postgres connection not available")
	}

	return r.connection.Resolver(ctx)
}

// findByIdempotencyKey loads a registration by its (organization_id, ledger_id, idempotency_key)
// tuple. Unlike the exported Find* methods, this is an internal helper for the upsert
// replay path: a miss here after a failed insert would indicate a genuine race (deleted
// between INSERT attempt and SELECT) rather than user-visible not-found, so we let
// sql.ErrNoRows propagate to the caller which wraps it appropriately.
func (r *AccountRegistrationPostgreSQLRepository) findByIdempotencyKey(ctx context.Context, organizationID, ledgerID uuid.UUID, idempotencyKey string) (*mmodel.AccountRegistration, error) {
	db, err := r.getDB(ctx)
	if err != nil {
		return nil, err
	}

	builder := squirrel.Select(columnList...).
		From(tableName).
		Where(squirrel.Eq{
			"organization_id": organizationID,
			"ledger_id":       ledgerID,
			"idempotency_key": idempotencyKey,
		}).
		Limit(1).
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("account_registration find_by_idempotency build: %w", err)
	}

	return r.scanOne(ctx, db, query, args)
}

// scanOne executes a single-row SELECT and decodes it into a domain entity.
func (r *AccountRegistrationPostgreSQLRepository) scanOne(ctx context.Context, db dbresolver.DB, query string, args []any) (*mmodel.AccountRegistration, error) {
	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, spanQuery := tracer.Start(ctx, "postgres.account_registration.query")
	defer spanQuery.End()

	row := db.QueryRowContext(ctx, query, args...)

	record := &AccountRegistrationPostgreSQLModel{}
	if err := row.Scan(
		&record.ID,
		&record.OrganizationID,
		&record.LedgerID,
		&record.HolderID,
		&record.IdempotencyKey,
		&record.RequestHash,
		&record.AccountID,
		&record.CRMAliasID,
		&record.Status,
		&record.FailureCode,
		&record.FailureMessage,
		&record.RetryCount,
		&record.NextRetryAt,
		&record.ClaimedBy,
		&record.ClaimedAt,
		&record.LastRecoveredAt,
		&record.CreatedAt,
		&record.UpdatedAt,
		&record.CompletedAt,
	); err != nil {
		return nil, err
	}

	return record.ToEntity(), nil
}

// execUpdate executes an UPDATE statement and surfaces a business not-found error when
// zero rows were affected. The span passed in is the PARENT method span; this helper
// attaches child-span timing for the exec and annotates the parent on failure.
func (r *AccountRegistrationPostgreSQLRepository) execUpdate(
	ctx context.Context,
	span trace.Span,
	db dbresolver.DB,
	builder squirrel.UpdateBuilder,
	op string,
	id uuid.UUID,
) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	query, args, err := builder.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build "+op+" query", err)

		logger.Log(ctx, libLog.LevelError, "Failed to build account_registration update",
			libLog.String("op", op), libLog.Err(err))

		return err
	}

	_, spanExec := tracer.Start(ctx, "postgres.account_registration."+op+".exec")

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanExec, "Failed to execute "+op, err)
		spanExec.End()

		logger.Log(ctx, libLog.LevelError, "Failed to execute account_registration update",
			libLog.String("op", op), libLog.Err(err))

		return fmt.Errorf("account_registration %s: %w", op, err)
	}

	spanExec.End()

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to read rows affected for "+op, err)

		return fmt.Errorf("account_registration %s rows_affected: %w", op, err)
	}

	if rowsAffected == 0 {
		businessErr := pkg.ValidateBusinessError(constant.ErrAccountRegistrationNotFound, constant.EntityAccountRegistration)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Account registration not found for "+op, businessErr)

		logger.Log(ctx, libLog.LevelWarn, "Account registration not found for update",
			libLog.String("op", op),
			libLog.String("id", id.String()))

		return businessErr
	}

	return nil
}
