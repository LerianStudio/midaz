// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package datasource

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	pkg "github.com/LerianStudio/midaz/v4/pkg"
)

func TestErrDataSourceNotFound_ImplementsError(t *testing.T) {
	t.Parallel()

	// Verify ErrDataSourceNotFound is defined and usable as an error sentinel.
	var err error = ErrDataSourceNotFound
	assert.NotNil(t, err, "ErrDataSourceNotFound should be defined")
	assert.NotEmpty(t, err.Error(), "error sentinel should carry a canonical code")
}

func TestErrDataSourceUnavailable_ImplementsError(t *testing.T) {
	t.Parallel()

	// Verify ErrDataSourceUnavailable is defined for D7 warnings.
	var err error = ErrDataSourceUnavailable
	assert.NotNil(t, err, "ErrDataSourceUnavailable should be defined")
	assert.NotEmpty(t, err.Error(), "error sentinel should carry a canonical code")
}

func TestErrorIntegrationWithDomainErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		sentinel     error
		expectedType string
		description  string
	}{
		{
			name:         "DataSourceNotFound maps to EntityNotFoundError",
			sentinel:     ErrDataSourceNotFound,
			expectedType: "EntityNotFoundError",
			description:  "should be mappable to pkg.EntityNotFoundError via ValidateBusinessError",
		},
		{
			name:         "DataSourceUnavailable maps to ServiceUnavailableError",
			sentinel:     ErrDataSourceUnavailable,
			expectedType: "ServiceUnavailableError",
			description:  "should be mappable to pkg.ValidationError for D7 warning pattern",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Verify the error sentinel can be wrapped and unwrapped with errors.Is
			wrappedErr := fmt.Errorf("some context: %w", tt.sentinel)
			assert.True(t, errors.Is(wrappedErr, tt.sentinel),
				"wrapped error should be unwrappable to sentinel via errors.Is")

			// Verify the sentinel integrates with the existing error mapping.
			// ValidateBusinessError from pkg/errors.go should recognize these sentinels.
			mappedErr := pkg.ValidateBusinessError(tt.sentinel, "DataSource")
			assert.NotNil(t, mappedErr, "mapped error should not be nil for sentinel: %s", tt.sentinel)

			switch tt.expectedType {
			case "EntityNotFoundError":
				var target pkg.EntityNotFoundError
				assert.True(t, errors.As(mappedErr, &target),
					"sentinel %s should map to EntityNotFoundError, got: %T", tt.sentinel, mappedErr)
			case "ValidationError":
				var target pkg.ValidationError
				assert.True(t, errors.As(mappedErr, &target),
					"sentinel %s should map to ValidationError, got: %T", tt.sentinel, mappedErr)
			case "UnprocessableOperationError":
				var target pkg.UnprocessableOperationError
				assert.True(t, errors.As(mappedErr, &target),
					"sentinel %s should map to UnprocessableOperationError, got: %T", tt.sentinel, mappedErr)
			case "InternalServerError":
				var target pkg.InternalServerError
				assert.True(t, errors.As(mappedErr, &target),
					"sentinel %s should map to InternalServerError, got: %T", tt.sentinel, mappedErr)
			case "ServiceUnavailableError":
				var target pkg.ServiceUnavailableError
				assert.True(t, errors.As(mappedErr, &target),
					"sentinel %s should map to ServiceUnavailableError, got: %T", tt.sentinel, mappedErr)
			}
		})
	}
}

func TestValidationWarningCode_DataSourceUnavailable(t *testing.T) {
	t.Parallel()

	// D7 pattern: when a data source is unavailable, the system should produce
	// a ValidationWarning with code DATA_SOURCE_UNAVAILABLE rather than a hard error.
	warning := ValidationWarning{
		Field:   "datasource:midaz_onboarding",
		Code:    WarningCodeDataSourceUnavailable,
		Message: "Data source is currently unavailable; results may be incomplete",
	}

	assert.Equal(t, "DATA_SOURCE_UNAVAILABLE", warning.Code,
		"WarningCodeDataSourceUnavailable should equal DATA_SOURCE_UNAVAILABLE")
	assert.NotEmpty(t, warning.Field, "warning should reference the affected datasource")
	assert.NotEmpty(t, warning.Message, "warning should include a human-readable message")
}
