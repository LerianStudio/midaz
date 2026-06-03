//go:build property

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"strings"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
)

// TestProperty_SanitizeOrigins_NeverProducesEmptySegments verifies that for any
// input string, sanitizeOrigins never produces consecutive commas (",,"), leading
// commas, or trailing commas in its output. This is a critical safety invariant
// because empty segments cause Fiber's cors.New() to panic.
func TestProperty_SanitizeOrigins_NeverProducesEmptySegments(t *testing.T) {
	t.Parallel()

	property := func(input string) bool {
		// Bound input length to prevent OOM
		if len(input) > 2000 {
			input = input[:2000]
		}

		result := sanitizeOrigins(input)

		// Empty result is valid (all origins filtered out)
		if result == "" {
			return true
		}

		// Must not contain consecutive commas
		if strings.Contains(result, ",,") {
			t.Logf("Found consecutive commas in output for input %q: %q", input, result)
			return false
		}

		// Must not start with a comma
		if strings.HasPrefix(result, ",") {
			t.Logf("Found leading comma in output for input %q: %q", input, result)
			return false
		}

		// Must not end with a comma
		if strings.HasSuffix(result, ",") {
			t.Logf("Found trailing comma in output for input %q: %q", input, result)
			return false
		}

		// Every segment must be non-empty after splitting
		parts := strings.Split(result, ",")
		for _, p := range parts {
			if strings.TrimSpace(p) == "" {
				t.Logf("Found empty segment in output for input %q: %q", input, result)
				return false
			}
		}

		return true
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err, "Property violated: sanitizeOrigins produced empty segments")
}

// TestProperty_SanitizeCommaSeparated_NeverProducesEmptySegments verifies the
// same no-empty-segment invariant for sanitizeCommaSeparated, which is used for
// HTTP methods and headers. For any input string, the output must never contain
// consecutive commas, leading commas, or trailing commas.
func TestProperty_SanitizeCommaSeparated_NeverProducesEmptySegments(t *testing.T) {
	t.Parallel()

	property := func(input string) bool {
		// Bound input length to prevent OOM
		if len(input) > 2000 {
			input = input[:2000]
		}

		result := sanitizeCommaSeparated(input)

		// Empty result is valid (all segments filtered out)
		if result == "" {
			return true
		}

		// Must not contain consecutive commas
		if strings.Contains(result, ",,") {
			t.Logf("Found consecutive commas in output for input %q: %q", input, result)
			return false
		}

		// Must not start with a comma
		if strings.HasPrefix(result, ",") {
			t.Logf("Found leading comma in output for input %q: %q", input, result)
			return false
		}

		// Must not end with a comma
		if strings.HasSuffix(result, ",") {
			t.Logf("Found trailing comma in output for input %q: %q", input, result)
			return false
		}

		// Every segment must be non-empty after splitting
		parts := strings.Split(result, ",")
		for _, p := range parts {
			if strings.TrimSpace(p) == "" {
				t.Logf("Found empty segment in output for input %q: %q", input, result)
				return false
			}
		}

		return true
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err, "Property violated: sanitizeCommaSeparated produced empty segments")
}

// TestProperty_CORSMiddleware_NeverPanics verifies that for any combination of
// CORSConfig field values, calling CORSMiddleware never panics. This is the core
// safety property: no user-supplied configuration should crash the application.
func TestProperty_CORSMiddleware_NeverPanics(t *testing.T) {
	t.Parallel()

	property := func(origins, methods, headers string) bool {
		// Bound input lengths to prevent OOM
		if len(origins) > 1000 {
			origins = origins[:1000]
		}

		if len(methods) > 500 {
			methods = methods[:500]
		}

		if len(headers) > 500 {
			headers = headers[:500]
		}

		// CORSMiddleware must not panic for any input
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("CORSMiddleware panicked with origins=%q, methods=%q, headers=%q: %v",
					origins, methods, headers, r)
			}
		}()

		handler := CORSMiddleware(CORSConfig{
			AllowedOrigins: origins,
			AllowedMethods: methods,
			AllowedHeaders: headers,
		})

		// Handler must be non-nil
		return handler != nil
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err, "Property violated: CORSMiddleware panicked")
}

