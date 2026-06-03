// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetEnvOrDefault(t *testing.T) {
	// Note: Cannot use t.Parallel() because subtests use t.Setenv

	tests := []struct {
		name         string
		envKey       string
		envValue     string
		defaultValue string
		expected     string
		setEnv       bool
	}{
		{
			name:         "Environment variable set",
			envKey:       "TEST_GET_ENV_VAR",
			envValue:     "test_value",
			defaultValue: "default",
			expected:     "test_value",
			setEnv:       true,
		},
		{
			name:         "Environment variable not set",
			envKey:       "TEST_GET_ENV_VAR_NOTSET",
			envValue:     "",
			defaultValue: "default_value",
			expected:     "default_value",
			setEnv:       false,
		},
		{
			name:         "Environment variable empty",
			envKey:       "TEST_GET_ENV_VAR_EMPTY",
			envValue:     "",
			defaultValue: "default_value",
			expected:     "default_value",
			setEnv:       true,
		},
		{
			name:         "Environment variable whitespace only",
			envKey:       "TEST_GET_ENV_VAR_WHITESPACE",
			envValue:     "   ",
			defaultValue: "default_value",
			expected:     "default_value",
			setEnv:       true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// Note: Cannot use t.Parallel() because t.Setenv is used

			// Ensure clean state before test
			t.Setenv(tt.envKey, "")

			if tt.setEnv {
				t.Setenv(tt.envKey, tt.envValue)
			}

			result := GetEnvOrDefault(tt.envKey, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetenvBoolOrDefault(t *testing.T) {
	// Note: Cannot use t.Parallel() because subtests use t.Setenv

	tests := []struct {
		name         string
		envKey       string
		envValue     string
		defaultValue bool
		expected     bool
		setEnv       bool
	}{
		{
			name:         "True value",
			envKey:       "TEST_BOOL_TRUE",
			envValue:     "true",
			defaultValue: false,
			expected:     true,
			setEnv:       true,
		},
		{
			name:         "False value",
			envKey:       "TEST_BOOL_FALSE",
			envValue:     "false",
			defaultValue: true,
			expected:     false,
			setEnv:       true,
		},
		{
			name:         "1 as true",
			envKey:       "TEST_BOOL_ONE",
			envValue:     "1",
			defaultValue: false,
			expected:     true,
			setEnv:       true,
		},
		{
			name:         "0 as false",
			envKey:       "TEST_BOOL_ZERO",
			envValue:     "0",
			defaultValue: true,
			expected:     false,
			setEnv:       true,
		},
		{
			name:         "Invalid value - returns default",
			envKey:       "TEST_BOOL_INVALID",
			envValue:     "invalid",
			defaultValue: true,
			expected:     true,
			setEnv:       true,
		},
		{
			name:         "Not set - returns default true",
			envKey:       "TEST_BOOL_NOTSET_TRUE",
			envValue:     "",
			defaultValue: true,
			expected:     true,
			setEnv:       false,
		},
		{
			name:         "Not set - returns default false",
			envKey:       "TEST_BOOL_NOTSET_FALSE",
			envValue:     "",
			defaultValue: false,
			expected:     false,
			setEnv:       false,
		},
		{
			name:         "TRUE uppercase",
			envKey:       "TEST_BOOL_UPPERCASE",
			envValue:     "TRUE",
			defaultValue: false,
			expected:     true,
			setEnv:       true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// Note: Cannot use t.Parallel() because t.Setenv is used

			// Ensure clean state before test
			t.Setenv(tt.envKey, "")

			if tt.setEnv {
				t.Setenv(tt.envKey, tt.envValue)
			}

			result := GetenvBoolOrDefault(tt.envKey, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetenvIntOrDefault(t *testing.T) {
	// Note: Cannot use t.Parallel() because subtests use t.Setenv

	tests := []struct {
		name         string
		envKey       string
		envValue     string
		defaultValue int64
		expected     int64
		setEnv       bool
	}{
		{
			name:         "Valid integer",
			envKey:       "TEST_INT_VALID",
			envValue:     "42",
			defaultValue: 0,
			expected:     42,
			setEnv:       true,
		},
		{
			name:         "Negative integer",
			envKey:       "TEST_INT_NEGATIVE",
			envValue:     "-100",
			defaultValue: 0,
			expected:     -100,
			setEnv:       true,
		},
		{
			name:         "Zero",
			envKey:       "TEST_INT_ZERO",
			envValue:     "0",
			defaultValue: 100,
			expected:     0,
			setEnv:       true,
		},
		{
			name:         "Large number",
			envKey:       "TEST_INT_LARGE",
			envValue:     "9223372036854775807",
			defaultValue: 0,
			expected:     9223372036854775807,
			setEnv:       true,
		},
		{
			name:         "Invalid value - returns default",
			envKey:       "TEST_INT_INVALID",
			envValue:     "not_a_number",
			defaultValue: 99,
			expected:     99,
			setEnv:       true,
		},
		{
			name:         "Float value - returns default",
			envKey:       "TEST_INT_FLOAT",
			envValue:     "3.14",
			defaultValue: 10,
			expected:     10,
			setEnv:       true,
		},
		{
			name:         "Not set - returns default",
			envKey:       "TEST_INT_NOTSET",
			envValue:     "",
			defaultValue: 123,
			expected:     123,
			setEnv:       false,
		},
		{
			name:         "Empty string - returns default",
			envKey:       "TEST_INT_EMPTY",
			envValue:     "",
			defaultValue: 456,
			expected:     456,
			setEnv:       true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// Note: Cannot use t.Parallel() because t.Setenv is used

			// Ensure clean state before test
			t.Setenv(tt.envKey, "")

			if tt.setEnv {
				t.Setenv(tt.envKey, tt.envValue)
			}

			result := GetenvIntOrDefault(tt.envKey, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSetConfigFromEnvVars(t *testing.T) {
	// Note: Cannot use t.Parallel() because subtests use t.Setenv

	type TestConfig struct {
		StringField string `env:"TEST_STRING_FIELD"`
		IntField    int64  `env:"TEST_INT_FIELD"`
		BoolField   bool   `env:"TEST_BOOL_FIELD"`
		NoTagField  string
	}

	t.Run("Success - Set all fields from env vars", func(t *testing.T) {
		// Note: Cannot use t.Parallel() because t.Setenv is used

		t.Setenv("TEST_STRING_FIELD", "test_string")
		t.Setenv("TEST_INT_FIELD", "42")
		t.Setenv("TEST_BOOL_FIELD", "true")

		config := &TestConfig{}
		err := SetConfigFromEnvVars(config)

		require.NoError(t, err)
		assert.Equal(t, "test_string", config.StringField)
		assert.Equal(t, int64(42), config.IntField)
		assert.True(t, config.BoolField)
	})

	t.Run("Error - Non-pointer argument returns error", func(t *testing.T) {
		t.Parallel()

		config := TestConfig{}
		err := SetConfigFromEnvVars(config)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be an pointer")
	})

	t.Run("Success - Fields without env tag are not modified", func(t *testing.T) {
		// Note: Cannot use t.Parallel() because t.Setenv is used

		t.Setenv("TEST_STRING_FIELD", "value")

		config := &TestConfig{NoTagField: "original"}
		err := SetConfigFromEnvVars(config)

		require.NoError(t, err)
		assert.Equal(t, "original", config.NoTagField)
	})

	t.Run("Success - Missing env vars result in zero values", func(t *testing.T) {
		// Note: Cannot use t.Parallel() because t.Setenv is used

		// Ensure vars are not set
		t.Setenv("TEST_STRING_FIELD", "")
		t.Setenv("TEST_INT_FIELD", "")
		t.Setenv("TEST_BOOL_FIELD", "")

		config := &TestConfig{}
		err := SetConfigFromEnvVars(config)

		require.NoError(t, err)
		assert.Equal(t, "", config.StringField)
		assert.Equal(t, int64(0), config.IntField)
		assert.False(t, config.BoolField)
	})
}

func TestSetConfigFromEnvVars_AllIntTypes(t *testing.T) {
	// Note: Cannot use t.Parallel() because t.Setenv is used

	type IntTypesConfig struct {
		Int   int   `env:"TEST_INT"`
		Int8  int8  `env:"TEST_INT8"`
		Int16 int16 `env:"TEST_INT16"`
		Int32 int32 `env:"TEST_INT32"`
		Int64 int64 `env:"TEST_INT64"`
	}

	t.Setenv("TEST_INT", "1")
	t.Setenv("TEST_INT8", "8")
	t.Setenv("TEST_INT16", "16")
	t.Setenv("TEST_INT32", "32")
	t.Setenv("TEST_INT64", "64")

	config := &IntTypesConfig{}
	err := SetConfigFromEnvVars(config)

	require.NoError(t, err)
	assert.Equal(t, 1, config.Int)
	assert.Equal(t, int8(8), config.Int8)
	assert.Equal(t, int16(16), config.Int16)
	assert.Equal(t, int32(32), config.Int32)
	assert.Equal(t, int64(64), config.Int64)
}

func TestEnsureConfigFromEnvVars(t *testing.T) {
	// Note: Cannot use t.Parallel() because subtests use t.Setenv

	type TestConfig struct {
		Field string `env:"TEST_ENSURE_FIELD"`
	}

	t.Run("Success - Valid pointer returns config", func(t *testing.T) {
		// Note: Cannot use t.Parallel() because t.Setenv is used

		t.Setenv("TEST_ENSURE_FIELD", "value")

		config := &TestConfig{}
		result, err := EnsureConfigFromEnvVars(config)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "value", config.Field)
	})

	t.Run("Error - Non-pointer returns error", func(t *testing.T) {
		t.Parallel()

		config := TestConfig{}

		result, err := EnsureConfigFromEnvVars(config)

		require.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestLocalEnvConfig(t *testing.T) {
	// Note: Cannot use t.Parallel() because subtests use t.Setenv

	// Note: InitLocalEnvConfig uses sync.Once, so we can only test it once per process
	// The test will depend on whether a .env file exists in the current directory

	t.Run("Success - Initialize local env config", func(t *testing.T) {
		// Note: Cannot use t.Parallel() because t.Setenv is used

		// Set ENV_NAME to something other than "local" to skip .env loading
		// t.Setenv automatically saves and restores the original value
		t.Setenv("ENV_NAME", "test")

		result := InitLocalEnvConfig()

		// When ENV_NAME is not "local", the function returns nil because
		// the sync.Once hasn't been triggered and localEnvConfig is not set
		// This tests the non-local environment branch
		if os.Getenv("ENV_NAME") != "local" {
			assert.Nil(t, result)
		}
	})
}

func TestGetEnvOrDefault_ValueWithLeadingAndTrailingSpaces(t *testing.T) {
	// Note: Cannot use t.Parallel() because t.Setenv is used

	t.Setenv("TEST_TRIMMED_ENV", "  real_value  ")

	// GetEnvOrDefault only trims to check emptiness, but returns the raw value
	result := GetEnvOrDefault("TEST_TRIMMED_ENV", "default")
	assert.Equal(t, "  real_value  ", result)
}

func TestGetenvBoolOrDefault_EmptyString(t *testing.T) {
	// Note: Cannot use t.Parallel() because t.Setenv is used

	t.Setenv("TEST_BOOL_EMPTY_STR", "")

	result := GetenvBoolOrDefault("TEST_BOOL_EMPTY_STR", true)
	assert.True(t, result, "empty string should return defaultValue")
}

func TestGetenvIntOrDefault_OverflowValue(t *testing.T) {
	// Note: Cannot use t.Parallel() because t.Setenv is used

	// Value that overflows int64
	t.Setenv("TEST_INT_OVERFLOW", "99999999999999999999999999")

	result := GetenvIntOrDefault("TEST_INT_OVERFLOW", 42)
	assert.Equal(t, int64(42), result, "overflow value should return defaultValue")
}

func TestEnsureConfigFromEnvVars_ReturnsOriginalPointer(t *testing.T) {
	// Note: Cannot use t.Parallel() because t.Setenv is used

	type Config struct {
		Name string `env:"TEST_ENSURE_PTR_NAME"`
	}

	t.Setenv("TEST_ENSURE_PTR_NAME", "test_value")

	original := &Config{}
	result, err := EnsureConfigFromEnvVars(original)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify result is the same pointer
	resultConfig, ok := result.(*Config)
	require.True(t, ok)
	assert.Equal(t, "test_value", resultConfig.Name)
	assert.Equal(t, original, resultConfig, "should return the same pointer")
}

func TestSetConfigFromEnvVars_InvalidBoolReturnsDefault(t *testing.T) {
	// Note: Cannot use t.Parallel() because t.Setenv is used

	type Config struct {
		Flag bool `env:"TEST_INVALID_BOOL_FIELD"`
	}

	t.Setenv("TEST_INVALID_BOOL_FIELD", "not_a_bool")

	config := &Config{Flag: true}
	err := SetConfigFromEnvVars(config)

	require.NoError(t, err)
	// GetenvBoolOrDefault returns false as default for invalid bool
	assert.False(t, config.Flag, "invalid bool should result in false (default)")
}

func TestSetConfigFromEnvVars_InvalidIntReturnsDefault(t *testing.T) {
	// Note: Cannot use t.Parallel() because t.Setenv is used

	type Config struct {
		Count int64 `env:"TEST_INVALID_INT_FIELD"`
	}

	t.Setenv("TEST_INVALID_INT_FIELD", "not_a_number")

	config := &Config{Count: 99}
	err := SetConfigFromEnvVars(config)

	require.NoError(t, err)
	// GetenvIntOrDefault returns 0 as default for invalid int
	assert.Equal(t, int64(0), config.Count, "invalid int should result in 0 (default)")
}

func TestSetConfigFromEnvVars_UnexportedFields(t *testing.T) {
	// Note: Cannot use t.Parallel() because t.Setenv is used

	// Struct with unexported fields tagged with env — CanSet() returns false,
	// so these fields should be silently skipped.
	type Config struct {
		Exported   string `env:"TEST_UNEXPORTED_EXPORTED"`
		unexported string `env:"TEST_UNEXPORTED_HIDDEN"` //nolint:unused
	}

	t.Setenv("TEST_UNEXPORTED_EXPORTED", "visible")
	t.Setenv("TEST_UNEXPORTED_HIDDEN", "should_be_ignored")

	config := &Config{}
	err := SetConfigFromEnvVars(config)

	require.NoError(t, err)
	assert.Equal(t, "visible", config.Exported)
	// The unexported field cannot be read directly, but the key point is
	// that SetConfigFromEnvVars did not panic or error.
}

func TestSetConfigFromEnvVars_UnsupportedFieldType(t *testing.T) {
	// Note: Cannot use t.Parallel() because t.Setenv is used

	// A float64 field hits the default case in the switch, which calls
	// fv.SetString(). This will panic because reflect cannot set a string
	// value on a float64 field. Verify this behavior.
	type Config struct {
		Ratio float64 `env:"TEST_UNSUPPORTED_FLOAT"`
	}

	t.Setenv("TEST_UNSUPPORTED_FLOAT", "3.14")

	config := &Config{}
	assert.Panics(t, func() {
		_ = SetConfigFromEnvVars(config)
	}, "SetString on a float64 field should panic")
}
