// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracerobligation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability/v4"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/tracerreservation"
)

// ReadAccountingStatus reads a durable, scoped projection on the primary. Missing
// rows and pending/unknown statuses are returned without manufacturing an abort.
// The transaction completer atomically persists the transaction and its operations;
// this reader cannot call the engine or repair a projection by changing balances.
func (r *Repository) ReadAccountingStatus(ctx context.Context, key tracerreservation.Key) (_ string, retErr error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	// Nest the repository work under its semantic coordination span.
	ctx, span := tracer.Start(ctx, "postgres.read_tracer_accounting_evidence")
	defer span.End()
	defer func() { finish(span, retErr) }()

	if err := key.Validate(); err != nil {
		return "", err
	}

	tx, err := r.begin(ctx)
	if err != nil {
		return "", err
	}

	defer func() { _ = tx.Rollback() }()

	var status string

	err = tx.QueryRowContext(ctx, `SELECT CASE WHEN octet_length(status)<=32 THEN status END FROM transaction WHERE id=$1 AND organization_id=$2 AND ledger_id=$3`, key.TransactionID, key.OrganizationID, key.LedgerID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}

	if err != nil {
		return "", fmt.Errorf("read durable accounting status: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("finish accounting evidence read: %w", err)
	}

	return status, nil
}
