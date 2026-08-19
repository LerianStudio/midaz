// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"fmt"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

const tracerOutcomePreparedTimeout = 30 * time.Second

func (handler *TransactionHandler) durableTracerOutcomeEnabled(
	settings mmodel.TracerSettings,
	transactionStatus string,
	honoredTracerSkip bool,
) bool {
	return handler.TracerOutcomeV2 && transactionStatus != constant.NOTED && !honoredTracerSkip &&
		handler.tracerReservationEnabled(settings)
}

func (handler *TransactionHandler) buildTracerOutcomeAttempt(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	transactionStatus string,
	existing *mmodel.BalanceExecutionAttempt,
) (*mmodel.BalanceExecutionAttempt, bool, error) {
	attempt := existing
	acquiredHere := false

	if attempt == nil {
		owner, err := newPodRequestToken()
		if err != nil {
			return nil, false, err
		}

		attempt = &mmodel.BalanceExecutionAttempt{
			ExecutionKey: utils.TransactionBalanceExecutionKey(organizationID, ledgerID, transactionID),
			OutcomeKey:   utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, transactionID),
			Owner:        owner, Outcome: mmodel.TransactionOutcomeCommitted, Identity: transactionID,
		}
		if transactionStatus == constant.PENDING {
			attempt.ExecutionKey = utils.TransactionPendingBalanceExecutionKey(organizationID, ledgerID, transactionID)
			attempt.OutcomeKey = utils.TransactionPendingBalanceOutcomeKey(organizationID, ledgerID, transactionID)
		}

		acquired, err := handler.Command.TransactionRedisRepo.AcquireOwnedKey(ctx, attempt.ExecutionKey, owner, 0)
		if err != nil {
			return nil, false, fmt.Errorf("reserve tracer outcome execution attempt: %w", err)
		}

		if !acquired {
			return nil, false, fmt.Errorf("reserve tracer outcome execution attempt: already owned")
		}

		acquiredHere = true
	}

	attempt.TracerOutcomeID = utils.TransactionTracerOutcomeID(transactionID)

	attempt.TracerOutcomeState = mmodel.TracerOutcomeCommitted
	if transactionStatus == constant.PENDING {
		attempt.TracerOutcomeState = mmodel.TracerOutcomePendingHeld
	}

	return attempt, acquiredHere, nil
}

func (handler *TransactionHandler) prepareTracerOutcome(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	attempt *mmodel.BalanceExecutionAttempt,
	plan *mmodel.ExpectedEconomicPlan,
) error {
	if handler.MultiTenantEnabled {
		if tmcore.GetTenantIDContext(ctx) == "" {
			return fmt.Errorf("prepare durable tracer outcome: tenant context is required")
		}
	}

	preparedAt := time.Now().UTC()

	record, err := handler.Command.TransactionRedisRepo.PrepareTracerOutcome(ctx, organizationID, ledgerID, transactionID,
		attempt.Owner, attempt.TracerOutcomeID, plan, preparedAt, preparedAt.Add(tracerOutcomePreparedTimeout))
	if err == nil {
		if record != nil && record.State == mmodel.TracerOutcomePrepared {
			return nil
		}

		state := "missing"
		if record != nil {
			state = record.State
		}

		err = fmt.Errorf("tracer outcome is already %s", state)
	}

	// A lost response after Redis committed PREPARED is safe to adopt only when
	// every deterministic identity and the final plan match exactly.
	record, readErr := handler.Command.TransactionRedisRepo.ReadTracerOutcome(ctx, organizationID, ledgerID, transactionID)
	if readErr == nil && record != nil && record.State == mmodel.TracerOutcomePrepared &&
		record.OutcomeID == attempt.TracerOutcomeID && record.Owner == attempt.Owner &&
		record.EconomicPlanVersion == fmt.Sprintf("%d", plan.Version) && record.EconomicPlanDigest == plan.Digest {
		return nil
	}

	return fmt.Errorf("prepare durable tracer outcome: %w", err)
}

func (handler *TransactionHandler) admitDurableTracerOutcome(ctx context.Context) error {
	if handler.FinancialRedisDurability == nil {
		return fmt.Errorf("financial Redis durability guard is not configured")
	}

	if err := handler.FinancialRedisDurability.FinancialDurability(ctx); err != nil {
		return fmt.Errorf("financial Redis durability: %w", err)
	}

	return nil
}

func (handler *TransactionHandler) abortPreparedTracerOutcome(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	attempt *mmodel.BalanceExecutionAttempt,
) error {
	_, err := handler.Command.TransactionRedisRepo.AbortPreparedTracerOutcome(ctx, organizationID, ledgerID,
		transactionID, attempt.Owner, attempt.TracerOutcomeID, time.Now().UTC())

	return err
}

func tracerOutcomeResult(record *mmodel.TracerOutcomeRecord, organizationID, ledgerID uuid.UUID) *mmodel.BalanceAtomicResult {
	if record == nil || record.EconomicOutcome == nil {
		return nil
	}

	result, err := balanceAtomicResultFromOutcome(record.EconomicOutcome, organizationID, ledgerID)
	if err != nil {
		// Corrupt durable evidence must not be adopted as a terminal result. The
		// caller maps the missing result to the reconciliation error.
		return nil
	}

	return result
}
