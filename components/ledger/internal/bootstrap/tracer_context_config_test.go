// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func contextTracerTestConfig() Config {
	return Config{
		TracerBaseURL: "https://tracer.test:4020", TracerTLSMode: "mtls", TracerTimeoutMs: 250,
		TracerContextMaxBodyBytes: 65536, TracerContextMaxAccounts: 10, TracerContextMaxEntries: 20, TracerContextMaxTextBytes: 256, TracerContextMaxIntegerDigits: 128, TracerContextMaxFractionDigits: "128", TracerContextMaxReservations: 100,
	}
}

// restTracerTestConfig is a REST integration with plugin auth and M2M
// credentials, the prerequisites of the bearer token.
func restTracerTestConfig() Config {
	cfg := contextTracerTestConfig()
	cfg.TracerTransport = "rest"
	cfg.AuthEnabled = true
	cfg.IDPM2MClientID, cfg.IDPM2MClientSecret = "ledger-client", "ledger-secret"

	return cfg
}

// testTracerAuthHost is a plugin-auth address; the test minters never dial it.
const testTracerAuthHost = "http://plugin-auth.test:4000"

// testTracerSecureAuthHost is a plugin-auth address the SaaS TLS gate accepts.
const testTracerSecureAuthHost = "https://plugin-auth.test:4000"

type countingTokenMinter struct {
	calls atomic.Int32
	err   error
}

func (m *countingTokenMinter) GetApplicationToken(context.Context, string, string) (string, error) {
	m.calls.Add(1)

	if m.err != nil {
		return "", m.err
	}

	return "dummy-m2m-token", nil
}

func closeContextTracerRuntime(t *testing.T, runtime *contextTracerRuntime) {
	t.Helper()

	if runtime != nil && runtime.close != nil {
		require.NoError(t, runtime.close())
	}
}

