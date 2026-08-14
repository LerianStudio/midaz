// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestReservationStatusIsValid(t *testing.T) {
	tests := []struct {
		name   string
		status ReservationStatus
		want   bool
	}{
		{name: "RESERVED is valid", status: StatusReserved, want: true},
		{name: "CONFIRMED is valid", status: StatusConfirmed, want: true},
		{name: "RELEASED is valid", status: StatusReleased, want: true},
		{name: "EXPIRED is valid", status: StatusExpired, want: true},
		{name: "SKIPPED is not a persisted status", status: ReservationStatus("SKIPPED"), want: false},
		{name: "empty is invalid", status: ReservationStatus(""), want: false},
		{name: "unknown is invalid", status: ReservationStatus("PENDING"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.status.IsValid())
		})
	}
}

func TestReservationStatusIsTerminal(t *testing.T) {
	tests := []struct {
		name   string
		status ReservationStatus
		want   bool
	}{
		{name: "RESERVED is not terminal", status: StatusReserved, want: false},
		{name: "CONFIRMED is terminal", status: StatusConfirmed, want: true},
		{name: "RELEASED is terminal", status: StatusReleased, want: true},
		{name: "EXPIRED is terminal", status: StatusExpired, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.status.IsTerminal())
		})
	}
}

func TestNewReservation(t *testing.T) {
	limitID := testutil.MustDeterministicUUID(1)
	txID := testutil.MustDeterministicUUID(2)
	createdAt := testutil.FixedTime()
	expiresAt := createdAt.Add(30 * time.Second)

	tests := []struct {
		name          string
		limitID       uuid.UUID
		transactionID uuid.UUID
		scopeKey      string
		periodKey     string
		amount        int64
		expiresAt     time.Time
		wantErr       error
	}{
		{
			name:          "valid reservation",
			limitID:       limitID,
			transactionID: txID,
			scopeKey:      "acct:abc-123",
			periodKey:     "2026-06-05",
			amount:        50000,
			expiresAt:     expiresAt,
			wantErr:       nil,
		},
		{
			name:          "zero amount is allowed",
			limitID:       limitID,
			transactionID: txID,
			scopeKey:      "acct:abc-123",
			periodKey:     "2026-06-05",
			amount:        0,
			expiresAt:     expiresAt,
			wantErr:       nil,
		},
		{
			name:          "nil limit ID rejected",
			limitID:       uuid.Nil,
			transactionID: txID,
			scopeKey:      "acct:abc-123",
			periodKey:     "2026-06-05",
			amount:        50000,
			expiresAt:     expiresAt,
			wantErr:       constant.ErrReservationLimitIDRequired,
		},
		{
			name:          "nil transaction ID rejected",
			limitID:       limitID,
			transactionID: uuid.Nil,
			scopeKey:      "acct:abc-123",
			periodKey:     "2026-06-05",
			amount:        50000,
			expiresAt:     expiresAt,
			wantErr:       constant.ErrReservationTransactionIDReq,
		},
		{
			name:          "empty scope key rejected",
			limitID:       limitID,
			transactionID: txID,
			scopeKey:      "   ",
			periodKey:     "2026-06-05",
			amount:        50000,
			expiresAt:     expiresAt,
			wantErr:       constant.ErrReservationScopeKeyRequired,
		},
		{
			name:          "empty period key rejected",
			limitID:       limitID,
			transactionID: txID,
			scopeKey:      "acct:abc-123",
			periodKey:     "",
			amount:        50000,
			expiresAt:     expiresAt,
			wantErr:       constant.ErrReservationPeriodKeyRequired,
		},
		{
			name:          "negative amount rejected",
			limitID:       limitID,
			transactionID: txID,
			scopeKey:      "acct:abc-123",
			periodKey:     "2026-06-05",
			amount:        -1,
			expiresAt:     expiresAt,
			wantErr:       constant.ErrReservationAmountInvalid,
		},
		{
			name:          "zero expiry rejected",
			limitID:       limitID,
			transactionID: txID,
			scopeKey:      "acct:abc-123",
			periodKey:     "2026-06-05",
			amount:        50000,
			expiresAt:     time.Time{},
			wantErr:       constant.ErrReservationExpiresAtRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewReservation(tt.limitID, tt.transactionID, tt.scopeKey, tt.periodKey, tt.amount, tt.expiresAt, createdAt)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, r)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, r)

			assert.NotEqual(t, uuid.Nil, r.ID)
			assert.Equal(t, tt.limitID, r.LimitID)
			assert.Equal(t, tt.transactionID, r.TransactionID)
			assert.Equal(t, tt.amount, r.Amount)
			assert.Equal(t, StatusReserved, r.Status)
			assert.Equal(t, expiresAt.UTC(), r.ReservationExpiresAt)
			assert.Equal(t, createdAt.UTC(), r.CreatedAt)
			assert.Nil(t, r.ConfirmedAt)
			assert.Nil(t, r.ReleasedAt)
		})
	}
}

