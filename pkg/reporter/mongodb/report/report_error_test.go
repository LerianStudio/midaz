// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build unit

package report_test

import (
	"context"
	"testing"

	tmCore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"

	libMongo "github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb"
)

func TestResolveDatabase_FailsClosed_WhenTenantIDExistsWithoutMongoDatabase(t *testing.T) {
	t.Parallel()

	ctx := tmCore.ContextWithTenantID(context.Background(), "tenant-orphan")

	db, err := libMongo.ResolveDatabase(ctx, nil, "reporter")
	require.Error(t, err)
	assert.Nil(t, db)
	assert.ErrorIs(t, err, tmCore.ErrTenantContextRequired)
}

func TestResolveDatabase_UsesTenantMongoWhenPresent(t *testing.T) {
	t.Parallel()

	client, err := mongo.Connect()
	require.NoError(t, err)

	tenantDB := client.Database("tenant_a")
	ctx := tmCore.ContextWithTenantID(context.Background(), "tenant-a")
	ctx = tmCore.ContextWithMB(ctx, tenantDB)

	db, err := libMongo.ResolveDatabase(ctx, nil, "reporter")
	require.NoError(t, err)
	assert.Equal(t, tenantDB, db)
}
