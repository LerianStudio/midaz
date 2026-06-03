// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

import "time"

// RabbitMQ Consumer Retry Configuration
const (
	// MaxMessageRetries is the maximum number of retry attempts before sending to DLQ.
	MaxMessageRetries = 5

	// RetryInitialBackoff is the base delay for exponential backoff calculation.
	RetryInitialBackoff = 1 * time.Second

	// RetryMaxBackoff is the upper bound for the backoff delay.
	RetryMaxBackoff = 30 * time.Second

	// RetryJitterMax is the maximum random jitter added to backoff to prevent thundering herd.
	RetryJitterMax = 500 * time.Millisecond

	// RetryCountHeader is the RabbitMQ message header key for tracking retry attempts.
	RetryCountHeader = "x-retry-count"

	// RetryFailureReasonHeader is the RabbitMQ message header key for tracking the last failure reason.
	RetryFailureReasonHeader = "x-failure-reason"

	// RetryFailureReasonMaxLen is the maximum length for the failure reason stored in message headers.
	// Truncation prevents leaking internal infrastructure details (e.g., connection strings from DB driver errors).
	RetryFailureReasonMaxLen = 256
)

// RabbitMQ Producer Retry Configuration (midaz-style reconnection)
const (
	// ProducerMaxRetries is the maximum number of publish retry attempts before giving up.
	ProducerMaxRetries = 5

	// ProducerInitialBackoff is the initial delay before the first retry attempt.
	ProducerInitialBackoff = 500 * time.Millisecond

	// ProducerMaxBackoff is the upper bound for the producer retry backoff delay.
	ProducerMaxBackoff = 10 * time.Second

	// ProducerBackoffFactor is the multiplier applied to the backoff on each successive retry.
	ProducerBackoffFactor = 2.0
)

// RabbitMQ Connection Monitor Configuration
const (
	// ConnectionMonitorInterval is the period between background RabbitMQ health checks.
	ConnectionMonitorInterval = 10 * time.Second
)
