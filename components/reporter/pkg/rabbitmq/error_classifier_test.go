// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/LerianStudio/reporter/pkg"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/stretchr/testify/assert"
)

func TestDefaultErrorClassifier_IsRetryable(t *testing.T) {
	t.Parallel()

	classifier := NewDefaultErrorClassifier()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"context canceled", context.Canceled, false},
		{"deadline exceeded", context.DeadlineExceeded, false},
		{"wrapped context canceled", fmt.Errorf("op failed: %w", context.Canceled), false},
		{"TPL code error", errors.New("TPL-0001: invalid template"), false},
		{"TPL wrapped", fmt.Errorf("render: %w", errors.New("TPL-0022: missing field")), false},
		{"validation error", pkg.ValidationError{Code: "V001"}, false},
		{"entity not found", pkg.EntityNotFoundError{EntityType: "report"}, false},
		{"known fields error", pkg.ValidationKnownFieldsError{}, false},
		{"unknown fields error", pkg.ValidationUnknownFieldsError{}, false},
		{"unprocessable error", pkg.UnprocessableOperationError{}, false},
		{"conflict error", pkg.EntityConflictError{}, false},
		{"forbidden error", pkg.ForbiddenError{}, false},
		{"unauthorized error", pkg.UnauthorizedError{}, false},
		{"precondition error", pkg.FailedPreconditionError{}, false},
		{"tenant not found", tmcore.ErrTenantNotFound, false},
		{"service not configured", tmcore.ErrServiceNotConfigured, false},
		{"manager closed", tmcore.ErrManagerClosed, false},
		{"network error", errors.New("connection refused"), true},
		{"unknown error", errors.New("something unexpected"), true},
		{"wrapped network error", fmt.Errorf("publish: %w", errors.New("broken pipe")), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, classifier.IsRetryable(tt.err))
		})
	}
}

func TestDefaultErrorClassifier_IsPermanentTenantError(t *testing.T) {
	t.Parallel()

	classifier := NewDefaultErrorClassifier()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"tenant not found", tmcore.ErrTenantNotFound, true},
		{"service not configured", tmcore.ErrServiceNotConfigured, true},
		{"manager closed", tmcore.ErrManagerClosed, true},
		{"generic error", errors.New("timeout"), false},
		{"wrapped tenant not found", fmt.Errorf("resolve: %w", tmcore.ErrTenantNotFound), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := classifier.IsPermanentTenantError(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDefaultErrorClassifier_ClassifyFailureReason(t *testing.T) {
	t.Parallel()

	classifier := NewDefaultErrorClassifier()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil error", nil, "unknown_error"},
		{"deadline exceeded", context.DeadlineExceeded, "deadline_exceeded"},
		{"context canceled", context.Canceled, "context_canceled"},
		{"tenant not found", tmcore.ErrTenantNotFound, "tenant_not_found"},
		{"service not configured", tmcore.ErrServiceNotConfigured, "service_not_configured"},
		{"generic error", errors.New("network timeout"), "retryable_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, classifier.ClassifyFailureReason(tt.err))
		})
	}
}
