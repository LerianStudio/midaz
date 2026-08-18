// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/revertclaim"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

const (
	revertIdempotencyModeLegacy = "legacy"
	revertIdempotencyModeBridge = "bridge"
	revertIdempotencyModeFinal  = "final"
	revertExecutionLeaseTTL     = time.Duration(300)
)

func (handler *TransactionHandler) activeRevertIdempotencyMode() string {
	// A zero-value handler predates the rollout wiring and must retain the
	// released legacy algorithm. Production bootstrap always injects an
	// explicit mode, so bridge/final can never bypass their rollout marker by
	// omission.
	if strings.TrimSpace(handler.RevertIdempotencyMode) == "" {
		return revertIdempotencyModeLegacy
	}
	if strings.EqualFold(handler.RevertIdempotencyMode, revertIdempotencyModeLegacy) {
		return revertIdempotencyModeLegacy
	}
	if strings.EqualFold(handler.RevertIdempotencyMode, revertIdempotencyModeFinal) {
		return revertIdempotencyModeFinal
	}

	return revertIdempotencyModeBridge
}

func (handler *TransactionHandler) requireRevertRolloutBarrier(ctx context.Context) error {
	if strings.TrimSpace(handler.RevertIdempotencyMode) == "" && handler.RevertUpdateFreeze == nil {
		return nil
	}

	mode := handler.activeRevertIdempotencyMode()
	if handler.RevertUpdateFreeze == nil {
		return pkg.ValidateBusinessError(constant.ErrRevertRolloutFreezeRequired, constant.EntityTransaction)
	}

	ready, err := handler.RevertUpdateFreeze.ReadyForMode(ctx, mode)
	if err != nil {
		return fmt.Errorf("read revert rollout barrier: %w", err)
	}
	if !ready {
		return pkg.ValidateBusinessError(constant.ErrRevertRolloutFreezeRequired, constant.EntityTransaction)
	}

	return nil
}

func (handler *TransactionHandler) acquireRevertRolloutRequest(
	ctx context.Context,
	organizationID, ledgerID, originID uuid.UUID,
) (string, string, string, func() error, error) {
	if strings.TrimSpace(handler.RevertIdempotencyMode) == "" && handler.RevertUpdateFreeze == nil {
		return "", "", "", nil, nil
	}
	if handler.RevertUpdateFreeze == nil {
		return "", "", "", nil, pkg.ValidateBusinessError(constant.ErrRevertRolloutFreezeRequired, constant.EntityTransaction)
	}

	mode := handler.activeRevertIdempotencyMode()
	// The origin token lets terminal recovery clear every attempt for one
	// economic origin. The separate attempt ID makes admission and release
	// idempotent under a lost Redis response without allowing one concurrent
	// HTTP request to remove another request's rollout barrier.
	token := libCommons.HashSHA256(strings.Join([]string{
		organizationID.String(), ledgerID.String(), originID.String(),
	}, ":"))
	attemptID := uuid.NewString()
	admitted, leaseHeld, phase, err := handler.RevertUpdateFreeze.AcquireRevert(ctx, mode, token, attemptID)
	if err != nil {
		// The Lua admission is idempotent for this attempt ID. A lost response
		// leaves a fail-closed barrier; retrying the exact admission or release
		// cannot inflate or erase another attempt.
		return "", "", "", nil, fmt.Errorf("acquire revert rollout request lease: %w", err)
	}
	if !admitted {
		return "", "", "", nil, pkg.ValidateBusinessError(constant.ErrRevertRolloutFreezeRequired, constant.EntityTransaction)
	}
	release := func() error {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()

		if err := handler.RevertUpdateFreeze.ReleaseRevert(releaseCtx, mode, token, attemptID); err != nil {
			return fmt.Errorf("release revert rollout request lease: %w", err)
		}

		return nil
	}
	generation, err := handler.RevertUpdateFreeze.FinancialDatasetGeneration(ctx)
	if err != nil {
		if leaseHeld {
			return "", "", "", nil, errors.Join(err, release())
		}

		return "", "", "", nil, err
	}
	if !leaseHeld {
		// Legacy/bridge can be admitted without a live attempt only when the
		// origin is already terminally sealed. Preserve its deterministic origin
		// token so the request follows the durable-claim replay path. Final mode
		// never owns a generation drain token.
		if mode != revertIdempotencyModeFinal {
			return phase, token, generation, nil, nil
		}
		return phase, "", generation, nil, nil
	}

	return phase, token, generation, release, nil
}

func (handler *TransactionHandler) revertRolloutHandoffPending(
	ctx context.Context,
	organizationID, ledgerID, originID, reverseID uuid.UUID,
) bool {
	if reverseID == uuid.Nil {
		return false
	}
	if handler.Command == nil || handler.Command.TransactionRedisRepo == nil || handler.Command.RevertClaimRepo == nil {
		return true
	}

	if handler.RevertUpdateFreeze == nil || handler.RevertUpdateFreeze.FinancialDurability(ctx) != nil {
		return true
	}
	claim, err := handler.Command.GetRevertClaim(ctx, organizationID, ledgerID, originID)
	if err != nil {
		return true
	}
	expectedGeneration := ""
	if claim != nil && claim.RedisGeneration != nil {
		expectedGeneration = *claim.RedisGeneration
	}
	evidence, generationMatches, err := handler.Command.TransactionRedisRepo.TransactionEconomicEvidenceExists(ctx,
		organizationID, ledgerID, reverseID, expectedGeneration)
	if err != nil || evidence || (claim != nil && !generationMatches) {
		return true
	}
	if claim == nil {
		return false
	}

	if claim.ReverseTransactionID != reverseID || claim.State != revertclaim.StateCompleted {
		return true
	}
	if claim.RolloutMode == nil && claim.RolloutToken == nil {
		return false
	}
	if claim.RolloutMode == nil || claim.RolloutToken == nil || handler.RevertUpdateFreeze == nil {
		return true
	}
	complete, err := handler.RevertUpdateFreeze.RevertTerminalHandoffComplete(ctx,
		*claim.RolloutMode, *claim.RolloutToken)

	return err != nil || !complete
}

// releaseAmbiguousRolloutAdmission reconciles a lost admission response before
// any money-path work starts. The unique token is the lease owner, so removing
// only that token is safe whether the admission Lua committed or not. If the
// removal itself is ambiguous, the original request still stops before any
// mutation and a surviving token keeps the rollout transition fail-closed.
func releaseAmbiguousRolloutAdmission(
	ctx context.Context,
	acquireErr error,
	release func(context.Context) error,
) error {
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	if err := release(releaseCtx); err != nil {
		return errors.Join(acquireErr, fmt.Errorf("reconcile ambiguous rollout admission: %w", err))
	}

	return acquireErr
}

func legacyRevertIdempotencyHash(input mtransaction.Transaction) (string, error) {
	return utils.LegacyTransactionIdempotencyHash(input)
}

