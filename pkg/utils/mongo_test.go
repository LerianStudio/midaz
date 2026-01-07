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