func TestContextTracerRequiresExplicitConfiguration(t *testing.T) {
	for _, scenario := range []string{"valid", "zero precision", "missing precision", "empty endpoint", "empty timeout", "empty body bound", "empty reservation bound", "invalid tls mode", "unknown transport"} {
		t.Run(scenario, func(t *testing.T) {
			cfg := contextTracerTestConfig()
			switch scenario {
			case "zero precision":
				cfg.TracerContextMaxFractionDigits = "0"
			case "missing precision":
				cfg.TracerContextMaxFractionDigits = ""
			case "empty endpoint":
				cfg.TracerBaseURL = ""
			case "empty timeout":
				cfg.TracerTimeoutMs = 0
			case "empty body bound":
				cfg.TracerContextMaxBodyBytes = 0
			case "empty reservation bound":
				cfg.TracerContextMaxReservations = 0
			case "invalid tls mode":
				cfg.TracerTLSMode = "tls"
			case "unknown transport":
				cfg.TracerTransport = "thrift"
			}
			parsed, err := parseContextTracerConfig(&cfg)
			if scenario == "valid" {
				require.NoError(t, err)
				require.Equal(t, tracerTransportGRPC, parsed.transport, "an empty TRACER_TRANSPORT defaults to gRPC")
				require.Equal(t, ApplicationName, parsed.integrationID)
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

func TestContextTracerGRPCRequiresMTLS(t *testing.T) {
	for _, mode := range []string{"", "mesh", " MESH "} {
		t.Run(mode, func(t *testing.T) {
			cfg := contextTracerTestConfig()
			cfg.TracerTransport, cfg.TracerTLSMode = " GRPC ", mode

			_, err := parseContextTracerConfig(&cfg)
			require.ErrorIs(t, err, constant.ErrTracerContractUnavailable)
			require.ErrorContains(t, err, "TRACER_TLS_MODE=mtls")
		})
	}
}

func TestContextTracerRESTAcceptsAnyTLSMode(t *testing.T) {
	for name, scenario := range map[string]struct {
		mode, baseURL string
		wantErr       bool
	}{
		"empty mode over http":  {mode: "", baseURL: "http://tracer.test:4020"},
		"mesh over http":        {mode: "mesh", baseURL: "http://tracer.test:4020"},
		"mtls over https":       {mode: "mtls", baseURL: "https://tracer.test:4020"},
		"mtls over http":        {mode: "mtls", baseURL: "http://tracer.test:4020", wantErr: true},
		"mesh without a scheme": {mode: "mesh", baseURL: "tracer.test:4020", wantErr: true},
		"mesh with a bad host":  {mode: "mesh", baseURL: "http://", wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := restTracerTestConfig()
			cfg.TracerTLSMode, cfg.TracerBaseURL = scenario.mode, scenario.baseURL

			parsed, err := parseContextTracerConfig(&cfg)
			if scenario.wantErr {
				require.ErrorIs(t, err, constant.ErrTracerContractUnavailable)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tracerTransportREST, parsed.transport)
		})
	}
}

func TestContextTracerIntegrationIDRoster(t *testing.T) {
	for name, scenario := range map[string]struct {
		applicationName string
		wantErr         bool
	}{
		"ledger":           {applicationName: "ledger"},
		"unset":            {applicationName: "  "},
		"outside roster":   {applicationName: "tracer", wantErr: true},
		"case is not lost": {applicationName: "Ledger", wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := contextTracerTestConfig()
			cfg.ApplicationName = scenario.applicationName

			parsed, err := parseContextTracerConfig(&cfg)
			if scenario.wantErr {
				require.ErrorIs(t, err, constant.ErrTracerContractUnavailable)
				require.ErrorContains(t, err, "APPLICATION_NAME")

				return
			}

			require.NoError(t, err)
			require.Equal(t, "ledger", parsed.integrationID)
		})
	}
}

func TestContextTracerDisabledBuildsNothing(t *testing.T) {
	runtime, err := buildContextTracer(&Config{}, nil, nil, testTracerAuthHost, newBootstrapTestLogger(t))
	require.NoError(t, err)
	require.Nil(t, runtime)
}

func TestContextTracerRuntimeBuildsCoordinator(t *testing.T) {
	certs := writeSeamCertFiles(t)

	for name, configure := range map[string]func(*Config){
		"grpc over mtls": func(cfg *Config) {
			cfg.TracerTransport = "grpc"
			cfg.TracerTLSCertFile, cfg.TracerTLSKeyFile, cfg.TracerTLSCAFile = certs.certFile, certs.keyFile, certs.caFile
		},
		"rest over mtls with a client cert": func(cfg *Config) {
			*cfg = restTracerTestConfig()
			cfg.TracerTLSCertFile, cfg.TracerTLSKeyFile, cfg.TracerTLSCAFile = certs.certFile, certs.keyFile, certs.caFile
		},
		"rest over mtls without a client cert": func(cfg *Config) {
			*cfg = restTracerTestConfig()
			cfg.TracerTLSCAFile = certs.caFile
		},
		"rest behind a mesh": func(cfg *Config) {
			*cfg = restTracerTestConfig()
			cfg.TracerTLSMode, cfg.TracerBaseURL = "mesh", "http://tracer.test:4020"
		},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := contextTracerTestConfig()
			configure(&cfg)

			runtime, err := buildContextTracer(&cfg, &libPostgres.Client{}, fixedTokenMinter{}, testTracerAuthHost, newBootstrapTestLogger(t))
			require.NoError(t, err)
			require.NotNil(t, runtime.coordinator)
			require.NoError(t, runtime.coordinator.ValidateActivation(t.Context()))
			closeContextTracerRuntime(t, runtime)

			_, err = buildContextTracer(&cfg, nil, fixedTokenMinter{}, testTracerAuthHost, newBootstrapTestLogger(t))
			require.Error(t, err, "no activation without the official facts store")
		})
	}
}

func TestContextTracerRESTMTLSRequiresVerifiableServer(t *testing.T) {
	certs := writeSeamCertFiles(t)

	for name, scenario := range map[string]struct {
		certFile, keyFile, caFile string
		wantMessage               string
	}{
		"no CA":                {wantMessage: "TRACER_TLS_CA_FILE"},
		"key without cert":     {keyFile: certs.keyFile, caFile: certs.caFile, wantMessage: "TRACER_TLS_CERT_FILE"},
		"cert without key":     {certFile: certs.certFile, caFile: certs.caFile, wantMessage: "TRACER_TLS_KEY_FILE"},
		"unreadable CA bundle": {caFile: certs.caFile + ".missing", wantMessage: "TRACER_TLS_CA_FILE"},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := restTracerTestConfig()
			cfg.TracerTLSCertFile, cfg.TracerTLSKeyFile, cfg.TracerTLSCAFile = scenario.certFile, scenario.keyFile, scenario.caFile

			runtime, err := buildContextTracer(&cfg, &libPostgres.Client{}, fixedTokenMinter{}, testTracerAuthHost, newBootstrapTestLogger(t))
			require.ErrorContains(t, err, scenario.wantMessage)
			require.Nil(t, runtime)
		})
	}
}

func TestContextTracerRESTRefusesDisabledPluginAuth(t *testing.T) {
	cfg := restTracerTestConfig()
	cfg.TracerTLSMode, cfg.TracerBaseURL = "mesh", "http://tracer.test:4020"
	cfg.AuthEnabled = false
	minter := &countingTokenMinter{}

	runtime, err := buildContextTracer(&cfg, &libPostgres.Client{}, minter, testTracerAuthHost, newBootstrapTestLogger(t))
	require.ErrorIs(t, err, constant.ErrTracerContractUnavailable)
	require.ErrorContains(t, err, "PLUGIN_AUTH_ENABLED=false")
	require.Nil(t, runtime)
	require.Zero(t, minter.calls.Load())
}

