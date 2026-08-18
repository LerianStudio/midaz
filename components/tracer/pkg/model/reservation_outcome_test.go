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

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestNewReservationWithDeliveryMode(t *testing.T) {
	t.Parallel()

	expiresAt := time.Date(2026, 8, 18, 13, 0, 0, 0, time.UTC)
	createdAt := expiresAt.Add(-time.Minute)

	reservation, err := NewReservationWithDeliveryMode(
		uuid.MustParse("10000000-0000-0000-0000-000000000001"),
		uuid.MustParse("20000000-0000-0000-0000-000000000002"),
		"account:1",
		"2026-08",
		500,
		expiresAt,
		createdAt,
		DeliveryModeLedgerOutcomeV2,
	)
	require.NoError(t, err)
	assert.Equal(t, DeliveryModeLedgerOutcomeV2, reservation.DeliveryMode)
	require.NoError(t, reservation.Validate())

	_, err = NewReservationWithDeliveryMode(
		uuid.MustParse("10000000-0000-0000-0000-000000000001"),
		uuid.MustParse("20000000-0000-0000-0000-000000000002"),
		"account:1",
		"2026-08",
		500,
		expiresAt,
		createdAt,
		ReservationDeliveryMode("ASYNC_V3"),
	)
	require.ErrorIs(t, err, constant.ErrReservationDeliveryModeInvalid)
}

func TestReservationDeliveryModeNormalize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   ReservationDeliveryMode
		want    ReservationDeliveryMode
		wantErr error
	}{
		{name: "unspecified preserves legacy", input: DeliveryModeUnspecified, want: DeliveryModeLegacy},
		{name: "legacy remains legacy", input: DeliveryModeLegacy, want: DeliveryModeLegacy},
		{name: "v2 remains v2", input: DeliveryModeLedgerOutcomeV2, want: DeliveryModeLedgerOutcomeV2},
		{name: "unknown mode is rejected", input: ReservationDeliveryMode("ASYNC_V3"), wantErr: constant.ErrReservationDeliveryModeInvalid},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := tc.input.Normalize()
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestReservationOutcomeTerminalStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		outcome ReservationOutcome
		want    ReservationStatus
		wantErr error
	}{
		{name: "committed confirms", outcome: OutcomeCommitted, want: StatusConfirmed},
		{name: "aborted releases", outcome: OutcomeAborted, want: StatusReleased},
		{name: "unspecified rejected", outcome: OutcomeUnspecified, wantErr: constant.ErrReservationOutcomeInvalid},
		{name: "unknown rejected", outcome: ReservationOutcome("REVERSED"), wantErr: constant.ErrReservationOutcomeInvalid},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := tc.outcome.TerminalStatus()
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
