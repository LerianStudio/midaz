// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type revertRolloutBarrierStub struct {
	ready bool
	err   error
}

func (s revertRolloutBarrierStub) ReadyForMode(context.Context, string) (bool, error) {
	return s.ready, s.err
}

func TestRevertRolloutBarrierChecker_MarkerOffBlocksReadiness(t *testing.T) {
	t.Parallel()

	checker := NewRevertRolloutBarrierChecker(revertRolloutBarrierStub{}, "bridge", true)
	handler := newReadyHandler(ReadyzHandlerConfig{Logger: newTestLogger(), Checkers: []DependencyChecker{checker}, DeploymentMode: "local"})
	app := fiber.New()
	app.Get("/readyz", handler.HandleReadyz)

	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.NoError(t, err)
	defer response.Body.Close()
	assert.Equal(t, http.StatusServiceUnavailable, response.StatusCode)

	var body ReadyzResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	assert.Equal(t, StatusDown, body.Checks[checker.Name()].Status)
}

func TestRevertRolloutBarrierChecker_FinalAcceptsFinalizedBarrier(t *testing.T) {
	t.Parallel()

	checker := NewRevertRolloutBarrierChecker(revertRolloutBarrierStub{ready: true}, "final", true)
	check := checker.Check(context.Background())
	assert.Equal(t, StatusUp, check.Status)
	assert.True(t, checker.TLSEnabled())
}

func TestRevertRolloutBarrierMode(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "legacy", revertRolloutBarrierMode("legacy"))
	assert.Equal(t, "bridge", revertRolloutBarrierMode("bridge"))
	assert.Equal(t, "final", revertRolloutBarrierMode("final"))
}
