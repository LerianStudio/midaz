//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestReusableContainerSupportsPackageLongDatabaseChurn(t *testing.T) {
	container := SetupReusableContainer(t)

	exitCode, output, err := container.Container.Exec(context.Background(), []string{"sh", "-c", "ulimit -n"}, tcexec.Multiplexed())
	require.NoError(t, err)
	require.Zero(t, exitCode)

	limitOutput, err := io.ReadAll(output)
	require.NoError(t, err)
	limit, err := strconv.ParseUint(strings.TrimSpace(string(limitOutput)), 10, 64)
	require.NoError(t, err)
	require.GreaterOrEqual(t, limit, uint64(65536), "MongoDB must retain enough file descriptors for package-long database churn")
}

func TestReusableContainerIsolatesDatabases(t *testing.T) {
	first := SetupReusableContainer(t)
	second := SetupReusableContainer(t)

	require.Equal(t, first.Container.GetContainerID(), second.Container.GetContainerID())
	require.NotEqual(t, first.DBName, second.DBName)

	_, err := first.Database.Collection("owners").InsertOne(context.Background(), bson.M{"owner": "first"})
	require.NoError(t, err)

	count, err := second.Database.Collection("owners").CountDocuments(context.Background(), bson.M{})
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestReusableContainerDropsTheOwningTestsDatabase(t *testing.T) {
	observer := SetupReusableContainer(t)

	var ownedDatabase string
	require.True(t, t.Run("owner", func(t *testing.T) {
		owned := SetupReusableContainer(t)
		ownedDatabase = owned.DBName
		require.NoError(t, owned.Database.CreateCollection(context.Background(), "owned"))
	}))

	databaseNames, err := observer.Client.ListDatabaseNames(context.Background(), bson.M{"name": ownedDatabase})
	require.NoError(t, err)
	require.Empty(t, databaseNames)
}

func TestCreateOwnedDatabaseDropsAdditionalTenantDatabase(t *testing.T) {
	observer := SetupReusableContainer(t)

	var ownedDatabase string
	require.True(t, t.Run("tenant-owner", func(t *testing.T) {
		database := CreateOwnedDatabase(t, observer)
		ownedDatabase = database.Name()
		require.NoError(t, database.CreateCollection(context.Background(), "owned"))
	}))

	databaseNames, err := observer.Client.ListDatabaseNames(context.Background(), bson.M{"name": ownedDatabase})
	require.NoError(t, err)
	require.Empty(t, databaseNames)
}
