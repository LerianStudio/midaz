// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	tmmongo "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/mongo"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetadataIndexHandler_MongoManagerSelection(t *testing.T) {
	t.Parallel()

	onboardingManager := &tmmongo.Manager{}
	transactionManager := &tmmongo.Manager{}

	handler := &MetadataIndexHandler{
		OnboardingMongoManager:  onboardingManager,
		TransactionMongoManager: transactionManager,
	}

	assert.Same(t, onboardingManager, handler.getMongoManager("organization"))
	assert.Same(t, onboardingManager, handler.getMongoManager("account"))
	assert.Same(t, transactionManager, handler.getMongoManager("transaction"))
	assert.Same(t, transactionManager, handler.getMongoManager("operation_route"))
	assert.Nil(t, handler.getMongoManager("unknown_entity"))
}

func TestMetadataIndexHandler_ContextHelpers_MissingManager(t *testing.T) {
	t.Parallel()

	handler := &MetadataIndexHandler{}

	type ctxKey string
	ctx := context.WithValue(context.Background(), ctxKey("sentinel"), uuid.NewString())

	ctxNoTenant, err := handler.contextForEntity(ctx, "transaction")
	require.NoError(t, err)
	assert.Equal(t, ctx, ctxNoTenant)

	ctxWithTenant := tmcore.ContextWithTenantID(ctx, "tenant-1")

	ctxEntity, err := handler.contextForEntity(ctxWithTenant, "transaction")
	require.Error(t, err)
	assert.Nil(t, ctxEntity)
	assert.Contains(t, err.Error(), "multi-tenant mongo manager not configured")

	ctxRepoNoTenant, err := handler.contextForRepoGroup(ctx, true)
	require.NoError(t, err)
	assert.Equal(t, ctx, ctxRepoNoTenant)

	ctxRepoOnboarding, err := handler.contextForRepoGroup(ctxWithTenant, true)
	require.Error(t, err)
	assert.Nil(t, ctxRepoOnboarding)
	assert.Contains(t, err.Error(), "onboarding")

	ctxRepoTransaction, err := handler.contextForRepoGroup(ctxWithTenant, false)
	require.Error(t, err)
	assert.Nil(t, ctxRepoTransaction)
	assert.Contains(t, err.Error(), "transaction")
}

func TestMetadataIndexHandler_ContextForEntity_NoTenantWithManagerPresent(t *testing.T) {
	t.Parallel()

	// When manager is configured but no tenant ID is set, should return error.
	handler := &MetadataIndexHandler{
		TransactionMongoManager: &tmmongo.Manager{},
	}

	ctx := context.Background() // no tenant ID

	result, err := handler.contextForEntity(ctx, "transaction")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "tenant id is required")
}

func TestMetadataIndexHandler_ContextForRepoGroup_NoTenantWithManagerPresent(t *testing.T) {
	t.Parallel()

	handler := &MetadataIndexHandler{
		OnboardingMongoManager:  &tmmongo.Manager{},
		TransactionMongoManager: &tmmongo.Manager{},
	}

	ctx := context.Background() // no tenant ID

	result, err := handler.contextForRepoGroup(ctx, true)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "tenant id is required")

	result, err = handler.contextForRepoGroup(ctx, false)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "tenant id is required")
}
