// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"time"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
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
	LongLived               bool `json:"longLived,omitempty"`
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
	Denied         bool        `json:"denied"`
	ReservationIDs []uuid.UUID `json:"reservationIds"`
}

// ReservationActionResponse is the body returned by confirm and release. Status is
// the terminal state the reservation resolves to (CONFIRMED or RELEASED). Confirm
// and release are idempotent: a retry against an already-terminal reservation
// returns the same terminal status with HTTP 200.
type ReservationActionResponse struct {
	ReservationID uuid.UUID `json:"reservationId" swaggertype:"string" format:"uuid"`
	Status        string    `json:"status"`
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
	Status        string    `json:"status"`
	Flipped       int       `json:"flipped"`
}
