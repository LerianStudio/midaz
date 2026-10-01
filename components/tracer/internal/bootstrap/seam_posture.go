// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
)

// seamIdentity is how the gRPC reservation seam identifies its caller.
type seamIdentity string

// Seam identities in priority order: the first one the configuration enables
// is the one the seam enforces.
const (
	// seamIdentityToken: the ledger's Access Manager application token
	// (PLUGIN_AUTH_ENABLED=true).
	seamIdentityToken seamIdentity = "token"
	// seamIdentityAPIKey: the tracer API key (API_KEY_ENABLED=true).
	seamIdentityAPIKey seamIdentity = "api_key"
	// seamIdentityTransport: the verified mTLS peer (TRACER_TLS_MODE=mtls) or
	// the mesh-verified peer (TRACER_TLS_MODE=mesh).
	seamIdentityTransport seamIdentity = "transport"
	// seamIdentityNone: nothing identifies the caller.
	seamIdentityNone seamIdentity = "none"
)

// Boot Warns of the seam posture gate, one structured line each.
const (
	// seamMeshTrustMsg: transport identity in mesh mode, where the app serves
	// the gRPC seam plaintext and the mesh alone verifies the caller.
	seamMeshTrustMsg = "gRPC reservation seam trusts x-tenant-id from the mesh-verified peer: the mesh must enforce STRICT mTLS and restrict the gRPC port to the ledger identity"
	// seamNoIdentityMsg: no identity outside saas.
	seamNoIdentityMsg = "reservation seam has no caller identity; any workload reaching :4021 can release reservations; set API_KEY_ENABLED, PLUGIN_AUTH_ENABLED, or TRACER_TLS_MODE=mtls|mesh"
	// seamCredentialInClearMsg: a token or API key on a plaintext seam outside
	// saas.
	seamCredentialInClearMsg = "reservation seam credential (Access Manager token or API key) travels in clear; set TRACER_TLS_MODE to server, mtls or mesh"
	// seamAllowlistEmptyLocalMsg: single-tenant token identity without an
	// allowlist under DEPLOYMENT_MODE=local, where boot proceeds but the
	// principal guard admits no sub.
	seamAllowlistEmptyLocalMsg = "TRACER_SEAM_ALLOWED_CLIENTS is empty: the reservation seam refuses every caller; set it to the ledger's Access Manager client id(s), comma-separated (the token sub claim)"
	// seamAllowlistIgnoredMsg: an allowlist set in multi-tenant mode.
	seamAllowlistIgnoredMsg = "TRACER_SEAM_ALLOWED_CLIENTS is ignored in multi-tenant mode: only each tenant's own ledger application client (ledger-m2m-tracer-{tenant}, matching its tenantId claim) is admitted"
	// seamAuthCacheDisabledMsg: token identity without a decision cache
	// outside saas.
	seamAuthCacheDisabledMsg = "AUTH_CACHE_TTL is unset or not a positive duration: every reservation seam call pays an Access Manager round trip; set AUTH_CACHE_TTL (e.g. 60s)"
	// seamNoInversionMsg: token identity without AUTH_M2M_INVERSION_ENABLED,
	// where the Access Manager checks the shared editor role, not the ledger
	// client's own grant, and the principal guard alone binds the caller.
	seamNoInversionMsg = "AUTH_M2M_INVERSION_ENABLED is not true: the Access Manager authorizes application tokens on the reservation seam under a shared editor role instead of the ledger's own client, so only the seam's client allowlist (single-tenant) or ledger client name binding (multi-tenant) restricts who may reserve; set AUTH_M2M_INVERSION_ENABLED=true once the Access Manager supports it"
)

// Process environment read by lib-auth that the seam posture depends on.
const (
	envAuthM2MInversion   = "AUTH_M2M_INVERSION_ENABLED"
	envMultiTenantEnabled = "MULTI_TENANT_ENABLED"
	envAuthCacheTTL       = "AUTH_CACHE_TTL"
)

// errSeamPostureNilConfig refuses a nil config instead of passing it.
var errSeamPostureNilConfig = errors.New("validate seam posture: nil config")

// seamPostureEnv is the lib-auth process environment the gate checks, read
// apart from Config because lib-auth reads it with os.Getenv, not through it.
type seamPostureEnv struct {
	// m2mInversion is AUTH_M2M_INVERSION_ENABLED == "true".
	m2mInversion bool
	// libAuthMultiTenant is the raw MULTI_TENANT_ENABLED value lib-auth sees.
	libAuthMultiTenant string
	// authCacheTTL is the raw AUTH_CACHE_TTL value lib-auth sees.
	authCacheTTL string
}

