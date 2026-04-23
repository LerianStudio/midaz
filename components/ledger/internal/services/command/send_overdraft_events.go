// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/shopspring/decimal"
)

// Overdraft event constants used in the Event envelope and routing key.
const (
	OverdraftEventType = "balance"

	// OverdraftActionDrawn is emitted when overdraft is consumed during a
	// debit that exceeds the credit balance's available funds.
	OverdraftActionDrawn = "overdraft.drawn"

	// OverdraftActionRepaid is emitted when an incoming credit repays
	// (partially or fully) the outstanding overdraft.
	OverdraftActionRepaid = "overdraft.repaid"

	// OverdraftActionCleared is emitted when overdraft usage reaches zero
	// after a repayment — the balance is no longer in an overdrafted state.
	OverdraftActionCleared = "overdraft.cleared"
)

// OverdraftEventPayload carries the business fields for an overdraft
// lifecycle event. It is marshalled as the Payload field inside mmodel.Event.
type OverdraftEventPayload struct {
	// AccountID is the account that holds the overdrafted balance.
	AccountID string `json:"accountId"`

	// TransactionID is the transaction that triggered the overdraft change.
	TransactionID string `json:"transactionId"`

	// Amount is the overdraft movement in this event:
	//   - For drawn: the deficit added to overdraft usage.
	//   - For repaid/cleared: the amount repaid.
	Amount decimal.Decimal `json:"amount"`

	// OverdraftBalance is the companion overdraft balance's Available value
	// AFTER the transaction. Equals the total outstanding overdraft.
	OverdraftBalance decimal.Decimal `json:"overdraftBalance"`

	// OverdraftLimit is the configured overdraft limit for the account.
	// nil means the limit is not available in the current event context.
	// Consumers should treat nil as "unknown" — not as "unlimited".
	// This field will be populated once T-010 (Operation record overdraftUsed
	// extension) lands, making overdraft state visible on every operation.
	OverdraftLimit *decimal.Decimal `json:"overdraftLimit,omitempty"`

	// Timestamp is the time the event was generated.
	Timestamp time.Time `json:"timestamp"`
}

// SendOverdraftEvents inspects a persisted transaction's operations for
// overdraft companion operations (BalanceKey == "overdraft") and emits the
// appropriate lifecycle events via the message broker.
//
// Three event actions are supported:
//   - overdraft.drawn:   a DEBIT on a direction=debit companion balance
//     (overdraft usage increased).
//   - overdraft.repaid:  a CREDIT on a direction=debit companion balance
//     (overdraft usage decreased but not fully cleared).
//   - overdraft.cleared: a CREDIT on a direction=debit companion balance
//     where the balance reaches zero (overdraft fully repaid).
//
// The function is non-blocking and fire-and-forget: emission failures are
// logged but never propagate to the caller. This mirrors the contract of
// SendTransactionEvents.
func (uc *UseCase) SendOverdraftEvents(ctx context.Context, tran *transaction.Transaction) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	if !isOverdraftEventEnabled() {
		logger.Log(ctx, libLog.LevelInfo, "Overdraft events not enabled",
			libLog.String("RABBITMQ_OVERDRAFT_EVENTS_ENABLED", os.Getenv("RABBITMQ_OVERDRAFT_EVENTS_ENABLED")))
		return
	}

	payloads := buildOverdraftEvents(tran)
	if len(payloads) == 0 {
		return
	}

	ctxSend, span := tracer.Start(ctx, "command.send_overdraft_events_async")
	defer span.End()

	exchange := os.Getenv("RABBITMQ_OVERDRAFT_EVENTS_EXCHANGE")

	for _, ep := range payloads {
		raw, err := json.Marshal(ep.payload)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to marshal overdraft event payload", err)
			logger.Log(ctx, libLog.LevelError, "Failed to marshal overdraft event payload",
				libLog.String("action", ep.action))

			continue
		}

		evt := mmodel.Event{
			Source:         Source,
			EventType:      OverdraftEventType,
			Action:         ep.action,
			TimeStamp:      time.Now(),
			Version:        os.Getenv("VERSION"),
			OrganizationID: tran.OrganizationID,
			LedgerID:       tran.LedgerID,
			Payload:        raw,
		}

		message, err := json.Marshal(evt)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to marshal overdraft event envelope", err)
			logger.Log(ctx, libLog.LevelError, "Failed to marshal overdraft event envelope",
				libLog.String("action", ep.action))

			continue
		}

		var key strings.Builder

		key.WriteString(Source)
		key.WriteString(".")
		key.WriteString(OverdraftEventType)
		key.WriteString(".")
		key.WriteString(ep.action)

		logger.Log(ctx, libLog.LevelInfo, "Sending overdraft event",
			libLog.String("key", key.String()),
			libLog.String("action", ep.action))

		if _, err := uc.RabbitMQRepo.ProducerDefault(
			ctxSend,
			exchange,
			key.String(),
			message,
		); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to send overdraft event to exchange", err)
			logger.Log(ctx, libLog.LevelError, "Failed to send overdraft event",
				libLog.String("action", ep.action),
				libLog.Err(err))
		}
	}
}

