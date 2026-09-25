// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"errors"
	"testing"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/stretchr/testify/require"
)

func contextTracerTestConfig() Config {
	return Config{
		TracerRecoveryMaxRetryIntervalMs: 300000,
		TracerContextEnabled:             true, TracerBaseURL: "https://tracer.test:4020", TracerTLSMode: "mtls", TracerTimeoutMs: 250, TracerIntegrationID: "producer", TracerAssetNamespace: "origin-a",
		TracerContextMaxBodyBytes: 65536, TracerContextMaxAccounts: 10, TracerContextMaxEntries: 20, TracerContextMaxTextBytes: 256, TracerContextMaxIntegerDigits: 128, TracerContextMaxFractionDigits: "128", TracerContextMaxReservations: 100,
		TracerRecoveryBatchSize: 10, TracerRecoveryIntervalMs: 1000, TracerRecoveryCycleTimeoutMs: 1000, TracerRecoveryTenantTimeoutMs: 500, TracerRecoveryAttemptTimeoutMs: 250, TracerRecoveryMaxTenants: 10, TracerRecoveryMaxCatalogTenants: 100,
	}
}

func TestContextTracerRequiresExplicitConfiguration(t *testing.T) {
	for _, scenario := range []string{"valid", "zero precision", "missing precision", "missing namespace", "missing producer", "mesh", "empty endpoint", "empty timeout", "empty interval", "empty batch", "empty catalog bound"} {
		t.Run(scenario, func(t *testing.T) {
			cfg := contextTracerTestConfig()
			switch scenario {
			case "zero precision":
				cfg.TracerContextMaxFractionDigits = "0"
			case "missing precision":
				cfg.TracerContextMaxFractionDigits = ""
			case "missing namespace":
				cfg.TracerAssetNamespace = ""
			case "missing producer":
				cfg.TracerIntegrationID = ""
			case "mesh":
				cfg.TracerTLSMode = "mesh"
			case "empty endpoint":
				cfg.TracerBaseURL = ""
			case "empty timeout":
				cfg.TracerTimeoutMs = 0
			case "empty interval":
				cfg.TracerRecoveryIntervalMs = 0
			case "empty batch":
				cfg.TracerRecoveryBatchSize = 0
			case "empty catalog bound":
				cfg.TracerRecoveryMaxCatalogTenants = 0
			}
			parsed, err := parseContextTracerConfig(&cfg, "ledger")
			if scenario == "valid" || scenario == "zero precision" {
				require.NoError(t, err)
				require.Equal(t, 250*time.Millisecond, parsed.coordinator.AdmissionTimeout)
				require.Equal(t, parsed.operationTimeout, parsed.coordinator.AdmissionTimeout)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestContextTracerDisabledWithoutIdentitySkipsDrain(t *testing.T) {
	runtime, err := buildContextTracer(&Config{}, contextTracerDependencies{})
	require.NoError(t, err)
	require.Nil(t, runtime)
}

func TestContextTracerRuntimeIncludesRecovery(t *testing.T) {
	certs := writeSeamCertFiles(t)
	for _, transport := range []string{"grpc", "rest"} {
		t.Run(transport, func(t *testing.T) {
			cfg := contextTracerTestConfig()
			cfg.TracerTransport = transport
			cfg.TracerTLSCertFile, cfg.TracerTLSKeyFile, cfg.TracerTLSCAFile = certs.certFile, certs.keyFile, certs.caFile
			deps := contextTracerDependencies{onboarding: &libPostgres.Client{}, transaction: &libPostgres.Client{}, service: "ledger", logger: libLog.NewNop()}
			runtime, err := buildContextTracer(&cfg, deps)
			require.NoError(t, err)
			require.NotNil(t, runtime.coordinator)
			require.NotNil(t, runtime.worker)
			require.NoError(t, runtime.coordinator.ValidateActivation(t.Context()))
			if runtime.close != nil {
				require.NoError(t, runtime.close())
			}
			apps := (&Service{TracerRecoveryWorker: runtime.worker}).launcherApps()
			found := false
			for _, app := range apps {
				if app.name == "Tracer Recovery Worker" {
					found = true
					require.Same(t, runtime.worker, app.app)
				}
			}
			require.True(t, found)
			cfg.MultiTenantEnabled = true
			_, err = buildContextTracer(&cfg, deps)
			require.Error(t, err, "no activation without tenant discovery and pool resolution")
		})
	}
}

func TestContextTracerRESTRefusesPlaintext(t *testing.T) {
	certs := writeSeamCertFiles(t)
	cfg := contextTracerTestConfig()
	cfg.TracerTransport, cfg.TracerBaseURL = "rest", "http://tracer.test:4020"
	cfg.TracerTLSCertFile, cfg.TracerTLSKeyFile, cfg.TracerTLSCAFile = certs.certFile, certs.keyFile, certs.caFile
	parsed, err := parseContextTracerConfig(&cfg, "ledger")
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
