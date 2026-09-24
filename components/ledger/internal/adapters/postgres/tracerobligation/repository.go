// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package tracerobligation persists recoverable Tracer coordination on the
// transaction tenant primary. No method can execute or retry accounting.
package tracerobligation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/bxcodec/dbresolver/v2"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/tracerreservation"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

type Repository struct {
	connection    *libPostgres.Client
	config        tracerreservation.Config
	requireTenant bool
	maxBatch      int
}

func NewRepository(connection *libPostgres.Client, cfg tracerreservation.Config, requireTenant bool, maxBatch int) (*Repository, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	if maxBatch <= 0 {
		return nil, constant.ErrInvalidRequestBody
	}

	return &Repository{connection: connection, config: cfg, requireTenant: requireTenant, maxBatch: maxBatch}, nil
}

func (r *Repository) database(ctx context.Context) (dbresolver.DB, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if r.requireTenant && tmcore.GetTenantIDContext(ctx) == "" {
		return nil, constant.ErrInternalServer
	}

	if db := tmcore.GetPGContext(ctx, constant.ModuleTransaction); db != nil {
		return db, nil
	}

	if db := tmcore.GetPGContext(ctx); db != nil {
		return db, nil
	}

	if r.requireTenant || r.connection == nil {
		return nil, constant.ErrInternalServer
	}

	return r.connection.Resolver(ctx)
}

func (r *Repository) begin(ctx context.Context) (dbresolver.Tx, error) {
	database, err := r.database(ctx)
	if err != nil {
		return nil, err
	}

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tracer coordination: %w", err)
	}

	return tx, nil
}

func finish(span trace.Span, err error) {
	if err == nil {
		return
	}

	classified := err
	for errors.Unwrap(classified) != nil {
		classified = errors.Unwrap(classified)
	}

	if pkg.IsBusinessError(pkg.ValidateBusinessError(classified, "TracerObligation")) {
		libOtel.HandleSpanBusinessErrorEvent(span, "Tracer coordination rejected", err)
		return
	}

	libOtel.HandleSpanError(span, "Tracer coordination persistence failed", err)
}

// Prepare commits the exact intent before its caller may send Reserve. A
// repeated identity returns the original intent; it never extends its deadline.
func (r *Repository) Prepare(ctx context.Context, intent tracerreservation.Intent) (_ *tracerreservation.Record, retErr error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	// Nest the repository work under its semantic coordination span.
	ctx, span := tracer.Start(ctx, "postgres.prepare_tracer_obligation")
	defer span.End()
	defer func() { finish(span, retErr) }()

	if err := intent.Validate(ctx, r.config); err != nil {
		return nil, err
	}

	if intent.Scope.TenantID != tmcore.GetTenantIDContext(ctx) {
		return nil, constant.ErrInvalidRequestBody
	}

	tx, err := r.begin(ctx)
	if err != nil {
		return nil, err
	}

	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `INSERT INTO tracer_reservation_obligation
 (organization_id,ledger_id,transaction_id,execution_id,tenant_id,integration_id,asset_namespace,contract_revision,fingerprint,payload,created_at,prepare_deadline,updated_at,next_attempt_at)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$11,$12) ON CONFLICT DO NOTHING`,
		intent.Key.OrganizationID, intent.Key.LedgerID, intent.Key.TransactionID, intent.ExecutionID, intent.Scope.TenantID, intent.Scope.IntegrationID, intent.Scope.AssetNamespace, tracercontract.ReserveContractRevision, intent.Fingerprint[:], intent.Payload, intent.CreatedAt, intent.PrepareDeadline)
	if err != nil {
		return nil, fmt.Errorf("persist tracer intent: %w", err)
	}

	stored, err := r.readIntent(ctx, tx, intent.Key)
	if err != nil {
		return nil, err
	}

	if stored.Intent.Fingerprint != intent.Fingerprint || stored.Intent.ExecutionID != intent.ExecutionID || stored.Intent.Scope.IntegrationID != intent.Scope.IntegrationID || stored.Intent.Scope.AssetNamespace != intent.Scope.AssetNamespace {
		return nil, constant.ErrReserveDecisionConflict
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tracer intent: %w", err)
	}

	return stored, nil
}

