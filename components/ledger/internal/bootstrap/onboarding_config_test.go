// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"reflect"
	"strings"
	"testing"
	"testing/quick"

	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		prefixed string
		fallback string
		want     string
	}{
		{
			name:     "prefixed non-empty returns prefixed",
			prefixed: "prefixed-value",
			fallback: "fallback-value",
			want:     "prefixed-value",
		},
		{
			name:     "prefixed empty returns fallback",
			prefixed: "",
			fallback: "fallback-value",
			want:     "fallback-value",
		},
		{
			name:     "prefixed non-empty with empty fallback returns prefixed",
			prefixed: "prefixed-value",
			fallback: "",
			want:     "prefixed-value",
		},
		{
			name:     "both empty returns empty",
			prefixed: "",
			fallback: "",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := utils.EnvFallback(tt.prefixed, tt.fallback)

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConfig_RedisEnvTags_UniquePerField(t *testing.T) {
	t.Parallel()

	configType := reflect.TypeFor[Config]()

	tests := []struct {
		name        string
		fieldName   string
		expectedTag string
	}{
		{
			name:        "RedisDB field has env tag REDIS_DB",
			fieldName:   "RedisDB",
			expectedTag: "REDIS_DB",
		},
		{
			name:        "RedisProtocol field has env tag REDIS_PROTOCOL",
			fieldName:   "RedisProtocol",
			expectedTag: "REDIS_PROTOCOL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			field, found := configType.FieldByName(tt.fieldName)
			require.True(t, found, "field %s must exist in Config struct", tt.fieldName)

			envTag := field.Tag.Get("env")
			assert.Equal(t, tt.expectedTag, envTag,
				"Config.%s must have env:\"%s\" but has env:\"%s\"",
				tt.fieldName, tt.expectedTag, envTag)
		})
	}
}

func TestEnvFallbackInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		prefixed int
		fallback int
		want     int
	}{
		{
			name:     "prefixed non-zero returns prefixed",
			prefixed: 10,
			fallback: 5,
			want:     10,
		},
		{
			name:     "prefixed zero returns fallback",
			prefixed: 0,
			fallback: 5,
			want:     5,
		},
		{
			name:     "prefixed non-zero with zero fallback returns prefixed",
			prefixed: 10,
			fallback: 0,
			want:     10,
		},
		{
			name:     "both zero returns zero",
			prefixed: 0,
			fallback: 0,
			want:     0,
		},
		{
			name:     "negative prefixed returns prefixed",
			prefixed: -5,
			fallback: 10,
			want:     -5,
		},
		{
			name:     "negative fallback used when prefixed is zero",
			prefixed: 0,
			fallback: -10,
			want:     -10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := utils.EnvFallbackInt(tt.prefixed, tt.fallback)

			assert.Equal(t, tt.want, got)
		})
	}
}

// TestOnboardingConfig_EnvTagsUnique verifies the invariant that no two fields
// in the onboarding Config struct share the same env tag value. Duplicate env tags cause
// silent configuration bugs where one field's value overwrites another.
func TestOnboardingConfig_EnvTagsUnique(t *testing.T) {
	t.Parallel()

	configType := reflect.TypeFor[Config]()
	seen := make(map[string]string) // env tag value -> field name

	for i := range configType.NumField() {
		field := configType.Field(i)

		envTag := field.Tag.Get("env")
		if envTag == "" {
			continue
		}

		if existingField, exists := seen[envTag]; exists {
			t.Fatalf("duplicate env tag %q found on fields %s and %s",
				envTag, existingField, field.Name)
		}

		seen[envTag] = field.Name
	}
}

// TestProperty_UtilsEnvFallback_Deterministic verifies the invariant that utils.EnvFallback is
// deterministic: non-empty prefixed always returns prefixed, empty prefixed always
// returns fallback. This holds for any arbitrary string input combination.
func TestProperty_UtilsEnvFallback_Deterministic(t *testing.T) {
	t.Parallel()

	property := func(prefixed, fallback string) bool {
		result := utils.EnvFallback(prefixed, fallback)

		if prefixed != "" {
			return result == prefixed
		}

		return result == fallback
	}

	cfg := &quick.Config{MaxCount: 1000}

	err := quick.Check(property, cfg)
	require.NoError(t, err, "utils.EnvFallback must be deterministic: non-empty prefixed returns prefixed, empty returns fallback")
}

// TestProperty_UtilsEnvFallbackInt_Deterministic verifies the invariant that utils.EnvFallbackInt is
// deterministic: non-zero prefixed always returns prefixed, zero prefixed always
// returns fallback. This holds for any arbitrary int input combination.
func TestProperty_UtilsEnvFallbackInt_Deterministic(t *testing.T) {
	t.Parallel()

	property := func(prefixed, fallback int) bool {
		result := utils.EnvFallbackInt(prefixed, fallback)

		if prefixed != 0 {
			return result == prefixed
		}

		return result == fallback
	}

	cfg := &quick.Config{MaxCount: 1000}

	err := quick.Check(property, cfg)
	require.NoError(t, err, "utils.EnvFallbackInt must be deterministic: non-zero prefixed returns prefixed, zero returns fallback")
}

