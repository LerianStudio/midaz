// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/revertclaim"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// FinalizeDurableTransactionPersistence is the single terminal handoff used by
// individual, fallback, and bulk consumers. It proves the exact transaction and
// complete operation set on PostgreSQL primary before it completes a revert
// claim or removes Redis economic evidence. Redelivery is an exact no-op.
// The boolean reports whether this payload used the durable outcome/revert
// protocol and therefore must not enter legacy key-only cleanup.
func (uc *UseCase) FinalizeDurableTransactionPersistence(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	payload transaction.TransactionProcessingPayload,
) (bool, error) {
	if payload.Transaction == nil {
		return false, fmt.Errorf("transaction persistence payload is required")
	}
	if (payload.AttemptOwner == "") != (payload.ExpectedOutcome == "") {
		return true, fmt.Errorf("incomplete transaction balance execution identity")
	}

	outcomeBacked := payload.AttemptOwner != ""
	reverse := payload.Transaction.ParentTransactionID != nil
	validRolloutMode := payload.RevertRolloutMode == "legacy" || payload.RevertRolloutMode == "bridge"
	if (payload.RevertRolloutToken == "") != (payload.RevertRolloutMode == "") ||
		(payload.RevertRolloutToken != "" && (!validRolloutMode || !reverse || payload.Input == nil ||
			payload.Input.IsEmpty() || !outcomeBacked)) {
		return true, fmt.Errorf("revert rollout handoff is incomplete")
	}
	if !outcomeBacked && !reverse {
		return false, nil
	}
	if uc.TransactionRepo == nil || uc.TransactionRedisRepo == nil {
		return true, fmt.Errorf("durable transaction persistence repositories are required")
	}

	transactionID := payload.Transaction.IDtoUUID()
	persisted, err := uc.TransactionRepo.FindWithOperations(readrouting.WithPrimaryRead(ctx),
		organizationID, ledgerID, transactionID)
	if err != nil {
		return true, fmt.Errorf("prove durable transaction on primary: %w", err)
	}
	if err := proveDurableTransactionPayload(persisted, payload.Transaction); err != nil {
		return true, err
	}

	var durableRevertClaim *revertclaim.Claim
	if reverse {
		durableRevertClaim, err = uc.finalizeDurableRevertClaim(ctx, organizationID, ledgerID, persisted, payload)
		if err != nil {
			return true, err
		}
	}

	if outcomeBacked {
		attempt := mmodel.BalanceExecutionAttempt{
			ExecutionKey: utils.TransactionBalanceExecutionKey(organizationID, ledgerID, transactionID),
			OutcomeKey:   utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, transactionID),
			Owner:        payload.AttemptOwner,
			Outcome:      payload.ExpectedOutcome,
			Identity:     transactionID,
		}
		if err := uc.TransactionRedisRepo.FinalizeTransactionPersistence(ctx,
			organizationID, ledgerID, transactionID, attempt, transactionOperationIDs(payload.Transaction)); err != nil {
			return true, fmt.Errorf("finalize durable transaction balance outcome: %w", err)
		}
	} else {
		originID, err := uuid.Parse(*persisted.ParentTransactionID)
		if err != nil {
			return true, fmt.Errorf("parse durable reverse origin: %w", err)
		}
		operationIDs := transactionOperationIDs(persisted)
		backupStatus := utils.ExpectedBackupStatusForCleanup(persisted.Status.Code, payload.Validate)
		if err := uc.TransactionRedisRepo.FinalizeLegacyTransactionPersistence(ctx,
			organizationID, ledgerID, transactionID, originID, backupStatus, operationIDs); err != nil {
			return true, fmt.Errorf("finalize durable legacy reverse: %w", err)
		}
	}

	if durableRevertClaim != nil && (durableRevertClaim.RolloutMode != nil || durableRevertClaim.RolloutToken != nil) {
		if !reverse || uc.RevertRolloutLease == nil {
			return true, fmt.Errorf("revert rollout lease cannot be finalized")
		}
		if durableRevertClaim.RolloutMode == nil || durableRevertClaim.RolloutToken == nil {
			return true, fmt.Errorf("durable revert rollout generation is incomplete")
		}
		if err := uc.RevertRolloutLease.CompleteRevert(ctx,
			*durableRevertClaim.RolloutMode, *durableRevertClaim.RolloutToken); err != nil {
			return true, fmt.Errorf("release durable revert rollout lease: %w", err)
		}
	} else if payload.RevertRolloutToken != "" {
		return true, fmt.Errorf("durable revert claim lost its rollout generation")
	}

	return true, nil
}

