// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tracerclient "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
)

// TestBuildTracerReserver_MTLSGuard pins the boot-time fail-fast guard after the
// mTLS rework: identity on the reservation seam is mutual TLS, so the
// discriminator is the transport's security, NOT tenancy. With the integration
// off (TRACER_BASE_URL empty) the reserver is a genuine nil in every tenancy
// mode. With it on and TRACER_TLS_MODE=mtls, missing cert/key/CA material fails
// fast ("reservation seam requires mTLS material"); complete material wires a
// non-nil reserver in BOTH single- and multi-tenant mode (MT no longer gates the
// seam). With TRACER_TLS_MODE=mesh the operator's sidecar terminates mTLS, so no
// cert material is required and the reserver wires non-nil.
func TestBuildTracerReserver_MTLSGuard(t *testing.T) {
	t.Parallel()

	logger := newBootstrapTestLogger(t)

	type certMode int

	const (
		certNone certMode = iota
		certPresent
	)

	tests := []struct {
		name               string
		multiTenantEnabled bool
		tracerBaseURL      string
		tlsMode            string
		certs              certMode
		wantErrContains    string
		wantReserverNil    bool
	}{
		{
			name:            "integration off boots disabled (single-tenant)",
			tracerBaseURL:   "",
			wantReserverNil: true,
		},
		{
			name:               "integration off boots disabled (multi-tenant)",
			multiTenantEnabled: true,
			tracerBaseURL:      "",
			wantReserverNil:    true,
		},
		{
			name:            "mtls with missing material fails fast",
			tracerBaseURL:   "https://tracer:4020",
			tlsMode:         "mtls",
			certs:           certNone,
			wantErrContains: "reservation seam requires mTLS material",
		},
		{
			name:          "mtls with material boots (single-tenant)",
			tracerBaseURL: "https://tracer:4020",
			tlsMode:       "mtls",
			certs:         certPresent,
		},
		{
			name:               "mtls with material boots (multi-tenant)",
			multiTenantEnabled: true,
			tracerBaseURL:      "https://tracer:4020",
			tlsMode:            "mtls",
			certs:              certPresent,
		},
		{
			name:          "mesh boots without certs (single-tenant)",
			tracerBaseURL: "https://tracer:4020",
			tlsMode:       "mesh",
			certs:         certNone,
		},
		{
			name:               "mesh boots without certs (multi-tenant)",
			multiTenantEnabled: true,
			tracerBaseURL:      "https://tracer:4020",
			tlsMode:            "mesh",
			certs:              certNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{
				MultiTenantEnabled: tt.multiTenantEnabled,
				TracerBaseURL:      tt.tracerBaseURL,
				TracerTLSMode:      tt.tlsMode,
			}

			if tt.certs == certPresent {
				files := writeSeamCertFiles(t)
				cfg.TracerTLSCertFile = files.certFile
				cfg.TracerTLSKeyFile = files.keyFile
				cfg.TracerTLSCAFile = files.caFile
			}

			reserver, err := buildTracerReserver(cfg, logger)

			if tt.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContains)
				assert.Nil(t, reserver)

				return
			}

			require.NoError(t, err)

			if tt.wantReserverNil {
				assert.Nil(t, reserver)
			} else {
				assert.NotNil(t, reserver)
			}

			if closer, ok := reserver.(interface{ Close() error }); ok {
				t.Cleanup(func() { _ = closer.Close() })
			}
		})
	}
}

// TestBuildTracerReserver_TransportSelection pins the TRACER_TRANSPORT toggle:
// "grpc" builds the gRPC client, "rest" (and the empty default) build the HTTP
// client, and an unknown value fails fast. The integration is single-tenant in
// every case so the multi-tenant boot guard does not fire. grpc.NewClient is
// lazy, so building the gRPC reserver never blocks on tracer reachability.
func TestBuildTracerReserver_TransportSelection(t *testing.T) {
	t.Parallel()

	logger := newBootstrapTestLogger(t)

	tests := []struct {
		name            string
		transport       string
		wantType        any
		wantErrContains string
	}{
		{
			name:      "grpc selects gRPC client",
			transport: "grpc",
			wantType:  &tracerclient.TracerGRPCClient{},
		},
		{
			name:      "rest selects HTTP client",
			transport: "rest",
			wantType:  &tracerclient.TracerClient{},
		},
		{
			name:      "empty defaults to REST",
			transport: "",
			wantType:  &tracerclient.TracerClient{},
		},
		{
			name:      "case-insensitive GRPC",
			transport: "GRPC",
			wantType:  &tracerclient.TracerGRPCClient{},
		},
		{
			name:            "unknown transport fails fast",
			transport:       "thrift",
			wantErrContains: "invalid TRACER_TRANSPORT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{
				TracerBaseURL:   "http://tracer:4020",
				TracerTransport: tt.transport,
			}

			reserver, err := buildTracerReserver(cfg, logger)

			if tt.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContains)
				assert.Nil(t, reserver)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, reserver)
			assert.IsType(t, tt.wantType, reserver)

			// The gRPC reserver holds a persistent connection; close it so its
			// background goroutines do not trip the package goleak check.
			if closer, ok := reserver.(interface{ Close() error }); ok {
				t.Cleanup(func() { _ = closer.Close() })
			}
		})
	}
}