func (handler *TransactionHandler) readLegacyRevert(ctx context.Context, organizationID, ledgerID uuid.UUID, legacyHash string) (*transaction.Transaction, string, error) {
	legacyKey := utils.IdempotencyInternalKey(organizationID, ledgerID, legacyHash)
	value, err := handler.Command.TransactionRedisRepo.Get(ctx, legacyKey)
	if errors.Is(err, redislib.Nil) {
		return nil, legacyKey, nil
	}
	if err != nil {
		return nil, legacyKey, fmt.Errorf("read legacy revert fence: %w", err)
	}
	if value == "" {
		return nil, legacyKey, nil
	}

	cached := &transaction.Transaction{}
	if err := json.Unmarshal([]byte(value), cached); err != nil {
		return nil, legacyKey, fmt.Errorf("decode legacy revert fence: %w", err)
	}

	return cached, legacyKey, nil
}

func reverseBelongsToOrigin(reverse *transaction.Transaction, originID uuid.UUID) bool {
	return reverse != nil && reverse.ParentTransactionID != nil && *reverse.ParentTransactionID == originID.String()
}

func reverseMatchesClaim(reverse *transaction.Transaction, claim *revertclaim.Claim) bool {
	return reverseBelongsToOrigin(reverse, claim.OriginTransactionID) && reverse.ID == claim.ReverseTransactionID.String()
}

func reverseReplayMatchesDurable(replay, durable *transaction.Transaction, claim *revertclaim.Claim) bool {
	if replay == nil || durable == nil || !reverseMatchesClaim(replay, claim) || !reverseMatchesClaim(durable, claim) ||
		len(replay.Operations) != len(durable.Operations) {
		return false
	}

	durableByID := make(map[string]*operation.Operation, len(durable.Operations))
	for _, op := range durable.Operations {
		if op == nil || op.ID == "" {
			return false
		}
		if _, duplicate := durableByID[op.ID]; duplicate {
			return false
		}
		durableByID[op.ID] = op
	}
	for _, replayOp := range replay.Operations {
		if replayOp == nil || replayOp.ID == "" {
			return false
		}
		durableOp, ok := durableByID[replayOp.ID]
		if !ok || !operation.EconomicEffectEqual(replayOp, durableOp) {
			return false
		}
		delete(durableByID, replayOp.ID)
	}

	return len(durableByID) == 0
}

func (handler *TransactionHandler) resolveDurableRevertClaim(ctx context.Context, claim *revertclaim.Claim) (*transaction.Transaction, bool, error) {
	persisted, err := handler.Query.GetParentByTransactionID(ctx, claim.OrganizationID, claim.LedgerID, claim.OriginTransactionID)
	if err != nil {
		return nil, false, err
	}

	if persisted != nil {
		if !reverseMatchesClaim(persisted, claim) {
			return nil, false, pkg.ValidateBusinessError(constant.ErrRevertReconciliationRequired, constant.EntityTransaction)
		}

		persisted, complete, err := handler.loadCompleteReverse(ctx, claim)
		if err != nil {
			return nil, false, err
		}
		if !complete {
			return nil, false, handler.requireRevertReconciliation(ctx, claim, "reverse_transaction_missing_operations")
		}
		if err := handler.finalizeDurableRevert(ctx, claim, persisted); err != nil {
			return nil, false, err
		}

		return persisted, true, nil
	}

	if claim.State == revertclaim.StateClaimed {
		committed, outcomeErr := handler.revertBalanceOutcomeCommitted(ctx, claim)
		if outcomeErr != nil {
			return nil, false, handler.requireRevertReconciliation(ctx, claim, "reverse_balance_outcome_unreadable")
		}
		if committed {
			return nil, false, handler.requireRevertReconciliation(ctx, claim, "reverse_balance_committed_before_persistence")
		}

		return nil, false, pkg.ValidateBusinessError(constant.ErrIdempotencyKey, "RevertTransaction", claim.OriginTransactionID.String())
	}

	return nil, false, pkg.ValidateBusinessError(constant.ErrRevertReconciliationRequired, constant.EntityTransaction)
}

func (handler *TransactionHandler) revertBalanceOutcomeCommitted(ctx context.Context, claim *revertclaim.Claim) (bool, error) {
	backup, err := handler.Command.TransactionRedisRepo.ReadMessageFromQueue(ctx,
		utils.TransactionInternalKey(claim.OrganizationID, claim.LedgerID, claim.ReverseTransactionID.String()))
	if errors.Is(err, redislib.Nil) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read claimed reverse balance outcome: %w", err)
	}

	queued := mmodel.TransactionRedisQueue{}
	if err := json.Unmarshal(backup, &queued); err != nil {
		return false, fmt.Errorf("decode claimed reverse balance outcome: %w", err)
	}
	if queued.TransactionID != claim.ReverseTransactionID {
		return false, fmt.Errorf("claimed reverse balance outcome transaction mismatch")
	}
	if queued.ParentTransactionID != nil && *queued.ParentTransactionID != claim.OriginTransactionID {
		return false, fmt.Errorf("claimed reverse balance outcome origin mismatch")
	}

	// The queue seed exists before Lua dispatch. BalancesAfter is written only
	// by the same Lua command that commits the movement, so it is the durable
	// distinction between a pre-movement seed and a lost post-movement response.
	return len(queued.BalancesAfter) > 0, nil
}

func (handler *TransactionHandler) adoptPersistedReverse(
	ctx context.Context,
	organizationID, ledgerID, originID uuid.UUID,
	persisted *transaction.Transaction,
	rolloutMode, rolloutToken, redisGeneration *string,
	legacyFenceKey ...*string,
) (*transaction.Transaction, bool, error) {
	reverseID, err := uuid.Parse(persisted.ID)
	if err != nil || !reverseBelongsToOrigin(persisted, originID) {
		return nil, false, pkg.ValidateBusinessError(constant.ErrRevertReconciliationRequired, constant.EntityTransaction)
	}

	var exactLegacyKey *string
	if len(legacyFenceKey) > 0 {
		exactLegacyKey = legacyFenceKey[0]
	}
	claim, _, err := handler.Command.ClaimRevert(ctx, organizationID, ledgerID, originID, reverseID,
		exactLegacyKey, nil, rolloutMode, rolloutToken, redisGeneration)
	if err != nil {
		return nil, false, err
	}
	if claim.ReverseTransactionID != reverseID {
		return nil, false, pkg.ValidateBusinessError(constant.ErrRevertReconciliationRequired, constant.EntityTransaction)
	}

	persisted, complete, err := handler.loadCompleteReverse(ctx, claim)
	if err != nil {
		return nil, false, err
	}
	if !complete {
		return nil, false, handler.requireRevertReconciliation(ctx, claim, "reverse_transaction_missing_operations")
	}
	if err := handler.finalizeDurableRevert(ctx, claim, persisted); err != nil {
		return nil, false, err
	}
	if err := handler.completeCurrentRevertRollout(ctx, claim, rolloutMode, rolloutToken); err != nil {
		return nil, false, err
	}

	return persisted, true, nil
}