func proveDurableTransactionPayload(
	persisted *transaction.Transaction,
	expected *transaction.Transaction,
) error {
	if persisted == nil || expected == nil || persisted.ID != expected.ID ||
		persisted.OrganizationID != expected.OrganizationID || persisted.LedgerID != expected.LedgerID {
		return fmt.Errorf("durable transaction identity mismatch")
	}
	if (persisted.ParentTransactionID == nil) != (expected.ParentTransactionID == nil) ||
		(persisted.ParentTransactionID != nil && *persisted.ParentTransactionID != *expected.ParentTransactionID) {
		return fmt.Errorf("durable transaction parent mismatch")
	}

	expectedStatus := expected.Status.Code
	if expectedStatus == constant.CREATED {
		expectedStatus = constant.APPROVED
	}
	if persisted.Status.Code != expectedStatus {
		return fmt.Errorf("durable transaction status mismatch")
	}
	operationsMatch := sameExactTransactionOperations(persisted, expected)
	if expected.ParentTransactionID == nil {
		// Commit/cancel append the terminal attempt's operations to the original
		// PENDING hold. The immutable payload proves the complete new operation
		// set; older hold operations are legitimate durable history.
		operationsMatch = containsExactTransactionOperations(persisted, expected)
	}
	if !operationsMatch {
		return fmt.Errorf("durable transaction operation set mismatch")
	}

	return nil
}

func containsExactTransactionOperations(all, expected *transaction.Transaction) bool {
	if len(all.Operations) < len(expected.Operations) {
		return false
	}
	durableByID := make(map[string]*operation.Operation, len(all.Operations))
	for _, durableOperation := range all.Operations {
		if durableOperation == nil || durableOperation.ID == "" {
			return false
		}
		if _, duplicate := durableByID[durableOperation.ID]; duplicate {
			return false
		}
		durableByID[durableOperation.ID] = durableOperation
	}
	seen := make(map[string]struct{}, len(expected.Operations))
	for _, expectedOperation := range expected.Operations {
		if expectedOperation == nil || expectedOperation.ID == "" {
			return false
		}
		if _, duplicate := seen[expectedOperation.ID]; duplicate {
			return false
		}
		seen[expectedOperation.ID] = struct{}{}
		durableOperation, ok := durableByID[expectedOperation.ID]
		if !ok || !operation.EconomicEffectEqual(durableOperation, expectedOperation) {
			return false
		}
	}

	return true
}

func sameExactTransactionOperations(left, right *transaction.Transaction) bool {
	return len(left.Operations) == len(right.Operations) && containsExactTransactionOperations(left, right)
}

func transactionOperationIDs(tran *transaction.Transaction) []string {
	ids := make([]string, 0, len(tran.Operations))
	for _, operation := range tran.Operations {
		ids = append(ids, operation.ID)
	}

	return ids
}

func (uc *UseCase) finalizeDurableRevertClaim(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	persisted *transaction.Transaction,
	payload transaction.TransactionProcessingPayload,
) (*revertclaim.Claim, error) {
	originID, err := uuid.Parse(*persisted.ParentTransactionID)
	if err != nil {
		return nil, fmt.Errorf("parse reverse origin transaction id: %w", err)
	}
	reverseID := persisted.IDtoUUID()
	var legacyFenceKey *string
	var legacyFenceOwner *string
	if payload.Input != nil {
		legacyHash, hashErr := utils.LegacyTransactionIdempotencyHash(*payload.Input)
		if hashErr != nil {
			return nil, fmt.Errorf("compute reverse legacy fence key: %w", hashErr)
		}
		key := utils.IdempotencyInternalKey(organizationID, ledgerID, legacyHash)
		legacyFenceKey = &key
		if payload.RevertRolloutToken != "" {
			owner := reverseID.String()
			legacyFenceOwner = &owner
		}
	}

	claim, _, err := uc.ClaimRevert(ctx, organizationID, ledgerID, originID, reverseID,
		legacyFenceKey, legacyFenceOwner, optionalString(payload.RevertRolloutMode), optionalString(payload.RevertRolloutToken))
	if err != nil {
		return nil, fmt.Errorf("adopt durable revert claim: %w", err)
	}
	if claim == nil || claim.ReverseTransactionID != reverseID {
		return nil, fmt.Errorf("durable revert claim identity mismatch")
	}
	if payload.RevertRolloutToken != "" && (claim.RolloutMode == nil || claim.RolloutToken == nil ||
		*claim.RolloutMode != payload.RevertRolloutMode || *claim.RolloutToken != payload.RevertRolloutToken) {
		return nil, fmt.Errorf("durable revert rollout generation mismatch")
	}

	if err := uc.MarkRevertClaim(ctx, organizationID, ledgerID, originID, reverseID,
		revertclaim.StateCompleted, nil); err != nil {
		return nil, fmt.Errorf("complete durable revert claim: %w", err)
	}

	replay := payload.Transaction
	if err := uc.completeDurableRevertReplay(ctx, originID, claim, replay); err != nil {
		return nil, err
	}

	return claim, nil
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}

	return &value
}

