package utils

import (
	"testing"

	"github.com/LerianStudio/midaz/v3/tests/utils/stubs"
	"github.com/stretchr/testify/assert"
)

func TestExtractMongoPortAndParameters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		port               string
		parameters         string
		expectedPort       string
		expectedParameters string
		expectWarning      bool
		warningSubstring   string
	}{
		{
			name:               "legacy_embedded_parameters",
			port:               "5703/replicaSet=rs0&authSource=admin",
			parameters:         "",
			expectedPort:       "5703",
			expectedParameters: "replicaSet=rs0&authSource=admin",
			expectWarning:      true,
			warningSubstring:   "DEPRECATED",
		},
		{
			name:               "legacy_embedded_with_question_mark",
			port:               "5703?replicaSet=rs0",
			parameters:         "",
			expectedPort:       "5703",
			expectedParameters: "replicaSet=rs0",
			expectWarning:      true,
			warningSubstring:   "DEPRECATED",
		},
		{
			name:               "new_clean_port_with_parameters",
			port:               "5703",
			parameters:         "replicaSet=rs0&authSource=admin",
			expectedPort:       "5703",
			expectedParameters: "replicaSet=rs0&authSource=admin",
			expectWarning:      false,
			warningSubstring:   "",
		},
		{
			name:               "transition_both_set_parameters_wins",
			port:               "5703/embedded=old",
			parameters:         "explicit=new",
			expectedPort:       "5703",
			expectedParameters: "explicit=new",
			expectWarning:      true,
			warningSubstring:   "takes precedence",
		},
		{
			name:               "default_no_parameters",
			port:               "27017",
			parameters:         "",
			expectedPort:       "27017",
			expectedParameters: "",
			expectWarning:      false,
			warningSubstring:   "",
		},
		{
			name:               "empty_port_and_parameters",
			port:               "",
			parameters:         "",
			expectedPort:       "",
			expectedParameters: "",
			expectWarning:      false,
			warningSubstring:   "",
		},
	}

	for _, tt := range tests {
		tt := tt // capture loop variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logger := &stubs.LoggerStub{}

			actualPort, actualParameters := ExtractMongoPortAndParameters(tt.port, tt.parameters, logger)

			assert.Equal(t, tt.expectedPort, actualPort, "port mismatch")
			assert.Equal(t, tt.expectedParameters, actualParameters, "parameters mismatch")

			if tt.expectWarning {
				assert.True(t, logger.WarningCount() > 0, "expected warning to be logged")
				assert.True(t, logger.HasWarning(tt.warningSubstring),
					"expected warning containing %q, got: %v", tt.warningSubstring, logger.Warnings)
			} else {
				assert.Equal(t, 0, logger.WarningCount(), "expected no warnings, got: %v", logger.Warnings)
			}
		})
	}
}

func TestBuildMongoConnectionString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		uri        string
		user       string
		password   string
		host       string
		port       string
		parameters string
		expected   string
	}{
		{
			name:       "basic_connection_no_parameters",
			uri:        "mongodb",
			user:       "admin",
			password:   "secret123",
			host:       "localhost",
			port:       "27017",
			parameters: "",
			expected:   "mongodb://admin:secret123@localhost:27017/",
		},
		{
			name:       "connection_with_single_parameter",
			uri:        "mongodb",
			user:       "admin",
			password:   "secret123",
			host:       "localhost",
			port:       "27017",
			parameters: "authSource=admin",
			expected:   "mongodb://admin:secret123@localhost:27017/?authSource=admin",
		},
		{
			name:       "connection_with_multiple_parameters",
			uri:        "mongodb",
			user:       "admin",
			password:   "secret123",
			host:       "mongo.example.com",
			port:       "5703",
			parameters: "replicaSet=rs0&authSource=admin&directConnection=true",
			expected:   "mongodb://admin:secret123@mongo.example.com:5703/?replicaSet=rs0&authSource=admin&directConnection=true",
		},
		{
			name:       "mongodb_srv_scheme",
			uri:        "mongodb+srv",
			user:       "user",
			password:   "pass",
			host:       "cluster.mongodb.net",
			port:       "27017",
			parameters: "retryWrites=true&w=majority",
			expected:   "mongodb+srv://user:pass@cluster.mongodb.net:27017/?retryWrites=true&w=majority",
		},
		{
			name:       "special_characters_in_password",
			uri:        "mongodb",
			user:       "admin",
			password:   "p@ss:word/123",
			host:       "localhost",
			port:       "27017",
			parameters: "",
			expected:   "mongodb://admin:p%40ss%3Aword%2F123@localhost:27017/",
		},
		{
			name:       "empty_parameters_no_question_mark",
			uri:        "mongodb",
			user:       "user",
			password:   "pass",
			host:       "db.local",
			port:       "27017",
			parameters: "",
			expected:   "mongodb://user:pass@db.local:27017/",
		},
	}

	for _, tt := range tests {
		tt := tt // capture loop variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Test without logger
			result := BuildMongoConnectionString(tt.uri, tt.user, tt.password, tt.host, tt.port, tt.parameters, nil)
			assert.Equal(t, tt.expected, result)

			// Test with logger (should produce same result)
			logger := &stubs.LoggerStub{}
			resultWithLogger := BuildMongoConnectionString(tt.uri, tt.user, tt.password, tt.host, tt.port, tt.parameters, logger)
			assert.Equal(t, tt.expected, resultWithLogger)
		})
	}
}

func TestBuildMongoConnectionString_LoggerMasksCredentials(t *testing.T) {
	t.Parallel()

	logger := &stubs.LoggerStub{}

	_ = BuildMongoConnectionString("mongodb", "dbuser", "supersecret", "localhost", "27017", "authSource=admin", logger)

	// Verify debug log was called and all credentials are masked
	assert.Len(t, logger.Debugs, 1, "expected exactly one debug log")
	assert.Contains(t, logger.Debugs[0], "<credentials>", "expected credentials to be masked")
	assert.NotContains(t, logger.Debugs[0], "dbuser", "username should not appear in logs")
	assert.NotContains(t, logger.Debugs[0], "supersecret", "password should not appear in logs")
}
