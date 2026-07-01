// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package transactionquarantine persists poison records evicted from the Redis
// backup queue into a durable PostgreSQL table. A poison record is the ONLY
// durable copy of an AUTHORIZED financial transaction that the backup consumer
// cannot replay; it MUST be persisted here before it is ever removed from Redis.
package transactionquarantine

import (
	"context"
	"fmt"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v5/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/Masterminds/squirrel"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// QuarantineRecord is the durable financial copy of a poison backup-queue
// record. Payload holds the raw backup JSON verbatim — it is the only surviving
// copy of an authorized transaction and MUST NOT be logged or attached to a span.
type QuarantineRecord struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	LedgerID       uuid.UUID
	TransactionID  uuid.UUID
	RedisKey       string
	Payload        []byte
	FailureReason  string
	Attempts       int
	FirstFailedAt  time.Time
	QuarantinedAt  time.Time
}

// Repository persists poison backup-queue records to PostgreSQL.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 --destination=transactionquarantine.postgresql_mock.go --package=transactionquarantine . Repository
type Repository interface {
	// Insert writes a quarantine record, returning nil if the record was stored
	// or already present. Idempotency is enforced via ON CONFLICT (redis_key)
	// DO NOTHING: a record quarantined across multiple consumer cycles lands
	// exactly once, and a no-op conflict is treated as success (confirmed
	// persistence) so the caller may safely delete the Redis copy.
	Insert(ctx context.Context, record *QuarantineRecord) error
}

// QuarantinePostgreSQLRepository is the PostgreSQL implementation of Repository.
type QuarantinePostgreSQLRepository struct {
	connection    *libPostgres.Client
	tableName     string
	requireTenant bool
}

// NewQuarantinePostgreSQLRepository returns a new repository using the given
// Postgres connection. Pass requireTenant=true in multi-tenant mode so the
// repository fails closed when no tenant connection is present in context.
func NewQuarantinePostgreSQLRepository(pc *libPostgres.Client, requireTenant ...bool) *QuarantinePostgreSQLRepository {
	r := &QuarantinePostgreSQLRepository{
		connection: pc,
		tableName:  "transaction_backup_quarantine",
	}
	if len(requireTenant) > 0 {
		r.requireTenant = requireTenant[0]
	}

	return r
}

// getDB resolves the PostgreSQL connection for the current request, mirroring
// the sibling adapters: a module-specific tenant connection takes precedence,
// then a generic tenant connection, then the static single-tenant connection.
func (r *QuarantinePostgreSQLRepository) getDB(ctx context.Context) (dbresolver.DB, error) {
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

func (r *QuarantinePostgreSQLRepository) Insert(ctx context.Context, record *QuarantineRecord) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.insert_transaction_backup_quarantine")
	defer span.End()

	// Span attributes carry scope IDs and the redis_key only — never the
	// payload, which is financial data (T9).
	span.SetAttributes(
		attribute.String("app.request.organization_id", record.OrganizationID.String()),
		attribute.String("app.request.ledger_id", record.LedgerID.String()),
		attribute.String("app.request.transaction_id", record.TransactionID.String()),
		attribute.String("app.request.redis_key", record.RedisKey),
	)

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return err
	}

	if record.ID == uuid.Nil {
		record.ID = uuid.New()
	}

	if record.QuarantinedAt.IsZero() {
		record.QuarantinedAt = time.Now()
	}

	insert := squirrel.Insert(r.tableName).
		Columns(
			"id",
			"organization_id",
			"ledger_id",
			"transaction_id",
			"redis_key",
			"payload",
			"failure_reason",
			"attempts",
			"first_failed_at",
			"quarantined_at",
		).
		Values(
			record.ID,
			record.OrganizationID,
			record.LedgerID,
			record.TransactionID,
			record.RedisKey,
			record.Payload,
			record.FailureReason,
			record.Attempts,
			record.FirstFailedAt,
			record.QuarantinedAt,
		).
		Suffix("ON CONFLICT (redis_key) DO NOTHING").
		PlaceholderFormat(squirrel.Dollar)

	query, args, err := insert.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build insert query", err)

		return err
	}

	_, spanExec := tracer.Start(ctx, "postgres.insert_transaction_backup_quarantine.exec")
	defer spanExec.End()

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanExec, "Failed to insert quarantine record", err)

		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(spanExec, "Failed to read rows affected", err)

		return err
	}

	spanExec.SetAttributes(attribute.Int64("db.rows_affected", rowsAffected))

	// rowsAffected == 0 means the record was already quarantined (ON CONFLICT
	// DO NOTHING). That is a successful, idempotent outcome: the durable copy
	// exists, so the caller may safely delete the Redis copy.
	logger.Log(ctx, libLog.LevelDebug, "Quarantine record persisted",
		libLog.String("redis_key", record.RedisKey),
		libLog.Int("attempts", record.Attempts),
	)

	return nil
}
