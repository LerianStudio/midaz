// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"errors"

	"github.com/LerianStudio/midaz/v4/pkg"
)

// ErrorClassifier classifies errors for retry eligibility in RabbitMQ message processing.
// Implementations determine whether a failed message should be retried or sent to the DLQ
// and produce a machine-readable reason recorded on the retry headers.
type ErrorClassifier interface {
	// IsRetryable returns true if the error is transient and the message should be retried.
	IsRetryable(err error) bool
	// ClassifyFailureReason returns a machine-readable failure reason string for headers.
	ClassifyFailureReason(err error) string
}

// DefaultClassifier is the generic, domain-agnostic error classifier.
//
// Business/domain errors (pkg.IsBusinessError) are permanent: retrying will not change
// the outcome. context.Canceled and context.DeadlineExceeded are retryable transient
// conditions. Every other error — including unknown errors — is treated as retryable;
// this is the conservative posture, bounded by the retry manager's MaxRetries gate so a
// genuinely permanent-but-unrecognized failure still terminates at the DLQ.
//
// Domain classifiers compose on top of this type, adding their own permanent-error checks
// before delegating the residual decision here.
type DefaultClassifier struct{}

// NewDefaultClassifier creates a new DefaultClassifier.
func NewDefaultClassifier() *DefaultClassifier {
	return &DefaultClassifier{}
}

// IsRetryable classifies an error as retryable or non-retryable.
func (c *DefaultClassifier) IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	if pkg.IsBusinessError(err) {
		return false
	}

	return true
}

// ClassifyFailureReason returns a machine-readable failure reason string
// for encoding in message headers during retry republish.
func (c *DefaultClassifier) ClassifyFailureReason(err error) string {
	switch {
	case err == nil:
		return "unknown_error"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	case errors.Is(err, context.Canceled):
		return "context_canceled"
	case pkg.IsBusinessError(err):
		return "business_error"
	default:
		return "retryable_error"
	}
}
