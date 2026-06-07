// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"

	pkgRabbitmq "github.com/LerianStudio/midaz/v4/pkg/rabbitmq"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"

	libConstants "github.com/LerianStudio/lib-commons/v5/commons/constants"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	amqp "github.com/rabbitmq/amqp091-go"
)

// GetRetryCount reads the retry count from RabbitMQ message headers.
// Thin re-export of the generic pkg/rabbitmq helper; retained for reporter call sites.
func GetRetryCount(headers amqp.Table) int {
	return pkgRabbitmq.RetryCountFromHeaders(headers)
}

// BuildRetryHeaders creates a new header table for a retry republish.
// Thin re-export of the generic pkg/rabbitmq helper; retained for reporter call sites.
func BuildRetryHeaders(original amqp.Table, currentRetryCount int, failureReason string) amqp.Table {
	return pkgRabbitmq.BuildRetryHeaders(original, currentRetryCount, failureReason)
}

// TenantIDFromHeaders extracts the tenant ID string from AMQP headers without
// modifying context. Thin re-export of the generic pkg/rabbitmq helper.
func TenantIDFromHeaders(headers amqp.Table) string {
	return pkgRabbitmq.TenantIDFromHeaders(headers)
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