// TestProperty_UtilsEnvFallback_Idempotent verifies that applying envFallback with the same
// arguments always produces the same result (referential transparency).
func TestProperty_UtilsEnvFallback_Idempotent(t *testing.T) {
	t.Parallel()

	property := func(prefixed, fallback string) bool {
		first := utils.EnvFallback(prefixed, fallback)
		second := utils.EnvFallback(prefixed, fallback)

		return first == second
	}

	cfg := &quick.Config{MaxCount: 1000}

	err := quick.Check(property, cfg)
	require.NoError(t, err, "utils.EnvFallback must be idempotent: same inputs always produce same output")
}

// TestProperty_UtilsEnvFallbackInt_Idempotent verifies that applying envFallbackInt with the
// same arguments always produces the same result (referential transparency).
func TestProperty_UtilsEnvFallbackInt_Idempotent(t *testing.T) {
	t.Parallel()

	property := func(prefixed, fallback int) bool {
		first := utils.EnvFallbackInt(prefixed, fallback)
		second := utils.EnvFallbackInt(prefixed, fallback)

		return first == second
	}

	cfg := &quick.Config{MaxCount: 1000}

	err := quick.Check(property, cfg)
	require.NoError(t, err, "utils.EnvFallbackInt must be idempotent: same inputs always produce same output")
}

// TestProperty_UtilsEnvFallback_ResultIsInput verifies the invariant that the result of
// utils.EnvFallback is always one of the two inputs -- it never fabricates a new value.
func TestProperty_UtilsEnvFallback_ResultIsInput(t *testing.T) {
	t.Parallel()

	property := func(prefixed, fallback string) bool {
		result := utils.EnvFallback(prefixed, fallback)

		return result == prefixed || result == fallback
	}

	cfg := &quick.Config{MaxCount: 1000}

	err := quick.Check(property, cfg)
	require.NoError(t, err, "envFallback result must always be one of the two inputs")
}

// TestProperty_UtilsEnvFallbackInt_ResultIsInput verifies the invariant that the result of
// utils.EnvFallbackInt is always one of the two inputs -- it never fabricates a new value.
func TestProperty_UtilsEnvFallbackInt_ResultIsInput(t *testing.T) {
	t.Parallel()

	property := func(prefixed, fallback int) bool {
		result := utils.EnvFallbackInt(prefixed, fallback)

		return result == prefixed || result == fallback
	}

	cfg := &quick.Config{MaxCount: 1000}

	err := quick.Check(property, cfg)
	require.NoError(t, err, "envFallbackInt result must always be one of the two inputs")
}

// FuzzEnvFallback_Inputs fuzzes envFallback to verify it never panics and
// correctly returns prefixed when non-empty, fallback otherwise.
func FuzzEnvFallback_Inputs(f *testing.F) {
	// Seed corpus: valid, empty, boundary, unicode, security categories
	f.Add("prefixed-value", "fallback-value")         // valid: both non-empty
	f.Add("", "fallback-value")                       // empty: prefixed empty
	f.Add("prefixed-value", "")                       // empty: fallback empty
	f.Add("", "")                                     // empty: both empty
	f.Add("\u00e9\u00e0\u00fc\u00f1", "fallback")     // unicode: accented chars
	f.Add("a", "b")                                   // boundary: single char
	f.Add("' OR 1=1 --", "<script>alert(1)</script>") // security: injection payloads
	f.Add(strings.Repeat("x", 1000), "short")         // boundary: long string

	f.Fuzz(func(t *testing.T, prefixed, fallback string) {
		// Bound input lengths to prevent OOM
		if len(prefixed) > 4096 || len(fallback) > 4096 {
			return
		}

		// Must not panic
		result := utils.EnvFallback(prefixed, fallback)

		// Invariant: if prefixed is non-empty, result == prefixed; otherwise result == fallback
		if prefixed != "" {
			assert.Equal(t, prefixed, result,
				"when prefixed is non-empty, result must equal prefixed")
		} else {
			assert.Equal(t, fallback, result,
				"when prefixed is empty, result must equal fallback")
		}
	})
}

// FuzzEnvFallbackInt_Inputs fuzzes envFallbackInt to verify it never panics and
// correctly returns prefixed when non-zero, fallback otherwise.
func FuzzEnvFallbackInt_Inputs(f *testing.F) {
	// Seed corpus: valid, zero, boundary, negative categories
	f.Add(10, 5)          // valid: both positive
	f.Add(0, 5)           // zero: prefixed zero
	f.Add(10, 0)          // zero: fallback zero
	f.Add(0, 0)           // zero: both zero
	f.Add(-1, 100)        // boundary: negative prefixed
	f.Add(0, -1)          // boundary: negative fallback
	f.Add(2147483647, 0)  // boundary: max int32
	f.Add(-2147483648, 0) // boundary: min int32

	f.Fuzz(func(t *testing.T, prefixed, fallback int) {
		// Must not panic
		result := utils.EnvFallbackInt(prefixed, fallback)

		// Invariant: if prefixed is non-zero, result == prefixed; otherwise result == fallback
		if prefixed != 0 {
			assert.Equal(t, prefixed, result,
				"when prefixed is non-zero, result must equal prefixed")
		} else {
			assert.Equal(t, fallback, result,
				"when prefixed is zero, result must equal fallback")
		}
	})
}
