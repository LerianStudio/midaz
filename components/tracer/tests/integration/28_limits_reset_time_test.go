// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	testutil_integration "github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil_integration"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

// A nighttime Pix limit: the window 23:00-09:00 UTC (20h-6h BRT) crosses
// midnight UTC, so only a 09:00 reset keeps one night in one counter.
const (
	nightWindowStart = "23:00"
	nightWindowEnd   = "09:00"
	nightResetTime   = "09:00"
	nightCap         = "1000.00"
)

// Server instants of the scenario, in RFC3339 for MOCK_TIME.
const (
	firstNightEvening    = "2026-10-01T23:30:00Z"
	firstNightAfterMidUT = "2026-10-02T00:30:00Z"
	firstNightLastMinute = "2026-10-02T08:59:00Z"
	secondNightEvening   = "2026-10-02T23:30:00Z"
)

// resetTimeReserveTimeout bounds one reservation RPC so a stalled seam fails
// the test instead of hanging it.
const resetTimeReserveTimeout = 10 * time.Second

// TestResetTime_ValidationCountsOvernightWindowInOnePeriod proves, on the
// validation path, that a DAILY limit reset at 09:00 counts the whole night in
// one period: the second Pix after midnight UTC is denied, the night's usage
// still counts one minute before the reset, and the next night opens a new
// period.
func TestResetTime_ValidationCountsOvernightWindowInOnePeriod(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)
	accountID := testutil.MustDeterministicUUID(98001).String()

	var limitID string

	atServerTime(t, firstNightEvening, func() {
		limitID = createLimitWithTimeWindowAndResetTime(t, accountID, nightWindowStart, nightWindowEnd, nightResetTime, nightCap)
		testutil.ActivateLimit(t, limitID)
		t.Cleanup(func() { testutil.CleanupLimit(t, limitID) })

		result := validatePix(t, accountID, "1000.00")
		assert.Equal(t, "ALLOW", result.Decision)
		assertLimitUsage(t, result, limitID, "1000.00", false)
	})

	atServerTime(t, firstNightAfterMidUT, func() {
		result := validatePix(t, accountID, "1000.00")
		assert.Equal(t, "DENY", result.Decision, "the second Pix of the night must hit the same counter")
		assert.Equal(t, "limit_exceeded", result.Reason)
		assertLimitUsage(t, result, limitID, "2000.00", true)
	})

	assert.Equal(t, map[string]string{"2026-10-01": "1000"}, counterUsageByPeriod(t, db, limitID),
		"the night of 10-01 must live in the 2026-10-01 period")

	atServerTime(t, firstNightLastMinute, func() {
		result := validatePix(t, accountID, "1.00")
		assert.Equal(t, "DENY", result.Decision, "the night's usage must still count before the 09:00 reset")
		assertLimitUsage(t, result, limitID, "1001.00", true)
	})

	atServerTime(t, secondNightEvening, func() {
		result := validatePix(t, accountID, "1000.00")
		assert.Equal(t, "ALLOW", result.Decision, "the next night must open a new period")
		assertLimitUsage(t, result, limitID, "1000.00", false)
	})

	assert.Equal(t, map[string]string{"2026-10-01": "1000", "2026-10-02": "1000"}, counterUsageByPeriod(t, db, limitID))
}

