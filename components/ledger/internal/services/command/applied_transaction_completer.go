// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
)

const (
	engineRecoveryAckDeferredMetricName = "engine_recovery_ack_deferred_total"
	engineRecoveryAckDeferredMetricDesc = "Number of synchronous engine recovery acknowledgments deferred or unconfirmed."
)

// AppliedTransactionCompleter durably projects an accounting result already
// applied by the engine and reports the transaction status confirmed by SQL.
// Completion and recovery must never invoke the engine or mutate balances.
type AppliedTransactionCompleter interface {
	Complete(context.Context, *TransactionCompletionRecord) (TransactionCompletionResult, error)
}

// EngineRecoveryAcknowledger removes the exact engine recovery record only
// after the corresponding transaction and metadata projections are durable.
// Acknowledgment never applies balances or completes persistence.
type EngineRecoveryAcknowledger interface {
	AcknowledgeEngineRecovery(context.Context, *TransactionCompletionRecord, TransactionCompletionResult) error
}

func (uc *UseCase) acknowledgeEngineRecovery(
	ctx context.Context,
	logger libLog.Logger,
	record *TransactionCompletionRecord,
	completion TransactionCompletionResult,
) {
	if uc.EngineRecoveryAcknowledger == nil {
		return
	}

	if err := uc.EngineRecoveryAcknowledger.AcknowledgeEngineRecovery(ctx, record, completion); err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Engine recovery acknowledgment deferred or unconfirmed",
			libLog.String("transaction_id", record.TransactionID.String()),
			libLog.String("execution_id", record.ExecutionID.String()),
			libLog.Err(err))

		if uc.MetricsFactory != nil {
			if metricErr := uc.MetricsFactory.AddCounter(
				ctx,
				engineRecoveryAckDeferredMetricName,
				engineRecoveryAckDeferredMetricDesc,
				"1",
				nil,
				1,
			); metricErr != nil {
				logger.Log(ctx, libLog.LevelDebug, "Unable to record deferred engine recovery acknowledgment", libLog.Err(metricErr))
			}
		}
	}
}
