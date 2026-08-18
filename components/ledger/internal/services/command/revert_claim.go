// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/revertclaim"
)

func (uc *UseCase) ClaimRevert(ctx context.Context, organizationID, ledgerID, originID, reverseID uuid.UUID, legacyFenceKey, legacyFenceOwner *string) (*revertclaim.Claim, bool, error) {
	if uc.RevertClaimRepo == nil {
		return nil, false, fmt.Errorf("revert claim repository not configured")
	}

	return uc.RevertClaimRepo.Claim(ctx, organizationID, ledgerID, originID, reverseID, legacyFenceKey, legacyFenceOwner)
}

func (uc *UseCase) GetRevertClaim(ctx context.Context, organizationID, ledgerID, originID uuid.UUID) (*revertclaim.Claim, error) {
	if uc.RevertClaimRepo == nil {
		return nil, fmt.Errorf("revert claim repository not configured")
	}

	return uc.RevertClaimRepo.Get(ctx, organizationID, ledgerID, originID)
}

func (uc *UseCase) GetRevertClaimByReverseID(ctx context.Context, organizationID, ledgerID, reverseID uuid.UUID) (*revertclaim.Claim, error) {
	if uc.RevertClaimRepo == nil {
		return nil, fmt.Errorf("revert claim repository not configured")
	}

	return uc.RevertClaimRepo.GetByReverseID(ctx, organizationID, ledgerID, reverseID)
}

func (uc *UseCase) MarkRevertClaim(ctx context.Context, organizationID, ledgerID, originID, reverseID uuid.UUID, state revertclaim.State, reason *string) error {
	if uc.RevertClaimRepo == nil {
		return fmt.Errorf("revert claim repository not configured")
	}

	return uc.RevertClaimRepo.Transition(ctx, organizationID, ledgerID, originID, reverseID, state, reason)
}

func (uc *UseCase) BeginPreMutationRevertRecovery(ctx context.Context, organizationID, ledgerID, originID, reverseID uuid.UUID) (bool, error) {
	if uc.RevertClaimRepo == nil {
		return false, fmt.Errorf("revert claim repository not configured")
	}

	return uc.RevertClaimRepo.BeginPreMutationRecovery(ctx, organizationID, ledgerID, originID, reverseID)
}

func (uc *UseCase) ReleaseRevertClaim(ctx context.Context, organizationID, ledgerID, originID, reverseID uuid.UUID) (bool, error) {
	if uc.RevertClaimRepo == nil {
		return false, fmt.Errorf("revert claim repository not configured")
	}

	return uc.RevertClaimRepo.Release(ctx, organizationID, ledgerID, originID, reverseID)
}

// CompleteRevertClaim adopts backups produced by old pods and completes claims
// produced by bridge/final pods through the same exact-ID gate. It never
// overwrites a reservation belonging to another reverse.
func (uc *UseCase) CompleteRevertClaim(
	ctx context.Context,
	organizationID, ledgerID, originID, reverseID uuid.UUID,
	legacyFenceKey, legacyFenceOwner *string,
) error {
	claim, _, err := uc.ClaimRevert(ctx, organizationID, ledgerID, originID, reverseID, legacyFenceKey, legacyFenceOwner)
	if err != nil {
		return fmt.Errorf("adopt durable revert claim: %w", err)
	}
	if claim.ReverseTransactionID != reverseID {
		return fmt.Errorf("revert claim reserved %s but backup contains %s", claim.ReverseTransactionID, reverseID)
	}

	return uc.MarkRevertClaim(ctx, organizationID, ledgerID, originID, reverseID, revertclaim.StateCompleted, nil)
}
