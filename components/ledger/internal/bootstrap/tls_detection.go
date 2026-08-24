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
	// Only enforce TLS in SaaS mode. Normalizing through ResolveDeploymentMode is what
	// keeps a padded value like " saas " from slipping past plain string equality and
	// silently disabling every gate below.
	if ResolveDeploymentMode(deploymentMode) != DeploymentModeSaaS {
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
	return ResolveDeploymentMode(deploymentMode) == DeploymentModeSaaS
}

// IsTLSRecommended returns true if TLS is recommended (but not required) for the deployment mode.
func IsTLSRecommended(deploymentMode string) bool {
	return ResolveDeploymentMode(deploymentMode) == DeploymentModeBYOC
}

// idpSchemeIsCleartext reports whether idpHost carries an explicit http:// scheme —
// the one case where the RI declaration publisher would dial the IdP in cleartext
// and ship the M2M client_credentials grant plus the resulting bearer token
// unencrypted. An https:// host is secure. A scheme-less or otherwise malformed
// host is rejected by the publisher's own config validation (lib-auth requires an
// absolute http(s):// URL) and fails open with no dial, so it is deliberately NOT
// treated as a cleartext dial here. Scheme comparison is case-insensitive.
func idpSchemeIsCleartext(idpHost string) bool {
	parsed, err := url.Parse(strings.TrimSpace(idpHost))
	if err != nil {
		return false
	}

	// A non-empty Host is required: opaque forms like "http:identity" or
	// "http:/identity" parse with an http scheme but no authority. The publisher
	// never dials those (lib-auth rejects them and fails open), so they are not a
	// cleartext dial and must not trip the gate.
	return strings.EqualFold(parsed.Scheme, "http") && parsed.Host != ""
}

// ValidateSaaSDeclarationTLS extends the SaaS TLS gate to the Responsibility-
// Inversion (RI) permission-declaration publisher's IdP dial. The publisher sends
// the M2M client_credentials grant and the resulting bearer token to IDP_HOST, so
// a Lerian-hosted deployment must not reach the IdP in cleartext — the same rule
// Postgres, Mongo, Redis and RabbitMQ already answer to.
//
// It is a no-op unless RI declaration is ENABLED: a disabled publisher opens no IdP
// connection at all. An empty or scheme-less IDP_HOST is likewise not this gate's
// concern — the publisher's own pre-flight Warn and lib-auth's config validation
// already cover incomplete/malformed config and fail open (no dial happens); a
// missing host is not an insecure dial. Only an explicit http:// scheme trips the
// gate. BYOC and local deployments keep their plaintext IdP.
//
// Like the Postgres/Mongo/Redis/RabbitMQ and streaming SaaS TLS gates, this does
// NOT honor the ALLOW_INSECURE_TLS escape hatch: that escape lives in the
// connection constructors, not in the SaaS TLS gate, so mirroring the existing
// gate means no escape here.
//
// Delegating to ValidateSaaSTLS keeps the error sentence byte-identical to its
// siblings; the suffix names the knob that fixes it. IDP_HOST is a plain host URL,
// never a credential — the M2M secret is not part of this value and is never
// referenced here, so nothing sensitive can reach the error message.
func ValidateSaaSDeclarationTLS(deploymentMode string, declarationEnabled bool, idpHost string) error {
	if !declarationEnabled || !idpSchemeIsCleartext(idpHost) {
		return nil
	}

	if err := ValidateSaaSTLS(deploymentMode, []TLSValidationResult{{
		Name:       "idp_declaration",
		TLSEnabled: false,
	}}); err != nil {
		return fmt.Errorf("%w (set IDP_HOST to an https:// URL)", err)
	}

	return nil
}

// ValidateSaaSStreamingTLS extends the SaaS TLS gate to the lib-streaming Kafka
// broker dial. It is separate from ValidateSaaSTLS because STREAMING_TLS_* is
// lib-streaming's own env contract and never lands on the midaz Config (binding
// it there is explicitly forbidden): the flag only exists once
// libStreaming.LoadConfig has run, so BuildStreamingEmitter resolves it and
// passes it in.
//
// The rule is the one every other managed dependency already answers to — in
// SaaS mode the transport is encrypted or the service does not boot. The check
// is reached only when streaming is ENABLED, because a disabled producer opens
// no broker connection at all. BYOC and local deployments keep their plaintext
// brokers.
//
// Delegating to ValidateSaaSTLS keeps the error sentence byte-identical to the
// Postgres/Mongo/Redis/RabbitMQ siblings; the suffix adds the one thing a
// dependency name cannot carry, the knob that fixes it.
func ValidateSaaSStreamingTLS(deploymentMode string, streamingTLSEnabled bool) error {
	if err := ValidateSaaSTLS(deploymentMode, []TLSValidationResult{{
		Name:       "streaming",
		TLSEnabled: streamingTLSEnabled,
	}}); err != nil {
		return fmt.Errorf("%w (set STREAMING_TLS_ENABLED=true)", err)
	}

	return nil
}