func TestContextTracerRESTRequiresM2MCredentials(t *testing.T) {
	for name, scenario := range map[string]struct {
		clientID, secret string
		missing          []string
	}{
		"both empty":   {missing: []string{"IDP_M2M_CLIENT_ID", "IDP_M2M_CLIENT_SECRET"}},
		"empty id":     {secret: "ledger-secret", missing: []string{"IDP_M2M_CLIENT_ID"}},
		"empty secret": {clientID: "ledger-client", missing: []string{"IDP_M2M_CLIENT_SECRET"}},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := restTracerTestConfig()
			cfg.TracerTLSMode, cfg.TracerBaseURL = "mesh", "http://tracer.test:4020"
			cfg.IDPM2MClientID, cfg.IDPM2MClientSecret = scenario.clientID, scenario.secret

			runtime, err := buildContextTracer(&cfg, &libPostgres.Client{}, fixedTokenMinter{}, testTracerAuthHost, newBootstrapTestLogger(t))
			require.ErrorIs(t, err, constant.ErrTracerContractUnavailable)
			require.Nil(t, runtime)

			for _, name := range scenario.missing {
				require.Contains(t, err.Error(), name)
			}

			if scenario.secret != "" {
				require.NotContains(t, err.Error(), scenario.secret)
			}
		})
	}
}

func TestContextTracerRESTPrewarmsTokenBestEffort(t *testing.T) {
	for name, minter := range map[string]*countingTokenMinter{
		"mint succeeds": {},
		"mint fails":    {err: errors.New("identity provider unreachable")},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := restTracerTestConfig()
			cfg.TracerTLSMode, cfg.TracerBaseURL = "mesh", "http://tracer.test:4020"

			runtime, err := buildContextTracer(&cfg, &libPostgres.Client{}, minter, testTracerAuthHost, newBootstrapTestLogger(t))
			require.NoError(t, err, "a failed boot-time mint never refuses boot")
			require.NotNil(t, runtime)
			require.Equal(t, int32(1), minter.calls.Load(), "boot mints one token ahead of the first reservation")
		})
	}
}

func TestContextTracerGRPCNeedsNoM2MCredentials(t *testing.T) {
	certs := writeSeamCertFiles(t)
	cfg := contextTracerTestConfig()
	cfg.TracerTransport = "grpc"
	cfg.TracerTLSCertFile, cfg.TracerTLSKeyFile, cfg.TracerTLSCAFile = certs.certFile, certs.keyFile, certs.caFile
	minter := &countingTokenMinter{}

	runtime, err := buildContextTracer(&cfg, &libPostgres.Client{}, minter, testTracerAuthHost, newBootstrapTestLogger(t))
	require.NoError(t, err)
	require.NotNil(t, runtime)
	require.Zero(t, minter.calls.Load(), "gRPC identity is the client certificate, never a token")

	closeContextTracerRuntime(t, runtime)
}

func TestContextTracerGRPCRequiresCertificateMaterial(t *testing.T) {
	cfg := contextTracerTestConfig()
	cfg.TracerTransport = "grpc"

	runtime, err := buildContextTracer(&cfg, &libPostgres.Client{}, nil, testTracerAuthHost, newBootstrapTestLogger(t))
	require.ErrorContains(t, err, "TRACER_TLS_CERT_FILE")
	require.Nil(t, runtime)
}

func TestContextTracerRESTRequiresPluginAuthHost(t *testing.T) {
	for name, host := range map[string]string{"empty": "", "blank": "   "} {
		t.Run(name, func(t *testing.T) {
			cfg := restTracerTestConfig()
			cfg.TracerTLSMode, cfg.TracerBaseURL = "mesh", "http://tracer.test:4020"
			minter := &countingTokenMinter{}

			runtime, err := buildContextTracer(&cfg, &libPostgres.Client{}, minter, host, newBootstrapTestLogger(t))
			require.ErrorIs(t, err, constant.ErrTracerContractUnavailable)
			require.ErrorContains(t, err, "PLUGIN_AUTH_HOST")
			require.Nil(t, runtime)
			require.Zero(t, minter.calls.Load())
		})
	}
}

