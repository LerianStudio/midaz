// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

// resolveDeploymentMode returns cfg.DeploymentMode, falling back to "local"
// when unset. lib-commons v4 SetConfigFromEnvVars does not honor envDefault
// tags, so the default lives in code (matches the convention used by
// ApplyMultiTenantDefaults).
//
// Defaulting to "local" is fail-OPEN by design. Tracer's tightest production
// safeguard — TLS enforcement on outbound Postgres — fires only when
// DeploymentMode == "saas" (see ValidateSaaSTLS). Local dev and BYOC
// (customer-hosted) modes both legitimately leave DEPLOYMENT_MODE unset; a
// fail-CLOSED policy would force every developer onboarding the repo to set
// the variable explicitly even when the safer-by-default behaviour (no
// SaaS-only checks) is what they want. SaaS deployments set
// DEPLOYMENT_MODE=saas explicitly via Helm/Kubernetes config, where omission
// would itself be the misconfiguration.
func resolveDeploymentMode(cfg *Config) string {
	if cfg == nil || cfg.DeploymentMode == "" {
		return "local"
	}

	return cfg.DeploymentMode
}
