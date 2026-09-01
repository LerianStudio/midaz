// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetEnvOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		envValue     string
		defaultValue string
		setEnv       bool
		expected     string
	}{
		{
			name:         "returns env value when set",
			key:          "TEST_ENV_VAR",
			envValue:     "custom_value",
			defaultValue: "default",
			setEnv:       true,
			expected:     "custom_value",
		},
		{
			name:         "returns default when env not set",
			key:          "TEST_ENV_VAR_UNSET",
			envValue:     "",
			defaultValue: "default_value",
			setEnv:       false,
			expected:     "default_value",
		},
		{
			name:         "returns default when env is empty string",
			key:          "TEST_ENV_VAR_EMPTY",
			envValue:     "",
			defaultValue: "default",
			setEnv:       true,
			expected:     "default",
		},
		{
			name:         "returns default when env is whitespace only",
			key:          "TEST_ENV_VAR_WHITESPACE",
			envValue:     "   ",
			defaultValue: "default",
			setEnv:       true,
			expected:     "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv(tt.key, tt.envValue)
			}

			result := GetEnvOrDefault(tt.key, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetenvBoolOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		envValue     string
		defaultValue bool
		setEnv       bool
		expected     bool
	}{
		{
			name:         "returns true when env is 'true'",
			key:          "TEST_BOOL_TRUE",
			envValue:     "true",
			defaultValue: false,
			setEnv:       true,
			expected:     true,
		},
		{
			name:         "returns false when env is 'false'",
			key:          "TEST_BOOL_FALSE",
			envValue:     "false",
			defaultValue: true,
			setEnv:       true,
			expected:     false,
		},
		{
			name:         "returns true when env is '1'",
			key:          "TEST_BOOL_ONE",
			envValue:     "1",
			defaultValue: false,
			setEnv:       true,
			expected:     true,
		},
		{
			name:         "returns false when env is '0'",
			key:          "TEST_BOOL_ZERO",
			envValue:     "0",
			defaultValue: true,
			setEnv:       true,
			expected:     false,
		},
		{
			name:         "returns default when env not set",
			key:          "TEST_BOOL_UNSET",
			envValue:     "",
			defaultValue: true,
			setEnv:       false,
			expected:     true,
		},
		{
			name:         "returns default when env is invalid",
			key:          "TEST_BOOL_INVALID",
			envValue:     "not_a_bool",
			defaultValue: true,
			setEnv:       true,
			expected:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv(tt.key, tt.envValue)
			}

			result := GetenvBoolOrDefault(tt.key, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetenvIntOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		envValue     string
		defaultValue int64
		setEnv       bool
		expected     int64
	}{
		{
			name:         "returns int value when valid",
			key:          "TEST_INT_VALID",
			envValue:     "42",
			defaultValue: 0,
			setEnv:       true,
			expected:     42,
		},
		{
			name:         "returns negative int value",
			key:          "TEST_INT_NEGATIVE",
			envValue:     "-100",
			defaultValue: 0,
			setEnv:       true,
			expected:     -100,
		},
		{
			name:         "returns default when env not set",
			key:          "TEST_INT_UNSET",
			envValue:     "",
			defaultValue: 999,
			setEnv:       false,
			expected:     999,
		},
		{
			name:         "returns default when env is invalid",
			key:          "TEST_INT_INVALID",
			envValue:     "not_an_int",
			defaultValue: 123,
			setEnv:       true,
			expected:     123,
		},
		{
			name:         "returns default when env is float",
			key:          "TEST_INT_FLOAT",
			envValue:     "3.14",
			defaultValue: 10,
			setEnv:       true,
			expected:     10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv(tt.key, tt.envValue)
			}

			result := GetenvIntOrDefault(tt.key, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Note: InitLocalEnvConfig uses sync.Once, making it difficult to test multiple scenarios
// in a single test run. The function is tested implicitly through integration tests.

func TestSetConfigFromEnvVars(t *testing.T) {
	type TestConfig struct {
		StringField string `env:"TEST_STRING_FIELD"`
		BoolField   bool   `env:"TEST_BOOL_FIELD"`
		IntField    int    `env:"TEST_INT_FIELD"`
		Int64Field  int64  `env:"TEST_INT64_FIELD"`
		NoTagField  string
	}

	t.Run("sets all field types from env vars", func(t *testing.T) {
		t.Setenv("TEST_STRING_FIELD", "test_value")
		t.Setenv("TEST_BOOL_FIELD", "true")
		t.Setenv("TEST_INT_FIELD", "42")
		t.Setenv("TEST_INT64_FIELD", "9999999999")

		config := &TestConfig{}
		err := SetConfigFromEnvVars(config)

		assert.NoError(t, err)
		assert.Equal(t, "test_value", config.StringField)
		assert.Equal(t, true, config.BoolField)
		assert.Equal(t, 42, config.IntField)
		assert.Equal(t, int64(9999999999), config.Int64Field)
		assert.Equal(t, "", config.NoTagField)
	})

	t.Run("returns error when not a pointer", func(t *testing.T) {
		config := TestConfig{}
		err := SetConfigFromEnvVars(config)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be an pointer")
	})

	t.Run("handles missing env vars with defaults", func(t *testing.T) {
		config := &TestConfig{
			StringField: "original",
			BoolField:   true,
			IntField:    100,
		}
		err := SetConfigFromEnvVars(config)

		assert.NoError(t, err)
		// String becomes empty, bool becomes false, int becomes 0 (defaults from GetenvXxxOrDefault)
		assert.Equal(t, "", config.StringField)
		assert.Equal(t, false, config.BoolField)
		assert.Equal(t, 0, config.IntField)
	})
}