func TestNewReservationTrimsScopeAndPeriod(t *testing.T) {
	r, err := NewReservation(
		testutil.MustDeterministicUUID(1),
		testutil.MustDeterministicUUID(2),
		"  acct:abc-123  ",
		"  2026-06-05  ",
		1000,
		testutil.FixedTime().Add(30*time.Second),
		testutil.FixedTime(),
	)
	require.NoError(t, err)
	assert.Equal(t, "acct:abc-123", r.ScopeKey)
	assert.Equal(t, "2026-06-05", r.PeriodKey)
}

func TestReservationValidate(t *testing.T) {
	limitID := testutil.MustDeterministicUUID(1)
	txID := testutil.MustDeterministicUUID(2)
	createdAt := testutil.FixedTime()
	expiresAt := createdAt.Add(30 * time.Second)

	valid := func() *Reservation {
		return &Reservation{
			ID:                   testutil.MustDeterministicUUID(3),
			LimitID:              limitID,
			ScopeKey:             "acct:abc-123",
			PeriodKey:            "2026-06-05",
			Amount:               50000,
			Status:               StatusReserved,
			TransactionID:        txID,
			ReservationExpiresAt: expiresAt,
			CreatedAt:            createdAt,
		}
	}

	tests := []struct {
		name    string
		mutate  func(*Reservation)
		wantErr error
	}{
		{name: "valid reservation passes", mutate: func(*Reservation) {}, wantErr: nil},
		{name: "confirmed status passes", mutate: func(r *Reservation) { r.Status = StatusConfirmed }, wantErr: nil},
		{name: "nil limit ID", mutate: func(r *Reservation) { r.LimitID = uuid.Nil }, wantErr: constant.ErrReservationLimitIDRequired},
		{name: "nil transaction ID", mutate: func(r *Reservation) { r.TransactionID = uuid.Nil }, wantErr: constant.ErrReservationTransactionIDReq},
		{name: "empty scope", mutate: func(r *Reservation) { r.ScopeKey = " " }, wantErr: constant.ErrReservationScopeKeyRequired},
		{name: "empty period", mutate: func(r *Reservation) { r.PeriodKey = "" }, wantErr: constant.ErrReservationPeriodKeyRequired},
		{name: "negative amount", mutate: func(r *Reservation) { r.Amount = -5 }, wantErr: constant.ErrReservationAmountInvalid},
		{name: "invalid status", mutate: func(r *Reservation) { r.Status = ReservationStatus("SKIPPED") }, wantErr: constant.ErrReservationInvalidStatus},
		{name: "zero expiry", mutate: func(r *Reservation) { r.ReservationExpiresAt = time.Time{} }, wantErr: constant.ErrReservationExpiresAtRequired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := valid()
			tt.mutate(r)
			err := r.Validate()

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
		})
	}
}
