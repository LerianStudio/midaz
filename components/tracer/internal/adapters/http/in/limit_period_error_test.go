// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"encoding/json"
	"fmt"
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

// periodRejections are the sentinels a request PAYLOAD can raise for a bad time
// window or a bad custom period, with the code and the exact status each must
// publish.
//
// The status is asserted exactly, not as "below 500". A range assertion passes
// for 400, 422, 404 and 302 alike, so it cannot tell a correct classification
// from a wrong one — and it is what let a server fault sit in this table
// unnoticed.
//
// The seven 400s are malformed or contradictory input. The two 422s are
// syntactically valid dates a business rule refuses, which is what RFC 9110
// reserves 422 for.
//
// ErrTimeOfDayInvalidFormat (0440) is NOT here on purpose: no payload can raise
// it, so it is not a client error. TestHuma_Limits_StoredTimeWindowFaultStaysAServerError
// pins that.
//
// Sources: pkg/model/limit.go ValidateTimeWindow + ValidateCustomPeriod +
// Limit.Update, and the create/update commands' own RFC3339 parse failures.
func periodRejections() []struct {
	name   string
	err    error
	code   string
	status int
} {
	return []struct {
		name   string
		err    error
		code   string
		status int
	}{
		{"partial time window", constant.ErrLimitTimeWindowMismatch, "0438", http.StatusBadRequest},
		{"zero-width time window", constant.ErrLimitTimeWindowZeroWidth, "0439", http.StatusBadRequest},
		{"custom dates on a non-custom limit", constant.ErrLimitCustomDatesNotAllowed, "0443", http.StatusBadRequest},
		{"custom period too long", constant.ErrLimitCustomPeriodTooLong, "0445", http.StatusUnprocessableEntity},
		{"custom period already ended", constant.ErrLimitCustomPeriodExpired, "0446", http.StatusUnprocessableEntity},
		{"malformed custom start date", constant.ErrLimitInvalidCustomStartFormat, "0447", http.StatusBadRequest},
		{"malformed custom end date", constant.ErrLimitInvalidCustomEndFormat, "0448", http.StatusBadRequest},
		{"custom dates missing", constant.ErrLimitCustomDatesRequired, "0449", http.StatusBadRequest},
		{"custom dates out of order", constant.ErrLimitCustomDatesOrder, "0450", http.StatusBadRequest},
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

			require.Equal(t, tc.status, resp.StatusCode,
				"a bad time window or custom period must publish its exact status, not merely something below 500; got %d with %v",
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

			require.Equal(t, tc.status, resp.StatusCode,
				"the update path must classify these exactly as create does; got %d with %v",
				resp.StatusCode, body)
			require.Equal(t, tc.code, body["code"],
				"the rejection MUST publish its own registry code; got %v", body)
		})
	}
}

// limitRoute names one registered limit operation and the spy field that makes
// that operation's service call fail, so a classification property can be
// asserted across the WHOLE surface rather than on the two write paths the
// period rejections happen to reach.
type limitRoute struct {
	name   string
	method string
	path   string
	body   []byte
	inject func(*tenantSpyLimitService, error)
}

// allLimitRoutes is every operation RegisterLimitRoutes registers — all nine.
// Six of them carry no request body at all, which is the whole point: a
// classification that assumes "an error here is the caller's fault" is wrong on
// those by construction.
func allLimitRoutes() []limitRoute {
	id := testutil.MustDeterministicUUID(7).String()
	patch, _ := json.Marshal(map[string]any{"description": "touch"})

	return []limitRoute{
		{"create", http.MethodPost, "/v1/limits", validCreateLimitBody(),
			func(s *tenantSpyLimitService, err error) { s.createErr = err }},
		{"get", http.MethodGet, "/v1/limits/" + id, nil,
			func(s *tenantSpyLimitService, err error) { s.getErr = err }},
		{"list", http.MethodGet, "/v1/limits", nil,
			func(s *tenantSpyLimitService, err error) { s.listErr = err }},
		{"update", http.MethodPatch, "/v1/limits/" + id, patch,
			func(s *tenantSpyLimitService, err error) { s.updateErr = err }},
		{"activate", http.MethodPost, "/v1/limits/" + id + "/activate", nil,
			func(s *tenantSpyLimitService, err error) { s.lifecycleErr = err }},
		{"deactivate", http.MethodPost, "/v1/limits/" + id + "/deactivate", nil,
			func(s *tenantSpyLimitService, err error) { s.lifecycleErr = err }},
		{"draft", http.MethodPost, "/v1/limits/" + id + "/draft", nil,
			func(s *tenantSpyLimitService, err error) { s.lifecycleErr = err }},
		{"delete", http.MethodDelete, "/v1/limits/" + id, nil,
			func(s *tenantSpyLimitService, err error) { s.deleteErr = err }},
		{"usage", http.MethodGet, "/v1/limits/" + id + "/usage", nil,
			func(s *tenantSpyLimitService, err error) { s.usageErr = err }},
	}
}

// storedTimeWindowFault reproduces the error the postgres adapter actually
// returns when a limits row holds an active_time_start the domain cannot parse:
// ErrTimeOfDayInvalidFormat behind the two %w wraps the read path adds
// (limit_postgresql_model.go ToEntity, then limit_repository.go scanLimit and
// GetByID). Wrapped, because errors.Is is what the classifier uses and an
// unwrapped sentinel would not exercise the same path.
func storedTimeWindowFault() error {
	return fmt.Errorf("failed to get limit: failed to convert to entity: invalid active_time_start in database: %w",
		constant.ErrTimeOfDayInvalidFormat)
}

// TestHuma_Limits_StoredTimeWindowFaultStaysAServerError is the classification
// lock across the whole limits surface: a corrupt stored time window is a data
// fault, so every one of the nine operations must report it as a server error.
//
// Six of the nine routes carry no request body, so answering them 400 with
// "Invalid time of day format, expected HH:MM." told an integrator to fix a
// field they never sent, and nothing they could send would ever work. It also
// removed a data-integrity fault from 5xx alerting and stopped clients
// retrying.
//
// This fails if ErrTimeOfDayInvalidFormat is put back into
// limitPeriodRejectionSentinels.
func TestHuma_Limits_StoredTimeWindowFaultStaysAServerError(t *testing.T) {
	for _, route := range allLimitRoutes() {
		t.Run(route.name, func(t *testing.T) {
			svc := &tenantSpyLimitService{}
			route.inject(svc, storedTimeWindowFault())

			app := buildHumaLimitApp(t, svc, "tenant-period")

			var reader io.Reader
			if route.body != nil {
				reader = bytes.NewReader(route.body)
			}

			req := httptest.NewRequest(route.method, route.path, reader)
			if route.body != nil {
				req.Header.Set("Content-Type", "application/json")
			}

			resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)

			body := decodeLimitProblem(t, resp)

			require.Equal(t, http.StatusInternalServerError, resp.StatusCode,
				"a corrupt stored time window is a data fault, not a caller mistake; got %d with %v",
				resp.StatusCode, body)
			require.Equal(t, "0046", body["code"],
				"the fault MUST publish the internal-server code so it stays inside 5xx alerting; got %v", body)
			require.NotEqual(t, "0440", body["code"],
				"0440 names a malformed time-of-day in a REQUEST; no request on this route carries one")
		})
	}
}