func (handler *TransactionHandler) completeCurrentRevertRollout(
	ctx context.Context,
	claim *revertclaim.Claim,
	rolloutMode, rolloutToken *string,
) error {
	if rolloutMode == nil && rolloutToken == nil {
		return nil
	}
	if rolloutMode == nil || rolloutToken == nil || handler.RevertUpdateFreeze == nil {
		return handler.requireRevertReconciliation(ctx, claim, "current_revert_rollout_generation_incomplete")
	}
	if claim.RolloutMode != nil && claim.RolloutToken != nil &&
		*claim.RolloutMode == *rolloutMode && *claim.RolloutToken == *rolloutToken {
		return nil
	}
	if err := handler.RevertUpdateFreeze.CompleteRevert(ctx, *rolloutMode, *rolloutToken); err != nil {
		return handler.requireRevertReconciliation(ctx, claim, "current_revert_rollout_generation_completion_failed")
	}

	return nil
}

func (handler *TransactionHandler) finalizeDurableRevert(
	ctx context.Context,
	claim *revertclaim.Claim,
	persisted *transaction.Transaction,
) error {
	if err := handler.Command.MarkRevertClaim(ctx, claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID, revertclaim.StateCompleted, nil); err != nil {
		return err
	}

	originKey := originRevertIdempotencyKey(claim)
	if err := handler.completeOriginRevertBarrier(ctx, &originKey, claim.ReverseTransactionID,
		persisted, pkgHTTP.ParseIdempotencyTTL("")); err != nil {
		return handler.requireRevertReconciliation(ctx, claim, "origin_revert_fence_completion_failed")
	}

	// A final pod adopting a bridge reverse must finish the exact H1 barrier
	// persisted by the bridge claimant. Recomputing it from the mutable origin
	// payload could complete or delete another economic origin's fence.
	if claim.LegacyFenceKey != nil {
		legacyKey, err := legacyRevertBarrierKeyFromClaim(claim)
		if err != nil {
			return handler.requireRevertReconciliation(ctx, claim, "legacy_revert_fence_recovery_input_failed")
		}
		var settleErr error
		if claim.LegacyFenceOwner == nil {
			settleErr = handler.completeUnownedLegacyRevertBarrier(ctx, claim, legacyKey, persisted)
		} else if *claim.LegacyFenceOwner != claim.ReverseTransactionID.String() {
			settleErr = handler.requireRevertReconciliation(ctx, claim, "legacy_revert_fence_owner_mismatch")
		} else if handler.activeRevertIdempotencyMode() == revertIdempotencyModeFinal {
			settleErr = handler.settleFinalLegacyRevertBarrier(ctx, claim, legacyKey, persisted)
		} else {
			settleErr = handler.completeLegacyRevertBarrier(ctx, claim, legacyKey, persisted)
		}
		if settleErr != nil {
			return settleErr
		}
	}

	// Cleanup is permitted only after the child, every operation, and terminal
	// claim were proven durable above. Outcome-backed envelopes require the exact
	// immutable owner/outcome; drained phase-zero envelopes require an exact
	// parent and operation-set proof and never touch a Redis outcome.
	if err := handler.finalizeDurableRevertPersistence(ctx, claim, persisted); err != nil {
		return pkg.ValidateBusinessError(constant.ErrRevertReconciliationRequired, constant.EntityTransaction)
	}
	if claim.RolloutMode != nil || claim.RolloutToken != nil {
		if claim.RolloutMode == nil || claim.RolloutToken == nil || handler.RevertUpdateFreeze == nil {
			return handler.requireRevertReconciliation(ctx, claim, "revert_rollout_generation_incomplete")
		}
		if err := handler.RevertUpdateFreeze.CompleteRevert(ctx, *claim.RolloutMode, *claim.RolloutToken); err != nil {
			return handler.requireRevertReconciliation(ctx, claim, "revert_rollout_generation_completion_failed")
		}
	}

	return nil
}

func (handler *TransactionHandler) completeUnownedLegacyRevertBarrier(
	ctx context.Context,
	claim *revertclaim.Claim,
	legacyKey string,
	reverse *transaction.Transaction,
) error {
	if !reverseMatchesClaim(reverse, claim) {
		return handler.requireRevertReconciliation(ctx, claim, "legacy_revert_result_claim_mismatch")
	}
	value, err := libCommons.StructToJSONString(*reverse)
	if err != nil {
		return handler.requireRevertReconciliation(ctx, claim, "legacy_revert_result_encode_failed")
	}
	completed, completeErr := handler.Command.TransactionRedisRepo.CompleteUnownedKey(ctx, legacyKey, value,
		pkgHTTP.ParseIdempotencyTTL(""))
	if completeErr == nil && completed {
		return nil
	}

	current, readErr := handler.Command.TransactionRedisRepo.Get(ctx, legacyKey)
	if readErr == nil && current != "" {
		existing := &transaction.Transaction{}
		if json.Unmarshal([]byte(current), existing) == nil && reverseReplayMatchesDurable(existing, reverse, claim) {
			return nil
		}
	}
	if handler.activeRevertIdempotencyMode() == revertIdempotencyModeFinal {
		// H1 is no longer an admission barrier after the machine-verifiable
		// bridge drain. A foreign or owner-bearing value is preserved, never
		// accepted as replay, and cannot block origin-scoped terminal cleanup.
		return nil
	}
	if completeErr != nil {
		return handler.requireRevertReconciliation(ctx, claim, "legacy_revert_fence_completion_failed")
	}

	return handler.requireRevertReconciliation(ctx, claim, "legacy_revert_fence_owner_lost")
}

func (handler *TransactionHandler) finalizeDurableRevertPersistence(
	ctx context.Context,
	claim *revertclaim.Claim,
	persisted *transaction.Transaction,
) error {
	transactionKey := utils.TransactionInternalKey(claim.OrganizationID, claim.LedgerID,
		claim.ReverseTransactionID.String())
	backup, err := handler.Command.TransactionRedisRepo.ReadMessageFromQueue(ctx, transactionKey)
	if errors.Is(err, redislib.Nil) {
		// The outcome may survive a lost cleanup response after its backup was
		// removed. The exact finalizer is idempotent when both are already absent.
		return handler.finalizeOutcomeBackedRevertPersistence(ctx, claim, persisted)
	}
	if err != nil {
		return fmt.Errorf("read reverse transaction backup for cleanup: %w", err)
	}

	queued := mmodel.TransactionRedisQueue{}
	if err := json.Unmarshal(backup, &queued); err != nil {
		return fmt.Errorf("decode reverse transaction backup for cleanup: %w", err)
	}
	if queued.TransactionID != claim.ReverseTransactionID {
		return fmt.Errorf("reverse transaction backup identity mismatch")
	}

	if queued.AttemptOwner != "" || queued.ExpectedOutcome != "" {
		if queued.AttemptOwner != claim.ReverseTransactionID.String() ||
			queued.ExpectedOutcome != mmodel.TransactionOutcomeCommitted {
			return fmt.Errorf("reverse transaction backup outcome mismatch")
		}

		return handler.finalizeOutcomeBackedRevertPersistence(ctx, claim, persisted)
	}

	operationIDs := make([]string, 0, len(persisted.Operations))
	for _, operation := range persisted.Operations {
		if operation == nil || operation.ID == "" {
			return fmt.Errorf("persisted reverse operation identity is required")
		}
		operationIDs = append(operationIDs, operation.ID)
	}

	return handler.Command.TransactionRedisRepo.FinalizeLegacyTransactionPersistence(ctx,
		claim.OrganizationID, claim.LedgerID, claim.ReverseTransactionID, claim.OriginTransactionID,
		utils.ExpectedBackupStatusForCleanup(persisted.Status.Code, queued.Validate), operationIDs)
}

