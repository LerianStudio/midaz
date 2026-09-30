// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

const (
	wantEmptyModeErr = `not "local": set TRACER_TLS_MODE to "mtls" or "mesh" (a plaintext seam boots only with DEPLOYMENT_MODE=local): the gRPC reservation seam trusts x-tenant-id only from a verified peer`
	wantUnsetModeErr = `DEPLOYMENT_MODE is unset, ` + wantEmptyModeErr
	wantAllowlistErr = `set TRACER_TLS_CLIENT_ALLOWED_NAMES so the gRPC seam accepts only the ledger's client certificate`
	wantMeshWarn     = "gRPC reservation seam trusts x-tenant-id from the mesh-verified peer: the mesh must enforce STRICT mTLS and restrict the gRPC port to the ledger identity"
)

// TestValidateSeamTransportPosture locks the boot gate on the gRPC
// reservation seam's transport: unless DEPLOYMENT_MODE is explicitly local the
// seam must run behind a verified peer (mtls or mesh), saas additionally pins
// mtls to a client identity allowlist, and mesh mode always warns that the
// mesh carries the trust.
func TestValidateSeamTransportPosture(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		deploymentMode string
		tlsMode        string
		allowedNames   string
		wantErr        string
		wantMeshWarn   bool
	}{
		{name: "local with empty mode boots plaintext", deploymentMode: "local"},
		{name: "unset deployment mode with empty mode is refused", deploymentMode: "", wantErr: wantUnsetModeErr},
		{name: "blank deployment mode with empty mode is refused", deploymentMode: "  ", wantErr: wantUnsetModeErr},
		{name: "unset deployment mode with mesh warns", deploymentMode: "", tlsMode: "mesh", wantMeshWarn: true},
		{name: "unset deployment mode with mtls boots", deploymentMode: "", tlsMode: "mtls"},
		{name: "padded mixed-case local is local", deploymentMode: " Local "},
		{name: "local with mtls and no allowlist boots", deploymentMode: "local", tlsMode: "mtls"},
		{name: "local with mesh warns", deploymentMode: "local", tlsMode: "mesh", wantMeshWarn: true},

		{name: "saas with empty mode is refused", deploymentMode: "saas", wantErr: wantEmptyModeErr},
		{name: "byoc with empty mode is refused", deploymentMode: "byoc", wantErr: `DEPLOYMENT_MODE="byoc" is ` + wantEmptyModeErr},
		{name: "undocumented mode with empty mode is refused", deploymentMode: "onprem", wantErr: wantEmptyModeErr},
		{name: "blank TLS mode is empty", deploymentMode: "byoc", tlsMode: "  ", wantErr: wantEmptyModeErr},

		{name: "saas with mtls and no allowlist is refused", deploymentMode: "saas", tlsMode: "mtls", wantErr: wantAllowlistErr},
		{name: "padded saas with blank allowlist is refused", deploymentMode: " SaaS ", tlsMode: " MTLS ", allowedNames: " , ", wantErr: wantAllowlistErr},
		{name: "saas with mtls and an allowlist boots", deploymentMode: "saas", tlsMode: "mtls", allowedNames: "ledger.midaz.svc"},
		{name: "saas with mesh warns", deploymentMode: "saas", tlsMode: "mesh", wantMeshWarn: true},

		{name: "byoc with mtls and no allowlist boots", deploymentMode: "byoc", tlsMode: "mtls"},
		{name: "byoc with mtls and an allowlist boots", deploymentMode: "byoc", tlsMode: "mtls", allowedNames: "ledger"},
		{name: "byoc with mesh warns", deploymentMode: "byoc", tlsMode: " Mesh ", wantMeshWarn: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logger := testutil.NewMockLogger()
			cfg := &Config{
				DeploymentMode:              tt.deploymentMode,
				TracerTLSMode:               tt.tlsMode,
				TracerTLSClientAllowedNames: tt.allowedNames,
				TracerGRPCPort:              DefaultTracerGRPCPort,
			}

			err := ValidateSeamTransportPosture(t.Context(), cfg, logger)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Empty(t, logger.Calls, "a refused boot logs nothing")

				return
			}

			require.NoError(t, err)

			if !tt.wantMeshWarn {
				assert.Empty(t, logger.Calls)

				return
			}

			require.Len(t, logger.Calls, 1)
			assert.Equal(t, "warn", logger.Calls[0].Level)
			assert.Equal(t, wantMeshWarn, logger.Calls[0].Message)
		})
	}
}

// TestValidateSeamTransportPosture_NilConfig proves a nil config is an error,
// never a silent pass.
func TestValidateSeamTransportPosture_NilConfig(t *testing.T) {
	t.Parallel()

	require.Error(t, ValidateSeamTransportPosture(t.Context(), nil, testutil.NewMockLogger()))
}

// TestInitCoreInfra_RefusesPlaintextSeamWhenDeploymentModeUnset proves the
// posture gate is wired into boot: with DEPLOYMENT_MODE unset every earlier
// gate treats the deployment as local, so only the seam posture gate can
// refuse the empty TRACER_TLS_MODE.
func TestInitCoreInfra_RefusesPlaintextSeamWhenDeploymentModeUnset(t *testing.T) {
	t.Parallel()

	cfg := &Config{LogLevel: "error", TracerGRPCPort: DefaultTracerGRPCPort}

	_, _, _, _, err := initCoreInfra(t.Context(), cfg)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "seam transport posture: "+wantUnsetModeErr)
}
