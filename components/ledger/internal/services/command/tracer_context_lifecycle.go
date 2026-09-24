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
