// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

const (
	// recoveryScanCount is the COUNT hint of one recovery page. It is a hint, so a
	// page may come back larger; the walk never drops what it receives.
	recoveryScanCount = 200

	// maxRecoveryWalkBytes bounds the recovery payload one walk reads. Spending it while
	// pages remain leaves absence unproven, which is refused: the budget bounds the cost
	// of the walk, never the strength of its conclusion.
	maxRecoveryWalkBytes = 16 << 20
)

// recoverySource is one recovery hash, walked with its own cursor because fields are
// unique only inside one hash. Legacy payloads are accepted only where they persist.
type recoverySource struct {
	source      txRedis.RecoveryQueueSource
	allowLegacy bool
}

var (
	// engineRecoverySources hold the only records that carry fee-debt changes.
	engineRecoverySources = []recoverySource{{source: txRedis.RecoveryQueueSourceEngineRecover}}
	allRecoverySources    = []recoverySource{
		{source: txRedis.RecoveryQueueSourceLegacyBackup, allowLegacy: true},
		{source: txRedis.RecoveryQueueSourceEngineRecover},
	}
)

// recoveryRecord is one record of the walked scope as persisted: the engine completion
// record or, where its source keeps them, the legacy backup record.
type recoveryRecord struct {
	engine *TransactionCompletionRecord
	legacy *mmodel.TransactionRedisQueue
}

// walkRecovery hands every in-scope record of sources to match and stops at its first
// refusal. Only a terminal cursor on every source proves absence: a failed scan, an
// unreadable record or a budget spent while pages remain is refused as 0520.
func (uc *UseCase) walkRecovery(ctx context.Context, organizationID, ledgerID uuid.UUID, sources []recoverySource, match func(recoveryRecord) error) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.walk_recovery_records")
	defer span.End()

	scanned, budget := 0, maxRecoveryWalkBytes

	for _, origin := range sources {
		for cursor, walked := uint64(0), false; !walked || cursor != 0; walked = true {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("walk recovery records: %w", err)
			}

			// Charged after a page is inspected and checked only before the next read, so
			// evidence in hand still decides and a walk that reached its end concludes.
			if budget < 0 {
				return refuseRecoveryWalk(ctx, span, logger, origin, errors.New("recovery byte budget spent before the cursors terminated"))
			}

			page, err := uc.TransactionRedisRepo.ScanRecoveryMessages(ctx, origin.source, cursor, recoveryScanCount)
			if err == nil {
				err = inspectRecoveryPage(page, origin.allowLegacy, organizationID, ledgerID, match)
			}

			if err != nil {
				return refuseRecoveryWalk(ctx, span, logger, origin, err)
			}

			scanned += len(page.Records)
			cursor, budget = page.Cursor, budget-page.Bytes
		}
	}

	span.SetAttributes(attribute.Int("app.recovery.records_scanned", scanned))

	return nil
}

// refuseRecoveryWalk ends a walk: a refusal of a record passes through, and any other
// failure left absence unproven, which is refused as indeterminate (0520).
func refuseRecoveryWalk(ctx context.Context, span trace.Span, logger libLog.Logger, origin recoverySource, err error) error {
	if pkg.IsBusinessError(err) {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "A recovery record refuses the operation", err)

		return err
	}

	libOpentelemetry.HandleSpanError(span, "The recovery walk could not prove absence", err)
	logger.Log(ctx, libLog.LevelError, "The recovery walk could not prove absence",
		libLog.String("recovery_source", string(origin.source)), libLog.Err(err))

	return pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)
}

func inspectRecoveryPage(page txRedis.RecoveryScanPage, allowLegacy bool, organizationID, ledgerID uuid.UUID, match func(recoveryRecord) error) error {
	for _, entry := range page.Records {
		record, err := readRecoveryRecord([]byte(entry.Payload), allowLegacy, organizationID, ledgerID)
		if err == nil && record != nil {
			err = match(*record)
		}

		if err != nil {
			return err
		}
	}

	return nil
}

// readRecoveryRecord decodes one record through the existing readers, so the formats stay
// untouched and a record neither accepts is an error rather than a silent "does not match".
// A record of another scope answers nil.
func readRecoveryRecord(raw []byte, allowLegacy bool, organizationID, ledgerID uuid.UUID) (*recoveryRecord, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, errors.New("recovery record is not a JSON object")
	}

	if _, versioned := fields["formatVersion"]; versioned {
		envelope, err := DecodeTransactionWriteBehindEnvelope(raw)
		if err != nil {
			return nil, fmt.Errorf("decode engine recovery record: %w", err)
		}

		if envelope.Record.OrganizationID != organizationID || envelope.Record.LedgerID != ledgerID {
			return nil, nil
		}

		return &recoveryRecord{engine: &envelope.Record}, nil
	}

	if !allowLegacy {
		return nil, errors.New("engine recovery record requires format version 2")
	}

	var legacy mmodel.TransactionRedisQueue
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return nil, fmt.Errorf("decode legacy recovery record: %w", err)
	}

	if legacy.TransactionID == uuid.Nil || legacy.OrganizationID == uuid.Nil || legacy.LedgerID == uuid.Nil {
		return nil, errors.New("legacy recovery record has incomplete identity")
	}

	if legacy.OrganizationID != organizationID || legacy.LedgerID != ledgerID {
		return nil, nil
	}

	return &recoveryRecord{legacy: &legacy}, nil
}

// owesFeeDebt reports whether the record changes a fee debt owed to one of creditRefs.
func (r recoveryRecord) owesFeeDebt(creditRefs []string) bool {
	if r.engine == nil {
		return false
	}

	for _, change := range r.engine.Result.FeeDebt {
		if slices.Contains(creditRefs, change.CreditRef) {
			return true
		}
	}

	return false
}

// touchesAccount reports whether the record describes work over accountID: the engine
// record names accounts on the balance of each projected operation, the legacy record on
// its balances and operations.
func (r recoveryRecord) touchesAccount(accountID uuid.UUID) (bool, error) {
	id := accountID.String()

	if r.legacy != nil {
		for _, balance := range slices.Concat(r.legacy.Balances, r.legacy.BalancesAfter) {
			if strings.EqualFold(balance.AccountID, id) {
				return true, nil
			}
		}

		return slices.ContainsFunc(r.legacy.Operations, func(op mmodel.OperationRedis) bool {
			return strings.EqualFold(op.AccountID, id)
		}), nil
	}

	plan, err := DecodeTransactionCompletionPlan([]byte(r.engine.Payload))
	if err != nil {
		return false, fmt.Errorf("decode engine recovery plan: %w", err)
	}

	return slices.ContainsFunc(plan.OperationSpecs, func(spec OperationRecordSpec) bool {
		return strings.EqualFold(spec.Balance.AccountID, id)
	}), nil
}

// refuseAccountInCompletion refuses a record of an execution over accountID still waiting
// for its completion. The completer finishes it, so the refusal is the temporary one and
// the caller never takes that job over or reapplies a movement.
func refuseAccountInCompletion(accountID uuid.UUID) func(recoveryRecord) error {
	return func(record recoveryRecord) error {
		touches, err := record.touchesAccount(accountID)
		if err != nil || !touches {
			return err
		}

		return pkg.ValidateBusinessError(constant.ErrAccountClosingPersistencePending, constant.EntityAccount)
	}
}
