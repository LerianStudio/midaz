// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"errors"
	"testing"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	"github.com/stretchr/testify/require"
)

func contextTracerTestConfig() Config {
	return Config{
		TracerContextEnabled: true, TracerBaseURL: "https://tracer.test:4020", TracerTLSMode: "mtls", TracerTimeoutMs: 250,
		TracerContextMaxBodyBytes: 65536, TracerContextMaxAccounts: 10, TracerContextMaxEntries: 20, TracerContextMaxTextBytes: 256, TracerContextMaxIntegerDigits: 128, TracerContextMaxFractionDigits: "128", TracerContextMaxReservations: 100,
	}
}

func TestContextTracerRequiresExplicitConfiguration(t *testing.T) {
	for _, scenario := range []string{"valid", "zero precision", "missing precision", "mesh", "empty endpoint", "empty timeout", "empty body bound", "empty reservation bound"} {
		t.Run(scenario, func(t *testing.T) {
			cfg := contextTracerTestConfig()
			switch scenario {
			case "zero precision":
				cfg.TracerContextMaxFractionDigits = "0"
			case "missing precision":
				cfg.TracerContextMaxFractionDigits = ""
			case "mesh":
				cfg.TracerTLSMode = "mesh"
			case "empty endpoint":
				cfg.TracerBaseURL = ""
			case "empty timeout":
				cfg.TracerTimeoutMs = 0
			case "empty body bound":
				cfg.TracerContextMaxBodyBytes = 0
			case "empty reservation bound":
				cfg.TracerContextMaxReservations = 0
			}
			parsed, err := parseContextTracerConfig(&cfg)
			if scenario == "valid" {
				require.NoError(t, err)
				require.Equal(t, 250*time.Millisecond, parsed.coordinator.AdmissionTimeout)
				require.Equal(t, parsed.operationTimeout, parsed.coordinator.AdmissionTimeout)
				require.Equal(t, parsed.client.Bounds, parsed.coordinator.Bounds)
				require.Equal(t, cfg.TracerContextMaxReservations, parsed.coordinator.MaxReservations)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestContextTracerDisabledBuildsNothing(t *testing.T) {
	runtime, err := buildContextTracer(&Config{}, nil)
	require.NoError(t, err)
	require.Nil(t, runtime)
}

func TestContextTracerRuntimeBuildsCoordinator(t *testing.T) {
	certs := writeSeamCertFiles(t)
	for _, transport := range []string{"grpc", "rest"} {
		t.Run(transport, func(t *testing.T) {
			cfg := contextTracerTestConfig()
			cfg.TracerTransport = transport
			cfg.TracerTLSCertFile, cfg.TracerTLSKeyFile, cfg.TracerTLSCAFile = certs.certFile, certs.keyFile, certs.caFile
			runtime, err := buildContextTracer(&cfg, &libPostgres.Client{})
			require.NoError(t, err)
			require.NotNil(t, runtime.coordinator)
			require.NoError(t, runtime.coordinator.ValidateActivation(t.Context()))
			if runtime.close != nil {
				require.NoError(t, runtime.close())
			}
			_, err = buildContextTracer(&cfg, nil)
			require.Error(t, err, "no activation without the official facts store")
		})
	}
}

func TestContextTracerRESTRefusesPlaintext(t *testing.T) {
	certs := writeSeamCertFiles(t)
	cfg := contextTracerTestConfig()
	cfg.TracerTransport, cfg.TracerBaseURL = "rest", "http://tracer.test:4020"
	cfg.TracerTLSCertFile, cfg.TracerTLSKeyFile, cfg.TracerTLSCAFile = certs.certFile, certs.keyFile, certs.caFile
	parsed, err := parseContextTracerConfig(&cfg)
	require.NoError(t, err)
	_, _, err = buildContextTracerClient(&cfg, parsed)
	require.Error(t, err)
}

func TestCombinedTracerClosersRunEveryClient(t *testing.T) {
	require.Nil(t, combineTracerClosers(nil, nil))
	first := errors.New("first close failed")
	calls := 0
	closeClients := combineTracerClosers(func() error { calls++; return first }, nil, func() error { calls++; return nil })
	require.ErrorIs(t, closeClients(), first)
	require.Equal(t, 2, calls)
}
