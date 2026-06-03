// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tracer/internal/adapters/cel"
	"tracer/internal/testutil"
	"tracer/pkg/model"
)

func TestValidateAuthConfig_Success_AuthDisabled_LogsWarning(t *testing.T) {
	t.Parallel()

	// Arrange
	logger := testutil.NewMockLogger()
	cfg := &Config{
		APIKeyEnabled: false,
		APIKey:        "",
	}

	// Act
	err := ValidateAuthConfig(t.Context(), cfg, logger)

	// Assert
	require.NoError(t, err)
	assert.Len(t, logger.Calls, 1, "expected exactly one warning when auth is disabled")
	assert.Contains(t, logger.Calls[0].Message, "API Key authentication is DISABLED")
}

func TestValidateAuthConfig_Error_AuthEnabledNoKey_ReturnsError(t *testing.T) {
	t.Parallel()

	// Arrange
	logger := testutil.NewMockLogger()
	cfg := &Config{
		APIKeyEnabled: true,
		APIKey:        "", // Empty key when enabled
	}

	// Act
	err := ValidateAuthConfig(t.Context(), cfg, logger)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API_KEY must be set when API_KEY_ENABLED=true")
}

func TestValidateAuthConfig_Success_AuthEnabledShortKey_LogsWarning(t *testing.T) {
	t.Parallel()

	// Arrange
	logger := testutil.NewMockLogger()
	cfg := &Config{
		APIKeyEnabled: true,
		APIKey:        "short_key", // Less than 32 characters
	}

	// Act
	err := ValidateAuthConfig(t.Context(), cfg, logger)

	// Assert
	require.NoError(t, err)
	assert.Len(t, logger.Calls, 1, "expected exactly one warning for short API key")
	assert.Contains(t, logger.Calls[0].Message, "API_KEY should be at least 32 characters")
}

func TestValidateAuthConfig_Success_AuthEnabledValidKey_NoError(t *testing.T) {
	t.Parallel()

	// Arrange
	logger := testutil.NewMockLogger()
	// Generate a key that is exactly 32 characters
	validKey := "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6" // 32 chars
	cfg := &Config{
		APIKeyEnabled: true,
		APIKey:        validKey,
	}

	// Act
	err := ValidateAuthConfig(t.Context(), cfg, logger)

	// Assert
	require.NoError(t, err)
	assert.Len(t, logger.Calls, 0, "expected no warnings for valid configuration")
}

func TestParseCELCostLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		expected    uint64
		expectError bool
	}{
		{
			name:        "empty string returns default",
			input:       "",
			expected:    10000,
			expectError: false,
		},
		{
			name:        "valid number",
			input:       "5000",
			expected:    5000,
			expectError: false,
		},
		{
			name:        "invalid string returns error",
			input:       "invalid",
			expected:    0,
			expectError: true,
		},
		{
			name:        "negative number returns error",
			input:       "-100",
			expected:    0,
			expectError: true,
		},
		{
			name:        "zero returns error",
			input:       "0",
			expected:    0,
			expectError: true,
		},
		{
			name:        "large number",
			input:       "1000000",
			expected:    1000000,
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseCELCostLimit(tc.input)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestValidateAuthConfig_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		apiKeyEnabled     bool
		apiKey            string
		expectError       bool
		expectedErrMsg    string
		expectedWarnCount int
	}{
		{
			name:              "Success - auth disabled logs warning",
			apiKeyEnabled:     false,
			apiKey:            "",
			expectError:       false,
			expectedWarnCount: 1,
		},
		{
			name:              "Error - auth enabled without key returns error",
			apiKeyEnabled:     true,
			apiKey:            "",
			expectError:       true,
			expectedErrMsg:    "API_KEY must be set when API_KEY_ENABLED=true",
			expectedWarnCount: 0,
		},
		{
			name:              "Success - auth enabled with short key logs warning",
			apiKeyEnabled:     true,
			apiKey:            "short_key_under_32_chars",
			expectError:       false,
			expectedWarnCount: 1,
		},
		{
			name:              "Success - auth enabled with valid 32 char key no warning",
			apiKeyEnabled:     true,
			apiKey:            "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6",
			expectError:       false,
			expectedWarnCount: 0,
		},
		{
			name:              "Success - auth enabled with long key no warning",
			apiKeyEnabled:     true,
			apiKey:            "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0",
			expectError:       false,
			expectedWarnCount: 0,
		},
		{
			name:              "Success - auth enabled with 31 char key logs warning",
			apiKeyEnabled:     true,
			apiKey:            "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p",
			expectError:       false,
			expectedWarnCount: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			logger := testutil.NewMockLogger()
			cfg := &Config{
				APIKeyEnabled: tc.apiKeyEnabled,
				APIKey:        tc.apiKey,
			}

			// Act
			err := ValidateAuthConfig(t.Context(), cfg, logger)

			// Assert
			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrMsg)
			} else {
				require.NoError(t, err)
			}
			assert.Len(t, logger.Calls, tc.expectedWarnCount,
				"expected %d warnings, got %d", tc.expectedWarnCount, len(logger.Calls))
		})
	}
}