func (handler *TransactionHandler) finalizeOutcomeBackedRevertPersistence(
	ctx context.Context,
	claim *revertclaim.Claim,
	persisted *transaction.Transaction,
) error {
	attempt := mmodel.BalanceExecutionAttempt{
		ExecutionKey: utils.TransactionBalanceExecutionKey(claim.OrganizationID, claim.LedgerID, claim.ReverseTransactionID),
		OutcomeKey:   utils.TransactionBalanceOutcomeKey(claim.OrganizationID, claim.LedgerID, claim.ReverseTransactionID),
		Owner:        claim.ReverseTransactionID.String(),
		Outcome:      mmodel.TransactionOutcomeCommitted,
		Identity:     claim.ReverseTransactionID,
	}
	if claim.RedisGeneration != nil {
		attempt.RedisGeneration = *claim.RedisGeneration
	}

	operationIDs := make([]string, 0, len(persisted.Operations))
	for _, operation := range persisted.Operations {
		if operation == nil || operation.ID == "" {
			return fmt.Errorf("persisted reverse operation identity is required")
		}
		operationIDs = append(operationIDs, operation.ID)
	}

	return handler.Command.TransactionRedisRepo.FinalizeTransactionPersistence(ctx,
		claim.OrganizationID, claim.LedgerID, claim.ReverseTransactionID, attempt, operationIDs)
}

func (handler *TransactionHandler) loadCompleteReverse(ctx context.Context, claim *revertclaim.Claim) (*transaction.Transaction, bool, error) {
	persisted, err := handler.Query.GetTransactionWithOperationsByID(ctx, claim.OrganizationID, claim.LedgerID, claim.ReverseTransactionID)
	if err != nil {
		return nil, false, err
	}
	if !reverseMatchesClaim(persisted, claim) || len(persisted.Operations) == 0 {
		return persisted, false, nil
	}

	backup, err := handler.Command.TransactionRedisRepo.ReadMessageFromQueue(ctx,
		utils.TransactionInternalKey(claim.OrganizationID, claim.LedgerID, claim.ReverseTransactionID.String()))
	if errors.Is(err, redislib.Nil) {
		// A terminal claim proves that the complete reverse was durable before its
		// backup was removed. A non-terminal claim without its authoritative
		// backup cannot prove whether Redis moved balances and must reconcile.
		return persisted, claim.State == revertclaim.StateCompleted, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read reverse transaction backup: %w", err)
	}

	queued := mmodel.TransactionRedisQueue{}
	if err := json.Unmarshal(backup, &queued); err != nil {
		return nil, false, fmt.Errorf("decode reverse transaction backup: %w", err)
	}
	if queued.TransactionID != claim.ReverseTransactionID || queued.ParentTransactionID == nil ||
		*queued.ParentTransactionID != claim.OriginTransactionID || len(queued.Operations) != len(persisted.Operations) {
		return persisted, false, nil
	}

	persistedOperations := make(map[string]*operation.Operation, len(persisted.Operations))
	for _, persistedOperation := range persisted.Operations {
		if persistedOperation == nil || persistedOperation.ID == "" {
			return persisted, false, nil
		}
		if _, duplicate := persistedOperations[persistedOperation.ID]; duplicate {
			return persisted, false, nil
		}
		persistedOperations[persistedOperation.ID] = persistedOperation
	}
	for _, queuedOperation := range queued.Operations {
		persistedOperation, ok := persistedOperations[queuedOperation.ID]
		if !ok || !queuedOperationBalanceMatchesPersisted(queuedOperation, persistedOperation) {
			return persisted, false, nil
		}
		delete(persistedOperations, queuedOperation.ID)
	}

	return persisted, len(persistedOperations) == 0, nil
}

func queuedOperationBalanceMatchesPersisted(queued mmodel.OperationRedis, persisted *operation.Operation) bool {
	return persisted != nil && operation.RedisEconomicEffectEqual(queued, persisted.ToRedis())
}

func legacyRevertBarrierKeyFromClaim(claim *revertclaim.Claim) (string, error) {
	if claim == nil || claim.LegacyFenceKey == nil || *claim.LegacyFenceKey == "" {
		return "", fmt.Errorf("durable legacy revert fence key is required")
	}

	return *claim.LegacyFenceKey, nil
}

func (handler *TransactionHandler) acquireLegacyRevertBarrier(ctx context.Context, claim *revertclaim.Claim) (string, *transaction.Transaction, error) {
	legacyKey, err := legacyRevertBarrierKeyFromClaim(claim)
	if err != nil {
		return "", nil, err
	}
	if claim.LegacyFenceOwner == nil || *claim.LegacyFenceOwner != claim.ReverseTransactionID.String() {
		return "", nil, fmt.Errorf("durable legacy revert fence owner is required")
	}
	// The bridge fence must outlive the request. A finite TTL would let an old
	// pod acquire the payload-scoped legacy key while this bridge request is
	// still moving or persisting balances. Proven pre-movement failures release
	// it explicitly; success atomically replaces it with a normal replay TTL.
	acquired, err := handler.Command.TransactionRedisRepo.AcquireOwnedKey(ctx, legacyKey, *claim.LegacyFenceOwner, 0)
	if err != nil {
		// The script may have acquired the persistent fence before its response
		// was lost. Return the resolved key so the pre-movement cleanup can issue
		// an owner-checked release before it releases the PostgreSQL claim.
		return legacyKey, nil, err
	}
	if acquired {
		return legacyKey, nil, nil
	}

	value, err := handler.Command.TransactionRedisRepo.Get(ctx, legacyKey)
	if err != nil && !errors.Is(err, redislib.Nil) {
		return "", nil, fmt.Errorf("read occupied legacy revert fence: %w", err)
	}
	if value == "" {
		return "", nil, pkg.ValidateBusinessError(constant.ErrIdempotencyKey,
			"RevertTransaction", claim.OriginTransactionID.String())
	}

	replay := &transaction.Transaction{}
	if err := json.Unmarshal([]byte(value), replay); err != nil {
		return "", nil, fmt.Errorf("decode occupied legacy revert fence: %w", err)
	}
	if !reverseBelongsToOrigin(replay, claim.OriginTransactionID) {
		return "", nil, pkg.ValidateBusinessError(constant.ErrIdempotencyKey,
			"RevertTransaction", claim.OriginTransactionID.String())
	}
	if replay.ID != claim.ReverseTransactionID.String() {
		return "", nil, pkg.ValidateBusinessError(constant.ErrRevertReconciliationRequired, constant.EntityTransaction)
	}

	return legacyKey, replay, nil
}