func (uc *UseCase) completeDurableRevertReplay(
	ctx context.Context,
	originID uuid.UUID,
	claim *revertclaim.Claim,
	replay *transaction.Transaction,
) error {
	encoded, err := libCommons.StructToJSONString(*replay)
	if err != nil {
		return fmt.Errorf("encode durable reverse replay: %w", err)
	}
	owner := claim.ReverseTransactionID.String()
	originHash := libCommons.HashSHA256(utils.RevertIdempotencyHashSource(originID))
	originKey := utils.IdempotencyInternalKey(claim.OrganizationID, claim.LedgerID, originHash)
	if err := uc.completeOwnedReplay(ctx, originKey, owner, encoded, replay); err != nil {
		return fmt.Errorf("complete durable origin replay: %w", err)
	}
	if claim.LegacyFenceKey != nil && *claim.LegacyFenceKey != "" {
		var err error
		if claim.LegacyFenceOwner == nil {
			err = uc.completeUnownedReplay(ctx, *claim.LegacyFenceKey, encoded, replay)
		} else {
			if *claim.LegacyFenceOwner != owner {
				return fmt.Errorf("durable legacy fence owner mismatch")
			}
			err = uc.completeOwnedReplay(ctx, *claim.LegacyFenceKey, *claim.LegacyFenceOwner, encoded, replay)
		}
		if err != nil {
			return fmt.Errorf("complete durable legacy replay: %w", err)
		}
	}

	return nil
}

func (uc *UseCase) completeUnownedReplay(
	ctx context.Context,
	key, encoded string,
	replay *transaction.Transaction,
) error {
	const replayTTL time.Duration = 300
	completed, err := uc.TransactionRedisRepo.CompleteUnownedKey(ctx, key, encoded, replayTTL)
	if err == nil && completed {
		return nil
	}
	current, readErr := uc.TransactionRedisRepo.Get(ctx, key)
	if readErr != nil && !errors.Is(readErr, redis.Nil) {
		return readErr
	}
	if current != "" {
		existing := &transaction.Transaction{}
		if json.Unmarshal([]byte(current), existing) == nil && replayIdentityMatches(existing, replay) {
			return nil
		}
		return fmt.Errorf("durable replay conflict")
	}
	if err != nil {
		return err
	}

	return fmt.Errorf("unowned durable replay completion rejected")
}

func (uc *UseCase) completeOwnedReplay(
	ctx context.Context,
	key, owner, encoded string,
	replay *transaction.Transaction,
) error {
	const replayTTL time.Duration = 300
	completed, err := uc.TransactionRedisRepo.CompleteOwnedKey(ctx, key, owner, encoded, replayTTL)
	if err == nil && completed {
		return nil
	}
	values, readErr := uc.TransactionRedisRepo.MGet(ctx, []string{key, key + ":owner"})
	if readErr != nil {
		return readErr
	}
	current := values[key]
	if current != "" {
		existing := &transaction.Transaction{}
		if json.Unmarshal([]byte(current), existing) == nil && replayIdentityMatches(existing, replay) {
			if values[key+":owner"] != "" {
				return fmt.Errorf("durable replay owner remains after terminal publication")
			}
			return nil
		}
		return fmt.Errorf("durable replay conflict")
	}
	if err != nil {
		return err
	}

	acquired, err := uc.TransactionRedisRepo.AcquireOwnedKey(ctx, key, owner, 0)
	if err != nil {
		return err
	}
	if !acquired {
		return fmt.Errorf("durable replay owner unavailable")
	}
	completed, err = uc.TransactionRedisRepo.CompleteOwnedKey(ctx, key, owner, encoded, replayTTL)
	if err != nil || !completed {
		if err != nil {
			return err
		}
		return fmt.Errorf("durable replay completion rejected")
	}

	return nil
}

func replayIdentityMatches(left, right *transaction.Transaction) bool {
	return left != nil && right != nil && left.ID == right.ID &&
		left.ParentTransactionID != nil && right.ParentTransactionID != nil &&
		*left.ParentTransactionID == *right.ParentTransactionID &&
		sameExactTransactionOperations(left, right)
}
