// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"

	pgdb "github.com/LerianStudio/midaz/v3/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/logging"
)

// executeInTx runs fn inside a database transaction.
//
// Behavior:
//   - Commits when fn returns nil.
//   - Rolls back when fn returns a non-nil error (the returned error is passed
//     through unchanged so callers can errors.Is/As against sentinels).
//   - Rolls back when fn panics, and returns the panic as a wrapped error.
//     Never re-raises: CLAUDE.md forbids panic propagation in production code.
//   - Wraps begin/commit failures with %w.
//
// Non-goals:
//   - This helper does not emit its own tracing span; callers already span
//     their own operations. Placing a span here would nest under the caller's
//     span with an ambiguous name.
//   - fn SHOULD be short-lived and contain only database operations. Remote
//     calls, CEL evaluation, or heavy computation inside fn hold row locks
//     for the duration and pressure the connection pool.
//   - Callers are responsible for honoring ctx cancellation inside fn; the
//     helper relies on the driver to propagate ctx through BeginTx, ExecContext,
//     Commit, and Rollback.
//
// Uses the defer-based rollback pattern; rollback errors are logged at WARN
// via libObservability.NewTrackingFromContext so the helper does not require a
// logger parameter.
func executeInTx(ctx context.Context, txBeginner pgdb.TxBeginner, fn func(pgdb.DB) error) (err error) {
	// Guard against nil txBeginner before dereferencing. pgdb.NewTxBeginnerAdapter
	// returns nil when given a nil dbresolver.DB, and tests may omit wiring; a
	// direct BeginTx call on a nil interface value would panic before any of the
	// downstream guards could fire.
	if txBeginner == nil {
		return pgdb.ErrNilConnection
	}

	tx, beginErr := txBeginner.BeginTx(ctx, nil)
	if beginErr != nil {
		return fmt.Errorf("failed to begin transaction: %w", beginErr)
	}

	// Defensive guard: BeginTx contract should never return (nil, nil), but
	// mock TxBeginner implementations could. This prevents a nil-interface
	// panic from propagating into every *WithTx consumer.
	if tx == nil {
		return fmt.Errorf("tx_helper: BeginTx returned nil transaction without error")
	}

	committed := false

	defer func() {
		// Recover any panic from fn(tx) and convert it into an error.
		// Never re-raise: CLAUDE.md forbids panic propagation in production code.
		if recovered := recover(); recovered != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				logger, _, _, _ := libObservability.NewTrackingFromContext(ctx)
				logger = logging.WithTrace(ctx, logger)
				logger.With(
					libLog.String("operation", "tx_helper.rollback"),
					libLog.String("error.message", rbErr.Error()),
				).Log(ctx, libLog.LevelWarn, "Failed to rollback transaction after panic")
			}

			if recoveredErr, ok := recovered.(error); ok {
				err = fmt.Errorf("transaction callback panicked: %w", recoveredErr)
			} else {
				err = fmt.Errorf("transaction callback panicked: %v", recovered)
			}

			return
		}

		if committed {
			return
		}

		if rbErr := tx.Rollback(); rbErr != nil {
			// Log at warn instead of shadowing the primary error. We obtain
			// the logger via tracking context to match the canonical
			// validation_service.go pattern without changing the helper's
			// signature.
			logger, _, _, _ := libObservability.NewTrackingFromContext(ctx)
			logger = logging.WithTrace(ctx, logger)
			logger.With(
				libLog.String("operation", "tx_helper.rollback"),
				libLog.String("error.message", rbErr.Error()),
			).Log(ctx, libLog.LevelWarn, "Failed to rollback transaction")
		}
	}()

	if err = fn(tx); err != nil {
		return err
	}

	if commitErr := tx.Commit(); commitErr != nil {
		return fmt.Errorf("failed to commit transaction: %w", commitErr)
	}

	committed = true

	return nil
}
