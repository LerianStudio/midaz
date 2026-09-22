// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	// accountClosingRecoveryScanCount is the COUNT hint of one recovery page. It is
	// a hint, so a page may come back larger; the walk never drops what it receives.
	accountClosingRecoveryScanCount = 200

	// maxAccountClosingRecoveryBytes bounds how much recovery payload one closing
	// attempt reads. Exhausting it ends the walk WITHOUT proof of absence, which is
	// refused rather than passed: the budget bounds the cost of the scan, never the
	// strength of its conclusion. It is charged only after the page it paid for has
	// been inspected, so evidence already in hand still decides.
	maxAccountClosingRecoveryBytes = 16 << 20
)

// accountClosingRecoverySources are the recovery origins a closing walks, each
// with its own cursor. Fields are unique only inside one hash, so the two are
// never merged; legacy payloads are accepted only where they are persisted.
var accountClosingRecoverySources = []struct {
	source      txRedis.RecoveryQueueSource
	allowLegacy bool
}{
	{source: txRedis.RecoveryQueueSourceLegacyBackup, allowLegacy: true},
	{source: txRedis.RecoveryQueueSourceEngineRecover, allowLegacy: false},
}

// verifyNoAccountClosingRecoveryPending refuses the closing while an execution
// that touched the account is still waiting for its completion.
//
// Both recovery origins are walked to their terminal cursor under the caller's
// deadline, reading records without changing their format, removing them or
// touching an acknowledgment. A record still there means the completer has work
// left on that execution, so the refusal is the temporary one: the existing
// completer finishes it, and the closing never takes that job over or reapplies a
// movement.
//
// One empty page proves nothing — HSCAN can pass a bucket without returning an
// entry — so only a terminal cursor on EVERY origin concludes the walk. A record
// that cannot be read, and a budget that runs out before the cursors terminate,
// both leave absence unproven, and unproven absence is refused as indeterminate.
// The page whose bytes exhaust the budget is inspected before the budget is
// charged: its read is already paid for, so a conclusive record on it decides
// rather than being lost to the weaker refusal.
//
// A record acknowledged while the walk is in flight simply stops appearing, which
// under the current acknowledgment contract means its persistence concluded. The
// SQL evidence is read AFTER this walk, so that conclusion is confirmed there
// rather than assumed here.
func (uc *UseCase) verifyNoAccountClosingRecoveryPending(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("verify account closing recovery: %w", err)
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.verify_account_closing_recovery")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_id", accountID.String()),
	)

	scanned, budget := 0, maxAccountClosingRecoveryBytes

	for _, origin := range accountClosingRecoverySources {
		for cursor, walked := uint64(0), false; !walked || cursor != 0; {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("verify account closing recovery: %w", err)
			}

			page, err := uc.TransactionRedisRepo.ScanRecoveryMessages(ctx, origin.source, cursor, accountClosingRecoveryScanCount)
			if err != nil {
				libOpentelemetry.HandleSpanError(span, "Failed to scan the recovery records for closing", err)
				logger.Log(ctx, libLog.LevelError, "Failed to scan the recovery records for closing",
					libLog.String("recovery_source", string(origin.source)), libLog.Err(err))

				return pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)
			}

			cursor, walked = page.Cursor, true
			scanned += len(page.Records)

			if err := inspectAccountClosingRecoveryPage(ctx, span, logger, page, origin.allowLegacy, organizationID, ledgerID, accountID); err != nil {
				return err
			}

			budget -= page.Bytes
			if budget < 0 {
				exhausted := errors.New("recovery scan byte budget exhausted before the cursors terminated")

				libOpentelemetry.HandleSpanError(span, "Recovery scan exceeded its byte budget before proving absence", exhausted)
				logger.Log(ctx, libLog.LevelError, "Recovery scan exceeded its byte budget before proving absence",
					libLog.String("recovery_source", string(origin.source)), libLog.Err(exhausted))

				return pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)
			}
		}
	}

	span.SetAttributes(attribute.Int("app.account_closing.recovery_records_scanned", scanned))

	return nil
}

