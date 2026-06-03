// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build testhooks

package in_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/bootstrap"
)

// TestHealth_ReturnsServiceUnavailable_BeforeSelfProbe verifies the K8s
// liveness contract: until RunSelfProbe completes successfully, /health
// MUST return 503 so the kubelet can restart pods that boot without their
// dependencies.
func TestHealth_ReturnsServiceUnavailable_BeforeSelfProbe(t *testing.T) {
	bootstrap.ResetSelfProbeForTest()
	t.Cleanup(bootstrap.ResetSelfProbeForTest)

	// Wire the production gate so the handler reads the same atomic that
	// bootstrap toggles. Mirrors what InitServers does at boot.
	prevGate := in.GetSelfProbeGateForTest()
	in.SetSelfProbeGate(bootstrap.IsSelfProbeOK)
	t.Cleanup(func() { in.SetSelfProbeGate(prevGate) })

	hc := in.NewTestableHealthChecker(nil)
	app := fiber.New()
	app.Get("/health", hc.LivenessHandler())

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode,
		"/health MUST be 503 before self-probe completes")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "self-probe",
		"503 body must explain why so operators can triage from a single line")
}

// TestHealth_Returns200_AfterSelfProbe verifies the gating flips: once
// RunSelfProbe has succeeded (selfProbeOK=true) the handler delegates to
// libHTTP.Ping and returns 200.
func TestHealth_Returns200_AfterSelfProbe(t *testing.T) {
	bootstrap.ResetSelfProbeForTest()
	t.Cleanup(bootstrap.ResetSelfProbeForTest)

	prevGate := in.GetSelfProbeGateForTest()
	in.SetSelfProbeGate(bootstrap.IsSelfProbeOK)
	t.Cleanup(func() { in.SetSelfProbeGate(prevGate) })

	bootstrap.MarkSelfProbeOKForTest()

	hc := in.NewTestableHealthChecker(nil)
	app := fiber.New()
	app.Get("/health", hc.LivenessHandler())

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"/health must return 200 once self-probe is OK")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "healthy",
		"libHTTP.Ping returns the canonical 'healthy' body")
}