func TestValidateAccessManagerConfig_Success_PluginDisabled_LogsWarning(t *testing.T) {
	t.Parallel()

	// Arrange
	logger := testutil.NewMockLogger()
	cfg := &Config{
		PluginAuthEnabled: false,
		PluginAuthAddress: "",
	}

	// Act
	err := ValidateAccessManagerConfig(t.Context(), cfg, logger)

	// Assert
	require.NoError(t, err)
	assert.Len(t, logger.Calls, 1, "expected exactly one warning when plugin auth is disabled")
	assert.Contains(t, logger.Calls[0].Message, "Access Manager plugin authentication is DISABLED")
}

func TestValidateAccessManagerConfig_Error_PluginEnabledNoAddress_ReturnsError(t *testing.T) {
	t.Parallel()

	// Arrange
	logger := testutil.NewMockLogger()
	cfg := &Config{
		PluginAuthEnabled: true,
		PluginAuthAddress: "",
	}

	// Act
	err := ValidateAccessManagerConfig(t.Context(), cfg, logger)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PLUGIN_AUTH_ADDRESS must be set when PLUGIN_AUTH_ENABLED=true")
}

func TestValidateAccessManagerConfig_Success_PluginEnabledWithAddress_NoError(t *testing.T) {
	t.Parallel()

	// Arrange
	logger := testutil.NewMockLogger()
	cfg := &Config{
		PluginAuthEnabled: true,
		PluginAuthAddress: "http://access-manager:8080",
	}

	// Act
	err := ValidateAccessManagerConfig(t.Context(), cfg, logger)

	// Assert
	require.NoError(t, err)
	assert.Len(t, logger.Calls, 0, "expected no warnings for valid configuration")
}

func TestValidateAccessManagerConfig_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		pluginEnabled     bool
		pluginAddress     string
		expectError       bool
		expectedErrMsg    string
		expectedWarnCount int
	}{
		{
			name:              "Success - plugin disabled logs warning",
			pluginEnabled:     false,
			pluginAddress:     "",
			expectError:       false,
			expectedWarnCount: 1,
		},
		{
			name:              "Success - plugin disabled with address still logs warning",
			pluginEnabled:     false,
			pluginAddress:     "http://access-manager:8080",
			expectError:       false,
			expectedWarnCount: 1,
		},
		{
			name:              "Error - plugin enabled without address returns error",
			pluginEnabled:     true,
			pluginAddress:     "",
			expectError:       true,
			expectedErrMsg:    "PLUGIN_AUTH_ADDRESS must be set when PLUGIN_AUTH_ENABLED=true",
			expectedWarnCount: 0,
		},
		{
			name:              "Success - plugin enabled with address no warning",
			pluginEnabled:     true,
			pluginAddress:     "http://access-manager:8080",
			expectError:       false,
			expectedWarnCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			logger := testutil.NewMockLogger()
			cfg := &Config{
				PluginAuthEnabled: tc.pluginEnabled,
				PluginAuthAddress: tc.pluginAddress,
			}

			// Act
			err := ValidateAccessManagerConfig(t.Context(), cfg, logger)

			// Assert
			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrMsg)
			} else {
				require.NoError(t, err)
			}
			assert.Len(t, logger.Calls, tc.expectedWarnCount,
				"expected %d warnings, got %d", tc.expectedWarnCount, len(logger.Calls))
		})
	}
}

