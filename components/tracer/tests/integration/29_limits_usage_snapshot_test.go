// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

// Server instants of the usage snapshot scenarios, in RFC3339 for MOCK_TIME.
const (
	usageFirstDayNoon  = "2026-10-01T12:00:00Z"
	usageSecondDayNoon = "2026-10-02T12:00:00Z"
)

// TestUsageSnapshot_ReadsCurrentPeriodOnly proves GET /v1/limits/{id}/usage
// reports the period that contains the server's current time: yesterday's
// consumption stays in its own counter and never surfaces as current usage.
func TestUsageSnapshot_ReadsCurrentPeriodOnly(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)
	accountID := testutil.MustDeterministicUUID(98101).String()

	var limitID string

	atServerTime(t, usageFirstDayNoon, func() {
		limitID = testutil.CreateLimitWithAccountScope(t, accountID, "1000.00")
		testutil.ActivateLimit(t, limitID)
		t.Cleanup(func() { testutil.CleanupLimit(t, limitID) })

		result := validatePix(t, accountID, "900.00")
		require.Equal(t, "ALLOW", result.Decision)

		assertUsageSnapshot(t, limitID, "900", 90.0, true)
	})

	atServerTime(t, usageSecondDayNoon, func() {
		result := validatePix(t, accountID, "100.00")
		require.Equal(t, "ALLOW", result.Decision, "a new day opens a new period")

		assertUsageSnapshot(t, limitID, "100", 10.0, false)
	})

	assert.Equal(t, map[string]string{"2026-10-01": "900", "2026-10-02": "100"}, counterUsageByPeriod(t, db, limitID),
		"both periods must still be stored; only the snapshot narrows to the current one")
}

// TestUsageSnapshot_ReportsMostConsumedScope proves the snapshot of a limit
// with several scopes reports its most consumed scope: every scope is enforced
// against maxAmount on its own, so the sum across scopes is not a usage any
// transaction can reach.
func TestUsageSnapshot_ReportsMostConsumedScope(t *testing.T) {
	accountA := testutil.MustDeterministicUUID(98102).String()
	accountB := testutil.MustDeterministicUUID(98103).String()

	atServerTime(t, usageFirstDayNoon, func() {
		limitID := testutil.CreateLimitWithScope(t, "Usage Two Accounts "+testutil.RandomSuffix(), "1000.00",
			[]testutil.ScopeInput{{AccountID: &accountA}, {AccountID: &accountB}})
		testutil.ActivateLimit(t, limitID)
		t.Cleanup(func() { testutil.CleanupLimit(t, limitID) })

		require.Equal(t, "ALLOW", validatePix(t, accountA, "900.00").Decision)
		require.Equal(t, "ALLOW", validatePix(t, accountB, "100.00").Decision)

		assertUsageSnapshot(t, limitID, "900", 90.0, true)
	})
}

// TestUsageSnapshot_CountsOutstandingReservations proves the snapshot reports
// the capacity enforcement guards: an unconfirmed reservation counts as usage
// and is denied against, confirming it leaves the usage unchanged, and
// releasing one returns its capacity.
func TestUsageSnapshot_CountsOutstandingReservations(t *testing.T) {
	accountID := testutil.MustDeterministicUUID(98104)
	transactionID := testutil.MustDeterministicUUID(98105)

	atServerTime(t, usageFirstDayNoon, func() {
		limitID := testutil.CreateLimitWithAccountScope(t, accountID.String(), "1000.00")
		testutil.ActivateLimit(t, limitID)
		t.Cleanup(func() { testutil.CleanupLimit(t, limitID) })

		got := reservePix(t, transactionID, accountID, "900.00")
		require.False(t, got.GetDenied())
		require.Equal(t, "ALLOW", got.GetDecision())
		require.Len(t, got.GetReservationIds(), 1)

		assertUsageSnapshot(t, limitID, "900", 90.0, true)
		assert.Equal(t, "DENY", validatePix(t, accountID.String(), "200.00").Decision,
			"900 held plus 200 exceeds the 1000 cap the snapshot reports against")

		confirmTransaction(t, transactionID)

		assertUsageSnapshot(t, limitID, "900", 90.0, true)

		heldID := testutil.MustDeterministicUUID(98106)
		held := reservePix(t, heldID, accountID, "50.00")
		require.False(t, held.GetDenied())

		assertUsageSnapshot(t, limitID, "950", 95.0, true)

		releaseTransaction(t, heldID)

		assertUsageSnapshot(t, limitID, "900", 90.0, true)
	})
}

// releaseTransaction releases the transaction's reservations, returning their
// held capacity.
func releaseTransaction(t *testing.T, transactionID uuid.UUID) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), resetTimeReserveTimeout)
	defer cancel()

	_, err := testutil.DialReservationClient(t).ReleaseByTransaction(ctx,
		&reservationv1.ReleaseByTransactionRequest{TransactionId: transactionID.String()})
	require.NoError(t, err, "ReleaseByTransaction must succeed over the gRPC seam")
}

// assertUsageSnapshot asserts the usage snapshot of a limit: currentUsage,
// utilizationPercent and nearLimit.
func assertUsageSnapshot(t *testing.T, limitID, usage string, utilization float64, nearLimit bool) {
	t.Helper()

	status, body := limitRequest(t, http.MethodGet, "/v1/limits/"+limitID+"/usage", nil)
	require.Equal(t, http.StatusOK, status, string(body))

	var snapshot struct {
		CurrentUsage       decimal.Decimal `json:"currentUsage"`
		UtilizationPercent float64         `json:"utilizationPercent"`
		NearLimit          bool            `json:"nearLimit"`
	}

	require.NoError(t, json.Unmarshal(body, &snapshot), string(body))

	assert.True(t, decimal.RequireFromString(usage).Equal(snapshot.CurrentUsage),
		"currentUsage: want %s, got %s", usage, snapshot.CurrentUsage)
	assert.InDelta(t, utilization, snapshot.UtilizationPercent, 0.001, "utilizationPercent")
	assert.Equal(t, nearLimit, snapshot.NearLimit, "nearLimit")
}