func (handler *TransactionHandler) completeLegacyRevertBarrier(ctx context.Context, claim *revertclaim.Claim, legacyKey string, reverse *transaction.Transaction) error {
	exactLegacyKey, err := legacyRevertBarrierKeyFromClaim(claim)
	if err != nil || legacyKey != exactLegacyKey {
		return handler.requireRevertReconciliation(ctx, claim, "legacy_revert_fence_key_mismatch")
	}
	if !reverseMatchesClaim(reverse, claim) {
		return handler.requireRevertReconciliation(ctx, claim, "legacy_revert_result_claim_mismatch")
	}
	if claim.LegacyFenceOwner == nil || *claim.LegacyFenceOwner != claim.ReverseTransactionID.String() {
		return handler.requireRevertReconciliation(ctx, claim, "legacy_revert_fence_owner_mismatch")
	}

	value, err := libCommons.StructToJSONString(*reverse)
	if err != nil {
		return handler.requireRevertReconciliation(ctx, claim, "legacy_revert_result_encode_failed")
	}

	completed, err := handler.Command.TransactionRedisRepo.CompleteOwnedKey(ctx, legacyKey,
		*claim.LegacyFenceOwner, value, pkgHTTP.ParseIdempotencyTTL(""))
	if err != nil {
		return handler.requireRevertReconciliation(ctx, claim, "legacy_revert_fence_completion_failed")
	}
	if completed {
		return nil
	}

	// A retry can observe an already-completed replay after the first request
	// lost its response. It is safe only when the cached reverse is the exact
	// transaction reserved by this origin's durable claim.
	existingValue, err := handler.Command.TransactionRedisRepo.Get(ctx, legacyKey)
	if err != nil && !errors.Is(err, redislib.Nil) {
		return handler.requireRevertReconciliation(ctx, claim, "legacy_revert_fence_verification_failed")
	}
	if existingValue != "" {
		existing := &transaction.Transaction{}
		if json.Unmarshal([]byte(existingValue), existing) == nil && reverseReplayMatchesDurable(existing, reverse, claim) {
			return nil
		}
	}

	// A crash can happen after the reverse is durable but before the owned
	// bridge fence is completed. When the key is absent, reclaim it with the
	// already-reserved reverse ID and materialize the replay. If an old request
	// owns an empty key, acquisition fails and reconciliation remains required.
	acquired, acquireErr := handler.Command.TransactionRedisRepo.AcquireOwnedKey(ctx, legacyKey,
		*claim.LegacyFenceOwner, 0)
	if acquireErr != nil {
		return handler.requireRevertReconciliation(ctx, claim, "legacy_revert_fence_recovery_failed")
	}
	if !acquired {
		existingValue, verifyErr := handler.Command.TransactionRedisRepo.Get(ctx, legacyKey)
		if verifyErr == nil && existingValue != "" {
			existing := &transaction.Transaction{}
			if json.Unmarshal([]byte(existingValue), existing) == nil && reverseReplayMatchesDurable(existing, reverse, claim) {
				return nil
			}
		}

		return handler.requireRevertReconciliation(ctx, claim, "legacy_revert_fence_recovery_owner_lost")
	}
	completed, completeErr := handler.Command.TransactionRedisRepo.CompleteOwnedKey(ctx, legacyKey,
		*claim.LegacyFenceOwner, value, pkgHTTP.ParseIdempotencyTTL(""))
	if completeErr == nil && completed {
		return nil
	}
	// Another retry can acquire the absent pair with the same durable reverse
	// ID. Completing with that exact owner is safe; if it won the race first or
	// this completion response was lost, accept only the exact replay.
	existingValue, verifyErr := handler.Command.TransactionRedisRepo.Get(ctx, legacyKey)
	if verifyErr == nil && existingValue != "" {
		existing := &transaction.Transaction{}
		if json.Unmarshal([]byte(existingValue), existing) == nil && reverseReplayMatchesDurable(existing, reverse, claim) {
			return nil
		}
	}

	return handler.requireRevertReconciliation(ctx, claim, "legacy_revert_fence_owner_lost")
}

// settleFinalLegacyRevertBarrier retires only the immutable H1 fence recorded
// by a bridge claim. H1 no longer arbitrates admission after bridge drains, so
// a foreign collision is preserved and ignored; an exact owner is completed
// to the exact durable reverse without ever recalculating the legacy key.
func (handler *TransactionHandler) settleFinalLegacyRevertBarrier(
	ctx context.Context,
	claim *revertclaim.Claim,
	legacyKey string,
	reverse *transaction.Transaction,
) error {
	exactLegacyKey, err := legacyRevertBarrierKeyFromClaim(claim)
	if err != nil || legacyKey != exactLegacyKey || !reverseMatchesClaim(reverse, claim) ||
		claim.LegacyFenceOwner == nil || *claim.LegacyFenceOwner != claim.ReverseTransactionID.String() {
		return handler.requireRevertReconciliation(ctx, claim, "legacy_revert_fence_recovery_input_failed")
	}

	values, err := handler.Command.TransactionRedisRepo.MGet(ctx, []string{legacyKey, legacyKey + ":owner"})
	if err != nil {
		return handler.requireRevertReconciliation(ctx, claim, "legacy_revert_fence_verification_failed")
	}
	owner := values[legacyKey+":owner"]
	if owner != *claim.LegacyFenceOwner {
		// Absent and foreign legacy states are non-authoritative in final mode.
		// Preserving a foreign state is essential: it may still be the exact H1
		// replay for another origin that collided under the old payload hash.
		return nil
	}
	if current := values[legacyKey]; current != "" {
		existing := &transaction.Transaction{}
		if json.Unmarshal([]byte(current), existing) != nil || !reverseReplayMatchesDurable(existing, reverse, claim) {
			// The value is the client-visible legacy fact. Even an owner companion
			// carrying this claim's ID cannot authorize overwriting a foreign H1
			// replay (for example after asymmetric eviction and old-pod recovery).
			return nil
		}
	}

	value, err := libCommons.StructToJSONString(*reverse)
	if err != nil {
		return handler.requireRevertReconciliation(ctx, claim, "legacy_revert_result_encode_failed")
	}
	completed, completeErr := handler.Command.TransactionRedisRepo.CompleteOwnedKey(ctx, legacyKey,
		*claim.LegacyFenceOwner, value, pkgHTTP.ParseIdempotencyTTL(""))
	if completeErr == nil && completed {
		return nil
	}

	// A completion response can be lost after Redis atomically materialized the
	// replay and removed the owner. Both records must prove that exact terminal
	// state; a surviving owner means the completion did not finish, while an
	// absent or foreign value cannot prove which side of the response loss won.
	verified, readErr := handler.Command.TransactionRedisRepo.MGet(ctx, []string{legacyKey, legacyKey + ":owner"})
	if readErr != nil {
		return handler.requireRevertReconciliation(ctx, claim, "legacy_revert_fence_verification_failed")
	}
	current := verified[legacyKey]
	existing := &transaction.Transaction{}
	if verified[legacyKey+":owner"] == "" && current != "" &&
		json.Unmarshal([]byte(current), existing) == nil && reverseReplayMatchesDurable(existing, reverse, claim) {
		return nil
	}

	return handler.requireRevertReconciliation(ctx, claim, "legacy_revert_fence_verification_failed")
}

