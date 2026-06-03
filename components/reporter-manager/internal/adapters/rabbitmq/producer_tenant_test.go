// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build unit

package rabbitmq

import (
	"context"
	"testing"

	pkgRabbitmq "github.com/LerianStudio/midaz/v3/pkg/reporter/rabbitmq"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/stretchr/testify/assert"
)

func TestProducer_Tenant_HeaderInjectedIntoNewProducerHeaders(t *testing.T) {
	t.Parallel()

	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant123")
	tenantID := tmcore.GetTenantIDContext(ctx)
	assert.Equal(t, "tenant123", tenantID)

	headers := pkgRabbitmq.NewProducerHeaders("req-1", tenantID)

	val, ok := headers["X-Tenant-ID"]
	assert.True(t, ok, "X-Tenant-ID must be present in headers when tenant is in context")
	assert.Equal(t, "tenant123", val)
}

func TestProducer_Tenant_HeaderAbsentWithoutContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tenantID := tmcore.GetTenantIDContext(ctx)
	assert.Empty(t, tenantID)

	headers := pkgRabbitmq.NewProducerHeaders("req-2", tenantID)

	_, ok := headers["X-Tenant-ID"]
	assert.False(t, ok, "X-Tenant-ID must NOT be present when no tenant in context")
}

func TestProducer_Tenant_ContextRoundTrip(t *testing.T) {
	t.Parallel()

	tenants := []string{"org-abc", "org-xyz", "acme-corp", "tenant-001"}

	for _, tenant := range tenants {
		t.Run(tenant, func(t *testing.T) {
			t.Parallel()

			ctx := tmcore.ContextWithTenantID(context.Background(), tenant)
			got := tmcore.GetTenantIDContext(ctx)
			assert.Equal(t, tenant, got)
		})
	}
}