// overdraftEventItem pairs a classified action with its pre-marshal payload.
// This is an internal type used between buildOverdraftEvents and SendOverdraftEvents
// so that marshal errors are handled where logger and span are available.
type overdraftEventItem struct {
	action  string
	payload OverdraftEventPayload
}

// buildOverdraftEvents scans tran.Operations for overdraft companion
// operations and returns the corresponding event items (pre-marshal structs).
// The caller (SendOverdraftEvents) handles JSON marshalling where it has
// access to logger and span for proper error reporting.
func buildOverdraftEvents(tran *transaction.Transaction) []overdraftEventItem {
	if tran == nil || len(tran.Operations) == 0 {
		return nil
	}

	var items []overdraftEventItem

	for _, op := range tran.Operations {
		if op == nil || op.BalanceKey != constant.OverdraftBalanceKey {
			continue
		}

		action, amount, afterAvail := classifyOverdraftOperation(op)
		if action == "" {
			continue
		}

		items = append(items, overdraftEventItem{
			action: action,
			payload: OverdraftEventPayload{
				AccountID:        op.AccountID,
				TransactionID:    tran.ID,
				Amount:           amount,
				OverdraftBalance: afterAvail,
				Timestamp:        time.Now(),
			},
		})
	}

	return items
}

// classifyOverdraftOperation determines the event action for a companion
// overdraft operation and returns the absolute movement amount together with
// the balance-after-available value.
//
// For a direction=debit companion balance:
//   - DEBIT increases Available (overdraft drawn)
//   - CREDIT decreases Available (overdraft repaid or cleared)
//
// The "cleared" action is emitted instead of "repaid" when the companion
// balance reaches zero after the credit.
//
// The third return value (afterAvail) is the companion balance's Available
// after the operation, eliminating redundant calls to overdraftBalanceAfter.
func classifyOverdraftOperation(op *operation.Operation) (action string, amount decimal.Decimal, afterAvail decimal.Decimal) {
	if op.Amount.Value == nil {
		return "", decimal.Zero, decimal.Zero
	}

	amt := *op.Amount.Value
	if amt.IsZero() {
		return "", decimal.Zero, decimal.Zero
	}

	after := overdraftBalanceAfter(op)

	switch strings.ToUpper(op.Type) {
	case "DEBIT":
		return OverdraftActionDrawn, amt, after

	case "CREDIT":
		if after.IsZero() {
			return OverdraftActionCleared, amt, after
		}

		return OverdraftActionRepaid, amt, after

	default:
		return "", decimal.Zero, decimal.Zero
	}
}

// overdraftBalanceAfter returns the companion overdraft balance's Available
// value after the operation. Falls back to zero if the balance snapshot is
// absent.
func overdraftBalanceAfter(op *operation.Operation) decimal.Decimal {
	if op.BalanceAfter.Available != nil {
		return *op.BalanceAfter.Available
	}

	return decimal.Zero
}

// isOverdraftEventEnabled returns true unless the environment variable
// RABBITMQ_OVERDRAFT_EVENTS_ENABLED is explicitly set to "false".
// Unset or any other value means enabled — consistent with the transaction
// event flag pattern.
func isOverdraftEventEnabled() bool {
	envValue := strings.ToLower(strings.TrimSpace(os.Getenv("RABBITMQ_OVERDRAFT_EVENTS_ENABLED")))
	return envValue != "false"
}
