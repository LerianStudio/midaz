// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"maps"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"

	libConstants "github.com/LerianStudio/lib-commons/v5/commons/constants"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	amqp "github.com/rabbitmq/amqp091-go"
)

// GetRetryCount reads the retry count from RabbitMQ message headers.
// Returns 0 if the header is missing or cannot be parsed, ensuring safe default
// behavior for messages that have not been retried yet.
//
// RabbitMQ headers can store values as different numeric types depending on
// the publisher and serialization. All common variants are handled safely.
func GetRetryCount(headers amqp.Table) int {
	if headers == nil {
		return 0
	}

	val, exists := headers[constant.RetryCountHeader]
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
// and failure reason. This ensures tracing headers (e.g., traceparent)
// and request IDs survive across retries.
func BuildRetryHeaders(original amqp.Table, currentRetryCount int, failureReason string) amqp.Table {
	headers := make(amqp.Table, len(original)+2)
	maps.Copy(headers, original) // safe with nil source (no-op)

	headers[constant.RetryCountHeader] = currentRetryCount + 1
	headers[constant.RetryFailureReasonHeader] = failureReason

	return headers
}

// NewProducerHeaders constructs the base AMQP header table for a new message.
// It sets the request-ID header and initializes the retry count to 0.
// When tenantID is non-empty (multi-tenant mode), the X-Tenant-ID header is
// included so the worker consumer can propagate tenant context downstream.
func NewProducerHeaders(reqID string, tenantID string) amqp.Table {
	headers := amqp.Table{
		libConstants.HeaderID:     reqID,
		constant.RetryCountHeader: 0,
	}

	if tenantID != "" {
		headers[constant.HeaderXTenantID] = tenantID
	}

	return headers
}

// ExtractTenantID reads the X-Tenant-ID header from AMQP message headers
// and, if present and non-empty, stores the tenant ID in the returned context
// using the lib-commons tenant-manager API.
//
// When the header is absent or not a string (e.g. legacy single-tenant messages),
// the context is returned unchanged, preserving full backward compatibility.
func ExtractTenantID(ctx context.Context, headers amqp.Table) context.Context {
	if headers == nil {
		return ctx
	}

	if tenantID, ok := headers[constant.HeaderXTenantID].(string); ok && tenantID != "" {
		return tmcore.ContextWithTenantID(ctx, tenantID)
	}

	return ctx
}

// TenantIDFromHeaders extracts the tenant ID string from AMQP headers
// without modifying context. Returns empty string if not present.
func TenantIDFromHeaders(headers amqp.Table) string {
	if headers == nil {
		return ""
	}

	tenantID, _ := headers[constant.HeaderXTenantID].(string)

	return tenantID
}
