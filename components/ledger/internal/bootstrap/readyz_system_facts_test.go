// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-system-facts/outbox"
	"github.com/LerianStudio/lib-system-facts/reconciler"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeForwarderStatus answers a fixed forwarder lifecycle snapshot.
type fakeForwarderStatus struct {
	status outbox.ForwarderStatus
}

func (f fakeForwarderStatus) Status() outbox.ForwarderStatus { return f.status }

// forwarderRunError carries a host, the kind of detail a Run error can hold
// and a readiness body must never echo.
var forwarderRunError = errors.New("outbox: forwarder run: dial tcp 10.0.0.7:5432: connection refused")

func TestSystemFactsForwarderChecker_Check(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     outbox.ForwarderStatus
		emitting   bool
		wantStatus DependencyStatus
		wantError  string
		wantReason string
	}{
		{
			name:       "idle is up because the Launcher starts apps concurrently",
			status:     outbox.ForwarderStatus{State: outbox.ForwarderStateIdle},
			emitting:   true,
			wantStatus: StatusUp,
		},
		{
			name:       "running is up",
			status:     outbox.ForwarderStatus{State: outbox.ForwarderStateRunning},
			emitting:   true,
			wantStatus: StatusUp,
		},
		{
			name:       "stopped is down with a fixed error",
			status:     outbox.ForwarderStatus{State: outbox.ForwarderStateStopped},
			emitting:   true,
			wantStatus: StatusDown,
			wantError:  "forwarder stopped",
		},
		{
			name:       "failed is down with a fixed error that never carries the run error",
			status:     outbox.ForwarderStatus{State: outbox.ForwarderStateFailed, Err: forwarderRunError},
			emitting:   true,
			wantStatus: StatusDown,
			wantError:  "forwarder failed",
		},
		{
			name:       "running with emission off stays up and names the preflight",
			status:     outbox.ForwarderStatus{State: outbox.ForwarderStateRunning},
			wantStatus: StatusUp,
			wantReason: "emission disabled by contract preflight",
		},
		{
			name:       "failed with emission off keeps the preflight reason",
			status:     outbox.ForwarderStatus{State: outbox.ForwarderStateFailed, Err: forwarderRunError},
			wantStatus: StatusDown,
			wantError:  "forwarder failed",
			wantReason: "emission disabled by contract preflight",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			checker := NewSystemFactsForwarderChecker("system_facts_forwarder", fakeForwarderStatus{status: tt.status},
				tt.emitting, "https://system-facts.lerian.internal", libLog.NewNop())

			got := checker.Check(context.Background())

			assert.Equal(t, "system_facts_forwarder", checker.Name())
			assert.Equal(t, tt.wantStatus, got.Status)
			assert.Equal(t, tt.wantError, got.Error)
			assert.Equal(t, tt.wantReason, got.Reason)
			assert.NotContains(t, got.Error+got.Reason, "10.0.0.7", "the run error must never reach the check")
		})
	}
}

func TestSystemFactsForwarderChecker_Check_NilForwarderIsSkipped(t *testing.T) {
	t.Parallel()

	checker := NewSystemFactsForwarderChecker("system_facts_forwarder", nil, false, "", libLog.NewNop())

	got := checker.Check(context.Background())

	assert.Equal(t, StatusSkipped, got.Status)
	assert.Equal(t, "System Facts emission disabled", got.Reason)
	assert.Empty(t, got.Error)
}

func TestSystemFactsForwarderChecker_TLSEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		want bool
	}{
		{name: "https is TLS", url: "https://system-facts.lerian.internal", want: true},
		{name: "scheme case and padding are ignored", url: "  HTTPS://system-facts  ", want: true},
		{name: "http is not TLS", url: "http://localhost:8080", want: false},
		{name: "empty is not TLS", url: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			checker := NewSystemFactsForwarderChecker("system_facts_forwarder", nil, false, tt.url, libLog.NewNop())

			assert.Equal(t, tt.want, checker.TLSEnabled())
		})
	}
}

// TestReadyzHandler_SystemFactsForwarderFailed_NeverExposesTheRunError runs
// the checker through the handler: the body carries the fixed text in local
// mode and the sanitized text elsewhere, and the run error in neither.
func TestReadyzHandler_SystemFactsForwarderFailed_NeverExposesTheRunError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		deploymentMode string
		wantError      string
	}{
		{name: "local keeps the fixed text", deploymentMode: DeploymentModeLocal, wantError: "forwarder failed"},
		{name: "saas sanitizes the fixed text", deploymentMode: DeploymentModeSaaS, wantError: "system_facts_forwarder check failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			checker := NewSystemFactsForwarderChecker("system_facts_forwarder",
				fakeForwarderStatus{status: outbox.ForwarderStatus{State: outbox.ForwarderStateFailed, Err: forwarderRunError}},
				true, "https://system-facts.lerian.internal", libLog.NewNop())

			handler := newReadyHandler(ReadyzHandlerConfig{
				Logger:         libLog.NewNop(),
				Checkers:       []DependencyChecker{checker},
				DeploymentMode: tt.deploymentMode,
			})

			app := fiber.New()
			app.Get("/readyz", handler.HandleReadyz)

			resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
			require.NoError(t, err)

			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
			assert.NotContains(t, string(body), "10.0.0.7", "the run error must never reach the response")

			var parsed ReadyzResponse

			require.NoError(t, json.Unmarshal(body, &parsed))
			assert.Equal(t, StatusDown, parsed.Checks["system_facts_forwarder"].Status)
			assert.Equal(t, tt.wantError, parsed.Checks["system_facts_forwarder"].Error)
		})
	}
}

func TestAppendSystemFactsForwarderChecker(t *testing.T) {
	t.Parallel()

	t.Run("disabled registers nothing", func(t *testing.T) {
		t.Parallel()

		checkers := appendSystemFactsForwarderChecker(nil, &Config{}, &systemFactsComponents{}, libLog.NewNop())

		assert.Empty(t, checkers)
	})

	t.Run("enabled without wiring registers a skipped checker", func(t *testing.T) {
		t.Parallel()

		checkers := appendSystemFactsForwarderChecker(nil, &Config{SystemFactsEnabled: true}, nil, libLog.NewNop())

		require.Len(t, checkers, 1)
		assert.Equal(t, "system_facts_forwarder", checkers[0].Name())
		assert.Equal(t, StatusSkipped, checkers[0].Check(context.Background()).Status)
	})

	t.Run("enabled producer registers the forwarder checker", func(t *testing.T) {
		t.Parallel()

		cfg := enabledSystemFactsConfig()
		cfg.SystemFactsURL = systemFactsLoopbackURL
		seam := &preflightRecorder{report: reconciler.Report{Outcome: reconciler.OutcomeRejected}, err: reconciler.ErrRejected}

		components, err := buildSystemFacts(context.Background(), &cfg, libLog.NewNop(), nil, nil, offlineTransactionClient(t), nil,
			systemFactsDeps{authClient: systemFactsAuthClient(), preflight: seam.preflight})
		require.NoError(t, err)

		checkers := appendSystemFactsForwarderChecker(nil, &cfg, components, libLog.NewNop())

		require.Len(t, checkers, 1)
		assert.Equal(t, "system_facts_forwarder", checkers[0].Name())
		assert.False(t, checkers[0].TLSEnabled(), "the loopback test URL is plain http")

		got := checkers[0].Check(context.Background())
		assert.Equal(t, StatusUp, got.Status, "a forwarder not started yet is idle, which is up")
		assert.Equal(t, "emission disabled by contract preflight", got.Reason)
	})
}
