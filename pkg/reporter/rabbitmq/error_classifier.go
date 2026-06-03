// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"errors"
	"strings"

	pkg "github.com/LerianStudio/midaz/v3/pkg/reporter"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
)

// ErrorClassifier classifies errors for retry eligibility in RabbitMQ message processing.
// Implementations determine whether a failed message should be retried or sent to DLQ.
type ErrorClassifier interface {
	// IsRetryable returns true if the error is transient and the message should be retried.
	IsRetryable(err error) bool
	// IsPermanentTenantError returns true if the error indicates a permanent tenant issue.
	IsPermanentTenantError(err error) bool
	// ClassifyFailureReason returns a machine-readable failure reason string for headers.
	ClassifyFailureReason(err error) string
}

// DefaultErrorClassifier is the standard error classifier for the reporter service.
// It classifies business validation errors, template errors (TPL-XXXX), and
// permanent tenant-manager errors as non-retryable. All other errors are retryable.
type DefaultErrorClassifier struct{}

// NewDefaultErrorClassifier creates a new DefaultErrorClassifier.
func NewDefaultErrorClassifier() *DefaultErrorClassifier {
	return &DefaultErrorClassifier{}
}

// IsRetryable classifies an error as retryable or non-retryable.
// Business validation errors (TPL-XXXX codes) are non-retryable because retrying
// will not change the outcome. Network, timeout, and unknown errors are retryable
// as transient failures may resolve on subsequent attempts.
func (c *DefaultErrorClassifier) IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	if strings.Contains(err.Error(), "TPL-") {
		return false
	}

	if isNonRetryableDomainError(err) {
		return false
	}

	if c.IsPermanentTenantError(err) {
		return false
	}

	return true
}

// IsPermanentTenantError classifies tenant-manager errors as permanent or transient.
// Permanent errors (tenant not found, suspended, service not configured, manager closed)
// will never succeed on retry. Transient errors (circuit breaker open, network issues)
// may resolve on subsequent attempts.
func (c *DefaultErrorClassifier) IsPermanentTenantError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, tmcore.ErrTenantNotFound) {
		return true
	}

	if errors.Is(err, tmcore.ErrServiceNotConfigured) {
		return true
	}

	if errors.Is(err, tmcore.ErrManagerClosed) {
		return true
	}

	if tmcore.IsTenantSuspendedError(err) {
		return true
	}

	return false
}

// ClassifyFailureReason returns a machine-readable failure reason string
// for encoding in message headers during retry republish.
func (c *DefaultErrorClassifier) ClassifyFailureReason(err error) string {
	switch {
	case err == nil:
		return "unknown_error"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	case errors.Is(err, context.Canceled):
		return "context_canceled"
	case tmcore.IsCircuitBreakerOpenError(err):
		return "circuit_breaker_open"
	case tmcore.IsTenantSuspendedError(err):
		return "tenant_suspended"
	case errors.Is(err, tmcore.ErrTenantNotFound):
		return "tenant_not_found"
	case errors.Is(err, tmcore.ErrServiceNotConfigured):
		return "service_not_configured"
	default:
		return "retryable_error"
	}
}

// isNonRetryableDomainError checks for known non-retryable error types
// from the reporter application domain.
func isNonRetryableDomainError(err error) bool {
	var validationErr pkg.ValidationError
	if errors.As(err, &validationErr) {
		return true
	}

	var notFoundErr pkg.EntityNotFoundError
	if errors.As(err, &notFoundErr) {
		return true
	}

	var knownFieldsErr pkg.ValidationKnownFieldsError
	if errors.As(err, &knownFieldsErr) {
		return true
	}

	var unknownFieldsErr pkg.ValidationUnknownFieldsError
	if errors.As(err, &unknownFieldsErr) {
		return true
	}

	var unprocessableErr pkg.UnprocessableOperationError
	if errors.As(err, &unprocessableErr) {
		return true
	}

	var conflictErr pkg.EntityConflictError
	if errors.As(err, &conflictErr) {
		return true
	}

	var forbiddenErr pkg.ForbiddenError
	if errors.As(err, &forbiddenErr) {
		return true
	}

	var unauthorizedErr pkg.UnauthorizedError
	if errors.As(err, &unauthorizedErr) {
		return true
	}

	var preconditionErr pkg.FailedPreconditionError

	return errors.As(err, &preconditionErr)
}