func (handler *TransactionHandler) legacyRevertBarrierKeyFromBackup(
	ctx context.Context,
	organizationID, ledgerID, originID, reverseID uuid.UUID,
) (string, error) {
	backup, err := handler.Command.TransactionRedisRepo.ReadMessageFromQueue(ctx,
		utils.TransactionInternalKey(organizationID, ledgerID, reverseID.String()))
	if err != nil {
		if errors.Is(err, redislib.Nil) {
			return "", pkg.ValidateBusinessError(constant.ErrRevertReconciliationRequired, constant.EntityTransaction)
		}

		return "", fmt.Errorf("read immutable legacy revert backup: %w", err)
	}

	queued := mmodel.TransactionRedisQueue{}
	if err := json.Unmarshal(backup, &queued); err != nil {
		return "", pkg.ValidateBusinessError(constant.ErrRevertReconciliationRequired, constant.EntityTransaction)
	}
	if queued.TransactionID != reverseID || queued.ParentTransactionID == nil ||
		*queued.ParentTransactionID != originID || queued.OrganizationID != organizationID ||
		queued.LedgerID != ledgerID || queued.TransactionInput.IsEmpty() {
		return "", pkg.ValidateBusinessError(constant.ErrRevertReconciliationRequired, constant.EntityTransaction)
	}

	legacyHash, err := legacyRevertIdempotencyHash(queued.TransactionInput)
	if err != nil {
		return "", err
	}

	return utils.IdempotencyInternalKey(organizationID, ledgerID, legacyHash), nil
}

func (handler *TransactionHandler) releaseFreshRevertClaim(ctx context.Context, claim *revertclaim.Claim, legacyKey string, allowAbsentLegacy bool) error {
	logger := libObservability.NewLoggerFromContext(ctx)

	if legacyKey != "" {
		exactLegacyKey, err := legacyRevertBarrierKeyFromClaim(claim)
		if err != nil || legacyKey != exactLegacyKey {
			return fmt.Errorf("release legacy revert fence: durable key mismatch")
		}
		if claim.LegacyFenceOwner == nil || *claim.LegacyFenceOwner != claim.ReverseTransactionID.String() {
			return fmt.Errorf("release legacy revert fence: durable owner mismatch")
		}
		released, err := handler.Command.TransactionRedisRepo.ReleaseOwnedKey(ctx, exactLegacyKey, *claim.LegacyFenceOwner)
		if err != nil {
			logger.Log(ctx, libLog.LevelWarn, "Failed to release legacy revert fence", libLog.Err(err))

			return fmt.Errorf("release legacy revert fence: %w", err)
		}
		if !released {
			if !allowAbsentLegacy {
				return fmt.Errorf("release legacy revert fence: owner mismatch")
			}

			// An ambiguous acquisition may have failed before the script ran.
			// Releasing PostgreSQL is safe only when both same-slot records are
			// absent. Any surviving key may be our orphan or an old pod's fence;
			// neither may be deleted without exact ownership.
			values, readErr := handler.Command.TransactionRedisRepo.MGet(ctx,
				[]string{exactLegacyKey, exactLegacyKey + ":owner"})
			if readErr != nil || len(values) != 0 {
				return fmt.Errorf("release legacy revert fence: ownership unresolved")
			}
		}
	}

	released, err := handler.Command.ReleaseRevertClaim(ctx, claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed to release pre-mutation revert claim", libLog.String("transaction_id", claim.ReverseTransactionID.String()), libLog.Err(err))

		return fmt.Errorf("release pre-mutation revert claim: %w", err)
	}
	if !released {
		return fmt.Errorf("release pre-mutation revert claim: claim was not released")
	}

	return nil
}

func (handler *TransactionHandler) failRevertClaim(ctx context.Context, claim *revertclaim.Claim, execution *revertExecutionState, legacyKey string, cause error) error {
	if isRevertExecutionFenceRejected(execution, cause) {
		// A recovery owner may already have released this old claim and installed
		// successor origin/legacy barriers. The same-slot Lua lease proved this
		// request did not move money; it must not delete or transition barriers
		// that may now belong to the successor.
		return cause
	}

	if mayReleaseRevertFences(execution, cause) {
		if execution.SeedWritten {
			backupKey := utils.TransactionInternalKey(claim.OrganizationID, claim.LedgerID, claim.ReverseTransactionID.String())
			removed, err := handler.Command.TransactionRedisRepo.RemoveMessageFromQueueIfStatus(ctx, backupKey,
				constant.CREATED, claim.ReverseTransactionID.String(), mmodel.TransactionOutcomeCommitted, true)
			if err != nil || !removed {
				return handler.requireRevertReconciliation(ctx, claim, "pre_movement_backup_cleanup_failed")
			}
		}
		if err := handler.releaseOwnedRevertOriginFence(ctx, claim); err != nil {
			return handler.requireRevertReconciliation(ctx, claim, "pre_movement_origin_fence_cleanup_failed")
		}
		released, err := handler.Command.TransactionRedisRepo.ReleaseOwnedKey(ctx, revertExecutionFenceKey(claim),
			claim.ReverseTransactionID.String())
		if err != nil || !released {
			return handler.requireRevertReconciliation(ctx, claim, "pre_movement_execution_fence_cleanup_failed")
		}
		if err := handler.releaseFreshRevertClaim(ctx, claim, legacyKey, false); err != nil {
			return handler.requireRevertReconciliation(ctx, claim, "pre_movement_fence_cleanup_failed")
		}

		return cause
	}

	reason := "balance_commit_outcome_ambiguous"
	if execution.SeedWriteAmbiguous {
		reason = "revert_seed_write_outcome_ambiguous"
	} else if execution.BalanceCommitted {
		reason = "post_balance_persistence_incomplete"
	}

	return handler.requireRevertReconciliation(ctx, claim, reason)
}

func originRevertIdempotencyKey(claim *revertclaim.Claim) string {
	hash := libCommons.HashSHA256(utils.RevertIdempotencyHashSource(claim.OriginTransactionID))

	return utils.IdempotencyInternalKey(claim.OrganizationID, claim.LedgerID, hash)
}

