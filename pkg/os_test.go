package pkg

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetenvOrDefault(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		envValue      string
		defaultValue  string
		expectedValue string
	}{
		{
			name:          "Key exists with value",
			key:           "EXISTING_KEY",
			envValue:      "some_value",
			defaultValue:  "default_value",
			expectedValue: "some_value",
		},
		{
			name:          "Key does not exist",
			key:           "NON_EXISTING_KEY",
			envValue:      "",
			defaultValue:  "default_value",
			expectedValue: "default_value",
		},
		{
			name:          "Key exists with empty value",
			key:           "EMPTY_KEY",
			envValue:      "",
			defaultValue:  "default_value",
			expectedValue: "default_value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.key, tt.envValue)
			result := GetenvOrDefault(tt.key, tt.defaultValue)
			assert.Equal(t, tt.expectedValue, result)
		})
	}
}

func TestGetenvBoolOrDefault(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		envValue      string
		defaultValue  bool
		expectedValue bool
	}{
		{
			name:          "Key exists with 'true' value",
			key:           "BOOL_KEY",
			envValue:      "true",
			defaultValue:  false,
			expectedValue: true,
		},
		{
			name:          "Key exists with 'false' value",
			key:           "BOOL_KEY",
			envValue:      "false",
			defaultValue:  true,
			expectedValue: false,
		},
		{
			name:          "Key exists with invalid value",
			key:           "BOOL_KEY",
			envValue:      "not_a_bool",
			defaultValue:  true,
			expectedValue: true,
		},
		{
			name:          "Key does not exist",
			key:           "NON_EXISTING_KEY",
			envValue:      "",
			defaultValue:  true,
			expectedValue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.key, tt.envValue)
			result := GetenvBoolOrDefault(tt.key, tt.defaultValue)
			assert.Equal(t, tt.expectedValue, result)
		})
	}
}

func TestGetenvIntOrDefault(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		envValue      string
		defaultValue  int64
		expectedValue int64
	}{
		{
			name:          "Key exists with valid int value",
			key:           "INT_KEY",
			envValue:      "42",
			defaultValue:  0,
			expectedValue: 42,
		},
		{
			name:          "Key exists with invalid int value",
			key:           "INT_KEY",
			envValue:      "invalid",
			defaultValue:  10,
			expectedValue: 10,
		},
		{
			name:          "Key does not exist",
			key:           "NON_EXISTING_KEY",
			envValue:      "",
			defaultValue:  100,
			expectedValue: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.key, tt.envValue)
			result := GetenvIntOrDefault(tt.key, tt.defaultValue)
			assert.Equal(t, tt.expectedValue, result)
		})
	}
}
