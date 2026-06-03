// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build unit

package rabbitmq

import (
	"testing"

	pkgRabbitmq "github.com/LerianStudio/reporter/pkg/rabbitmq"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	amqp091 "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
)

// TestConsumer_ExtractsTenantID_FromHeaders verifies that when an AMQP message
// carries an X-Tenant-ID header, the tenant ID is correctly extracted and stored
// in the context so downstream repository calls can use tenant-scoped connections.
func TestConsumer_ExtractsTenantID_FromHeaders(t *testing.T) {
	t.Parallel()

	headers := amqp091.Table{
		"X-Tenant-ID": "tenant-xyz",
	}

	ctx := pkgRabbitmq.ExtractTenantID(t.Context(), headers)

	tenantID := tmcore.GetTenantIDContext(ctx)
	assert.Equal(t, "tenant-xyz", tenantID, "tenant ID must be propagated from AMQP header into context")
}

// TestConsumer_NoTenantInContext_WhenHeaderAbsent verifies backward compatibility.
func TestConsumer_NoTenantInContext_WhenHeaderAbsent(t *testing.T) {
	t.Parallel()

	headers := amqp091.Table{
		"x-retry-count": 0,
	}

	ctx := pkgRabbitmq.ExtractTenantID(t.Context(), headers)

	tenantID := tmcore.GetTenantIDContext(ctx)
	assert.Equal(t, "", tenantID, "tenant ID must be empty when X-Tenant-ID header is absent")
}

// TestConsumer_NoTenantInContext_WhenHeaderEmpty verifies blank header is ignored.
func TestConsumer_NoTenantInContext_WhenHeaderEmpty(t *testing.T) {
	t.Parallel()

	headers := amqp091.Table{
		"X-Tenant-ID": "",
	}

	ctx := pkgRabbitmq.ExtractTenantID(t.Context(), headers)

	tenantID := tmcore.GetTenantIDContext(ctx)
	assert.Equal(t, "", tenantID, "an empty X-Tenant-ID header must not inject a blank tenant ID")
}

// TestConsumer_NoTenantInContext_WhenHeaderWrongType verifies non-string header is safe.
func TestConsumer_NoTenantInContext_WhenHeaderWrongType(t *testing.T) {
	t.Parallel()

	headers := amqp091.Table{
		"X-Tenant-ID": 12345,
	}

	ctx := pkgRabbitmq.ExtractTenantID(t.Context(), headers)

	tenantID := tmcore.GetTenantIDContext(ctx)
	assert.Equal(t, "", tenantID, "a non-string X-Tenant-ID header must not crash or inject garbage")
}
