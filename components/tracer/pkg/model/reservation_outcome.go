// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"time"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// ReservationDeliveryMode selects who owns reservation termination.
type ReservationDeliveryMode string

const (
	// DeliveryModeUnspecified is the additive wire default and preserves the
	// legacy autonomous-expiry protocol.
	DeliveryModeUnspecified ReservationDeliveryMode = "UNSPECIFIED"
	// DeliveryModeLegacy keeps confirm/release and TTL expiry behavior unchanged.
	DeliveryModeLegacy ReservationDeliveryMode = "LEGACY"
	// DeliveryModeLedgerOutcomeV2 makes the ledger's durable terminal outcome the
	// only authority allowed to terminate a reservation.
	DeliveryModeLedgerOutcomeV2 ReservationDeliveryMode = "LEDGER_OUTCOME_V2"
)

// Normalize maps the additive unspecified value to legacy and rejects unknown
// values before persistence.
func (m ReservationDeliveryMode) Normalize() (ReservationDeliveryMode, error) {
	switch m {
	case "", DeliveryModeUnspecified, DeliveryModeLegacy:
		return DeliveryModeLegacy, nil
	case DeliveryModeLedgerOutcomeV2:
		return DeliveryModeLedgerOutcomeV2, nil
	default:
		return "", constant.ErrReservationDeliveryModeInvalid
	}
}

// ReservationOutcome is the ledger's durable terminal decision.
type ReservationOutcome string

const (
	OutcomeUnspecified ReservationOutcome = "UNSPECIFIED"
	OutcomeCommitted   ReservationOutcome = "COMMITTED"
	OutcomeAborted     ReservationOutcome = "ABORTED"
)

// TerminalStatus maps a ledger outcome to the persisted reservation state.
func (o ReservationOutcome) TerminalStatus() (ReservationStatus, error) {
	switch o {
	case OutcomeCommitted:
		return StatusConfirmed, nil
	case OutcomeAborted:
		return StatusReleased, nil
	default:
		return "", constant.ErrReservationOutcomeInvalid
	}
}

// ReservationOutcomeReceipt is the durable idempotency proof for one ledger
// transaction's terminal outcome.
type ReservationOutcomeReceipt struct {
	TransactionID    uuid.UUID
	OutcomeID        uuid.UUID
	Outcome          ReservationOutcome
	ReservationCount int
	AppliedAt        time.Time
}