// inspectAccountClosingRecoveryPage classifies one page against the account.
func inspectAccountClosingRecoveryPage(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	page txRedis.RecoveryScanPage,
	allowLegacy bool,
	organizationID, ledgerID, accountID uuid.UUID,
) error {
	for _, record := range page.Records {
		touches, err := recoveryRecordTouchesAccount([]byte(record.Payload), allowLegacy, organizationID, ledgerID, accountID)
		if err != nil {
			indeterminate := pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)

			libOpentelemetry.HandleSpanError(span, "Failed to read a recovery record while closing an account", err)
			logger.Log(ctx, libLog.LevelError, "Failed to read a recovery record while closing an account",
				libLog.String("recovery_source", string(page.Source)), libLog.Err(err))

			return indeterminate
		}

		if !touches {
			continue
		}

		pending := pkg.ValidateBusinessError(constant.ErrAccountClosingPersistencePending, constant.EntityAccount)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "An execution of this account is still in completion", pending)
		logger.Log(ctx, libLog.LevelWarn, "An execution of this account is still in completion",
			libLog.String("recovery_source", string(page.Source)))

		return pending
	}

	return nil
}

// recoveryRecordTouchesAccount reports whether one recovery record describes work
// over the given account inside the given scope.
//
// The two persisted shapes are read as they are: the versioned engine envelope
// names its accounts through the balance snapshot of each projected operation,
// while the legacy record names them on its balances and operations. Both are
// decoded through the existing readers, so the formats stay untouched and a record
// neither reader accepts is an error rather than a silent "does not touch".
func recoveryRecordTouchesAccount(raw []byte, allowLegacy bool, organizationID, ledgerID, accountID uuid.UUID) (bool, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return false, errors.New("recovery record is not a JSON object")
	}

	if _, versioned := fields["formatVersion"]; versioned {
		return engineRecoveryRecordTouchesAccount(raw, organizationID, ledgerID, accountID)
	}

	if !allowLegacy {
		return false, errors.New("engine recovery record requires format version 2")
	}

	return legacyRecoveryRecordTouchesAccount(raw, organizationID, ledgerID, accountID)
}

func engineRecoveryRecordTouchesAccount(raw []byte, organizationID, ledgerID, accountID uuid.UUID) (bool, error) {
	writeBehind, err := DecodeTransactionWriteBehindEnvelope(raw)
	if err != nil {
		return false, fmt.Errorf("decode engine recovery record: %w", err)
	}
	record := writeBehind.Record

	if record.OrganizationID != organizationID || record.LedgerID != ledgerID {
		return false, nil
	}

	plan, err := DecodeTransactionCompletionPlan([]byte(record.Payload))
	if err != nil {
		return false, fmt.Errorf("decode engine recovery plan: %w", err)
	}

	for _, spec := range plan.OperationSpecs {
		if strings.EqualFold(spec.Balance.AccountID, accountID.String()) {
			return true, nil
		}
	}

	return false, nil
}

func legacyRecoveryRecordTouchesAccount(raw []byte, organizationID, ledgerID, accountID uuid.UUID) (bool, error) {
	var legacy mmodel.TransactionRedisQueue
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return false, fmt.Errorf("decode legacy recovery record: %w", err)
	}

	if legacy.TransactionID == uuid.Nil || legacy.OrganizationID == uuid.Nil || legacy.LedgerID == uuid.Nil {
		return false, errors.New("legacy recovery record has incomplete identity")
	}

	if legacy.OrganizationID != organizationID || legacy.LedgerID != ledgerID {
		return false, nil
	}

	for _, balances := range [][]mmodel.BalanceRedis{legacy.Balances, legacy.BalancesAfter} {
		for _, balance := range balances {
			if strings.EqualFold(balance.AccountID, accountID.String()) {
				return true, nil
			}
		}
	}

	for _, op := range legacy.Operations {
		if strings.EqualFold(op.AccountID, accountID.String()) {
			return true, nil
		}
	}

	return false, nil
}
