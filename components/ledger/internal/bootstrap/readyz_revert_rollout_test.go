// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type revertRolloutBarrierStub struct {
	ready         bool
	err           error
	durabilityErr error
	generation    string
	generationErr error
	phase         string
}

func (s revertRolloutBarrierStub) ReadyForMode(context.Context, string) (bool, error) {
	return s.ready, s.err
}

func (s revertRolloutBarrierStub) FinancialDurability(context.Context) error {
	return s.durabilityErr
}

func (s revertRolloutBarrierStub) FinancialDatasetGeneration(context.Context) (string, error) {
	if s.generation == "" && s.generationErr == nil {
		return "test-generation", nil
	}

	return s.generation, s.generationErr
}

func (s revertRolloutBarrierStub) ValidateFinancialDatasetGeneration(context.Context) error {
	return s.generationErr
}

func (s revertRolloutBarrierStub) Phase(context.Context) (string, error) {
	return s.phase, s.err
}

func TestRevertRolloutBarrierChecker_MarkerOffBlocksReadiness(t *testing.T) {
	t.Parallel()

	checker := NewRevertRolloutBarrierChecker(revertRolloutBarrierStub{}, "bridge", "active", true)
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

	checker := NewRevertRolloutBarrierChecker(revertRolloutBarrierStub{ready: true}, "final", "finalized", true)
	check := checker.Check(context.Background())
	assert.Equal(t, StatusUp, check.Status)
	assert.True(t, checker.TLSEnabled())
}

func TestRevertRolloutBarrierChecker_RejectsUnsafeFinancialRedis(t *testing.T) {
	t.Parallel()

	checker := NewRevertRolloutBarrierChecker(revertRolloutBarrierStub{
		ready: true, phase: "active", durabilityErr: errors.New("appendonly must be enabled"),
	}, "legacy", "active", true)
	check := checker.Check(context.Background())
	assert.Equal(t, StatusDown, check.Status)
	assert.ErrorContains(t, errors.New(check.Error), "appendonly must be enabled")
}

func TestRevertRolloutBarrierChecker_RejectsDifferentFinancialGeneration(t *testing.T) {
	t.Parallel()

	checker := NewRevertRolloutBarrierChecker(revertRolloutBarrierStub{
		ready: true, phase: "prepared", generationErr: errors.New("financial Redis dataset generation differs"),
	}, "legacy", "prepared", true)
	check := checker.Check(context.Background())
	assert.Equal(t, StatusDown, check.Status)
	assert.ErrorContains(t, errors.New(check.Error), "generation differs")
}

func TestRevertRolloutBarrierChecker_ReleasedLegacyDoesNotRequireDurableRolloutStorage(t *testing.T) {
	t.Parallel()

	checker := NewRevertRolloutBarrierChecker(revertRolloutBarrierStub{
		ready: true, durabilityErr: errors.New("appendonly must be enabled"),
	}, "legacy", "", true)
	assert.Equal(t, StatusUp, checker.Check(context.Background()).Status)
}

func TestRevertRolloutBarrierChecker_LegacyActiveTargetRejectsLostMarker(t *testing.T) {
	t.Parallel()

	checker := NewRevertRolloutBarrierChecker(revertRolloutBarrierStub{ready: false},
		"legacy", "active", true)
	check := checker.Check(context.Background())
	assert.Equal(t, StatusDown, check.Status)
	assert.Contains(t, check.Reason, "does not admit")
}

func TestRevertRolloutBarrierMode(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "legacy", revertRolloutBarrierMode("legacy"))
	assert.Equal(t, "bridge", revertRolloutBarrierMode("bridge"))
	assert.Equal(t, "final", revertRolloutBarrierMode("final"))
}
