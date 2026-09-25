// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracerobligation

import (
	"context"
	"database/sql"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/bxcodec/dbresolver/v2"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// HasUndelivered checks the transaction primary without loading financial facts.
// An absent journal is valid before migration; a failed query is never proof
// of drainage. The check includes every state and owner in this tenant database.
func HasUndelivered(ctx context.Context, database dbresolver.DB) (_ bool, retErr error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.check_tracer_drain")
	defer span.End()
	defer func() { finish(span, retErr) }()

	if database == nil {
		return false, constant.ErrTracerContractUnavailable
	}

	tx, err := database.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return false, fmt.Errorf("begin tracer drain check: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT to_regclass('tracer_reservation_obligation') IS NOT NULL`).Scan(&exists); err != nil {
		return false, fmt.Errorf("locate tracer journal: %w", err)
	}

	if !exists {
		return false, nil
	}

	var pending bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM tracer_reservation_obligation WHERE delivered_at IS NULL)`).Scan(&pending); err != nil {
		return false, fmt.Errorf("check tracer obligations: %w", err)
	}

	return pending, nil
}
