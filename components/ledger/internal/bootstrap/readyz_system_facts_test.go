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
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/tests/utils/stubs"
)

// fakeForwarderStatus answers a fixed forwarder lifecycle snapshot.
type fakeForwarderStatus struct {
	status outbox.ForwarderStatus
}

func (f fakeForwarderStatus) Status() outbox.ForwarderStatus { return f.status }

// fakeContractGate answers a fixed gate state.
type fakeContractGate struct {
	state systemFactsContractState
}

func (f fakeContractGate) contractState() systemFactsContractState { return f.state }

// forwarderRunError carries a host, the kind of detail a Run error can hold
// and a readiness body must never echo.
var forwarderRunError = errors.New("outbox: forwarder run: dial tcp 10.0.0.7:5432: connection refused")

func TestSystemFactsForwarderChecker_Check(t *testing.T) {
	t.Parallel()

	const notDraining = "; facts are not draining on this pod"

	tests := []struct {
		name       string
		gate       systemFactsContractStateReader
		status     outbox.ForwarderStatus
		wantReason string
		wantWarn   bool
	}{
		{
			name:       "pending gate with an idle forwarder names the contract",
			gate:       fakeContractGate{state: systemFactsContractPending},
			status:     outbox.ForwarderStatus{State: outbox.ForwarderStateIdle},
			wantReason: "contract not active",
		},
		{
			name:       "pending gate hides the forwarder state, which cannot have run yet",
			gate:       fakeContractGate{state: systemFactsContractPending},
			status:     outbox.ForwarderStatus{State: outbox.ForwarderStateFailed, Err: forwarderRunError},
			wantReason: "contract not active",
		},
		{
			name:       "nil gate reader is pending",
			status:     outbox.ForwarderStatus{State: outbox.ForwarderStateRunning},
			wantReason: "contract not active",
		},
		{
			name:       "rejected gate with an idle forwarder names the refusal",
			gate:       fakeContractGate{state: systemFactsContractRejected},
			status:     outbox.ForwarderStatus{State: outbox.ForwarderStateIdle},
			wantReason: "contract rejected",
		},
		{
			name:       "rejected gate with a stopped forwarder still names only the refusal",
			gate:       fakeContractGate{state: systemFactsContractRejected},
			status:     outbox.ForwarderStatus{State: outbox.ForwarderStateStopped},
			wantReason: "contract rejected",
		},
		{
			name:   "active gate with an idle forwarder is up because the Launcher starts apps concurrently",
			gate:   fakeContractGate{state: systemFactsContractActive},
			status: outbox.ForwarderStatus{State: outbox.ForwarderStateIdle},
		},
		{
			name:   "active gate with a running forwarder is up",
			gate:   fakeContractGate{state: systemFactsContractActive},
			status: outbox.ForwarderStatus{State: outbox.ForwarderStateRunning},
		},
		{
			name:       "active gate with a stopped forwarder stays up with a fixed reason",
			gate:       fakeContractGate{state: systemFactsContractActive},
			status:     outbox.ForwarderStatus{State: outbox.ForwarderStateStopped},
			wantReason: "forwarder stopped" + notDraining,
			wantWarn:   true,
		},
		{
			name:       "active gate with a failed forwarder stays up with a fixed reason that never carries the run error",
			gate:       fakeContractGate{state: systemFactsContractActive},
			status:     outbox.ForwarderStatus{State: outbox.ForwarderStateFailed, Err: forwarderRunError},
			wantReason: "forwarder failed" + notDraining,
			wantWarn:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logger := &stubs.LoggerStub{}
			checker := NewSystemFactsForwarderChecker("system_facts_forwarder", fakeForwarderStatus{status: tt.status},
				tt.gate, "https://system-facts.lerian.internal", logger)

			got := checker.Check(context.Background())

			assert.Equal(t, "system_facts_forwarder", checker.Name())
			assert.Equal(t, StatusUp, got.Status, "neither the gate nor the forwarder state takes the pod out of rotation")
			assert.Empty(t, got.Error)
			assert.Equal(t, tt.wantReason, got.Reason)
			assert.NotContains(t, got.Reason, "10.0.0.7", "the run error must never reach the check")

			if !tt.wantWarn {
				assert.Empty(t, logger.Warnings)

				return
			}

			require.Len(t, logger.Warnings, 1, "a forwarder that is not running is logged at Warn")
			assert.Contains(t, logger.Warnings[0], "state="+string(tt.status.State))

			if tt.status.Err != nil {
				assert.Contains(t, logger.Warnings[0], "10.0.0.7", "the run error goes to the log, never the response")
			}
		})
	}
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

			checker := NewSystemFactsForwarderChecker("system_facts_forwarder", fakeForwarderStatus{}, fakeContractGate{}, tt.url, libLog.NewNop())

			assert.Equal(t, tt.want, checker.TLSEnabled())
		})
	}
}