// TestResetTime_ReserveCountsOvernightWindowInOnePeriod proves the same
// boundary on the ledger's reservation seam: after the first night's
// reservation is confirmed, a reserve after midnight UTC is refused for the
// limit.
func TestResetTime_ReserveCountsOvernightWindowInOnePeriod(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)
	accountID := testutil.MustDeterministicUUID(98002)

	var limitID string

	atServerTime(t, firstNightEvening, func() {
		limitID = createLimitWithTimeWindowAndResetTime(t, accountID.String(), nightWindowStart, nightWindowEnd, nightResetTime, nightCap)
		testutil.ActivateLimit(t, limitID)
		t.Cleanup(func() { testutil.CleanupLimit(t, limitID) })

		transactionID := testutil.MustDeterministicUUID(98003)
		got := reservePix(t, transactionID, accountID, "1000.00")
		assert.False(t, got.GetDenied())
		assert.Equal(t, "ALLOW", got.GetDecision())
		require.Len(t, got.GetReservationIds(), 1)

		confirmTransaction(t, transactionID)
	})

	atServerTime(t, firstNightAfterMidUT, func() {
		got := reservePix(t, testutil.MustDeterministicUUID(98004), accountID, "1000.00")
		assert.True(t, got.GetDenied(), "the second Pix of the night must hit the same counter")
		assert.Equal(t, "DENY", got.GetDecision())
		assert.Equal(t, "limit_exceeded", got.GetReason())
		assert.Empty(t, got.GetReservationIds())
	})

	assert.Equal(t, map[string]string{"2026-10-01": "1000"}, counterUsageByPeriod(t, db, limitID),
		"the confirmed night spend must live in the 2026-10-01 period")
}

// TestResetTime_AbsentKeepsMidnightPeriods proves a limit created without a
// reset time keeps today's behavior: the same night splits at 00:00 UTC, so
// the second Pix is allowed and lands in the next day's period.
func TestResetTime_AbsentKeepsMidnightPeriods(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)
	accountID := testutil.MustDeterministicUUID(98005).String()

	var limitID string

	atServerTime(t, firstNightEvening, func() {
		limitID = createLimitWithTimeWindow(t, accountID, nightWindowStart, nightWindowEnd, nightCap)
		testutil.ActivateLimit(t, limitID)
		t.Cleanup(func() { testutil.CleanupLimit(t, limitID) })

		result := validatePix(t, accountID, "1000.00")
		assert.Equal(t, "ALLOW", result.Decision)
		assertLimitUsage(t, result, limitID, "1000.00", false)
	})

	atServerTime(t, firstNightAfterMidUT, func() {
		result := validatePix(t, accountID, "1000.00")
		assert.Equal(t, "ALLOW", result.Decision, "without a reset time the counter splits at midnight UTC")
		assertLimitUsage(t, result, limitID, "1000.00", false)
	})

	assert.Equal(t, map[string]string{"2026-10-01": "1000", "2026-10-02": "1000"}, counterUsageByPeriod(t, db, limitID),
		"absent reset time must keep the midnight-UTC period keys")
}

// TestResetTime_ResetAtReportedFromCurrentTime proves resetAt in the limit and
// usage responses is the next boundary after the server's current time: the
// configured reset time for a limit that has one, and the next midnight UTC
// for a limit created days earlier without one.
func TestResetTime_ResetAtReportedFromCurrentTime(t *testing.T) {
	var resetLimitID, staleLimitID string

	atServerTime(t, "2026-09-28T12:00:00Z", func() {
		staleLimitID = testutil.CreateLimitWithAccountScope(t, testutil.MustDeterministicUUID(98006).String(), nightCap)
		t.Cleanup(func() { testutil.CleanupLimit(t, staleLimitID) })
	})

	atServerTime(t, firstNightEvening, func() {
		resetLimitID = createLimitWithTimeWindowAndResetTime(t, testutil.MustDeterministicUUID(98007).String(), nightWindowStart, nightWindowEnd, nightResetTime, nightCap)
		t.Cleanup(func() { testutil.CleanupLimit(t, resetLimitID) })
	})

	atServerTime(t, firstNightAfterMidUT, func() {
		assertResetAt(t, resetLimitID, "2026-10-02T09:00:00Z")
		assertResetAt(t, staleLimitID, "2026-10-03T00:00:00Z")
	})

	atServerTime(t, secondNightEvening, func() {
		assertResetAt(t, resetLimitID, "2026-10-03T09:00:00Z")
	})
}

