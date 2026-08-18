// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"time"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// ReserveRequest is the body of POST /v1/reservations. It mirrors the validation
// request shape (amount, currency, account/segment/portfolio/merchant context,
// transaction type and timestamp) and adds the ledger transactionId — the
// correlation handle the two-phase reservation lifecycle is keyed on. The embedded
// ValidationRequest carries the scope fields and reuses its NormalizeAndValidate
// and ToCheckLimitsInput logic so the reserve path never drifts from the
// synchronous validate path's input contract.
type ReserveRequest struct {
	// TransactionID is the ledger transaction correlation id. It is the
	// idempotency grain for retried reserves and the handle the ledger later
	// confirms or releases. Not a foreign key — the ledger transaction lives in a
	// different service.
	TransactionID uuid.UUID `json:"transactionId" validate:"required" swaggertype:"string" format:"uuid"`
	// LongLived selects the reservation lifetime. false (the default, a direct
	// transaction) gets the short reaper-swept TTL; true (a PENDING transaction)
	// gets the long-lived TTL so the reservation does not expire under a
	// still-valid pending that has no existing sweep (R18). It is a sibling wire
	// field, NOT part of the embedded ValidationRequest, so the relaxed reserve
	// validation never sees it.
	LongLived               bool                          `json:"longLived,omitempty" example:"false"`
	DeliveryMode            model.ReservationDeliveryMode `json:"deliveryMode,omitempty" enum:"UNSPECIFIED,LEGACY,LEDGER_OUTCOME_V2" example:"LEGACY"`
	model.ValidationRequest `swaggerignore:"true"`
}

// NormalizeAndReserveValidate validates the reserve body: the transactionId must be
// present, then the embedded validation-request fields are normalized and validated
// with the relaxed reserve rules (NormalizeAndValidateForReserve) — requestId,
// amount, currency and timestamp stay mandatory, but transactionType and account
// are optional because the ledger (a double-entry ledger with external-source
// transactions) cannot always supply a card-rail type or an internal account UUID
// at the reserve anchor. now drives the timestamp-window check (injected clock for
// MOCK_TIME determinism in tests).
func (r *ReserveRequest) NormalizeAndReserveValidate(now time.Time) error {
	if r.TransactionID == uuid.Nil {
		return constant.ErrReservationTransactionIDReq
	}

	deliveryMode, err := r.DeliveryMode.Normalize()
	if err != nil {
		return err
	}

	r.DeliveryMode = deliveryMode

	return r.NormalizeAndValidateForReserve(now)
}

// ToReserveInput builds the CheckLimitsInput the reservation service resolves
// against. It delegates to the embedded ValidationRequest so the scope-key inputs
// are identical to the synchronous validate path.
func (r *ReserveRequest) ToReserveInput() *model.CheckLimitsInput {
	return r.ToCheckLimitsInput()
}

// ReserveResponse is the handle returned on a successful reserve. Denied is the
// limit-exceeded decision (no capacity held, ReservationIDs empty); otherwise
// ReservationIDs holds one id per counter-backed limit the ledger must confirm or
// release in phase two.
type ReserveResponse struct {
	TransactionID  uuid.UUID   `json:"transactionId" swaggertype:"string" format:"uuid"`
	Denied         bool        `json:"denied" example:"false"`
	ReservationIDs []uuid.UUID `json:"reservationIds" swaggertype:"array,string" format:"uuid"`
}

// ReservationActionResponse is the body returned by confirm and release. Status is
// the terminal state the reservation resolves to (CONFIRMED or RELEASED). Confirm
// and release are idempotent: a retry against an already-terminal reservation
// returns the same terminal status with HTTP 200.
type ReservationActionResponse struct {
	ReservationID uuid.UUID `json:"reservationId" swaggertype:"string" format:"uuid"`
	Status        string    `json:"status" enum:"CONFIRMED,RELEASED" example:"CONFIRMED"`
}

// TransactionActionResponse is the body returned by the by-transaction confirm and
// release endpoints. The ledger /commit and /cancel address reservations by the
// transaction id alone (the per-reservation handle does not survive the separate
// state-transition request), so the tracer flips every RESERVED reservation the
// transaction holds and reports how many were transitioned. Flipped=0 is a valid,
// idempotent no-op success: the transaction never reserved or every reservation was
// already terminal.
type TransactionActionResponse struct {
	TransactionID uuid.UUID `json:"transactionId" swaggertype:"string" format:"uuid"`
	Status        string    `json:"status" enum:"CONFIRMED,RELEASED" example:"CONFIRMED"`
	Flipped       int       `json:"flipped" example:"2"`
}

// ApplyOutcomeRequest is the durable Ledger-owned terminal decision for a V2
// reservation transaction. OutcomeID identifies the delivery attempt across
// retries; Outcome is the immutable accounting decision.
type ApplyOutcomeRequest struct {
	OutcomeID string                   `json:"outcomeId" format:"uuid"`
	Outcome   model.ReservationOutcome `json:"outcome" enum:"COMMITTED,ABORTED" example:"COMMITTED"`
}

func (r *ApplyOutcomeRequest) Validate() (uuid.UUID, error) {
	outcomeID, err := uuid.Parse(r.OutcomeID)
	if err != nil || outcomeID == uuid.Nil {
		return uuid.Nil, constant.ErrReservationOutcomeIDRequired
	}

	_, err = r.Outcome.TerminalStatus()

	return outcomeID, err
}

// ApplyOutcomeResponse returns the stored receipt. Replayed distinguishes the
// first application from an exact delivery retry without changing semantics.
type ApplyOutcomeResponse struct {
	TransactionID    uuid.UUID                `json:"transactionId" swaggertype:"string" format:"uuid"`
	OutcomeID        uuid.UUID                `json:"outcomeId" swaggertype:"string" format:"uuid"`
	Outcome          model.ReservationOutcome `json:"outcome" enum:"COMMITTED,ABORTED"`
	ReservationCount int                      `json:"reservationCount" example:"2"`
	Replayed         bool                     `json:"replayed" example:"false"`
}
