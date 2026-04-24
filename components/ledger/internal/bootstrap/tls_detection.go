// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"fmt"
	"net/url"
	"strings"
)

// detectPostgresTLS returns true if the PostgreSQL DSN has TLS enabled.
// PostgreSQL DSNs use key=value format with sslmode parameter.
// Returns false if sslmode is "disable" or not set.
func detectPostgresTLS(dsn string) bool {
	if dsn == "" {
		return false
	}

	// PostgreSQL DSN format: "host=x user=y password=z dbname=d port=p sslmode=s"
	// Parse as space-separated key=value pairs
	params := make(map[string]string)

	for _, part := range strings.Fields(dsn) {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			params[strings.ToLower(kv[0])] = kv[1]
		}
	}

	sslmode, exists := params["sslmode"]
	if !exists {
		return false
	}

	// sslmode values that indicate TLS is enabled:
	// require, verify-ca, verify-full
	// sslmode=disable means no TLS
	// sslmode=allow/prefer means TLS is optional (we report as false for determinism)
	switch strings.ToLower(sslmode) {
	case "require", "verify-ca", "verify-full":
		return true
	default:
		return false
	}
}

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

// detectRedisTLS returns true if Redis TLS is enabled.
// TLS is enabled when:
//   - tlsEnabled config flag is true
//   - Host string uses "rediss://" scheme
//
// This function does not return an error as Redis host format is simpler.
func detectRedisTLS(host string, tlsEnabled bool) bool {
	if tlsEnabled {
		return true
	}

	if host == "" {
		return false
	}

	// Check for rediss:// scheme (note: double 's')
	return strings.HasPrefix(strings.ToLower(host), "rediss://")
}

// detectAMQPTLS returns true if the AMQP URI uses TLS.
// TLS is enabled when the URI scheme is "amqps" (instead of "amqp").
// Returns error for malformed URI syntax.
func detectAMQPTLS(uri string) (bool, error) {
	if uri == "" {
		return false, nil
	}

	parsed, err := url.Parse(uri)
	if err != nil {
		return false, fmt.Errorf("invalid AMQP URI: %w", err)
	}

	return strings.ToLower(parsed.Scheme) == "amqps", nil
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

	// Only enforce TLS in SaaS mode - this is the Monetarie incident prevention
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
