// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// =============================================================================
// Time-window and custom-period rejections on the limits surface.
//
// The create and update commands raise these as plain sentinels. They are all in
// the platform error registry as validation errors with their own code, title and
// message, but the handler's classifier had no arm for any of them, so every one
// fell through to the internal-server default: the caller got 500 with code 0046
// and the detail scrubbed to "internal error".
//
// An integrator who sent one half of a time window, or custom dates on a monthly
// limit, could not tell a bad payload from a Tracer outage — so the reflex was
// to retry and escalate when the fix was one field.
// =============================================================================

// periodRejections are the sentinels the limit write paths raise for a bad time
// window or a bad custom period, with the code each must publish.
//
// Sources: pkg/model/limit.go ValidateTimeWindow + ValidateCustomPeriod +
// Limit.Update, pkg/model/time_of_day.go ParseTimeOfDay, and the create/update
// commands' own RFC3339 parse failures.
func periodRejections() []struct {
	name string
	err  error
	code string
} {
	return []struct {
		name string
		err  error
		code string
	}{
		{"partial time window", constant.ErrLimitTimeWindowMismatch, "0438"},
		{"zero-width time window", constant.ErrLimitTimeWindowZeroWidth, "0439"},
		{"malformed time of day", constant.ErrTimeOfDayInvalidFormat, "0440"},
		{"custom dates on a non-custom limit", constant.ErrLimitCustomDatesNotAllowed, "0443"},
		{"custom period too long", constant.ErrLimitCustomPeriodTooLong, "0445"},
		{"custom period already ended", constant.ErrLimitCustomPeriodExpired, "0446"},
		{"malformed custom start date", constant.ErrLimitInvalidCustomStartFormat, "0447"},
		{"malformed custom end date", constant.ErrLimitInvalidCustomEndFormat, "0448"},
		{"custom dates missing", constant.ErrLimitCustomDatesRequired, "0449"},
		{"custom dates out of order", constant.ErrLimitCustomDatesOrder, "0450"},
	}
}

func decodeLimitProblem(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body), "body must be JSON: %s", raw)

	return body
}

// TestHuma_CreateLimit_PeriodRejectionsAreClientErrors drives a real request per
// sentinel and decodes the body. Each must come back as a client rejection
// carrying its own code, never as 500/0046 with the detail scrubbed away.
func TestHuma_CreateLimit_PeriodRejectionsAreClientErrors(t *testing.T) {
	for _, tc := range periodRejections() {
		t.Run(tc.name, func(t *testing.T) {
			svc := &tenantSpyLimitService{createErr: tc.err}
			app := buildHumaLimitApp(t, svc, "tenant-period")

			req := httptest.NewRequest(http.MethodPost, "/v1/limits", bytes.NewReader(validCreateLimitBody()))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)

			body := decodeLimitProblem(t, resp)

			require.Less(t, resp.StatusCode, http.StatusInternalServerError,
				"a bad time window or custom period is the caller's mistake, not a server failure; got %d with %v",
				resp.StatusCode, body)
			require.Equal(t, tc.code, body["code"],
				"the rejection MUST publish its own registry code so the caller can act on it; got %v", body)
			require.NotEqual(t, "internal error", body["detail"],
				"the detail MUST name what is wrong with the payload, not be scrubbed as a server fault")
			require.NotEmpty(t, body["title"], "the rejection MUST carry its registry title")
		})
	}
}

// TestHuma_UpdateLimit_PeriodRejectionsAreClientErrors covers the PATCH path,
// which raises the same sentinels through the same classifier.
func TestHuma_UpdateLimit_PeriodRejectionsAreClientErrors(t *testing.T) {
	for _, tc := range periodRejections() {
		t.Run(tc.name, func(t *testing.T) {
			svc := &tenantSpyLimitService{updateErr: tc.err}
			app := buildHumaLimitApp(t, svc, "tenant-period")

			patch, err := json.Marshal(map[string]any{"description": "touch"})
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPatch,
				"/v1/limits/"+testutil.MustDeterministicUUID(7).String(), bytes.NewReader(patch))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)

			body := decodeLimitProblem(t, resp)

			require.Less(t, resp.StatusCode, http.StatusInternalServerError,
				"the update path must classify these the same way as create; got %d with %v",
				resp.StatusCode, body)
			require.Equal(t, tc.code, body["code"],
				"the rejection MUST publish its own registry code; got %v", body)
		})
	}
}