func (r *Repository) readIntent(ctx context.Context, tx dbresolver.Tx, key tracerreservation.Key) (*tracerreservation.Record, error) {
	record := &tracerreservation.Record{Intent: tracerreservation.Intent{Key: key}}

	var (
		fingerprint []byte
		revision    string
	)

	err := tx.QueryRowContext(ctx, `SELECT execution_id,tenant_id,integration_id,asset_namespace,contract_revision,fingerprint,
 CASE WHEN octet_length(payload)<=$5 THEN payload END,created_at,prepare_deadline,state,updated_at,delivered_at
 FROM tracer_reservation_obligation WHERE organization_id=$1 AND ledger_id=$2 AND transaction_id=$3 AND tenant_id=$4`,
		key.OrganizationID, key.LedgerID, key.TransactionID, tmcore.GetTenantIDContext(ctx), r.config.MaxBodyBytes).Scan(
		&record.Intent.ExecutionID, &record.Intent.Scope.TenantID, &record.Intent.Scope.IntegrationID, &record.Intent.Scope.AssetNamespace, &revision, &fingerprint,
		&record.Intent.Payload, &record.Intent.CreatedAt, &record.Intent.PrepareDeadline, &record.State, &record.UpdatedAt, &record.DeliveredAt,
	)
	if err != nil {
		return nil, fmt.Errorf("read tracer intent on primary: %w", err)
	}

	if len(fingerprint) != len(record.Intent.Fingerprint) || revision != tracercontract.ReserveContractRevision {
		return nil, constant.ErrInvalidRequestBody
	}

	copy(record.Intent.Fingerprint[:], fingerprint)

	record.Intent.Scope.SingleTenant = !r.requireTenant
	if err := record.Intent.Validate(ctx, r.config); err != nil {
		return nil, err
	}

	return record, nil
}

// BeginExecution is a one-time CAS. A replay or expired/preempted intent cannot
// dispatch accounting. An indeterminate commit is returned, never retried.
func (r *Repository) BeginExecution(ctx context.Context, key tracerreservation.Key, now time.Time) (retErr error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	// Nest the repository work under its semantic coordination span.
	ctx, span := tracer.Start(ctx, "postgres.start_tracer_accounting")
	defer span.End()
	defer func() { finish(span, retErr) }()

	if err := key.Validate(); err != nil {
		return err
	}

	if now.IsZero() {
		return constant.ErrInvalidRequestBody
	}

	tx, err := r.begin(ctx)
	if err != nil {
		return err
	}

	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `UPDATE tracer_reservation_obligation SET state='EXECUTING',updated_at=GREATEST(updated_at,$5)
 WHERE organization_id=$1 AND ledger_id=$2 AND transaction_id=$3 AND tenant_id=$4 AND state='PREPARED' AND prepare_deadline>$5`,
		key.OrganizationID, key.LedgerID, key.TransactionID, tmcore.GetTenantIDContext(ctx), now.UTC())
	if err != nil {
		return fmt.Errorf("acquire accounting dispatch: %w", err)
	}

	if err := requireOne(result); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit accounting dispatch: %w", err)
	}

	return nil
}