// TestReadyzHandler_SystemFactsForwarderFailed_KeepsThePodReady runs the
// checker through the handler: a failed forwarder behind an active gate answers
// 200 with the fixed reason in every mode, and the run error reaches the body
// in none.
func TestReadyzHandler_SystemFactsForwarderFailed_KeepsThePodReady(t *testing.T) {
	t.Parallel()

	for _, deploymentMode := range []string{DeploymentModeLocal, DeploymentModeSaaS} {
		t.Run(deploymentMode, func(t *testing.T) {
			t.Parallel()

			checker := NewSystemFactsForwarderChecker("system_facts_forwarder",
				fakeForwarderStatus{status: outbox.ForwarderStatus{State: outbox.ForwarderStateFailed, Err: forwarderRunError}},
				fakeContractGate{state: systemFactsContractActive}, "https://system-facts.lerian.internal", libLog.NewNop())

			handler := newReadyHandler(ReadyzHandlerConfig{
				Logger:         libLog.NewNop(),
				Checkers:       []DependencyChecker{checker},
				DeploymentMode: deploymentMode,
			})

			app := fiber.New()
			app.Get("/readyz", handler.HandleReadyz)

			resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
			require.NoError(t, err)

			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusOK, resp.StatusCode, "a stopped forwarder never takes the pod out of rotation")
			assert.NotContains(t, string(body), "10.0.0.7", "the run error must never reach the response")

			var parsed ReadyzResponse

			require.NoError(t, json.Unmarshal(body, &parsed))
			assert.Equal(t, "healthy", parsed.Status)
			assert.Equal(t, StatusUp, parsed.Checks["system_facts_forwarder"].Status)
			assert.Empty(t, parsed.Checks["system_facts_forwarder"].Error)
			assert.Equal(t, "forwarder failed; facts are not draining on this pod", parsed.Checks["system_facts_forwarder"].Reason)
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

	t.Run("enabled producer registers the forwarder checker over the gate", func(t *testing.T) {
		t.Parallel()

		cfg := enabledSystemFactsConfig()
		cfg.SystemFactsURL = systemFactsLoopbackURL

		components, err := buildSystemFacts(context.Background(), &cfg, libLog.NewNop(), nil, nil, offlineTransactionClient(t), nil,
			systemFactsDeps{authClient: systemFactsAuthClient()})
		require.NoError(t, err)

		checkers := appendSystemFactsForwarderChecker(nil, &cfg, components, libLog.NewNop())

		require.Len(t, checkers, 1)
		assert.Equal(t, "system_facts_forwarder", checkers[0].Name())
		assert.False(t, checkers[0].TLSEnabled(), "the loopback test URL is plain http")

		got := checkers[0].Check(context.Background())
		assert.Equal(t, StatusUp, got.Status, "a contract not reconciled yet keeps the pod ready")
		assert.Equal(t, "contract not active", got.Reason)
	})
}
