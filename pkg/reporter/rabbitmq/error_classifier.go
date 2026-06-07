// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"errors"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgRabbitmq "github.com/LerianStudio/midaz/v4/pkg/rabbitmq"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
)

// permanentReporterCodes are canonical wire codes that map to InternalServerError
// (5xx, not part of the business-error set) yet represent permanent failures that
// will never succeed on retry. They are inspected via the typed error's Code field.
var permanentReporterCodes = map[string]struct{}{
	constant.ErrTemplateRenderFailed.Error(): {}, // 0289: render failure, deterministic
	constant.ErrExtractionJobFailed.Error():  {}, // 0287: extraction job failure
}

// ErrorClassifier classifies errors for retry eligibility in RabbitMQ message processing.
// It extends the generic pkg/rabbitmq classifier with the reporter-specific
// permanent-tenant-error check the consumer consults directly.
type ErrorClassifier interface {
	pkgRabbitmq.ErrorClassifier
	// IsPermanentTenantError returns true if the error indicates a permanent tenant issue.
	IsPermanentTenantError(err error) bool
}

// DefaultErrorClassifier is the standard error classifier for the reporter service.
// It classifies business/domain errors, permanent reporter codes, and permanent
// tenant-manager errors as non-retryable, delegating the residual decision to the
// generic pkg/rabbitmq classifier. All other errors are retryable.
type DefaultErrorClassifier struct {
	base pkgRabbitmq.DefaultClassifier
}

// Compile-time interface satisfaction check.
var _ ErrorClassifier = (*DefaultErrorClassifier)(nil)

// NewDefaultErrorClassifier creates a new DefaultErrorClassifier.
func NewDefaultErrorClassifier() *DefaultErrorClassifier {
	return &DefaultErrorClassifier{}
}

// IsRetryable classifies an error as retryable or non-retryable.
// Context cancellation, permanent reporter codes, and permanent tenant-manager
// errors are non-retryable because retrying will not change the outcome; the
// residual decision (business errors vs. network/unknown) is delegated to the
// generic classifier.
//
// Note: the reporter treats context.Canceled / context.DeadlineExceeded as
// non-retryable (worker shutdown / consumer-side deadline), which is stricter
// than the generic classifier's transient posture — so it is checked here.
func (c *DefaultErrorClassifier) IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	if isPermanentReporterCode(err) {
		return false
	}

	if c.IsPermanentTenantError(err) {
		return false
	}

	// FailedPreconditionError is permanent for the reporter but is not part of the
	// generic business-error set, so it is checked here before delegating.
	var preconditionErr pkg.FailedPreconditionError
	if errors.As(err, &preconditionErr) {
		return false
	}

	return c.base.IsRetryable(err)
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

// isPermanentReporterCode reports whether err carries a canonical reporter code
// that maps to a 5xx typed error yet is permanent (won't succeed on retry).
// These are not part of the business-error set, so the generic classifier does
// not catch them; they are matched by their Code field.
func isPermanentReporterCode(err error) bool {
	var internalErr pkg.InternalServerError
	if errors.As(err, &internalErr) {
		if _, ok := permanentReporterCodes[internalErr.Code]; ok {
			return true
		}
	}

	return false
}
