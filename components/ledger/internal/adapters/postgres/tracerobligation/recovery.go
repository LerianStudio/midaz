// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracerobligation

import (
	"context"
	"fmt"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/tracerreservation"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// ClaimDue schedules the next attempt before returning bounded, body-free work.
// SKIP LOCKED lets workers share a tenant; retry after a crash or lost remote
// acknowledgement is intentional. It never assigns an accounting outcome.
func (r *Repository) ClaimDue(ctx context.Context, now, nextAttempt time.Time, limit int) (_ []tracerreservation.Pending, retErr error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	// Nest the repository work under its semantic coordination span.
	ctx, span := tracer.Start(ctx, "postgres.claim_tracer_recovery")
	defer span.End()
	defer func() { finish(span, retErr) }()

	if now.IsZero() || !nextAttempt.After(now) || limit <= 0 || limit > r.maxBatch {
		return nil, constant.ErrInvalidRequestBody
	}

	tx, err := r.begin(ctx)
	if err != nil {
		return nil, err
	}

	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `WITH due AS (
 SELECT organization_id,ledger_id,transaction_id FROM tracer_reservation_obligation
 WHERE tenant_id=$1 AND delivered_at IS NULL AND NOT recovery_quarantined AND next_attempt_at<=$2
 AND (state<>'PREPARED' OR prepare_deadline<=$2)
 ORDER BY (state IN ('CONFIRMED','RELEASED')) DESC,next_attempt_at,organization_id,ledger_id,transaction_id
 LIMIT $4 FOR UPDATE SKIP LOCKED
 ) UPDATE tracer_reservation_obligation o SET next_attempt_at=$3,
 recovery_attempts=LEAST(o.recovery_attempts::bigint+1,2147483647)::integer FROM due
 WHERE o.organization_id=due.organization_id AND o.ledger_id=due.ledger_id AND o.transaction_id=due.transaction_id
 RETURNING o.organization_id,o.ledger_id,o.transaction_id,o.execution_id,o.tenant_id,o.integration_id,o.asset_namespace,o.contract_revision,o.state,o.prepare_deadline,o.created_at,o.recovery_attempts`, tmcore.GetTenantIDContext(ctx), now.UTC(), nextAttempt.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("claim tracer recovery: %w", err)
	}
	defer rows.Close()

	pending := []tracerreservation.Pending{}

	for rows.Next() {
		var entry tracerreservation.Pending
		if err := rows.Scan(&entry.Key.OrganizationID, &entry.Key.LedgerID, &entry.Key.TransactionID, &entry.ExecutionID, &entry.Scope.TenantID, &entry.Scope.IntegrationID, &entry.Scope.AssetNamespace, &entry.ContractRevision, &entry.State, &entry.PrepareDeadline, &entry.CreatedAt, &entry.RecoveryAttempts); err != nil {
			return nil, fmt.Errorf("read tracer recovery: %w", err)
		}

		entry.Scope.SingleTenant = !r.requireTenant
		pending = append(pending, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tracer recovery: %w", err)
	}

	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close tracer recovery: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tracer recovery claim: %w", err)
	}

	return pending, nil
}

// ScheduleRetry cannot postpone a newer claim or an outcome recorded since this
// attempt. Quarantine preserves the obligation and its financial state.
func (r *Repository) ScheduleRetry(ctx context.Context, record tracerreservation.Pending, next time.Time, quarantine bool) (retErr error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.schedule_tracer_recovery")
	defer span.End()
	defer func() { finish(span, retErr) }()

	if err := record.Key.Validate(); err != nil {
		return err
	}

	if next.IsZero() || record.RecoveryAttempts <= 0 {
		return constant.ErrInvalidRequestBody
	}

	tx, err := r.begin(ctx)
	if err != nil {
		return err
	}

	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `UPDATE tracer_reservation_obligation
 SET next_attempt_at=$5,recovery_quarantined=$6
 WHERE organization_id=$1 AND ledger_id=$2 AND transaction_id=$3 AND tenant_id=$4
 AND delivered_at IS NULL AND recovery_attempts=$7 AND state=$8`, record.Key.OrganizationID, record.Key.LedgerID, record.Key.TransactionID, tmcore.GetTenantIDContext(ctx), next.UTC(), quarantine, record.RecoveryAttempts, record.State)
	if err != nil {
		return fmt.Errorf("schedule tracer recovery: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tracer retry schedule: %w", err)
	}

	return nil
}
