//go:build property

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"strings"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
)

// productionConfig returns a Config with all required fields populated and
// EnvName set to "production", including all production-mandatory settings
// (telemetry, auth, real secrets). Only CORS fields are left empty for the
// caller to fill.
func productionConfig() *Config {
	return &Config{
		EnvName:                     "production",
		ServerAddress:               "0.0.0.0:4005",
		MongoDBHost:                 "mongo.prod.internal",
		MongoDBName:                 "reporter",
		MongoDBPassword:             "real-secret-password",
		MongoMaxPoolSize:            "100",
		MongoMinPoolSize:            "10",
		MongoMaxConnIdleTime:        "60s",
		RabbitMQHost:                "rabbitmq.prod.internal",
		RabbitMQPortAMQP:            "5672",
		RabbitMQUser:                "prod-user",
		RabbitMQPass:                "real-rabbitmq-password",
		RabbitMQGenerateReportQueue: "reporter.generate-report.queue",
		RabbitMQExchange:            "reporter.generate-report.exchange",
		RabbitMQGenerateReportKey:   "reporter.generate-report.key",
		RedisHost:                   "redis.prod.internal:6379",
		RedisPassword:               "real-redis-password",
		ObjectStorageEndpoint:       "https://s3.prod.internal",
		ObjectStorageSecretKey:      "real-s3-secret",
		EnableTelemetry:             true,
		AuthEnabled:                 true,
	}
}

// TestProperty_ValidateProductionCORS_BlocksWildcard verifies that for any
// origins string containing "*", validateProductionCORS always appends an
// error when EnvName is "production". This is the core security invariant:
// wildcard origins are never allowed in production.
func TestProperty_ValidateProductionCORS_BlocksWildcard(t *testing.T) {
	t.Parallel()

	property := func(prefix, suffix string) bool {
		// Bound input lengths to prevent OOM
		if len(prefix) > 500 {
			prefix = prefix[:500]
		}

		if len(suffix) > 500 {
			suffix = suffix[:500]
		}

		cfg := productionConfig()
		cfg.CORSAllowedOrigins = prefix + "*" + suffix

		var errs []string
		errs = cfg.validateProductionCORS(errs)

		// Must always produce an error for wildcard in production
		if len(errs) == 0 {
			t.Logf("No error for origins containing '*': %q", cfg.CORSAllowedOrigins)
			return false
		}

		// The error must mention wildcard
		found := false

		for _, e := range errs {
			if strings.Contains(e, "wildcard") {
				found = true
				break
			}
		}

		if !found {
			t.Logf("Error does not mention wildcard: %v", errs)
			return false
		}

		return true
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err, "Property violated: validateProductionCORS did not block wildcard in production")
}

// TestProperty_ValidateProductionCORS_AllowsNonProduction verifies that for
// any origins string and any EnvName that is not "production", validation
// always passes (returns no additional errors). Non-production environments
// have no CORS restrictions.
func TestProperty_ValidateProductionCORS_AllowsNonProduction(t *testing.T) {
	t.Parallel()

	property := func(origins, envName string) bool {
		// Bound input lengths to prevent OOM
		if len(origins) > 1000 {
			origins = origins[:1000]
		}

		if len(envName) > 100 {
			envName = envName[:100]
		}

		// Skip the "production" environment -- that is the other invariant
		if envName == "production" {
			return true
		}

		cfg := validManagerConfig()
		cfg.EnvName = envName
		cfg.CORSAllowedOrigins = origins

		// validateProductionConfig only runs checks when EnvName == "production"
		// so calling Validate() on a non-production config should not fail on CORS
		var errs []string
		errs = cfg.validateProductionCORS(errs)

		// Since validateProductionCORS is called inside validateProductionConfig
		// which exits early for non-production, the direct call should still
		// not add errors -- but validateProductionCORS itself does not check
		// the env. The check is done in validateProductionConfig.
		// We test through Validate() to verify the full path.

		// Reset errs and use the full validation path
		cfg2 := validManagerConfig()
		cfg2.EnvName = envName
		cfg2.CORSAllowedOrigins = origins

		validationErr := cfg2.Validate()

		// Non-production configs should never fail validation on CORS
		if validationErr != nil {
			errMsg := validationErr.Error()
			if strings.Contains(errMsg, "CORS") {
				t.Logf("CORS error in non-production env %q with origins %q: %s",
					envName, origins, errMsg)
				return false
			}
		}

		return true
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err, "Property violated: validateProductionCORS rejected non-production config")
}

// TestProperty_ValidateProductionCORS_BlocksEmptyOrigins verifies that in
// production, empty CORS origins always produce a validation error. This
// ensures the system never runs in production without explicit origin
// configuration.
func TestProperty_ValidateProductionCORS_BlocksEmptyOrigins(t *testing.T) {
	t.Parallel()

	// This property tests a single known-critical case repeatedly to verify
	// stability: empty origins must always be blocked in production.
	property := func(seed uint8) bool {
		cfg := productionConfig()
		cfg.CORSAllowedOrigins = ""

		var errs []string
		errs = cfg.validateProductionCORS(errs)

		if len(errs) == 0 {
			t.Log("No error for empty origins in production")
			return false
		}

		// The error must mention empty origins
		found := false

		for _, e := range errs {
			if strings.Contains(e, "must not be empty") {
				found = true
				break
			}
		}

		if !found {
			t.Logf("Error does not mention empty origins: %v", errs)
			return false
		}

		return true
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err, "Property violated: validateProductionCORS allowed empty origins in production")
}

// TestProperty_ValidateProductionCORS_AcceptsValidOrigins verifies that in
// production, any origins string that does not contain "*" and is not empty
// passes CORS validation. This ensures that legitimate production origins
// are never incorrectly blocked.
func TestProperty_ValidateProductionCORS_AcceptsValidOrigins(t *testing.T) {
	t.Parallel()

	property := func(origin string) bool {
		// Bound input length
		if len(origin) > 500 {
			origin = origin[:500]
		}

		// Skip empty and wildcard-containing origins (covered by other properties)
		if origin == "" || strings.Contains(origin, "*") {
			return true
		}

		cfg := productionConfig()
		cfg.CORSAllowedOrigins = origin

		var errs []string
		errs = cfg.validateProductionCORS(errs)

		if len(errs) > 0 {
			t.Logf("Unexpected error for valid origins %q in production: %v", origin, errs)
			return false
		}

		return true
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err, "Property violated: validateProductionCORS rejected valid origins")
}

// TestProperty_ValidateProductionCORS_NeverPanics verifies that for any
// combination of origins strings, calling validateProductionCORS never panics.
// This is a baseline safety property for all validation functions.
func TestProperty_ValidateProductionCORS_NeverPanics(t *testing.T) {
	t.Parallel()

	property := func(origins string) bool {
		if len(origins) > 2000 {
			origins = origins[:2000]
		}

		defer func() {
			if r := recover(); r != nil {
				t.Errorf("validateProductionCORS panicked with origins=%q: %v", origins, r)
			}
		}()

		cfg := productionConfig()
		cfg.CORSAllowedOrigins = origins

		var errs []string
		_ = cfg.validateProductionCORS(errs)

		return true
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err, "Property violated: validateProductionCORS panicked")
}
