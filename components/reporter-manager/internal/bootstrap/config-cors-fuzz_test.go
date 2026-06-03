//go:build fuzz

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"strings"
	"testing"
)

// FuzzValidateProductionCORS_Origins fuzz tests the validateProductionCORS method
// with random origin strings to ensure it never panics regardless of input.
// The method validates comma-separated origin strings and rejects wildcards in production.
func FuzzValidateProductionCORS_Origins(f *testing.F) {
	// Seed corpus: 5 categories per Ring fuzz standards
	// Category 1: Valid inputs
	f.Add("https://app.example.com")
	f.Add("https://app.example.com,https://admin.example.com")
	f.Add("http://localhost:3000")

	// Category 2: Empty/boundary values
	f.Add("")
	f.Add(",")
	f.Add(",,,,")
	f.Add(strings.Repeat("https://a.com,", 100))

	// Category 3: Unicode
	f.Add("\u65e5\u672c\u8a9e.example.com")
	f.Add("https://\u00e9xample.com")

	// Category 4: Invalid formats
	f.Add("*")
	f.Add("*,https://app.example.com")
	f.Add("https://app.example.com,*")
	f.Add("not-a-url")

	// Category 5: Security payloads
	f.Add("<script>alert('xss')</script>")
	f.Add("' OR 1=1 --")
	f.Add("https://evil.com\r\nX-Injected: header")
	f.Add("https://evil.com%0d%0aX-Injected:%20header")

	f.Fuzz(func(t *testing.T, origins string) {
		// Bound input to prevent resource exhaustion
		if len(origins) > 4096 {
			origins = origins[:4096]
		}

		cfg := validManagerConfig()
		cfg.EnvName = "production"
		cfg.EnableTelemetry = true
		cfg.AuthEnabled = true
		cfg.MongoDBPassword = "real-password"
		cfg.RabbitMQPass = "real-password"
		cfg.RedisPassword = "real-password"
		cfg.ObjectStorageSecretKey = "real-secret"
		cfg.CORSAllowedOrigins = origins

		// Must not panic -- this is the core fuzz assertion.
		// Errors are expected for empty or wildcard origins; we only care about no panics.
		errs := cfg.validateProductionCORS(nil)

		// Verify invariants that must always hold:
		// 1. If origins is empty, there must be an error
		if origins == "" && len(errs) == 0 {
			t.Errorf("expected error for empty origins, got none")
		}

		// 2. If origins contains "*", there must be an error
		if strings.Contains(origins, "*") && len(errs) == 0 {
			t.Errorf("expected error for wildcard in origins %q, got none", origins)
		}

		// 3. If origins is non-empty and has no wildcard, no error from CORS validation
		if origins != "" && !strings.Contains(origins, "*") && len(errs) > 0 {
			t.Errorf("unexpected CORS error for valid origins %q: %v", origins, errs)
		}
	})
}

// FuzzValidateProductionCORS_FullValidation fuzz tests the full Config.Validate()
// path in production mode with random CORS origins. This exercises the complete
// validation pipeline including required fields, pool bounds, and CORS checks.
func FuzzValidateProductionCORS_FullValidation(f *testing.F) {
	// Seed corpus: 5 categories
	// Category 1: Valid inputs
	f.Add("https://app.lerian.studio")
	f.Add("https://a.com,https://b.com,https://c.com")

	// Category 2: Empty/boundary values
	f.Add("")
	f.Add(strings.Repeat("x", 512))

	// Category 3: Unicode
	f.Add("https://\u4f8b\u3048.jp")

	// Category 4: Invalid formats
	f.Add("*")
	f.Add("  *  ")
	f.Add("https://ok.com,*,https://also-ok.com")

	// Category 5: Security payloads
	f.Add("javascript:alert(1)")
	f.Add("data:text/html,<h1>evil</h1>")
	f.Add("\x00\x01\x02\x03")

	f.Fuzz(func(t *testing.T, origins string) {
		// Bound input to prevent resource exhaustion
		if len(origins) > 4096 {
			origins = origins[:4096]
		}

		cfg := validManagerConfig()
		cfg.EnvName = "production"
		cfg.EnableTelemetry = true
		cfg.AuthEnabled = true
		cfg.MongoDBPassword = "real-password"
		cfg.RabbitMQPass = "real-password"
		cfg.RedisPassword = "real-password"
		cfg.ObjectStorageSecretKey = "real-secret"
		cfg.CORSAllowedOrigins = origins

		// Must not panic -- errors are expected for invalid origins.
		err := cfg.Validate()

		// Invariant: empty origins in production must always fail
		if origins == "" && err == nil {
			t.Errorf("expected validation error for empty origins in production, got nil")
		}

		// Invariant: wildcard in origins in production must always fail
		if strings.Contains(origins, "*") && err == nil {
			t.Errorf("expected validation error for wildcard origins %q in production, got nil", origins)
		}
	})
}
