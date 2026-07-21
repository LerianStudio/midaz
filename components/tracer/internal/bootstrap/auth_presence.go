// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"strings"

	libLog "github.com/LerianStudio/lib-observability/log"
)

// ValidateAuthPresence is the cross-check that ValidateAuthConfig and
// ValidateAccessManagerConfig lack: each warns when its own mechanism is
// disabled, but neither forces at least one mechanism on. With both off,
// AuthGuard.Protect falls through to APIKeyAuth, which calls c.Next()
// unconditionally when disabled — every /v1 route serves unauthenticated.
//
// Posture mirrors ValidateSaaSTLS: DeploymentMode is the production signal
// (tracer has no ENV_NAME). Any mode that is not "local" is gated — including
// undocumented non-empty strings passed through by resolveDeploymentMode.
// Local/dev keeps Warn-and-continue so an empty .env still boots.
//
// MUST be called from bootstrap after the per-mechanism validators, before
// any connection opens. One function, one call site.
func ValidateAuthPresence(ctx context.Context, cfg *Config, logger libLog.Logger) error {
	if cfg.APIKeyEnabled || cfg.PluginAuthEnabled {
		return nil
	}

	mode := resolveDeploymentMode(cfg)

	// Normalize (case + whitespace) so values like "Local " cannot be
	// mistaken for a gated tier — mirrors ValidateSaaSTLS normalization.
	if strings.EqualFold(strings.TrimSpace(mode), "local") {
		logger.With(
			libLog.String("config", "API_KEY_ENABLED"),
			libLog.String("config_alt", "PLUGIN_AUTH_ENABLED"),
		).Log(ctx, libLog.LevelWarn, "ALL authentication is DISABLED — every /v1 route is open (acceptable for local development only)")

		return nil
	}

	return fmt.Errorf(
		"DEPLOYMENT_MODE=%q requires at least one auth mechanism: set API_KEY_ENABLED=true or PLUGIN_AUTH_ENABLED=true; running non-local without authentication leaves every /v1 route open",
		mode,
	)
}