// TestResetTime_LimitContract proves the resetTime field through the running
// service and PostgreSQL: it round-trips on create and read, is stored in
// limits.reset_time, is refused with the documented codes when invalid, and is
// immutable on update, while a window update is checked against the stored
// reset time.
func TestResetTime_LimitContract(t *testing.T) {
	db := testutil.SetupIntegrationDB(t)

	t.Run("round-trips through create, read and storage", func(t *testing.T) {
		limitID := createLimitWithTimeWindowAndResetTime(t, testutil.MustDeterministicUUID(98008).String(), nightWindowStart, nightWindowEnd, "9:00", nightCap)
		t.Cleanup(func() { testutil.CleanupLimit(t, limitID) })

		status, body := limitRequest(t, http.MethodGet, "/v1/limits/"+limitID, nil)
		require.Equal(t, http.StatusOK, status, string(body))
		assert.Equal(t, "09:00", decodeJSONObject(t, body)["resetTime"], "the stored reset time is normalized to HH:MM")
		assert.Equal(t, sql.NullString{String: "09:00", Valid: true}, storedResetTime(t, db, limitID))
	})

	t.Run("absent reset time is stored as NULL and omitted", func(t *testing.T) {
		limitID := createLimitWithTimeWindow(t, testutil.MustDeterministicUUID(98009).String(), nightWindowStart, nightWindowEnd, nightCap)
		t.Cleanup(func() { testutil.CleanupLimit(t, limitID) })

		status, body := limitRequest(t, http.MethodGet, "/v1/limits/"+limitID, nil)
		require.Equal(t, http.StatusOK, status, string(body))
		assert.NotContains(t, decodeJSONObject(t, body), "resetTime")
		assert.False(t, storedResetTime(t, db, limitID).Valid)
	})

	t.Run("reset time equal to the window start is accepted", func(t *testing.T) {
		limitID := createLimitWithTimeWindowAndResetTime(t, testutil.MustDeterministicUUID(98010).String(), nightWindowStart, nightWindowEnd, nightWindowStart, nightCap)
		t.Cleanup(func() { testutil.CleanupLimit(t, limitID) })
	})

	for _, tc := range []struct {
		name     string
		override map[string]any
		code     string
	}{
		{"hour 24 is a malformed time", map[string]any{"resetTime": "24:00"}, "0440"},
		{"9h is a malformed time", map[string]any{"resetTime": "9h"}, "0440"},
		{"per-transaction limits have no period", map[string]any{
			"limitType": "PER_TRANSACTION", "activeTimeStart": nil, "activeTimeEnd": nil,
		}, "0537"},
		{"custom limits have no recurring boundary", map[string]any{
			"limitType": "CUSTOM", "activeTimeStart": nil, "activeTimeEnd": nil,
			"customStartDate": "2026-10-01T00:00:00Z", "customEndDate": "2026-10-31T23:59:59Z",
		}, "0537"},
		{"midnight inside the overnight window", map[string]any{"resetTime": "00:00"}, "0538"},
	} {
		t.Run("create refuses "+tc.name, func(t *testing.T) {
			body := nightLimitBody(testutil.MustDeterministicUUID(98011).String())
			for key, value := range tc.override {
				if value == nil {
					delete(body, key)
				} else {
					body[key] = value
				}
			}

			status, respBody := limitRequest(t, http.MethodPost, "/v1/limits", body)
			assert.Equal(t, http.StatusBadRequest, status, string(respBody))
			assert.Equal(t, tc.code, testutil.ParseErrorResponse(t, respBody).Code)
		})
	}

	t.Run("update refuses any reset time like the other immutable fields", func(t *testing.T) {
		limitID := createLimitWithTimeWindowAndResetTime(t, testutil.MustDeterministicUUID(98012).String(), nightWindowStart, nightWindowEnd, nightResetTime, nightCap)
		t.Cleanup(func() { testutil.CleanupLimit(t, limitID) })

		status, respBody := limitRequest(t, http.MethodPatch, "/v1/limits/"+limitID, map[string]any{"resetTime": "10:00"})
		assert.Equal(t, http.StatusUnprocessableEntity, status, string(respBody))
		assert.Equal(t, "0380", testutil.ParseErrorResponse(t, respBody).Code)
		assert.Equal(t, sql.NullString{String: nightResetTime, Valid: true}, storedResetTime(t, db, limitID))
	})

	t.Run("update refuses a window that would contain the stored reset time", func(t *testing.T) {
		limitID := createLimitWithTimeWindowAndResetTime(t, testutil.MustDeterministicUUID(98013).String(), nightWindowStart, nightWindowEnd, nightResetTime, nightCap)
		t.Cleanup(func() { testutil.CleanupLimit(t, limitID) })

		status, respBody := limitRequest(t, http.MethodPatch, "/v1/limits/"+limitID,
			map[string]any{"activeTimeStart": "08:00", "activeTimeEnd": "10:00"})
		assert.Equal(t, http.StatusBadRequest, status, string(respBody))
		assert.Equal(t, "0538", testutil.ParseErrorResponse(t, respBody).Code)

		status, body := limitRequest(t, http.MethodGet, "/v1/limits/"+limitID, nil)
		require.Equal(t, http.StatusOK, status, string(body))
		limit := decodeJSONObject(t, body)
		assert.Equal(t, nightWindowStart, limit["activeTimeStart"], "a refused window update must not be applied")
		assert.Equal(t, nightWindowEnd, limit["activeTimeEnd"])
	})
}

