// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/tracerreservation"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// BeginExecution fences dispatch before the caller invokes accounting exactly
// once. Fail-open concerns Tracer availability, never uncertain journal commits.
func (c *ContextTracerCoordinator) BeginExecution(ctx context.Context, attempt ContextTracerAttempt) error {
	if attempt.Skipped || !attempt.IntentAttempted {
		return nil
	}

	if !attempt.Frozen {
		return constant.ErrTracerContractUnavailable
	}

	ctx, cancel := context.WithTimeout(ctx, c.recovery.config.AttemptTimeout)
	defer cancel()

	if err := c.recovery.store.BeginExecution(ctx, attempt.Key, c.recovery.now().UTC()); err != nil {
		return fmt.Errorf("fence tracer accounting dispatch: %w", err)
	}

	return nil
}

// BeginBatchExecution acquires every participating obligation atomically before
// the one batch engine call. It cannot partially authorize accounting dispatch.
func (c *ContextTracerCoordinator) BeginBatchExecution(ctx context.Context, attempts []ContextTracerAttempt) error {
	var keys []tracerreservation.Key

	for _, attempt := range attempts {
		if attempt.Skipped || !attempt.IntentAttempted {
			continue
		}

		if !attempt.Frozen {
			return constant.ErrTracerContractUnavailable
		}

		if len(keys) >= c.recovery.config.MaxBatch {
			return constant.ErrInvalidRequestBody
		}

		keys = append(keys, attempt.Key)
	}

	if len(keys) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, c.recovery.config.AttemptTimeout)
	defer cancel()

	if err := c.recovery.store.BeginExecutions(ctx, keys, c.recovery.now().UTC()); err != nil {
		return fmt.Errorf("fence tracer batch dispatch: %w", err)
	}

	return nil
}

// Conclude records a proven terminal outcome for asynchronous delivery. A
// canceled request cannot discard the obligation after accounting has run.
// Unknown accounting outcomes must never call this method: recovery reads proof.
func (c *ContextTracerCoordinator) Conclude(ctx context.Context, attempt ContextTracerAttempt, outcome tracerreservation.State) error {
	if !attempt.Frozen {
		return nil
	}

	if !outcome.Terminal() {
		return constant.ErrInvalidRequestBody
	}
	// Preserve the tenant and trace while bounding persistence independently of
	// the caller, whose deadline may have elapsed after the accounting commit.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.recovery.config.AttemptTimeout)
	defer cancel()

	if err := c.recovery.store.SetOutcome(ctx, attempt.Key, outcome, c.recovery.now().UTC()); err != nil {
		return fmt.Errorf("persist tracer accounting outcome: %w", err)
	}

	return nil
}

// CompleteExisting settles create-time participation independently of current
// settings. Lookup failures are handled by durable recovery, never by falling
// through to a legacy API that cannot close a context evaluation atomically.
func (c *ContextTracerCoordinator) CompleteExisting(ctx context.Context, key tracerreservation.Key, outcome tracerreservation.State) (bool, error) {
	if !outcome.Terminal() {
		return true, constant.ErrInvalidRequestBody
	}
	// Accounting has already completed; preserve tenant/trace with a separate
	// bounded persistence budget even if the original request was canceled.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.recovery.config.AttemptTimeout)
	defer cancel()

	record, err := c.recovery.store.Find(ctx, key)
	if err != nil {
		return true, fmt.Errorf("find pending tracer obligation: %w", err)
	}

	if record == nil {
		return false, nil
	}

	if record.Key != key {
		return true, constant.ErrTracerContractUnavailable
	}

	if err := c.recovery.validatePending(ctx, *record); err != nil {
		return true, err
	}

	if err := c.recovery.store.SetOutcome(ctx, key, outcome, c.recovery.now().UTC()); err != nil {
		return true, fmt.Errorf("persist pending tracer outcome: %w", err)
	}

	return true, nil
}