func TestContextTracerRESTSaaSRefusesCleartextWithoutExplicitMesh(t *testing.T) {
	certs := writeSeamCertFiles(t)

	for name, scenario := range map[string]struct {
		deploymentMode, baseURL, tlsMode string
		refused                          bool
	}{
		"saas http without a mode":  {deploymentMode: "saas", baseURL: "http://tracer.test:4020", refused: true},
		"padded saas http":          {deploymentMode: " SaaS ", baseURL: "http://tracer.test:4020", refused: true},
		"saas http behind a mesh":   {deploymentMode: "saas", baseURL: "http://tracer.test:4020", tlsMode: "mesh"},
		"saas https over mtls":      {deploymentMode: "saas", baseURL: "https://tracer.test:4020", tlsMode: "mtls"},
		"saas https without a mode": {deploymentMode: "saas", baseURL: "https://tracer.test:4020"},
		"byoc http without a mode":  {deploymentMode: "byoc", baseURL: "http://tracer.test:4020"},
		"local http without a mode": {deploymentMode: "local", baseURL: "http://tracer.test:4020"},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := restTracerTestConfig()
			cfg.DeploymentMode, cfg.TracerBaseURL, cfg.TracerTLSMode = scenario.deploymentMode, scenario.baseURL, scenario.tlsMode
			cfg.TracerTLSCAFile = certs.caFile

			runtime, err := buildContextTracer(&cfg, &libPostgres.Client{}, fixedTokenMinter{}, testTracerSecureAuthHost, newBootstrapTestLogger(t))
			if scenario.refused {
				require.ErrorIs(t, err, constant.ErrTracerContractUnavailable)
				require.ErrorContains(t, err, "DEPLOYMENT_MODE=saas")
				require.ErrorContains(t, err, "TRACER_TLS_MODE=mesh")
				require.Nil(t, runtime)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, runtime)
			closeContextTracerRuntime(t, runtime)
		})
	}
}

// TestContextTracerRESTSaaSRefusesCleartextAuthHost pins the SaaS TLS gate on
// the client-credentials hop: the M2M secret reaches plugin-auth at the
// address resolved after service discovery, so an http:// address boots in
// SaaS only behind an explicit mesh.
func TestContextTracerRESTSaaSRefusesCleartextAuthHost(t *testing.T) {
	certs := writeSeamCertFiles(t)

	for name, scenario := range map[string]struct {
		deploymentMode, baseURL, tlsMode, authHost string
		refused                                    bool
	}{
		"saas http auth over mtls":       {deploymentMode: "saas", baseURL: "https://tracer.test:4020", tlsMode: "mtls", authHost: "http://plugin-auth.test:4000", refused: true},
		"saas http auth without a mode":  {deploymentMode: "saas", baseURL: "https://tracer.test:4020", authHost: "http://plugin-auth.test:4000", refused: true},
		"padded saas http auth":          {deploymentMode: " SaaS ", baseURL: "https://tracer.test:4020", authHost: " HTTP://plugin-auth.test:4000 ", refused: true},
		"saas http auth behind a mesh":   {deploymentMode: "saas", baseURL: "http://tracer.test:4020", tlsMode: "mesh", authHost: "http://plugin-auth.test:4000"},
		"saas https auth":                {deploymentMode: "saas", baseURL: "https://tracer.test:4020", tlsMode: "mtls", authHost: testTracerSecureAuthHost},
		"saas https auth without a mode": {deploymentMode: "saas", baseURL: "https://tracer.test:4020", authHost: testTracerSecureAuthHost},
		"byoc http auth":                 {deploymentMode: "byoc", baseURL: "https://tracer.test:4020", authHost: "http://plugin-auth.test:4000"},
		"local http auth":                {deploymentMode: "local", baseURL: "http://tracer.test:4020", authHost: "http://plugin-auth.test:4000"},
		"saas service-discovered http":   {deploymentMode: "saas", baseURL: "https://tracer.test:4020", authHost: "http://10.0.0.7:4000", refused: true},
		"saas service-discovered https":  {deploymentMode: "saas", baseURL: "https://tracer.test:4020", authHost: "https://10.0.0.7:4000"},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := restTracerTestConfig()
			cfg.DeploymentMode, cfg.TracerBaseURL, cfg.TracerTLSMode = scenario.deploymentMode, scenario.baseURL, scenario.tlsMode
			cfg.TracerTLSCAFile = certs.caFile
			minter := &countingTokenMinter{}

			runtime, err := buildContextTracer(&cfg, &libPostgres.Client{}, minter, scenario.authHost, newBootstrapTestLogger(t))
			if scenario.refused {
				require.ErrorIs(t, err, constant.ErrTracerContractUnavailable)
				require.ErrorContains(t, err, "DEPLOYMENT_MODE=saas")
				require.ErrorContains(t, err, "tracer_m2m_auth")
				require.ErrorContains(t, err, "TRACER_TLS_MODE=mesh")
				require.NotContains(t, err.Error(), strings.TrimSpace(scenario.authHost), "the address is never reported")
				require.Nil(t, runtime)
				require.Zero(t, minter.calls.Load(), "no credential is sent before the gate passes")

				return
			}

			require.NoError(t, err)
			require.NotNil(t, runtime)
			closeContextTracerRuntime(t, runtime)
		})
	}
}