// atServerTime restarts the server with its clock frozen at instant, runs step,
// and restores the real clock. Counters and limits persist across restarts in
// the suite database.
func atServerTime(t *testing.T, instant string, step func()) {
	t.Helper()

	cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{"MOCK_TIME": instant})
	require.NoError(t, err, "restart server at %s", instant)

	defer func() {
		if cleanupErr := cleanup(); cleanupErr != nil {
			t.Errorf("restore server clock after %s: %v", instant, cleanupErr)
		}
	}()

	step()
}

// validatePix validates a Pix for accountID at the server's current time.
func validatePix(t *testing.T, accountID, amount string) testutil.ValidationResponse {
	t.Helper()

	resp, body := testutil.CreateValidation(t, &testutil.ValidationRequest{
		RequestID:            uuid.New().String(),
		TransactionType:      "PIX",
		Amount:               decimal.RequireFromString(amount),
		Asset:                "BRL",
		TransactionTimestamp: testutil.TestNow().Add(-30 * time.Second).Format(time.RFC3339),
		Account:              &testutil.AccountContext{ID: accountID},
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))

	var result testutil.ValidationResponse
	require.NoError(t, json.Unmarshal(body, &result))

	return result
}

// assertLimitUsage asserts the one limit detail of a validation: the usage it
// reports (projected usage when exceeded) and whether it was exceeded.
func assertLimitUsage(t *testing.T, result testutil.ValidationResponse, limitID, usage string, exceeded bool) {
	t.Helper()

	require.Len(t, result.LimitUsageDetails, 1)

	detail := result.LimitUsageDetails[0]
	assert.Equal(t, limitID, detail.LimitID)
	assert.False(t, detail.Skipped, "the transaction is inside the window")
	assert.Equal(t, exceeded, detail.Exceeded)
	assert.True(t, detail.CurrentUsage.Equal(decimal.RequireFromString(usage)),
		"want usage %s, got %s", usage, detail.CurrentUsage)
}

// reservePix reserves a Pix for accountID over the gRPC seam at the server's
// current time.
func reservePix(t *testing.T, transactionID, accountID uuid.UUID, amount string) *reservationv1.ReserveResult {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), resetTimeReserveTimeout)
	defer cancel()

	got, err := testutil.DialReservationClient(t).Reserve(ctx, &reservationv1.ReserveRequest{
		TransactionId:        transactionID.String(),
		RequestId:            uuid.New().String(),
		Amount:               amount,
		Asset:                "BRL",
		TransactionType:      "PIX",
		TransactionTimestamp: testutil.TestNow().Add(-30 * time.Second).Format(time.RFC3339),
		Account:              &reservationv1.ReserveAccount{AccountId: accountID.String()},
	})
	require.NoError(t, err, "Reserve must succeed over the gRPC seam")
	require.NotNil(t, got)

	return got
}

