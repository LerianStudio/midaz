// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"maps"

	amqp "github.com/rabbitmq/amqp091-go"
)

// AMQP header keys for retry orchestration and tenant propagation.
//
// These names are part of the on-the-wire contract: in-flight messages already
// carry them. Renaming any of these silently breaks retry-count tracking and
// tenant-aware republish for messages produced before the rename. The
// HeaderKeyLock test asserts the exact byte values.
const (
	// RetryCountHeader is the AMQP message header key tracking retry attempts.
	RetryCountHeader = "x-retry-count"

	// RetryFailureReasonHeader is the AMQP message header key recording the last failure reason.
	RetryFailureReasonHeader = "x-failure-reason"

	// TenantIDHeader is the AMQP header propagating tenant identity across the queue.
	TenantIDHeader = "X-Tenant-ID"
)

// RetryCountFromHeaders reads the retry count from AMQP message headers.
// Returns 0 if the header is missing or cannot be parsed, ensuring safe default
// behavior for messages that have not been retried yet.
//
// AMQP headers can store values as different numeric types depending on the
// publisher and serialization. All common variants are handled safely; negative
// values are clamped to 0.
func RetryCountFromHeaders(headers amqp.Table) int {
	if headers == nil {
		return 0
	}

	val, exists := headers[RetryCountHeader]
	if !exists {
		return 0
	}

	switch v := val.(type) {
	case int:
		if v < 0 {
			return 0
		}

		return v
	case int32:
		if v < 0 {
			return 0
		}

		return int(v)
	case int64:
		if v < 0 {
			return 0
		}

		return int(v)
	case float64:
		if v < 0 {
			return 0
		}

		return int(v)
	default:
		return 0
	}
}

// BuildRetryHeaders creates a new header table for a retry republish.
// It copies all original headers, then overwrites the retry count (incremented)
// and failure reason. This ensures tracing headers (e.g., traceparent) and
// request IDs survive across retries. The original table is never mutated.
func BuildRetryHeaders(original amqp.Table, currentRetryCount int, failureReason string) amqp.Table {
	headers := make(amqp.Table, len(original)+2)
	maps.Copy(headers, original) // safe with nil source (no-op)

	headers[RetryCountHeader] = currentRetryCount + 1
	headers[RetryFailureReasonHeader] = failureReason

	return headers
}

// TenantIDFromHeaders extracts the tenant ID string from AMQP headers without
// modifying context. Returns empty string if the header is absent or not a string.
func TenantIDFromHeaders(headers amqp.Table) string {
	if headers == nil {
		return ""
	}

	tenantID, _ := headers[TenantIDHeader].(string)

	return tenantID
}