// ValidateSeamPosture gates the gRPC reservation seam at boot, before any
// listener binds. The seam identity is, in priority order: the Access Manager
// token (PLUGIN_AUTH_ENABLED=true), the API key (API_KEY_ENABLED=true), the
// transport (TRACER_TLS_MODE=mtls|mesh), or none.
//
//   - MULTI_TENANT_ENABLED=true without token identity ⇒ error: the tenant
//     must come from the caller's credential.
//   - token ⇒ AUTH_M2M_INVERSION_ENABLED other than "true" is one Warn in
//     every deployment mode (lib-auth then authorizes application tokens
//     under a shared editor role, so the principal guard alone binds the
//     caller); in single-tenant mode TRACER_SEAM_ALLOWED_CLIENTS must name at least one
//     client unless DEPLOYMENT_MODE=local, where its absence is a Warn; in
//     multi-tenant mode lib-auth must see MULTI_TENANT_ENABLED as exactly
//     "true" (it gates the md-tenant-id the tenant is resolved from), and an
//     allowlist is ignored with a Warn. Without a decision cache
//     (AUTH_CACHE_TTL unset, unparseable or not positive, as lib-auth reads
//     it) every call pays an Access Manager round trip: an error under
//     DEPLOYMENT_MODE=saas, one Warn elsewhere.
//   - token or API key with an empty TRACER_TLS_MODE ⇒ error under
//     DEPLOYMENT_MODE=saas, one Warn elsewhere: the secret travels in clear.
//   - transport ⇒ DEPLOYMENT_MODE=saas + mtls requires
//     TRACER_TLS_CLIENT_ALLOWED_NAMES; mesh logs one Warn.
//   - none ⇒ error under DEPLOYMENT_MODE=saas; elsewhere (BYOC, local or
//     unset) one Warn naming the exposure and the three remedies.
//
// ValidateAuthPresence runs first and already refuses a saas or multi-tenant
// boot with neither API key nor token, so the saas transport/none rules and
// the multi-tenant rule for transport/none are defense in depth that a boot
// never reaches.
//
// DEPLOYMENT_MODE is read raw rather than through resolveDeploymentMode,
// which defaults an unset value to local; an unset mode is never local here.
// Deployment and TLS modes are normalized (case + whitespace).
func ValidateSeamPosture(ctx context.Context, cfg *Config, logger libLog.Logger) error {
	return validateSeamPosture(ctx, cfg, logger, seamPostureEnv{
		m2mInversion:       os.Getenv(envAuthM2MInversion) == "true",
		libAuthMultiTenant: os.Getenv(envMultiTenantEnabled),
		authCacheTTL:       os.Getenv(envAuthCacheTTL),
	})
}

// validateSeamPosture is ValidateSeamPosture over an explicit environment.
func validateSeamPosture(ctx context.Context, cfg *Config, logger libLog.Logger, env seamPostureEnv) error {
	if cfg == nil {
		return errSeamPostureNilConfig
	}

	tlsMode := normalizedTLSMode(cfg)
	if !isKnownTLSMode(tlsMode) {
		return invalidTLSModeError(cfg.TracerTLSMode)
	}

	identity := resolveSeamIdentity(cfg)

	if cfg.MultiTenantEnabled && identity != seamIdentityToken {
		return errors.New(
			"MULTI_TENANT_ENABLED=true requires PLUGIN_AUTH_ENABLED=true on the gRPC reservation seam: the tenant must come from the caller's Access Manager token",
		)
	}

	deploymentMode := strings.ToLower(strings.TrimSpace(cfg.DeploymentMode))

	switch identity {
	case seamIdentityToken:
		warns, err := tokenSeamPosture(cfg, env, deploymentMode)
		if err != nil {
			return err
		}

		cacheWarn, err := authCachePosture(env, deploymentMode)
		if err != nil {
			return err
		}

		clearWarn, err := credentialTransportPosture(tlsMode, deploymentMode)
		if err != nil {
			return err
		}

		warns = append(warns, cacheWarn...)
		logSeamWarns(ctx, logger, cfg, append(warns, clearWarn...))
	case seamIdentityAPIKey:
		clearWarn, err := credentialTransportPosture(tlsMode, deploymentMode)
		if err != nil {
			return err
		}

		logSeamWarns(ctx, logger, cfg, clearWarn)
	case seamIdentityTransport:
		if tlsMode == tlsModeMTLS && isSaaSMode(deploymentMode) && len(parseClientAllowedNames(cfg.TracerTLSClientAllowedNames)) == 0 {
			return errors.New(
				"DEPLOYMENT_MODE=saas with TRACER_TLS_MODE=mtls: set TRACER_TLS_CLIENT_ALLOWED_NAMES so the gRPC seam accepts only the ledger's client certificate",
			)
		}

		if tlsMode == tlsModeMesh {
			logSeamWarns(ctx, logger, cfg, []string{seamMeshTrustMsg})
		}
	case seamIdentityNone:
		if isSaaSMode(deploymentMode) {
			return errors.New(
				`DEPLOYMENT_MODE=saas: the gRPC reservation seam must identify its caller; set PLUGIN_AUTH_ENABLED=true, API_KEY_ENABLED=true, or TRACER_TLS_MODE to "mtls" or "mesh"`,
			)
		}

		logSeamWarns(ctx, logger, cfg, []string{seamNoIdentityMsg})
	}

	return nil
}