func TestCelCompilerAdapter_Compile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		expression  string
		expectError bool
	}{
		{
			name:        "Success - compiles valid expression",
			expression:  "amount > 1000",
			expectError: false,
		},
		{
			name:        "Success - compiles boolean expression",
			expression:  "amount > 500 && amount < 10000",
			expectError: false,
		},
		{
			name:        "Error - invalid expression syntax",
			expression:  "invalid syntax !!@#",
			expectError: true,
		},
		{
			name:        "Error - undeclared variable",
			expression:  "unknown_var == true",
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			logger := testutil.NewMockLogger()
			adapter, err := cel.NewAdapter(cel.AdapterConfig{
				CostLimit: 10000,
			}, logger)
			require.NoError(t, err)

			compiler := &celCompilerAdapter{adapter: adapter}

			// Act
			result, err := compiler.Compile(context.Background(), tc.expression)

			// Assert
			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

func TestParseDefaultDecision(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		expected    model.Decision
		expectError bool
	}{
		{
			name:        "empty string returns ALLOW",
			input:       "",
			expected:    model.DecisionAllow,
			expectError: false,
		},
		{
			name:        "ALLOW returns ALLOW",
			input:       "ALLOW",
			expected:    model.DecisionAllow,
			expectError: false,
		},
		{
			name:        "DENY returns DENY",
			input:       "DENY",
			expected:    model.DecisionDeny,
			expectError: false,
		},
		{
			name:        "invalid value returns error",
			input:       "INVALID",
			expectError: true,
		},
		{
			name:        "lowercase allow returns error",
			input:       "allow",
			expectError: true,
		},
		{
			name:        "REVIEW returns error",
			input:       "REVIEW",
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseDefaultDecision(tc.input)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestParseMaxRulesPerRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		expected    int
		expectError bool
	}{
		{
			name:        "empty string returns default 1000",
			input:       "",
			expected:    1000,
			expectError: false,
		},
		{
			name:        "valid number",
			input:       "500",
			expected:    500,
			expectError: false,
		},
		{
			name:        "invalid string returns error",
			input:       "invalid",
			expectError: true,
		},
		{
			name:        "negative number returns error",
			input:       "-100",
			expectError: true,
		},
		{
			name:        "zero returns error",
			input:       "0",
			expectError: true,
		},
		{
			name:        "large number",
			input:       "10000",
			expected:    10000,
			expectError: false,
		},
		{
			name:        "maximum allowed value",
			input:       "100000",
			expected:    100000,
			expectError: false,
		},
		{
			name:        "exceeds maximum returns error",
			input:       "100001",
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseMaxRulesPerRequest(tc.input)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestLoadEvaluationConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		defaultDecision     string
		maxRules            string
		expectedDecision    model.Decision
		expectedMaxRules    int
		expectError         bool
		expectedErrContains string
	}{
		{
			name:             "default values when empty",
			defaultDecision:  "",
			maxRules:         "",
			expectedDecision: model.DecisionAllow,
			expectedMaxRules: 1000,
			expectError:      false,
		},
		{
			name:             "custom DENY default",
			defaultDecision:  "DENY",
			maxRules:         "",
			expectedDecision: model.DecisionDeny,
			expectedMaxRules: 1000,
			expectError:      false,
		},
		{
			name:             "custom max rules",
			defaultDecision:  "",
			maxRules:         "500",
			expectedDecision: model.DecisionAllow,
			expectedMaxRules: 500,
			expectError:      false,
		},
		{
			name:             "both custom values",
			defaultDecision:  "DENY",
			maxRules:         "2000",
			expectedDecision: model.DecisionDeny,
			expectedMaxRules: 2000,
			expectError:      false,
		},
		{
			name:                "invalid decision returns error",
			defaultDecision:     "INVALID",
			maxRules:            "",
			expectError:         true,
			expectedErrContains: "invalid DEFAULT_DECISION_WHEN_NO_MATCH",
		},
		{
			name:                "invalid max rules returns error",
			defaultDecision:     "",
			maxRules:            "invalid",
			expectError:         true,
			expectedErrContains: "invalid MAX_RULES_PER_REQUEST",
		},
		{
			name:                "both invalid returns first error (decision)",
			defaultDecision:     "INVALID",
			maxRules:            "invalid",
			expectError:         true,
			expectedErrContains: "invalid DEFAULT_DECISION_WHEN_NO_MATCH",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				DefaultDecisionWhenNoMatch: tc.defaultDecision,
				MaxRulesPerRequest:         tc.maxRules,
			}

			logger := testutil.NewMockLogger()
			result, err := LoadEvaluationConfig(t.Context(), cfg, logger)

			if tc.expectError {
				require.Error(t, err)
				if tc.expectedErrContains != "" {
					assert.Contains(t, err.Error(), tc.expectedErrContains)
				}

				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tc.expectedDecision, result.DefaultDecisionWhenNoMatch)
				assert.Equal(t, tc.expectedMaxRules, result.MaxRulesPerRequest)
			}
		})
	}
}