// confirmTransaction confirms the transaction's reservations, so its spend
// counts as usage and no reaper can release it.
func confirmTransaction(t *testing.T, transactionID uuid.UUID) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), resetTimeReserveTimeout)
	defer cancel()

	got, err := testutil.DialReservationClient(t).ConfirmByTransaction(ctx,
		&reservationv1.ConfirmByTransactionRequest{TransactionId: transactionID.String()})
	require.NoError(t, err, "ConfirmByTransaction must succeed over the gRPC seam")
	assert.Equal(t, uint32(1), got.GetConfirmed())
}

// counterUsageByPeriod returns the limit's usage (current plus reserved) per
// period key, as stored.
func counterUsageByPeriod(t *testing.T, db *sql.DB, limitID string) map[string]string {
	t.Helper()

	rows, err := db.Query(
		`SELECT period_key, current_usage + reserved_usage FROM usage_counters WHERE limit_id = $1`, limitID,
	)
	require.NoError(t, err)

	defer func() { _ = rows.Close() }()

	usage := make(map[string]string)

	for rows.Next() {
		var (
			periodKey string
			amount    decimal.Decimal
		)

		require.NoError(t, rows.Scan(&periodKey, &amount))

		usage[periodKey] = amount.String()
	}

	require.NoError(t, rows.Err())

	return usage
}

// assertResetAt asserts resetAt on both GET /v1/limits/{id} and its usage
// snapshot.
func assertResetAt(t *testing.T, limitID, want string) {
	t.Helper()

	wantAt, err := time.Parse(time.RFC3339, want)
	require.NoError(t, err)

	for _, path := range []string{"/v1/limits/" + limitID, "/v1/limits/" + limitID + "/usage"} {
		status, body := limitRequest(t, http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, status, string(body))

		raw, ok := decodeJSONObject(t, body)["resetAt"].(string)
		require.True(t, ok, "%s must carry resetAt: %s", path, string(body))

		got, err := time.Parse(time.RFC3339, raw)
		require.NoError(t, err)
		assert.True(t, wantAt.Equal(got), "%s resetAt: want %s, got %s", path, want, raw)
	}
}

// nightLimitBody is the create body of the nighttime Pix limit reset at 09:00.
func nightLimitBody(accountID string) map[string]any {
	return map[string]any{
		"name":            "Night Pix " + testutil.RandomSuffix(),
		"limitType":       "DAILY",
		"maxAmount":       nightCap,
		"asset":           "BRL",
		"activeTimeStart": nightWindowStart,
		"activeTimeEnd":   nightWindowEnd,
		"resetTime":       nightResetTime,
		"scopes":          []map[string]any{{"accountId": accountID}},
	}
}

// limitRequest sends an authenticated JSON request to the limits API and
// returns the status and body.
func limitRequest(t *testing.T, method, path string, payload any) (int, []byte) {
	t.Helper()

	var reqBody io.Reader = http.NoBody

	if payload != nil {
		encoded, err := json.Marshal(payload)
		require.NoError(t, err)

		reqBody = bytes.NewReader(encoded)
	}

	req, err := http.NewRequest(method, testutil.GetBaseURL()+path, reqBody)
	require.NoError(t, err)
	req.Header.Set("X-API-Key", testutil.GetAPIKey())
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, body
}

// decodeJSONObject decodes a JSON object response body.
func decodeJSONObject(t *testing.T, body []byte) map[string]any {
	t.Helper()

	var object map[string]any
	require.NoError(t, json.Unmarshal(body, &object), string(body))

	return object
}

// storedResetTime reads limits.reset_time for the limit.
func storedResetTime(t *testing.T, db *sql.DB, limitID string) sql.NullString {
	t.Helper()

	var resetTime sql.NullString
	require.NoError(t, db.QueryRow(`SELECT reset_time FROM limits WHERE id = $1`, limitID).Scan(&resetTime))

	return resetTime
}
