// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strings"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
)

// seamMeshTrustMsg is the boot Warn logged in mesh mode, where the app serves
// the gRPC seam plaintext and the mesh alone verifies the caller.
const seamMeshTrustMsg = "gRPC reservation seam trusts x-tenant-id from the mesh-verified peer: the mesh must enforce STRICT mTLS and restrict the gRPC port to the ledger identity"

// errSeamPostureNilConfig refuses a nil config instead of passing it.
var errSeamPostureNilConfig = errors.New("validate seam transport posture: nil config")

// ValidateSeamTransportPosture gates the transport of the gRPC reservation
// seam. The seam resolves the tenant from x-tenant-id metadata and trusts it
// only because the peer is verified, so a plaintext seam lets any workload
// that reaches the port confirm or release reservations in any tenant.
//
//   - empty TRACER_TLS_MODE + DEPLOYMENT_MODE unset or other than local ⇒
//     error.
//   - DEPLOYMENT_MODE=saas + TRACER_TLS_MODE=mtls + empty
//     TRACER_TLS_CLIENT_ALLOWED_NAMES ⇒ error: any certificate the client CA
//     signed would pass as the ledger.
//   - TRACER_TLS_MODE=mesh (any deployment mode) ⇒ one Warn: the mesh must
//     enforce STRICT mTLS and restrict the port to the ledger identity.
//   - DEPLOYMENT_MODE=local + empty TRACER_TLS_MODE ⇒ plaintext local
//     development.
//
// The plaintext branch reads the raw DEPLOYMENT_MODE rather than
// resolveDeploymentMode: that helper defaults an unset value to local, and
// BYOC deployments leave it unset, so the default would boot a plaintext seam
// in production. Deployment and TLS modes are normalized (case + whitespace)
// so a padded or mixed-case value cannot slip the gate. An unrecognized TLS
// mode is left to buildSeamTLSConfig, which refuses it. MUST run at boot
// before any listener binds.
func ValidateSeamTransportPosture(ctx context.Context, cfg *Config, logger libLog.Logger) error {
	if cfg == nil {
		return errSeamPostureNilConfig
	}

	deploymentMode := resolveDeploymentMode(cfg)
	tlsMode := strings.ToLower(strings.TrimSpace(cfg.TracerTLSMode))

	switch tlsMode {
	case "":
		return plaintextSeamAllowed(cfg.DeploymentMode)
	case tlsModeMTLS:
		if isSaaSMode(deploymentMode) && len(parseClientAllowedNames(cfg.TracerTLSClientAllowedNames)) == 0 {
			return errors.New(
				"DEPLOYMENT_MODE=saas with TRACER_TLS_MODE=mtls: set TRACER_TLS_CLIENT_ALLOWED_NAMES so the gRPC seam accepts only the ledger's client certificate",
			)
		}
	case tlsModeMesh:
		logger.Log(ctx, libLog.LevelWarn, seamMeshTrustMsg, libLog.String("grpc_port", cfg.TracerGRPCPort))
	}

	return nil
}

// plaintextSeamAllowed admits an empty TRACER_TLS_MODE only when
// DEPLOYMENT_MODE is explicitly local; unset and every other value refuse.
func plaintextSeamAllowed(rawDeploymentMode string) error {
	mode := strings.ToLower(strings.TrimSpace(rawDeploymentMode))
	if mode == "local" {
		return nil
	}

	const remedy = `not "local": set TRACER_TLS_MODE to "mtls" or "mesh" (a plaintext seam boots only with DEPLOYMENT_MODE=local): the gRPC reservation seam trusts x-tenant-id only from a verified peer`

	if mode == "" {
		return errors.New("DEPLOYMENT_MODE is unset, " + remedy)
	}

	return fmt.Errorf("DEPLOYMENT_MODE=%q is %s", strings.TrimSpace(rawDeploymentMode), remedy)
}
