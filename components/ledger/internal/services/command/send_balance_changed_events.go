// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"strings"
	"time"

	libObs "github.com/LerianStudio/lib-observability"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/shopspring/decimal"
)

// SendBalanceChangedEvents emits one generic, domain-agnostic balance.changed
// event per balance-affecting operation of a committed transaction.
//
// Fire-and-forget: launched as a goroutine alongside SendTransactionEvents /
// SendOverdraftEvents. Emission failures are logged/span-recorded but never
// propagate — this MUST NOT fail the parent transaction (mirrors the
// EmitImportant contract). Emission happens whenever streaming is enabled:
// when STREAMING_ENABLED=false the streaming client is a NoopEmitter, so this
// simply becomes a no-op — the same posture as balance.created.
//
// The event carries only Midaz identities + a generic Reason. It carries NO
// consumer-domain fields — the consumer (e.g. the br-sisbajud Connector) maps
// accountId -> its own domain key and produces its own trigger.
func (uc *UseCase) SendBalanceChangedEvents(ctx context.Context, tran *transaction.Transaction) {
	logger, tracer, _, _ := libObs.NewTrackingFromContext(ctx)

	if uc.Streaming == nil {
		return
	}

	if tran == nil || len(tran.Operations) == 0 {
		return
	}

	ctxSend, span := tracer.Start(ctx, "command.send_balance_changed_events_async")
	defer span.End()

	occurredAt := time.Now().UTC()

	for _, op := range tran.Operations {
		if op == nil || !op.BalanceAffected {
			continue
		}

		// src is declared INSIDE the loop so each buildFn closure captures its
		// own copy — avoids the classic loop-variable capture bug.
		src := events.BalanceChangedSource{
			OrganizationID: tran.OrganizationID,
			LedgerID:       tran.LedgerID,
			AccountID:      op.AccountID,
			BalanceID:      op.BalanceID,
			Alias:          op.AccountAlias,
			AssetCode:      op.AssetCode,
			BalanceKey:     op.BalanceKey,
			Available:      decimalOrZero(op.BalanceAfter.Available),
			OnHold:         decimalOrZero(op.BalanceAfter.OnHold),
			Version:        int64OrZero(op.BalanceAfter.Version),
			Reason:         reasonForOperation(op.Type),
			OperationType:  op.Type,
			Direction:      op.Direction,
			Amount:         decimalOrZero(op.Amount.Value),
			TransactionID:  tran.ID,
			OperationID:    op.ID,
			OccurredAt:     occurredAt,
		}

		buildFn := func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewBalanceChanged(src).ToEmitRequest(tenantID, src.OccurredAt)
		}

		pkgStreaming.EmitImportant(ctxSend, span, logger, uc.Streaming, events.BalanceChangedDefinition.Key(), buildFn)
	}
}

// decimalOrZero dereferences a *decimal.Decimal, treating nil as zero.
func decimalOrZero(d *decimal.Decimal) decimal.Decimal {
	if d == nil {
		return decimal.Zero
	}

	return *d
}

// int64OrZero dereferences a *int64, treating nil as zero.
func int64OrZero(v *int64) int64 {
	if v == nil {
		return 0
	}

	return *v
}

// reasonForOperation maps a Midaz Operation.Type into the generic,
// domain-agnostic balance.changed Reason discriminator. Unknown types fall back
// to "adjust" so the event stays emittable for any future op type without a
// code change (the consumer treats "adjust" as "generic mutation"). The op-type
// labels are the canonical values exported by pkg/constant.
func reasonForOperation(opType string) string {
	switch strings.ToUpper(strings.TrimSpace(opType)) {
	case constant.CREDIT:
		return events.BalanceChangeReasonCredit
	case constant.DEBIT:
		return events.BalanceChangeReasonDebit
	case constant.BLOCK:
		return events.BalanceChangeReasonBlock
	case constant.UNBLOCK:
		return events.BalanceChangeReasonUnblock
	case constant.ONHOLD:
		return events.BalanceChangeReasonHold
	case constant.RELEASE:
		return events.BalanceChangeReasonRelease
	case constant.OVERDRAFT:
		return events.BalanceChangeReasonOverdraft
	default:
		return events.BalanceChangeReasonAdjust
	}
}