// TestProperty_SanitizeOrigins_Idempotent verifies that sanitizeOrigins is
// idempotent: applying it twice produces the same result as applying it once.
// This ensures the function is a stable normalization operation.
func TestProperty_SanitizeOrigins_Idempotent(t *testing.T) {
	t.Parallel()

	property := func(input string) bool {
		if len(input) > 2000 {
			input = input[:2000]
		}

		once := sanitizeOrigins(input)
		twice := sanitizeOrigins(once)

		if once != twice {
			t.Logf("Not idempotent for input %q: once=%q, twice=%q", input, once, twice)
			return false
		}

		return true
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err, "Property violated: sanitizeOrigins is not idempotent")
}

// TestProperty_SanitizeCommaSeparated_Idempotent verifies that
// sanitizeCommaSeparated is idempotent: applying it twice produces the same
// result as applying it once.
func TestProperty_SanitizeCommaSeparated_Idempotent(t *testing.T) {
	t.Parallel()

	property := func(input string) bool {
		if len(input) > 2000 {
			input = input[:2000]
		}

		once := sanitizeCommaSeparated(input)
		twice := sanitizeCommaSeparated(once)

		if once != twice {
			t.Logf("Not idempotent for input %q: once=%q, twice=%q", input, once, twice)
			return false
		}

		return true
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err, "Property violated: sanitizeCommaSeparated is not idempotent")
}

// TestProperty_SanitizeOrigins_WildcardPreserved verifies that the wildcard
// origin "*" is always preserved in the output when present in the input. This
// is a domain-specific invariant: Fiber handles wildcards explicitly, so they
// must survive sanitization.
func TestProperty_SanitizeOrigins_WildcardPreserved(t *testing.T) {
	t.Parallel()

	property := func(prefix, suffix string) bool {
		if len(prefix) > 500 {
			prefix = prefix[:500]
		}

		if len(suffix) > 500 {
			suffix = suffix[:500]
		}

		input := prefix + ",*," + suffix
		result := sanitizeOrigins(input)

		// The wildcard must appear in the output
		parts := strings.Split(result, ",")
		for _, p := range parts {
			if p == "*" {
				return true
			}
		}

		t.Logf("Wildcard not found in output for input %q: %q", input, result)

		return false
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err, "Property violated: sanitizeOrigins did not preserve wildcard")
}

// TestProperty_SanitizeCommaSeparated_PreservesNonEmptySegments verifies that
// every non-empty, non-whitespace segment in the input survives sanitization.
// This ensures that sanitizeCommaSeparated only removes empty/whitespace
// segments and does not silently drop valid values.
func TestProperty_SanitizeCommaSeparated_PreservesNonEmptySegments(t *testing.T) {
	t.Parallel()

	property := func(input string) bool {
		if len(input) > 2000 {
			input = input[:2000]
		}

		result := sanitizeCommaSeparated(input)

		// Compute expected non-empty segments from input
		inputParts := strings.Split(input, ",")
		var expectedParts []string

		for _, p := range inputParts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				expectedParts = append(expectedParts, trimmed)
			}
		}

		expectedResult := strings.Join(expectedParts, ",")

		if result != expectedResult {
			t.Logf("Output mismatch for input %q: got %q, expected %q", input, result, expectedResult)
			return false
		}

		return true
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err, "Property violated: sanitizeCommaSeparated dropped valid segments")
}

// TestProperty_SanitizeOrigins_OutputSubsetOfInput verifies that every origin
// in the output was present in the input. sanitizeOrigins must only filter, not
// generate new values.
func TestProperty_SanitizeOrigins_OutputSubsetOfInput(t *testing.T) {
	t.Parallel()

	property := func(input string) bool {
		if len(input) > 2000 {
			input = input[:2000]
		}

		result := sanitizeOrigins(input)

		if result == "" {
			return true
		}

		// Build set of trimmed input parts
		inputParts := strings.Split(input, ",")
		inputSet := make(map[string]bool)

		for _, p := range inputParts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				inputSet[trimmed] = true
			}
		}

		// Every output part must exist in the input set
		outputParts := strings.Split(result, ",")
		for _, p := range outputParts {
			if !inputSet[p] {
				t.Logf("Output contains %q which was not in input %q", p, input)
				return false
			}
		}

		return true
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err, "Property violated: sanitizeOrigins output is not a subset of input")
}