func TestLoadEvaluationConfig_NilConfig(t *testing.T) {
	t.Parallel()

	logger := testutil.NewMockLogger()
	result, err := LoadEvaluationConfig(t.Context(), nil, logger)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "config cannot be nil")
	assert.Nil(t, result)
}

func TestLoadEvaluationConfig_NilLogger(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		DefaultDecisionWhenNoMatch: "",
		MaxRulesPerRequest:         "",
	}
	result, err := LoadEvaluationConfig(t.Context(), cfg, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "logger cannot be nil")
	assert.Nil(t, result)
}

func TestLoadEvaluationConfig_DefaultALLOW_LogsWarning(t *testing.T) {
	t.Parallel()

	logger := testutil.NewMockLogger()
	cfg := &Config{
		DefaultDecisionWhenNoMatch: "", // Empty = default ALLOW
		MaxRulesPerRequest:         "",
	}

	result, err := LoadEvaluationConfig(t.Context(), cfg, logger)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, model.DecisionAllow, result.DefaultDecisionWhenNoMatch)

	// Verify warning was logged
	require.Len(t, logger.Calls, 1, "expected warning for default ALLOW")
	assert.Contains(t, logger.Calls[0].Message, "fail-open")
}

func TestParseCleanupIntervalHours(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		expected    time.Duration
		expectError bool
	}{
		{
			name:        "empty string returns default 24 hours",
			input:       "",
			expected:    24 * time.Hour,
			expectError: false,
		},
		{
			name:        "valid number",
			input:       "12",
			expected:    12 * time.Hour,
			expectError: false,
		},
		{
			name:        "1 hour",
			input:       "1",
			expected:    1 * time.Hour,
			expectError: false,
		},
		{
			name:        "invalid string returns error",
			input:       "invalid",
			expectError: true,
		},
		{
			name:        "negative number returns error",
			input:       "-1",
			expectError: true,
		},
		{
			name:        "zero returns error",
			input:       "0",
			expectError: true,
		},
		{
			name:        "large number",
			input:       "168", // 1 week
			expected:    168 * time.Hour,
			expectError: false,
		},
		{
			name:        "maximum allowed value - 1 year",
			input:       "8760",
			expected:    8760 * time.Hour,
			expectError: false,
		},
		{
			name:        "exceeds maximum returns error",
			input:       "8761",
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseCleanupIntervalHours(tc.input)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestParseRuleSyncPollInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		expected    time.Duration
		expectError bool
	}{
		{
			name:        "empty string returns default 10 seconds",
			input:       "",
			expected:    10 * time.Second,
			expectError: false,
		},
		{
			name:        "valid number",
			input:       "30",
			expected:    30 * time.Second,
			expectError: false,
		},
		{
			name:        "invalid string returns error",
			input:       "invalid",
			expectError: true,
		},
		{
			name:        "zero returns error",
			input:       "0",
			expectError: true,
		},
		{
			name:        "negative number returns error",
			input:       "-1",
			expectError: true,
		},
		{
			name:        "maximum allowed value - 1 hour",
			input:       "3600",
			expected:    3600 * time.Second,
			expectError: false,
		},
		{
			name:        "exceeds maximum returns error",
			input:       "3601",
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseRuleSyncPollInterval(tc.input)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestParseRuleSyncStalenessThreshold(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		expected    time.Duration
		expectError bool
	}{
		{
			name:        "empty string returns default 50 seconds",
			input:       "",
			expected:    50 * time.Second,
			expectError: false,
		},
		{
			name:        "valid number",
			input:       "120",
			expected:    120 * time.Second,
			expectError: false,
		},
		{
			name:        "invalid string returns error",
			input:       "invalid",
			expectError: true,
		},
		{
			name:        "zero returns error",
			input:       "0",
			expectError: true,
		},
		{
			name:        "negative number returns error",
			input:       "-5",
			expectError: true,
		},
		{
			name:        "maximum allowed value - 1 hour",
			input:       "3600",
			expected:    3600 * time.Second,
			expectError: false,
		},
		{
			name:        "exceeds maximum returns error",
			input:       "3601",
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseRuleSyncStalenessThreshold(tc.input)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestParseRuleSyncOverlapBuffer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		expected    time.Duration
		expectError bool
	}{
		{
			name:        "empty string returns default 2 seconds",
			input:       "",
			expected:    2 * time.Second,
			expectError: false,
		},
		{
			name:        "valid number",
			input:       "5",
			expected:    5 * time.Second,
			expectError: false,
		},
		{
			name:        "zero is allowed",
			input:       "0",
			expected:    0,
			expectError: false,
		},
		{
			name:        "invalid string returns error",
			input:       "invalid",
			expectError: true,
		},
		{
			name:        "negative number returns error",
			input:       "-1",
			expectError: true,
		},
		{
			name:        "maximum allowed value - 60 seconds",
			input:       "60",
			expected:    60 * time.Second,
			expectError: false,
		},
		{
			name:        "exceeds maximum returns error",
			input:       "61",
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseRuleSyncOverlapBuffer(tc.input)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestLoadRuleSyncWorkerConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		pollInterval         string
		stalenessThreshold   string
		overlapBuffer        string
		expectedPollInterval time.Duration
		expectedStaleness    time.Duration
		expectedOverlap      time.Duration
		expectError          bool
		expectedErrContains  string
	}{
		{
			name:                 "default values when empty",
			pollInterval:         "",
			stalenessThreshold:   "",
			overlapBuffer:        "",
			expectedPollInterval: 10 * time.Second,
			expectedStaleness:    50 * time.Second,
			expectedOverlap:      2 * time.Second,
			expectError:          false,
		},
		{
			name:                 "custom poll interval",
			pollInterval:         "30",
			stalenessThreshold:   "",
			overlapBuffer:        "",
			expectedPollInterval: 30 * time.Second,
			expectedStaleness:    50 * time.Second,
			expectedOverlap:      2 * time.Second,
			expectError:          false,
		},
		{
			name:                 "custom staleness threshold",
			pollInterval:         "",
			stalenessThreshold:   "120",
			overlapBuffer:        "",
			expectedPollInterval: 10 * time.Second,
			expectedStaleness:    120 * time.Second,
			expectedOverlap:      2 * time.Second,
			expectError:          false,
		},
		{
			name:                 "custom overlap buffer",
			pollInterval:         "",
			stalenessThreshold:   "",
			overlapBuffer:        "5",
			expectedPollInterval: 10 * time.Second,
			expectedStaleness:    50 * time.Second,
			expectedOverlap:      5 * time.Second,
			expectError:          false,
		},
		{
			name:                 "all custom values",
			pollInterval:         "15",
			stalenessThreshold:   "60",
			overlapBuffer:        "3",
			expectedPollInterval: 15 * time.Second,
			expectedStaleness:    60 * time.Second,
			expectedOverlap:      3 * time.Second,
			expectError:          false,
		},
		{
			name:                "invalid poll interval returns error",
			pollInterval:        "invalid",
			stalenessThreshold:  "",
			overlapBuffer:       "",
			expectError:         true,
			expectedErrContains: "invalid RULE_SYNC_POLL_INTERVAL_SECONDS",
		},
		{
			name:                "invalid staleness threshold returns error",
			pollInterval:        "",
			stalenessThreshold:  "invalid",
			overlapBuffer:       "",
			expectError:         true,
			expectedErrContains: "invalid RULE_SYNC_STALENESS_THRESHOLD_SECONDS",
		},
		{
			name:                "invalid overlap buffer returns error",
			pollInterval:        "",
			stalenessThreshold:  "",
			overlapBuffer:       "invalid",
			expectError:         true,
			expectedErrContains: "invalid RULE_SYNC_OVERLAP_BUFFER_SECONDS",
		},
		{
			name:                 "zero overlap buffer is allowed",
			pollInterval:         "",
			stalenessThreshold:   "",
			overlapBuffer:        "0",
			expectedPollInterval: 10 * time.Second,
			expectedStaleness:    50 * time.Second,
			expectedOverlap:      0,
			expectError:          false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				RuleSyncPollIntervalSeconds:       tc.pollInterval,
				RuleSyncStalenessThresholdSeconds: tc.stalenessThreshold,
				RuleSyncOverlapBufferSeconds:      tc.overlapBuffer,
			}

			logger := testutil.NewMockLogger()
			result, err := LoadRuleSyncWorkerConfig(t.Context(), cfg, logger)

			if tc.expectError {
				require.Error(t, err)
				if tc.expectedErrContains != "" {
					assert.Contains(t, err.Error(), tc.expectedErrContains)
				}

				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tc.expectedPollInterval, result.PollInterval)
				assert.Equal(t, tc.expectedStaleness, result.StalenessThreshold)
				assert.Equal(t, tc.expectedOverlap, result.OverlapBuffer)
			}
		})
	}
}

func TestLoadRuleSyncWorkerConfig_NilConfig(t *testing.T) {
	t.Parallel()

	logger := testutil.NewMockLogger()
	result, err := LoadRuleSyncWorkerConfig(t.Context(), nil, logger)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "config cannot be nil")
	assert.Nil(t, result)
}

