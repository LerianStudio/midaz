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

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/revertclaim"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

const (
	revertIdempotencyModeBridge = "bridge"
	revertIdempotencyModeFinal  = "final"
)

func (handler *TransactionHandler) activeRevertIdempotencyMode() string {
	if strings.EqualFold(handler.RevertIdempotencyMode, revertIdempotencyModeFinal) {
		return revertIdempotencyModeFinal
	}

	return revertIdempotencyModeBridge
}

func legacyRevertIdempotencyHash(input mtransaction.Transaction) (string, error) {
	mtransaction.ApplyDefaultBalanceKeys(input.Send.Source.From)
	mtransaction.ApplyDefaultBalanceKeys(input.Send.Distribute.To)

	serialized, err := libCommons.StructToJSONString(input)
	if err != nil {
		return "", err
	}

	return libCommons.HashSHA256(serialized), nil
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

func (handler *TransactionHandler) resolveDurableRevertClaim(ctx context.Context, claim *revertclaim.Claim) (*transaction.Transaction, bool, error) {
	persisted, err := handler.Query.GetParentByTransactionID(ctx, claim.OrganizationID, claim.LedgerID, claim.OriginTransactionID)
	if err != nil {
		return nil, false, err
	}

	if persisted != nil {
		if !reverseMatchesClaim(persisted, claim) {
			return nil, false, pkg.ValidateBusinessError(constant.ErrRevertReconciliationRequired, constant.EntityTransaction)
		}

		if err := handler.Command.MarkRevertClaim(ctx, claim.OrganizationID, claim.LedgerID, claim.OriginTransactionID,
			claim.ReverseTransactionID, revertclaim.StateCompleted, nil); err != nil {
			return nil, false, err
		}

		return persisted, true, nil
	}

	if claim.State == revertclaim.StateClaimed {
		return nil, false, pkg.ValidateBusinessError(constant.ErrIdempotencyKey, "RevertTransaction", claim.OriginTransactionID.String())
	}

	return nil, false, pkg.ValidateBusinessError(constant.ErrRevertReconciliationRequired, constant.EntityTransaction)
}

func (handler *TransactionHandler) adoptPersistedReverse(ctx context.Context, organizationID, ledgerID, originID uuid.UUID, persisted *transaction.Transaction) (*transaction.Transaction, bool, error) {
	reverseID, err := uuid.Parse(persisted.ID)
	if err != nil || !reverseBelongsToOrigin(persisted, originID) {
		return nil, false, pkg.ValidateBusinessError(constant.ErrRevertReconciliationRequired, constant.EntityTransaction)
	}

	claim, _, err := handler.Command.ClaimRevert(ctx, organizationID, ledgerID, originID, reverseID)
	if err != nil {
		return nil, false, err
	}
	if claim.ReverseTransactionID != reverseID {
		return nil, false, pkg.ValidateBusinessError(constant.ErrRevertReconciliationRequired, constant.EntityTransaction)
	}

	if err := handler.Command.MarkRevertClaim(ctx, organizationID, ledgerID, originID, reverseID, revertclaim.StateCompleted, nil); err != nil {
		return nil, false, err
	}

	return persisted, true, nil
}

func (handler *TransactionHandler) acquireLegacyRevertBarrier(ctx context.Context, organizationID, ledgerID, originID uuid.UUID, legacyHash string, claim *revertclaim.Claim) (string, *transaction.Transaction, error) {
	result, err := handler.Command.CreateOrCheckTransactionIdempotency(ctx, organizationID, ledgerID, "", legacyHash, pkgHTTP.ParseIdempotencyTTL(""))
	if err != nil {
		return "", nil, err
	}
	if result.Replay == nil {
		return *result.InternalKey, nil, nil
	}
	if !reverseBelongsToOrigin(result.Replay, originID) {
		return "", nil, pkg.ValidateBusinessError(constant.ErrIdempotencyKey, "RevertTransaction", originID.String())
	}
	if result.Replay.ID != claim.ReverseTransactionID.String() {
		return "", nil, pkg.ValidateBusinessError(constant.ErrRevertReconciliationRequired, constant.EntityTransaction)
	}

	return *result.InternalKey, result.Replay, nil
}

func (handler *TransactionHandler) releaseFreshRevertClaim(ctx context.Context, claim *revertclaim.Claim, legacyKey string) {
	logger := libObservability.NewLoggerFromContext(ctx)

	if legacyKey != "" {
		if err := handler.Command.TransactionRedisRepo.Del(ctx, legacyKey); err != nil {
			logger.Log(ctx, libLog.LevelWarn, "Failed to release legacy revert fence", libLog.Err(err))
		}
	}

	if _, err := handler.Command.ReleaseRevertClaim(ctx, claim.OrganizationID, claim.LedgerID, claim.OriginTransactionID, claim.ReverseTransactionID); err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed to release pre-mutation revert claim", libLog.String("transaction_id", claim.ReverseTransactionID.String()), libLog.Err(err))
	}
}

func (handler *TransactionHandler) failRevertClaim(ctx context.Context, claim *revertclaim.Claim, execution *revertExecutionState, legacyKey string, cause error) error {
	if mayReleaseRevertFences(execution, cause) {
		handler.releaseFreshRevertClaim(ctx, claim, legacyKey)

		return cause
	}

	reason := "balance_commit_outcome_ambiguous"
	if execution.BalanceCommitted {
		reason = "post_balance_persistence_incomplete"
	}

	return handler.requireRevertReconciliation(ctx, claim, reason)
}

func (handler *TransactionHandler) requireRevertReconciliation(ctx context.Context, claim *revertclaim.Claim, reason string) error {
	if err := handler.Command.MarkRevertClaim(ctx, claim.OrganizationID, claim.LedgerID, claim.OriginTransactionID,
		claim.ReverseTransactionID, revertclaim.StateReconciliationRequired, &reason); err != nil {
		return fmt.Errorf("mark revert reconciliation required: %w", err)
	}

	return pkg.ValidateBusinessError(constant.ErrRevertReconciliationRequired, constant.EntityTransaction)
}

// mayReleaseRevertFences is the single release decision for all three reversal
// barriers: the PostgreSQL claim and the legacy/origin Redis keys. Once the
// balance Lua command was dispatched, only a script-declared rejection proves
// that every mutation rolled back. Transport failures preserve every barrier
// because Redis may have committed the balances and atomic backup marker after
// the client lost the response.
func mayReleaseRevertFences(execution *revertExecutionState, cause error) bool {
	if execution == nil || !execution.BalanceAttempted {
		return true
	}

	return !execution.BalanceCommitted && isDefinitiveBalanceRejection(cause)
}
