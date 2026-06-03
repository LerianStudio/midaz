// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build unit

package rabbitmq

import (
	"testing"

	pkgRabbitmq "github.com/LerianStudio/midaz/v3/pkg/reporter/rabbitmq"

	"github.com/stretchr/testify/assert"
)

// Tests for producer header building — now delegated to pkgRabbitmq.NewProducerHeaders.

func TestProducer_HeadersContainTenantID_WhenTenantInContext(t *testing.T) {
	t.Parallel()

	headers := pkgRabbitmq.NewProducerHeaders("req-1", "tenant-abc")

	val, ok := headers["X-Tenant-ID"]
	assert.True(t, ok, "X-Tenant-ID header must be present when tenant ID is provided")
	assert.Equal(t, "tenant-abc", val)
}

func TestProducer_NoTenantHeader_WhenNoTenantContext(t *testing.T) {
	t.Parallel()

	headers := pkgRabbitmq.NewProducerHeaders("req-2", "")

	_, ok := headers["X-Tenant-ID"]
	assert.False(t, ok, "X-Tenant-ID header must NOT be present when tenant ID is empty")
}

func TestProducer_EmptyTenantIDNotInjected(t *testing.T) {
	t.Parallel()

	headers := pkgRabbitmq.NewProducerHeaders("req-3", "")

	_, ok := headers["X-Tenant-ID"]
	assert.False(t, ok, "X-Tenant-ID header must NOT be present when tenant ID is empty string")
}