func TestLoadRuleSyncWorkerConfig_NilLogger(t *testing.T) {
	t.Parallel()

	cfg := &Config{}
	result, err := LoadRuleSyncWorkerConfig(t.Context(), cfg, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "logger cannot be nil")
	assert.Nil(t, result)
}

func TestLoadCleanupWorkerConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                    string
		cleanupWorkerEnabled    bool
		cleanupIntervalHours    string
		expectedInterval        time.Duration
		expectNilConfig         bool
		expectError             bool
		expectedErrContains     string
		expectedInfoLogContains string
	}{
		{
			name:                    "disabled worker returns nil config",
			cleanupWorkerEnabled:    false,
			cleanupIntervalHours:    "",
			expectNilConfig:         true,
			expectError:             false,
			expectedInfoLogContains: "DISABLED",
		},
		{
			name:                 "enabled with defaults",
			cleanupWorkerEnabled: true,
			cleanupIntervalHours: "",
			expectedInterval:     24 * time.Hour,
			expectNilConfig:      false,
			expectError:          false,
		},
		{
			name:                 "enabled with custom interval",
			cleanupWorkerEnabled: true,
			cleanupIntervalHours: "12",
			expectedInterval:     12 * time.Hour,
			expectNilConfig:      false,
			expectError:          false,
		},
		{
			name:                 "invalid interval returns error",
			cleanupWorkerEnabled: true,
			cleanupIntervalHours: "invalid",
			expectError:          true,
			expectedErrContains:  "invalid CLEANUP_INTERVAL_HOURS",
		},
		{
			name:                 "zero interval returns error",
			cleanupWorkerEnabled: true,
			cleanupIntervalHours: "0",
			expectError:          true,
			expectedErrContains:  "invalid CLEANUP_INTERVAL_HOURS",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				CleanupWorkerEnabled: tc.cleanupWorkerEnabled,
				CleanupIntervalHours: tc.cleanupIntervalHours,
			}

			logger := testutil.NewMockLogger()
			result, err := LoadCleanupWorkerConfig(t.Context(), cfg, logger)

			if tc.expectError {
				require.Error(t, err)
				if tc.expectedErrContains != "" {
					assert.Contains(t, err.Error(), tc.expectedErrContains)
				}

				assert.Nil(t, result)
			} else {
				require.NoError(t, err)

				if tc.expectNilConfig {
					assert.Nil(t, result)
					// Verify info log was generated
					if tc.expectedInfoLogContains != "" {
						require.GreaterOrEqual(t, len(logger.Calls), 1)
						assert.Contains(t, logger.Calls[0].Message, tc.expectedInfoLogContains)
					}
				} else {
					require.NotNil(t, result)
					assert.Equal(t, tc.expectedInterval, result.CleanupInterval)
				}
			}
		})
	}
}

func TestLoadCleanupWorkerConfig_NilConfig(t *testing.T) {
	t.Parallel()

	logger := testutil.NewMockLogger()
	result, err := LoadCleanupWorkerConfig(t.Context(), nil, logger)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "config cannot be nil")
	assert.Nil(t, result)
}

func TestLoadCleanupWorkerConfig_NilLogger(t *testing.T) {
	t.Parallel()

	cfg := &Config{CleanupWorkerEnabled: true}
	result, err := LoadCleanupWorkerConfig(t.Context(), cfg, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "logger cannot be nil")
	assert.Nil(t, result)
}
