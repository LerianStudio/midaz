// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracerobligation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/tracerreservation"
)

// Find reads only coordination identity on the tenant primary. Absence permits
// an explicit legacy completion path; a failed lookup never does.
func (r *Repository) Find(ctx context.Context, key tracerreservation.Key) (_ *tracerreservation.Pending, retErr error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)
	// Nest the repository work under its semantic coordination span.
	ctx, span := tracer.Start(ctx, "postgres.find_tracer_obligation")
	defer span.End()
	defer func() { finish(span, retErr) }()

	if err := key.Validate(); err != nil {
		return nil, err
	}

	tx, err := r.begin(ctx)
	if err != nil {
		return nil, err
	}

	defer func() { _ = tx.Rollback() }()

	pending := &tracerreservation.Pending{Key: key}

	err = tx.QueryRowContext(ctx, `SELECT execution_id,tenant_id,integration_id,asset_namespace,contract_revision,state,prepare_deadline,created_at
 FROM tracer_reservation_obligation WHERE organization_id=$1 AND ledger_id=$2 AND transaction_id=$3 AND tenant_id=$4`, key.OrganizationID, key.LedgerID, key.TransactionID, tmcore.GetTenantIDContext(ctx)).Scan(&pending.ExecutionID, &pending.Scope.TenantID, &pending.Scope.IntegrationID, &pending.Scope.AssetNamespace, &pending.ContractRevision, &pending.State, &pending.PrepareDeadline, &pending.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("find tracer coordination identity: %w", err)
	}

	pending.Scope.SingleTenant = !r.requireTenant

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("finish tracer identity lookup: %w", err)
	}

	return pending, nil
}
