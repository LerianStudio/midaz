// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package security

import (
	"errors"
	"strings"
)

// ErrInsecureSkipVerifyInProduction is returned when TLS_INSECURE_SKIP_VERIFY=true in a production environment.
var ErrInsecureSkipVerifyInProduction = errors.New("TLS_INSECURE_SKIP_VERIFY=true is only allowed in explicitly non-production environments")

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
func ValidateRuntimeConfig(cfg RuntimeConfig) ([]string, error) {
	const maxWarnings = 3

	nonProduction := IsNonProductionEnvironment(cfg.Environment)
	warnings := make([]string, 0, maxWarnings)

	if !cfg.TLSEnabled && !nonProduction {
		warnings = append(warnings, "TLS is disabled in a production-like environment")
	}

	if cfg.TLSEnabled && !cfg.SASLEnabled && !nonProduction {
		warnings = append(warnings, "TLS is enabled without SASL authentication in a production-like environment")
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
