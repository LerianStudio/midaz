// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// The two phase-two actions the ledger owes the tracer once the balance move is
// decided: confirm consumes the held capacity, release gives it back.
const (
	reservationActionConfirm = "confirm"
	reservationActionRelease = "release"
)

// reservationTransition is one confirm or release the ledger owes the tracer,
// carrying everything needed to REPORT it as well as to address it.
//
// Addressing needs only an id. Reporting needs the rest: a confirm that never
// reaches the tracer is a committed spend the customer's limit will never
// count, and an operator who has to act on that needs the transaction and the
// amount from the log line itself — the tracer's own store is not available to
// join against when the tracer is the thing that is down.
//
// LimitID is deliberately absent. The ledger never learns it: the reserve
// response returns reservation ids only, and which limits those ids belong to
// is the tracer's own resolution. The reservation id is the handle that leads
// there once the tracer is reachable again.
type reservationTransition struct {
	// Action is confirm or release.
	Action string

	// TransactionID is the transaction whose capacity is being settled. It is
	// always set, and it is the address the by-transaction form uses.
	TransactionID uuid.UUID

	// ReservationID addresses one held reservation. It is uuid.Nil for the
	// by-transaction form, which settles every reservation the transaction
	// holds in one call.
	ReservationID uuid.UUID

	// Amount and Asset are the fee-inclusive value the reservation held. They
	// are what makes a lost confirm quantifiable.
	Amount decimal.Decimal
	Asset  string
}

// byTransaction reports whether this transition is addressed by transaction id
// rather than by a single reservation id.
func (t reservationTransition) byTransaction() bool {
	return t.ReservationID == uuid.Nil
}

// lossConsequence names, in the operator's terms, what it costs when this
// transition is never placed. The two actions fail in OPPOSITE directions and a
// single sentence written for one is false for the other: a lost confirm means
// money moved that the limit will never count, so the cap under-enforces; a lost
// release means capacity held against a transaction that moved no money at all,
// so the cap over-enforces and denies the customer inside their own limit. The
// remediation differs too, which is why this belongs in the message an operator
// is paged on and not only in a structured field.
func (t reservationTransition) lossConsequence() string {
	if t.Action == reservationActionRelease {
		return "the capacity stays held against a transaction that moved no money, so the limit over-enforces until the hold expires"
	}

	return "the spend will not be counted against the limit"
}

// delayConsequence is the same distinction for a transition that did land, late:
// it names what was wrong in the window between the failed first attempt and the
// delivery.
func (t reservationTransition) delayConsequence() string {
	if t.Action == reservationActionRelease {
		return "the limit over-enforced until now"
	}

	return "the limit was under-enforced until now"
}

// logFields renders the transition's identity for a structured log line. The
// reservation id is emitted only when the transition carries one, so the
// by-transaction form does not log a nil uuid that reads like a real handle.
func (t reservationTransition) logFields() []libLog.Field {
	fields := []libLog.Field{
		libLog.String("reservation_action", t.Action),
		libLog.String("transaction_id", t.TransactionID.String()),
		libLog.String("amount", t.Amount.String()),
		libLog.String("asset", t.Asset),
		libLog.Bool("by_transaction", t.byTransaction()),
	}

	if !t.byTransaction() {
		fields = append(fields, libLog.String("reservation_id", t.ReservationID.String()))
	}

	return fields
}

// transitionByTransaction builds the by-transaction form of a transition from a
// handle that carries the identity but no reservation ids — the shape /commit
// and /cancel work with, where the create-time handle did not survive into the
// request.
func (h reservationHandle) transitionByTransaction(action string) reservationTransition {
	return reservationTransition{
		Action:        action,
		TransactionID: h.TransactionID,
		Amount:        h.Amount,
		Asset:         h.Asset,
	}
}