func (handler *TransactionHandler) acquireOriginRevertBarrier(ctx context.Context, organizationID, ledgerID,
	reverseID uuid.UUID, key, hash string) (*string, *transaction.Transaction, error) {
	if key == "" {
		key = hash
	}
	internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)
	acquired, err := handler.Command.TransactionRedisRepo.AcquireOwnedKey(ctx, internalKey, reverseID.String(), 0)
	if err != nil {
		// The script may have committed before its response was lost. Returning
		// the exact key lets failRevertClaim compare-delete by reserved reverse.
		return &internalKey, nil, fmt.Errorf("claim origin revert fence: %w", err)
	}
	if acquired {
		return &internalKey, nil, nil
	}

	value, err := handler.Command.TransactionRedisRepo.Get(ctx, internalKey)
	if err != nil && !errors.Is(err, redislib.Nil) {
		return &internalKey, nil, fmt.Errorf("read origin revert fence: %w", err)
	}
	if value == "" {
		return &internalKey, nil, pkg.ValidateBusinessError(constant.ErrIdempotencyKey,
			"RevertTransaction", key)
	}

	replay := &transaction.Transaction{}
	if err := json.Unmarshal([]byte(value), replay); err != nil {
		return &internalKey, nil, fmt.Errorf("decode origin revert fence: %w", err)
	}

	return &internalKey, replay, nil
}

func revertExecutionFenceKey(claim *revertclaim.Claim) string {
	return utils.TransactionBalanceExecutionKey(claim.OrganizationID, claim.LedgerID, claim.ReverseTransactionID)
}

func (handler *TransactionHandler) releaseOwnedRevertOriginFence(ctx context.Context, claim *revertclaim.Claim) error {
	originKey := originRevertIdempotencyKey(claim)
	released, err := handler.Command.TransactionRedisRepo.ReleaseOwnedKey(ctx, originKey, claim.ReverseTransactionID.String())
	if err != nil {
		return fmt.Errorf("release origin revert fence: %w", err)
	}
	if released {
		return nil
	}

	// An acquisition response can be lost before either same-slot record is
	// created. Absence of both is the only owner-less state that may proceed;
	// any main value or owner token can belong to a successor and is never
	// deleted by this request.
	values, err := handler.Command.TransactionRedisRepo.MGet(ctx, []string{originKey, originKey + ":owner"})
	if err != nil || len(values) != 0 {
		return fmt.Errorf("release origin revert fence: ownership unresolved")
	}

	return nil
}

func (handler *TransactionHandler) completeOriginRevertBarrier(ctx context.Context, internalKey *string, reverseID uuid.UUID,
	reverse *transaction.Transaction, ttl time.Duration) error {
	if internalKey == nil || reverse == nil || reverse.ID != reverseID.String() || reverse.ParentTransactionID == nil {
		return fmt.Errorf("complete origin revert fence: reverse mismatch")
	}
	originID, err := uuid.Parse(*reverse.ParentTransactionID)
	if err != nil {
		return fmt.Errorf("complete origin revert fence: invalid origin: %w", err)
	}
	claim := &revertclaim.Claim{OriginTransactionID: originID, ReverseTransactionID: reverseID}

	value, err := libCommons.StructToJSONString(*reverse)
	if err != nil {
		return fmt.Errorf("complete origin revert fence: %w", err)
	}
	completed, err := handler.Command.TransactionRedisRepo.CompleteOwnedKey(ctx, *internalKey, reverseID.String(), value, ttl)
	if err == nil && completed {
		return nil
	}

	existingValue, readErr := handler.Command.TransactionRedisRepo.Get(ctx, *internalKey)
	if readErr != nil && !errors.Is(readErr, redislib.Nil) {
		return fmt.Errorf("verify origin revert fence completion: %w", readErr)
	}
	if existingValue != "" {
		existing := &transaction.Transaction{}
		if json.Unmarshal([]byte(existingValue), existing) == nil && reverseReplayMatchesDurable(existing, reverse, claim) {
			return nil
		}

		return fmt.Errorf("complete origin revert fence: replay mismatch")
	}
	if err != nil {
		return fmt.Errorf("complete origin revert fence: %w", err)
	}

	// A retry can discover a persisted old-pod reverse or a crash after
	// PostgreSQL persistence but before the origin replay was materialized.
	// Reclaim only an absent pair with the same reserved reverse ID.
	_, acquireErr := handler.Command.TransactionRedisRepo.AcquireOwnedKey(ctx, *internalKey, reverseID.String(), 0)
	if acquireErr != nil {
		return fmt.Errorf("reclaim origin revert fence: %w", acquireErr)
	}
	completed, err = handler.Command.TransactionRedisRepo.CompleteOwnedKey(ctx, *internalKey, reverseID.String(), value, ttl)
	if err != nil {
		return fmt.Errorf("complete reclaimed origin revert fence: %w", err)
	}
	if completed {
		return nil
	}
	// A concurrent retry may already have materialized the exact replay. The
	// durable reverse ID is the shared owner token, so both retries may safely
	// attempt completion; neither can complete another origin's pair.
	existingValue, verifyErr := handler.Command.TransactionRedisRepo.Get(ctx, *internalKey)
	if verifyErr == nil && existingValue != "" {
		existing := &transaction.Transaction{}
		if json.Unmarshal([]byte(existingValue), existing) == nil && reverseReplayMatchesDurable(existing, reverse, claim) {
			return nil
		}
	}

	return fmt.Errorf("complete reclaimed origin revert fence: owner mismatch")
}