// SetOutcome accepts only proven terminal results. The caller owns the proof;
// neither an elapsed lease nor an absent receipt is accepted as such evidence.
func (r *Repository) SetOutcome(ctx context.Context, key tracerreservation.Key, outcome tracerreservation.State, now time.Time) (retErr error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	// Nest the repository work under its semantic coordination span.
	ctx, span := tracer.Start(ctx, "postgres.set_tracer_outcome")
	defer span.End()
	defer func() { finish(span, retErr) }()

	if err := key.Validate(); err != nil {
		return err
	}

	if !outcome.Terminal() || now.IsZero() {
		return constant.ErrInvalidRequestBody
	}

	tx, err := r.begin(ctx)
	if err != nil {
		return err
	}

	defer func() { _ = tx.Rollback() }()

	var state tracerreservation.State

	err = tx.QueryRowContext(ctx, `SELECT state FROM tracer_reservation_obligation WHERE organization_id=$1 AND ledger_id=$2 AND transaction_id=$3 AND tenant_id=$4 FOR UPDATE`, key.OrganizationID, key.LedgerID, key.TransactionID, tmcore.GetTenantIDContext(ctx)).Scan(&state)
	if err != nil {
		return fmt.Errorf("lock tracer outcome: %w", err)
	}

	if !state.CanTransition(outcome) {
		return constant.ErrReserveOperationConflict
	}

	if state != outcome {
		_, err = tx.ExecContext(ctx, `UPDATE tracer_reservation_obligation SET state=$5,updated_at=GREATEST(updated_at,$6),next_attempt_at=$6
 WHERE organization_id=$1 AND ledger_id=$2 AND transaction_id=$3 AND tenant_id=$4`, key.OrganizationID, key.LedgerID, key.TransactionID, tmcore.GetTenantIDContext(ctx), outcome, now.UTC())
		if err != nil {
			return fmt.Errorf("persist tracer outcome: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tracer outcome: %w", err)
	}

	return nil
}

func (r *Repository) ExpirePrepared(ctx context.Context, key tracerreservation.Key, now time.Time) (_ bool, retErr error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	// Nest the repository work under its semantic coordination span.
	ctx, span := tracer.Start(ctx, "postgres.expire_tracer_preparation")
	defer span.End()
	defer func() { finish(span, retErr) }()

	if err := key.Validate(); err != nil {
		return false, err
	}

	if now.IsZero() {
		return false, constant.ErrInvalidRequestBody
	}

	tx, err := r.begin(ctx)
	if err != nil {
		return false, err
	}

	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `UPDATE tracer_reservation_obligation SET state='RELEASED',updated_at=GREATEST(updated_at,$5),next_attempt_at=$5
 WHERE organization_id=$1 AND ledger_id=$2 AND transaction_id=$3 AND tenant_id=$4 AND state='PREPARED' AND prepare_deadline<=$5`, key.OrganizationID, key.LedgerID, key.TransactionID, tmcore.GetTenantIDContext(ctx), now.UTC())
	if err != nil {
		return false, fmt.Errorf("fence abandoned tracer preparation: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("count fenced tracer preparations: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit tracer preparation expiry: %w", err)
	}

	return count == 1, nil
}

func (r *Repository) MarkDelivered(ctx context.Context, key tracerreservation.Key, outcome tracerreservation.State, now time.Time) (retErr error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	// Nest the repository work under its semantic coordination span.
	ctx, span := tracer.Start(ctx, "postgres.acknowledge_tracer_delivery")
	defer span.End()
	defer func() { finish(span, retErr) }()

	if err := key.Validate(); err != nil {
		return err
	}

	if !outcome.Terminal() || now.IsZero() {
		return constant.ErrInvalidRequestBody
	}

	tx, err := r.begin(ctx)
	if err != nil {
		return err
	}

	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `UPDATE tracer_reservation_obligation SET delivered_at=COALESCE(delivered_at,$6),updated_at=GREATEST(updated_at,$6)
 WHERE organization_id=$1 AND ledger_id=$2 AND transaction_id=$3 AND tenant_id=$4 AND state=$5`, key.OrganizationID, key.LedgerID, key.TransactionID, tmcore.GetTenantIDContext(ctx), outcome, now.UTC())
	if err != nil {
		return fmt.Errorf("acknowledge tracer delivery: %w", err)
	}

	if err := requireOne(result); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tracer acknowledgement: %w", err)
	}

	return nil
}

func requireOne(result sql.Result) error {
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count tracer coordination updates: %w", err)
	}

	if count != 1 {
		return constant.ErrReserveOperationConflict
	}

	return nil
}
