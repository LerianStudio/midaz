// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/stretchr/testify/assert"
)

func TestDefaultClassifier_IsRetryable(t *testing.T) {
	t.Parallel()

	classifier := NewDefaultClassifier()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"context canceled is retryable", context.Canceled, true},
		{"deadline exceeded is retryable", context.DeadlineExceeded, true},
		{"wrapped deadline exceeded is retryable", fmt.Errorf("fetch rows: %w", context.DeadlineExceeded), true},
		{"validation error is permanent", pkg.ValidationError{Code: "V001"}, false},
		{"wrapped validation error is permanent", fmt.Errorf("validate: %w", pkg.ValidationError{Code: "V001"}), false},
		{"entity not found is permanent", pkg.EntityNotFoundError{EntityType: "report"}, false},
		{"conflict is permanent", pkg.EntityConflictError{}, false},
		{"unauthorized is permanent", pkg.UnauthorizedError{}, false},
		{"forbidden is permanent", pkg.ForbiddenError{}, false},
		{"unprocessable is permanent", pkg.UnprocessableOperationError{}, false},
		{"known fields error is permanent", pkg.ValidationKnownFieldsError{}, false},
		{"unknown fields error is permanent", pkg.ValidationUnknownFieldsError{}, false},
		{"network error is retryable", errors.New("connection refused"), true},
		{"wrapped network error is retryable", fmt.Errorf("publish: %w", errors.New("broken pipe")), true},
		{"unknown error is retryable", errors.New("something unexpected"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, classifier.IsRetryable(tt.err))
		})
	}
}

func TestDefaultClassifier_ClassifyFailureReason(t *testing.T) {
	t.Parallel()

	classifier := NewDefaultClassifier()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil error", nil, "unknown_error"},
		{"deadline exceeded", context.DeadlineExceeded, "deadline_exceeded"},
		{"context canceled", context.Canceled, "context_canceled"},
		{"business error", pkg.ValidationError{Code: "V001"}, "business_error"},
		{"wrapped business error", fmt.Errorf("validate: %w", pkg.EntityNotFoundError{}), "business_error"},
		{"generic error", errors.New("network timeout"), "retryable_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, classifier.ClassifyFailureReason(tt.err))
		})
	}
}
