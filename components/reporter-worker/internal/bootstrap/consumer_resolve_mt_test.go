// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build unit

package bootstrap

import (
	"context"
	"testing"

	tmmongo "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/mongo"
	"github.com/LerianStudio/lib-observability/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveMultiTenantMongo_NilManager_ReturnsOriginalContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mq := &MultiQueueConsumer{
		logger:       &log.NopLogger{},
		mongoManager: nil, // Single-tenant mode — no mongoManager
	}

	resultCtx, err := mq.resolveMultiTenantMongo(ctx)

	require.NoError(t, err, "nil mongoManager must not produce an error")
	assert.Equal(t, ctx, resultCtx, "context must be returned unchanged when mongoManager is nil")
}

func TestResolveMultiTenantMongo_EmptyTenantID_ReturnsOriginalContext(t *testing.T) {
	t.Parallel()

	// Context WITHOUT tenant ID set — simulates single-tenant message.
	// Even with a non-nil mongoManager, empty tenant ID should short-circuit.
	ctx := context.Background()
	mq := &MultiQueueConsumer{
		logger:       &log.NopLogger{},
		mongoManager: &tmmongo.Manager{}, // Non-nil manager, but tenant ID is empty
	}

	resultCtx, err := mq.resolveMultiTenantMongo(ctx)

	require.NoError(t, err, "empty tenant ID must not produce an error")
	assert.Equal(t, ctx, resultCtx, "context must be returned unchanged when tenant ID is empty")
}

func TestRegisterNotificationConsumerMultiTenant_AcceptsMongoManager(t *testing.T) {
	t.Parallel()

	// Verify the function signature accepts a mongoManager parameter.
	// Uses the existing mockMultiTenantConsumer from backward_compat_test.go.
	mockConsumer := &mockMultiTenantConsumer{}

	err := registerNotificationConsumerMultiTenant(
		mockConsumer,
		nil, // service — not needed for signature test
		&log.NopLogger{},
		nil, // mongoManager — nil is valid (single-tenant safe)
	)

	require.NoError(t, err, "registerNotificationConsumerMultiTenant must accept mongoManager parameter")

	mockConsumer.mu.Lock()
	defer mockConsumer.mu.Unlock()

	assert.Len(t, mockConsumer.registerCalls, 1, "Register must be called exactly once")
}

func TestService_ReconcilerCancel_Field_Exists(t *testing.T) {
	t.Parallel()

	// Verify that the Service struct has a reconcilerCancel field.
	svc := &Service{}
	assert.Nil(t, svc.reconcilerCancel, "reconcilerCancel must exist on Service and default to nil")
}
