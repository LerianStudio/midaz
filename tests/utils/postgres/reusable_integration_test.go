//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReusableMigratedContainerIsolatesDatabases(t *testing.T) {
	first := SetupMigratedContainer(t, "onboarding")
	second := SetupMigratedContainer(t, "onboarding")

	require.Equal(t, first.Container.GetContainerID(), second.Container.GetContainerID())
	require.NotEqual(t, first.Config.DBName, second.Config.DBName)

	CreateTestOrganization(t, first.DB)

	var count int
	require.NoError(t, second.DB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM organization`).Scan(&count))
	require.Zero(t, count)
}

func TestReusableMigratedContainerDropsTheOwningTestsDatabase(t *testing.T) {
	observer := SetupMigratedContainer(t, "onboarding")

	var ownedDatabase string
	require.True(t, t.Run("owner", func(t *testing.T) {
		owned := SetupMigratedContainer(t, "onboarding")
		ownedDatabase = owned.Config.DBName
	}))

	var exists bool
	require.NoError(t, observer.DB.QueryRowContext(
		context.Background(),
		`SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`,
		ownedDatabase,
	).Scan(&exists))
	require.False(t, exists)
}
