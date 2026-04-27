// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"fmt"
	"net/url"
	"strings"
)

// detectMongoTLS returns true if the MongoDB URI has TLS enabled.
// TLS is enabled when:
//   - URI scheme is "mongodb+srv" (always uses TLS)
//   - Query parameter "tls=true" is present (case-insensitive key and value)
//   - Query parameter "ssl=true" is present (legacy, case-insensitive key and value)
//
// Returns error for malformed URI syntax.
func detectMongoTLS(uri string) (bool, error) {
	if uri == "" {
		return false, nil
	}

	parsed, err := url.Parse(uri)
	if err != nil {
		return false, fmt.Errorf("invalid MongoDB URI: %w", err)
	}

	// mongodb+srv:// always uses TLS
	if strings.ToLower(parsed.Scheme) == "mongodb+srv" {
		return true, nil
	}

	// Check query parameters for tls=true or ssl=true (case-insensitive)
	// url.Query() preserves original case, so we need to iterate all keys
	query := parsed.Query()

	for key, values := range query {
		lowerKey := strings.ToLower(key)
		if lowerKey == "tls" || lowerKey == "ssl" {
			for _, v := range values {
				if strings.EqualFold(v, "true") {
					return true, nil
				}
			}
		}
	}

	return false, nil
}

// TLSValidationResult contains the TLS validation status for a dependency.
type TLSValidationResult struct {
	Name       string
	TLSEnabled bool
}

// DeploymentMode constants define the valid deployment modes for TLS enforcement.
const (
	// DeploymentModeSaaS is the Lerian-hosted multi-tenant mode where TLS is MANDATORY.
	DeploymentModeSaaS = "saas"
	// DeploymentModeBYOC is the customer-hosted mode where TLS is recommended but not enforced.
	DeploymentModeBYOC = "byoc"
	// DeploymentModeLocal is the developer workstation mode where TLS is optional.
	DeploymentModeLocal = "local"

	// DefaultDeploymentMode is the deployment mode used when none is specified.
	// Defaults to "local" for safe developer experience.
	DefaultDeploymentMode = DeploymentModeLocal
)

// ResolveDeploymentMode normalizes the deployment mode string.
// Returns DefaultDeploymentMode if input is empty or whitespace.
// Otherwise returns the lowercased input for consistent comparison.
func ResolveDeploymentMode(mode string) string {
	trimmed := strings.TrimSpace(mode)
	if trimmed == "" {
		return DefaultDeploymentMode
	}

	return strings.ToLower(trimmed)
}

// ValidateSaaSTLS enforces TLS for all dependencies when DEPLOYMENT_MODE=saas.
// This is a centralized function called from bootstrap BEFORE any connection is opened.
//
// Deployment mode semantics:
//   - "saas": TLS MANDATORY for ALL dependencies - hard fail at startup if any lacks TLS
//   - "byoc": TLS recommended but not hard-enforced (returns nil, caller may log warning)
//   - "local" or unset: TLS optional - no enforcement
//
// Returns an error ONLY when DEPLOYMENT_MODE=saas and any dependency lacks TLS.
// The error includes the specific dependency name(s) that failed validation.
func ValidateSaaSTLS(deploymentMode string, dependencies []TLSValidationResult) error {
	lowerMode := strings.ToLower(deploymentMode)

	// Only enforce TLS in SaaS mode
	if lowerMode != DeploymentModeSaaS {
		return nil
	}

	// Collect all dependencies without TLS
	var insecureDeps []string

	for _, dep := range dependencies {
		if !dep.TLSEnabled {
			insecureDeps = append(insecureDeps, dep.Name)
		}
	}

	if len(insecureDeps) > 0 {
		return fmt.Errorf("DEPLOYMENT_MODE=saas: TLS required for %s but not configured",
			strings.Join(insecureDeps, ", "))
	}

	return nil
}

// IsTLSEnforcementRequired returns true if the deployment mode requires TLS enforcement.
func IsTLSEnforcementRequired(deploymentMode string) bool {
	return strings.ToLower(deploymentMode) == DeploymentModeSaaS
}

// IsTLSRecommended returns true if TLS is recommended (but not required) for the deployment mode.
func IsTLSRecommended(deploymentMode string) bool {
	return strings.ToLower(deploymentMode) == DeploymentModeBYOC
}
