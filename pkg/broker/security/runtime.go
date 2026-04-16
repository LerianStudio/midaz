// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package security

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInsecureSkipVerifyInProduction is returned when TLS_INSECURE_SKIP_VERIFY=true in a production environment.
var ErrInsecureSkipVerifyInProduction = errors.New("TLS_INSECURE_SKIP_VERIFY=true is only allowed in explicitly non-production environments")

// ErrTLSRequiredInProduction is returned when TLS is disabled in a production-like environment.
// D6: previously this was only a WARN, which is insufficient — an unencrypted broker transport
// exposes tenant commit intents, balance operations, and audit frames to any attacker on the
// network path. Bootstrap must refuse to start rather than emit a log line operators may miss.
var ErrTLSRequiredInProduction = errors.New("broker TLS must be enabled in production-like environments; set REDPANDA_TLS_ENABLED=true (or AUTHORIZER_REDPANDA_TLS_ENABLED=true)")

// ErrSASLRequiredInProduction is returned when TLS is on but SASL is off in a production-like environment.
// D6: TLS alone authenticates the server to the client; SASL authenticates the client to the server.
// Without SASL, any process that can reach the broker can publish or consume tenant data.
var ErrSASLRequiredInProduction = errors.New("broker SASL must be enabled in production-like environments when TLS is on; set REDPANDA_SASL_ENABLED=true and configure mechanism/username/password")

// RuntimeConfig contains runtime policy inputs used to validate Redpanda security posture.
type RuntimeConfig struct {
	Environment           string
	TLSEnabled            bool
	TLSInsecureSkipVerify bool
	SASLEnabled           bool
}

// IsNonProductionEnvironment returns true when an environment is explicitly non-production.
func IsNonProductionEnvironment(envName string) bool {
	resolved := strings.ToLower(strings.TrimSpace(envName))

	switch resolved {
	case "dev", "development", "local", "test", "testing", "sandbox", "qa", "ci", "staging", "stg", "stage", "uat":
		return true
	default:
		return false
	}
}

// ValidateRuntimeConfig validates Redpanda transport/auth settings for the given environment.
//
// D6 hardening: the "TLS disabled" and "TLS without SASL" checks are now hard errors
// in production-like environments (previously WARN-only). The classification matches
// the one used by the authorizer's validatePublisherSelection so a single ENV_NAME
// flip cannot create inconsistent security postures between the publisher and
// broker-security layers.
func ValidateRuntimeConfig(cfg RuntimeConfig) ([]string, error) {
	const maxWarnings = 3

	nonProduction := IsNonProductionEnvironment(cfg.Environment)
	warnings := make([]string, 0, maxWarnings)

	if !cfg.TLSEnabled && !nonProduction {
		return warnings, fmt.Errorf("environment=%q: %w", cfg.Environment, ErrTLSRequiredInProduction)
	}

	if cfg.TLSEnabled && !cfg.SASLEnabled && !nonProduction {
		return warnings, fmt.Errorf("environment=%q: %w", cfg.Environment, ErrSASLRequiredInProduction)
	}

	if !cfg.TLSInsecureSkipVerify {
		return warnings, nil
	}

	if !nonProduction {
		return warnings, ErrInsecureSkipVerifyInProduction
	}

	warnings = append(warnings, "TLS_INSECURE_SKIP_VERIFY=true: server certificate verification is disabled")

	return warnings, nil
}
