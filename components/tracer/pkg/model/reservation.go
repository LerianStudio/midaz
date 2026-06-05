// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
)

// ReservationStatus represents the lifecycle status of a usage reservation.
// These are the four PERSISTED states (usage_reservations.status). The audit-only
// SKIPPED disposition is modeled as an AuditEventType, not a ReservationStatus,
// because no reservation row is written when the tracer is skipped fail-open.
type ReservationStatus string

const (
	// StatusReserved is the initial state: the amount is held in
	// usage_counters.reserved_usage but not yet committed.
	StatusReserved ReservationStatus = "RESERVED"
	// StatusConfirmed marks a reservation whose amount has moved from
	// reserved_usage into current_usage (the ledger transaction committed).
	StatusConfirmed ReservationStatus = "CONFIRMED"
	// StatusReleased marks a reservation whose amount was returned to capacity
	// by an explicit release (the ledger transaction aborted).
	StatusReleased ReservationStatus = "RELEASED"
	// StatusExpired marks a reservation released by the TTL reaper because no
	// confirm/release arrived before reservation_expires_at.
	StatusExpired ReservationStatus = "EXPIRED"
)

// IsValid reports whether the status is one of the four persisted enum values.
func (s ReservationStatus) IsValid() bool {
	switch s {
	case StatusReserved, StatusConfirmed, StatusReleased, StatusExpired:
		return true
	default:
		return false
	}
}

// IsTerminal reports whether the status is a final state. A confirm or release
// against a terminal reservation is a no-op (idempotent retry).
func (s ReservationStatus) IsTerminal() bool {
	switch s {
	case StatusConfirmed, StatusReleased, StatusExpired:
		return true
	default:
		return false
	}
}

// Reservation is a single two-phase reservation row, mirroring the
// usage_reservations table. Amount is stored in the smallest currency unit
// (int64 / BIGINT) to match usage_counters.current_usage and reserved_usage —
// not decimal, because the counter columns the reservation moves against are
// BIGINT.
//
// TransactionID is the ledger transaction correlation id. It is NOT a foreign
// key: the ledger transaction lives in a different service, so the reference is
// by value only. The (TransactionID, LimitID, ScopeKey, PeriodKey) tuple is the
// idempotency grain for retried reserves.
type Reservation struct {
	ID                   uuid.UUID         `json:"reservationId" swaggertype:"string" format:"uuid"`
	LimitID              uuid.UUID         `json:"limitId" swaggertype:"string" format:"uuid"`
	ScopeKey             string            `json:"scopeKey"`
	PeriodKey            string            `json:"periodKey"`
	Amount               int64             `json:"amount" example:"50000" minimum:"0"`
	Status               ReservationStatus `json:"status"`
	TransactionID        uuid.UUID         `json:"transactionId" swaggertype:"string" format:"uuid"`
	ReservationExpiresAt time.Time         `json:"reservationExpiresAt" format:"date-time"`
	CreatedAt            time.Time         `json:"createdAt" format:"date-time"`
	ConfirmedAt          *time.Time        `json:"confirmedAt,omitempty" format:"date-time"`
	ReleasedAt           *time.Time        `json:"releasedAt,omitempty" format:"date-time"`
}

// NewReservation creates a RESERVED reservation after validating its invariants:
// non-nil limit and transaction IDs, non-empty scope and period keys, a
// non-negative amount, and a non-zero expiry. Scope and period keys are trimmed.
// Returns (*Reservation, error) per the tracer constructor convention.
func NewReservation(
	limitID uuid.UUID,
	transactionID uuid.UUID,
	scopeKey string,
	periodKey string,
	amount int64,
	reservationExpiresAt time.Time,
	createdAt time.Time,
) (*Reservation, error) {
	if limitID == uuid.Nil {
		return nil, constant.ErrReservationLimitIDRequired
	}

	if transactionID == uuid.Nil {
		return nil, constant.ErrReservationTransactionIDReq
	}

	normalizedScopeKey := strings.TrimSpace(scopeKey)
	if normalizedScopeKey == "" {
		return nil, constant.ErrReservationScopeKeyRequired
	}

	normalizedPeriodKey := strings.TrimSpace(periodKey)
	if normalizedPeriodKey == "" {
		return nil, constant.ErrReservationPeriodKeyRequired
	}

	if amount < 0 {
		return nil, constant.ErrReservationAmountInvalid
	}

	if reservationExpiresAt.IsZero() {
		return nil, constant.ErrReservationExpiresAtRequired
	}

	return &Reservation{
		ID:                   uuid.New(),
		LimitID:              limitID,
		ScopeKey:             normalizedScopeKey,
		PeriodKey:            normalizedPeriodKey,
		Amount:               amount,
		Status:               StatusReserved,
		TransactionID:        transactionID,
		ReservationExpiresAt: reservationExpiresAt.UTC(),
		CreatedAt:            createdAt.UTC(),
	}, nil
}

// Validate checks the reservation invariants at the persistence boundary.
// It mirrors the constructor invariants plus a status enum check so a
// reservation loaded from or about to be written to the database is well-formed.
func (r *Reservation) Validate() error {
	if r.LimitID == uuid.Nil {
		return constant.ErrReservationLimitIDRequired
	}

	if r.TransactionID == uuid.Nil {
		return constant.ErrReservationTransactionIDReq
	}

	if strings.TrimSpace(r.ScopeKey) == "" {
		return constant.ErrReservationScopeKeyRequired
	}

	if strings.TrimSpace(r.PeriodKey) == "" {
		return constant.ErrReservationPeriodKeyRequired
	}

	if r.Amount < 0 {
		return constant.ErrReservationAmountInvalid
	}

	if !r.Status.IsValid() {
		return constant.ErrReservationInvalidStatus
	}

	if r.ReservationExpiresAt.IsZero() {
		return constant.ErrReservationExpiresAtRequired
	}

	return nil
}
