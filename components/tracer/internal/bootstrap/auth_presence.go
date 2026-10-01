// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"strings"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
)

// Boot Warns of ValidateAuthPresence when neither auth mechanism is enabled.
const (
	authDisabledLocalMsg    = "ALL authentication is DISABLED — every /v1 route is open (acceptable for local development only)"
	authDisabledNonLocalMsg = "ALL authentication is DISABLED — every /v1 route is open; set API_KEY_ENABLED=true or PLUGIN_AUTH_ENABLED=true"
)

// ValidateAuthPresence is the cross-check that ValidateAuthConfig and
// ValidateAccessManagerConfig lack: each warns when its own mechanism is
// disabled, but neither looks at both. With both off, AuthGuard.Protect falls
// through to APIKeyAuth, which calls c.Next() unconditionally when disabled —
// every /v1 route serves unauthenticated.
//
// With neither API_KEY_ENABLED nor PLUGIN_AUTH_ENABLED:
//   - DEPLOYMENT_MODE=saas (raw, normalized) or MULTI_TENANT_ENABLED=true ⇒
//     error: SaaS never serves an open API, and multi-tenant needs the token
//     to know the tenant.
//   - local (or unset, which resolveDeploymentMode treats as local) ⇒ one
//     Warn, so an empty .env still boots.
//   - any other mode (BYOC, undocumented values) ⇒ one Warn naming both
//     variables: the deployment owns its network perimeter.
//
// MUST be called from bootstrap after the per-mechanism validators, before
// any connection opens. One function, one call site.
func ValidateAuthPresence(ctx context.Context, cfg *Config, logger libLog.Logger) error {
	if cfg.APIKeyEnabled || cfg.PluginAuthEnabled {
		return nil
	}

	if isSaaSMode(cfg.DeploymentMode) || cfg.MultiTenantEnabled {
		return fmt.Errorf(
			"DEPLOYMENT_MODE=%q with MULTI_TENANT_ENABLED=%t requires at least one auth mechanism: set API_KEY_ENABLED=true or PLUGIN_AUTH_ENABLED=true; saas and multi-tenant deployments never serve the /v1 routes without authentication",
			strings.TrimSpace(cfg.DeploymentMode), cfg.MultiTenantEnabled,
		)
	}

	msg := authDisabledNonLocalMsg

	// Normalize (case + whitespace) so values like "Local " are local.
	if strings.EqualFold(strings.TrimSpace(resolveDeploymentMode(cfg)), "local") {
		msg = authDisabledLocalMsg
	}

	logger.With(
		libLog.String("config", "API_KEY_ENABLED"),
		libLog.String("config_alt", "PLUGIN_AUTH_ENABLED"),
	).Log(ctx, libLog.LevelWarn, msg)

	return nil
}