// resolveSeamIdentity returns the identity the seam enforces for cfg.
func resolveSeamIdentity(cfg *Config) seamIdentity {
	switch {
	case cfg.PluginAuthEnabled:
		return seamIdentityToken
	case cfg.APIKeyEnabled:
		return seamIdentityAPIKey
	}

	switch normalizedTLSMode(cfg) {
	case tlsModeMTLS, tlsModeMesh:
		return seamIdentityTransport
	default:
		return seamIdentityNone
	}
}

// tokenSeamPosture checks the token identity's own preconditions and returns
// the Warns it raises.
func tokenSeamPosture(cfg *Config, env seamPostureEnv, deploymentMode string) ([]string, error) {
	var warns []string

	if !env.m2mInversion {
		warns = append(warns, seamNoInversionMsg)
	}

	allowlisted := len(parseSeamAllowedClients(cfg.TracerSeamAllowedClients)) > 0

	if cfg.MultiTenantEnabled {
		if env.libAuthMultiTenant != "true" {
			return nil, fmt.Errorf(
				`MULTI_TENANT_ENABLED must be exactly "true" (got %q): lib-auth reads the raw value to propagate the token's tenantId claim as md-tenant-id, the tenant the reservation seam resolves`,
				env.libAuthMultiTenant,
			)
		}

		if allowlisted {
			return append(warns, seamAllowlistIgnoredMsg), nil
		}

		return warns, nil
	}

	if allowlisted {
		return warns, nil
	}

	if deploymentMode == "local" {
		return append(warns, seamAllowlistEmptyLocalMsg), nil
	}

	return nil, errors.New(
		"PLUGIN_AUTH_ENABLED=true in single-tenant mode requires TRACER_SEAM_ALLOWED_CLIENTS outside DEPLOYMENT_MODE=local: set it to the ledger's Access Manager client id(s), comma-separated (the token sub claim)",
	)
}

// authCachePosture refuses a token seam without an Access Manager decision
// cache under saas and returns the round-trip Warn elsewhere. The cache is on
// exactly when lib-auth turns it on: AUTH_CACHE_TTL, trimmed, parses as a
// positive Go duration.
func authCachePosture(env seamPostureEnv, deploymentMode string) ([]string, error) {
	if ttl, err := time.ParseDuration(strings.TrimSpace(env.authCacheTTL)); err == nil && ttl > 0 {
		return nil, nil
	}

	if isSaaSMode(deploymentMode) {
		return nil, errors.New(
			"DEPLOYMENT_MODE=saas with PLUGIN_AUTH_ENABLED=true requires AUTH_CACHE_TTL set to a positive duration (e.g. 60s): without the Access Manager decision cache every reservation seam call pays an Access Manager round trip",
		)
	}

	return []string{seamAuthCacheDisabledMsg}, nil
}

// credentialTransportPosture refuses a secret-bearing identity on a plaintext
// seam under saas and returns the clear-text Warn elsewhere.
func credentialTransportPosture(tlsMode, deploymentMode string) ([]string, error) {
	if tlsMode != "" {
		return nil, nil
	}

	if isSaaSMode(deploymentMode) {
		return nil, errors.New(
			`DEPLOYMENT_MODE=saas: the reservation seam credential (Access Manager token or API key) must not travel in clear; set TRACER_TLS_MODE to "server", "mtls" or "mesh"`,
		)
	}

	return []string{seamCredentialInClearMsg}, nil
}

// logSeamWarns logs each Warn as one structured line carrying the listener.
func logSeamWarns(ctx context.Context, logger libLog.Logger, cfg *Config, warns []string) {
	for _, msg := range warns {
		logger.Log(ctx, libLog.LevelWarn, msg, libLog.String("grpc_port", cfg.TracerGRPCPort))
	}
}

// parseSeamAllowedClients splits TRACER_SEAM_ALLOWED_CLIENTS into trimmed,
// non-empty client ids. Client ids are compared verbatim (case-sensitive).
func parseSeamAllowedClients(raw string) []string {
	var clients []string

	for _, entry := range strings.Split(raw, ",") {
		if client := strings.TrimSpace(entry); client != "" {
			clients = append(clients, client)
		}
	}

	return clients
}