// recoverProvenPreMovementRevert handles the only crash state that can be
// retried automatically: a valid queue seed exists but the balance Lua outcome
// is absent. The PostgreSQL transition elects one cleanup owner. It keeps the
// durable claim in RECOVERING until every Redis barrier is cleared, then
// releases the claim last so a new request cannot race stale cleanup.
func (handler *TransactionHandler) recoverProvenPreMovementRevert(ctx context.Context, claim *revertclaim.Claim) (bool, error) {
	if claim.State != revertclaim.StateClaimed && claim.State != revertclaim.StateRecovering {
		return false, nil
	}
	if claim.RedisGeneration == nil || strings.TrimSpace(*claim.RedisGeneration) == "" {
		return false, handler.requireRevertReconciliation(ctx, claim, "pre_movement_redis_generation_missing")
	}
	if handler.RevertUpdateFreeze == nil {
		return false, handler.requireRevertReconciliation(ctx, claim, "pre_movement_redis_durability_unavailable")
	}
	if err := handler.RevertUpdateFreeze.FinancialDurability(ctx); err != nil {
		return false, handler.requireRevertReconciliation(ctx, claim, "pre_movement_redis_durability_unhealthy")
	}

	// Revalidate the complete Redis proof on every recovery election, including
	// a re-election after a RECOVERING owner crashed. A stale database state can
	// never authorize cleanup if an outcome or live attempt appeared meanwhile.
	evidence, generationMatches, err := handler.Command.TransactionRedisRepo.TransactionEconomicEvidenceExists(ctx,
		claim.OrganizationID, claim.LedgerID, claim.ReverseTransactionID, *claim.RedisGeneration)
	if err != nil {
		return false, fmt.Errorf("read revert economic evidence: %w", err)
	}
	if !generationMatches {
		return false, handler.requireRevertReconciliation(ctx, claim, "pre_movement_redis_generation_mismatch")
	}

	outcomeValue, err := handler.Command.TransactionRedisRepo.Get(ctx,
		utils.TransactionBalanceOutcomeKey(claim.OrganizationID, claim.LedgerID, claim.ReverseTransactionID))
	if err != nil {
		return false, fmt.Errorf("read revert balance outcome: %w", err)
	}
	if outcomeValue != "" {
		return false, nil
	}
	executionKey := revertExecutionFenceKey(claim)
	executionValues, err := handler.Command.TransactionRedisRepo.MGet(ctx,
		[]string{executionKey, executionKey + ":owner"})
	if err != nil {
		return false, fmt.Errorf("read revert execution attempt: %w", err)
	}
	if len(executionValues) != 0 {
		return false, nil
	}

	backup, err := handler.Command.TransactionRedisRepo.ReadMessageFromQueue(ctx,
		utils.TransactionInternalKey(claim.OrganizationID, claim.LedgerID, claim.ReverseTransactionID.String()))
	if err != nil && !errors.Is(err, redislib.Nil) {
		return false, fmt.Errorf("read pre-movement revert seed: %w", err)
	}
	backupPresent := err == nil
	if evidence && !backupPresent {
		// The atomic snapshot observed some evidence, but the follow-up reads did
		// not find an exact seed. A concurrent or partial cleanup is not proof of
		// pre-movement absence.
		return false, nil
	}
	queued := mmodel.TransactionRedisQueue{}
	if backupPresent {
		if err := json.Unmarshal(backup, &queued); err != nil || queued.TransactionID != claim.ReverseTransactionID ||
			queued.ParentTransactionID == nil || *queued.ParentTransactionID != claim.OriginTransactionID ||
			queued.AttemptOwner != claim.ReverseTransactionID.String() ||
			queued.ExpectedOutcome != mmodel.TransactionOutcomeCommitted || len(queued.BalancesAfter) > 0 {
			return false, nil
		}
	}

	recoveryOwner, err := handler.Command.BeginPreMutationRevertRecovery(ctx, claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID)
	if err != nil {
		return false, err
	}
	if !recoveryOwner {
		return false, nil
	}

	expectedStatus := constant.CREATED
	if backupPresent {
		expectedStatus = queued.TransactionStatus
	}
	attempt := mmodel.BalanceExecutionAttempt{
		ExecutionKey:    utils.TransactionBalanceExecutionKey(claim.OrganizationID, claim.LedgerID, claim.ReverseTransactionID),
		OutcomeKey:      utils.TransactionBalanceOutcomeKey(claim.OrganizationID, claim.LedgerID, claim.ReverseTransactionID),
		Owner:           claim.ReverseTransactionID.String(),
		Outcome:         mmodel.TransactionOutcomeCommitted,
		Identity:        claim.ReverseTransactionID,
		RedisGeneration: *claim.RedisGeneration,
	}
	releasedEvidence, generationMatches, releaseErr := handler.Command.TransactionRedisRepo.ReleaseProvenPreMovementRevert(ctx,
		claim.OrganizationID, claim.LedgerID, claim.OriginTransactionID, claim.ReverseTransactionID,
		expectedStatus, attempt)
	if releaseErr != nil {
		return false, handler.requireRevertReconciliation(ctx, claim, "pre_movement_backup_cleanup_failed")
	}
	if !releasedEvidence {
		if !generationMatches {
			return false, handler.requireRevertReconciliation(ctx, claim, "pre_movement_redis_generation_changed_during_cleanup")
		}

		return false, handler.requireRevertReconciliation(ctx, claim, "pre_movement_backup_cleanup_failed")
	}
	if err := handler.releaseOwnedRevertOriginFence(ctx, claim); err != nil {
		return false, handler.requireRevertReconciliation(ctx, claim, "pre_movement_origin_fence_cleanup_failed")
	}
	if claim.LegacyFenceKey != nil && *claim.LegacyFenceKey != "" {
		legacyKey := *claim.LegacyFenceKey
		if claim.LegacyFenceOwner == nil || *claim.LegacyFenceOwner != claim.ReverseTransactionID.String() {
			return false, handler.requireRevertReconciliation(ctx, claim, "pre_movement_legacy_fence_owner_missing")
		}
		released, err := handler.Command.TransactionRedisRepo.ReleaseOwnedKey(ctx, legacyKey, *claim.LegacyFenceOwner)
		if err != nil {
			return false, handler.requireRevertReconciliation(ctx, claim, "pre_movement_legacy_fence_cleanup_failed")
		}
		if !released {
			values, readErr := handler.Command.TransactionRedisRepo.MGet(ctx, []string{legacyKey, legacyKey + ":owner"})
			if readErr != nil {
				return false, handler.requireRevertReconciliation(ctx, claim, "pre_movement_legacy_fence_owner_mismatch")
			}

			ownedByClaim := values[legacyKey+":owner"] == claim.ReverseTransactionID.String()
			if value := values[legacyKey]; value != "" {
				replay := &transaction.Transaction{}
				ownedByClaim = ownedByClaim || (json.Unmarshal([]byte(value), replay) == nil && reverseMatchesClaim(replay, claim))
			}

			// Bridge still shares the payload-hash barrier with phase zero, so
			// any surviving pair is unresolved. Final only cleans an exact bridge
			// owner; a foreign legacy collision belongs to another origin and must
			// neither be deleted nor block origin-scoped recovery.
			if ownedByClaim || (handler.activeRevertIdempotencyMode() != revertIdempotencyModeFinal && len(values) != 0) {
				return false, handler.requireRevertReconciliation(ctx, claim, "pre_movement_legacy_fence_owner_mismatch")
			}
		}
	}

	released, err := handler.Command.ReleaseRevertClaim(ctx, claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID)
	if err != nil {
		return false, err
	}
	if !released {
		persistedClaim, readErr := handler.Command.GetRevertClaim(ctx, claim.OrganizationID, claim.LedgerID, claim.OriginTransactionID)
		if readErr != nil {
			return false, readErr
		}
		if persistedClaim != nil {
			return false, handler.requireRevertReconciliation(ctx, claim, "pre_movement_claim_release_failed")
		}
	}

	return true, nil
}

func (handler *TransactionHandler) requireRevertReconciliation(ctx context.Context, claim *revertclaim.Claim, reason string) error {
	if err := handler.Command.MarkRevertClaim(ctx, claim.OrganizationID, claim.LedgerID, claim.OriginTransactionID,
		claim.ReverseTransactionID, revertclaim.StateReconciliationRequired, &reason); err != nil {
		return fmt.Errorf("mark revert reconciliation required: %w", err)
	}

	return pkg.ValidateBusinessError(constant.ErrRevertReconciliationRequired, constant.EntityTransaction)
}

// mayReleaseRevertFences is the single release decision for the reversal
// claim, execution lease, and legacy/origin Redis barriers. Once the balance
// Lua command was dispatched, only a script-declared rejection proves that
// every mutation rolled back. Transport failures preserve every barrier
// because Redis may have committed the balances and atomic backup marker after
// the client lost the response.
func mayReleaseRevertFences(execution *revertExecutionState, cause error) bool {
	if execution == nil {
		return true
	}
	if execution.SeedWriteAmbiguous {
		return false
	}
	if !execution.BalanceAttempted {
		return true
	}

	return !execution.BalanceCommitted && isDefinitiveBalanceRejection(cause)
}
