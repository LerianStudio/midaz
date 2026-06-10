// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build unit

package rabbitmq

import (
	"testing"

	pkgRabbitmq "github.com/LerianStudio/midaz/v4/pkg/reporter/rabbitmq"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	amqp091 "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
)

// Tests for tenant ID extraction — now delegated to pkgRabbitmq.ExtractTenantID.

func TestConsumer_Tenant_ExtractedFromXTenantIDHeader(t *testing.T) {
	t.Parallel()

	headers := amqp091.Table{"X-Tenant-ID": "tenant-from-producer"}
	ctx := pkgRabbitmq.ExtractTenantID(t.Context(), headers)

	tenantID := tmcore.GetTenantIDContext(ctx)
	assert.Equal(t, "tenant-from-producer", tenantID)
}

func TestConsumer_Tenant_NotInjectedWhenHeaderAbsent(t *testing.T) {
	t.Parallel()

	headers := amqp091.Table{"x-request-id": "some-request-id"}
	ctx := pkgRabbitmq.ExtractTenantID(t.Context(), headers)

	tenantID := tmcore.GetTenantIDContext(ctx)
	assert.Empty(t, tenantID)
}

func TestConsumer_Tenant_NotInjectedWhenHeaderIsNilTable(t *testing.T) {
	t.Parallel()

	ctx := pkgRabbitmq.ExtractTenantID(t.Context(), nil)
	tenantID := tmcore.GetTenantIDContext(ctx)
	assert.Empty(t, tenantID)
}

func TestConsumer_Tenant_MultipleMessages_IndependentContexts(t *testing.T) {
	t.Parallel()

	headersA := amqp091.Table{"X-Tenant-ID": "tenant-A"}
	headersB := amqp091.Table{"X-Tenant-ID": "tenant-B"}

	ctxA := pkgRabbitmq.ExtractTenantID(t.Context(), headersA)
	ctxB := pkgRabbitmq.ExtractTenantID(t.Context(), headersB)

	tenantA := tmcore.GetTenantIDContext(ctxA)
	tenantB := tmcore.GetTenantIDContext(ctxB)

	assert.Equal(t, "tenant-A", tenantA)
	assert.Equal(t, "tenant-B", tenantB)
	assert.NotEqual(t, tenantA, tenantB)
}
