// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	transactionredis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
)

type revertRolloutTransition interface {
	InitializeFinancialDatasetGeneration(context.Context) error
	ValidatePrepared(context.Context) error
	Activate(context.Context) error
	MarkPhaseZeroDrained(context.Context) error
	Finalize(context.Context) error
}

func validateRevertRolloutConfiguration(configuredMode, configuredTarget, configuredGeneration string) error {
	mode := strings.ToLower(strings.TrimSpace(configuredMode))
	target := strings.ToLower(strings.TrimSpace(configuredTarget))
	generation := strings.TrimSpace(configuredGeneration)

	if mode != "legacy" && mode != "bridge" && mode != "final" {
		return fmt.Errorf("invalid REVERT_IDEMPOTENCY_MODE %q: expected legacy, bridge, or final", configuredMode)
	}
	if target != "" && target != transactionredis.RevertUpdateFreezeInitialize &&
		target != transactionredis.RevertUpdateFreezePrepared && target != transactionredis.RevertUpdateFreezeActive &&
		target != transactionredis.RevertUpdateFreezeDrained && target != transactionredis.RevertUpdateFreezeFinalized {
		return fmt.Errorf("invalid REVERT_ROLLOUT_TARGET %q: expected initialize, prepared, active, phase-zero-drained, finalized, or empty", configuredTarget)
	}

	compatible := target == "" ||
		((target == transactionredis.RevertUpdateFreezeInitialize || target == transactionredis.RevertUpdateFreezePrepared) && mode == "legacy") ||
		(target == transactionredis.RevertUpdateFreezeActive && (mode == "legacy" || mode == "bridge")) ||
		(target == transactionredis.RevertUpdateFreezeDrained && (mode == "bridge" || mode == "final")) ||
		(target == transactionredis.RevertUpdateFreezeFinalized && mode == "final")
	if !compatible {
		return fmt.Errorf("REVERT_ROLLOUT_TARGET %q is incompatible with REVERT_IDEMPOTENCY_MODE %q", configuredTarget, configuredMode)
	}
	if target == "" {
		if generation != "" {
			return fmt.Errorf("REVERT_REDIS_DATASET_GENERATION requires a non-empty REVERT_ROLLOUT_TARGET")
		}

		return nil
	}
	if _, err := uuid.Parse(generation); err != nil {
		return fmt.Errorf("REVERT_REDIS_DATASET_GENERATION must be a UUID for rollout target %q", configuredTarget)
	}

	return nil
}

// applyRevertRolloutTarget makes marker transitions part of the deployed
// configuration. Each transition is atomic and fails startup while a retiring
// generation still has an admitted request, so rollout safety is not a direct
// Redis mutation hidden in an operator runbook.
func applyRevertRolloutTarget(ctx context.Context, guard revertRolloutTransition, configured string) error {
	target := strings.ToLower(strings.TrimSpace(configured))
	switch target {
	case "":
		return nil
	case transactionredis.RevertUpdateFreezeInitialize:
		return guard.InitializeFinancialDatasetGeneration(ctx)
	case transactionredis.RevertUpdateFreezePrepared:
		return guard.ValidatePrepared(ctx)
	case transactionredis.RevertUpdateFreezeActive:
		return guard.Activate(ctx)
	case transactionredis.RevertUpdateFreezeDrained:
		return guard.MarkPhaseZeroDrained(ctx)
	case transactionredis.RevertUpdateFreezeFinalized:
		return guard.Finalize(ctx)
	default:
		return fmt.Errorf("invalid REVERT_ROLLOUT_TARGET %q: expected initialize, prepared, active, phase-zero-drained, finalized, or empty", configured)
	}
}
