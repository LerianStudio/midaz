//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"fmt"
	"testing"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestIntegration_GetAllAccount_PaginationUnion verifies that the union of all
// paginated pages equals the full set of items, with no duplicates.
func TestIntegration_GetAllAccount_PaginationUnion(t *testing.T) {
	// Setup container
	container := pgtestutil.SetupContainer(t)

	// Setup repository and use case
	logger := libZap.InitializeLogger()
	migrationsPath := pgtestutil.FindMigrationsPath(t, "onboarding")
	connStr := pgtestutil.BuildConnectionString(container.Host, container.Port, container.Config)

	conn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: connStr,
		ConnectionStringReplica: connStr,
		PrimaryDBName:           container.Config.DBName,
		ReplicaDBName:           container.Config.DBName,
		MigrationsPath:          migrationsPath,
		Logger:                  logger,
	}

	accountRepo := account.NewAccountPostgreSQLRepository(conn)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockMetadata := mongodb.NewMockRepository(ctrl)
	mockMetadata.EXPECT().FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockMetadata.EXPECT().FindByEntityIDs(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	uc := &UseCase{
		AccountRepo:  accountRepo,
		MetadataRepo: mockMetadata,
	}

	ctx := context.Background()

	// Setup: org + ledger + asset + 7 accounts
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	pgtestutil.CreateTestAsset(t, container.DB, orgID, ledgerID, "USD")

	createdAliases := make([]string, 7)
	for i := 0; i < 7; i++ {
		alias := fmt.Sprintf("pagtest-%02d", i)
		createdAliases[i] = alias
		pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil,
			fmt.Sprintf("Account %d", i), alias, "USD", nil)
	}

	// Fetch pages with limit=3 (expect 3 pages: 3 + 3 + 1)
	allAliases := make([]string, 0, 7)
	seenAliases := make(map[string]int)

	for page := 1; page <= 3; page++ {
		filter := http.QueryHeader{
			Limit:     3,
			Page:      page,
			SortOrder: "asc",
			StartDate: time.Now().Add(-24 * time.Hour),
			EndDate:   time.Now().Add(24 * time.Hour),
		}

		accounts, err := uc.GetAllAccount(ctx, orgID, ledgerID, nil, filter)
		require.NoError(t, err, "GetAllAccount page %d should succeed", page)

		for _, acc := range accounts {
			if acc.Alias != nil {
				alias := *acc.Alias
				allAliases = append(allAliases, alias)
				seenAliases[alias]++
			}
		}
	}

	// Assert: no duplicates across pages
	for alias, count := range seenAliases {
		assert.Equal(t, 1, count, "alias %s should appear exactly once, found %d times", alias, count)
	}

	// Assert: all created aliases are present
	for _, wantAlias := range createdAliases {
		assert.Contains(t, seenAliases, wantAlias, "created alias %s should be in paginated results", wantAlias)
	}
}

// TestIntegration_GetAllAccount_PaginationStableOrder verifies that consecutive reads of the
// same page return items in the same order.
func TestIntegration_GetAllAccount_PaginationStableOrder(t *testing.T) {
	// Setup container
	container := pgtestutil.SetupContainer(t)

	// Setup repository and use case
	logger := libZap.InitializeLogger()
	migrationsPath := pgtestutil.FindMigrationsPath(t, "onboarding")
	connStr := pgtestutil.BuildConnectionString(container.Host, container.Port, container.Config)

	conn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: connStr,
		ConnectionStringReplica: connStr,
		PrimaryDBName:           container.Config.DBName,
		ReplicaDBName:           container.Config.DBName,
		MigrationsPath:          migrationsPath,
		Logger:                  logger,
	}

	accountRepo := account.NewAccountPostgreSQLRepository(conn)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockMetadata := mongodb.NewMockRepository(ctrl)
	mockMetadata.EXPECT().FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockMetadata.EXPECT().FindByEntityIDs(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	uc := &UseCase{
		AccountRepo:  accountRepo,
		MetadataRepo: mockMetadata,
	}

	ctx := context.Background()

	// Setup: org + ledger + asset + accounts
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	pgtestutil.CreateTestAsset(t, container.DB, orgID, ledgerID, "USD")

	for i := 0; i < 5; i++ {
		alias := fmt.Sprintf("stable-%02d", i)
		pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil,
			fmt.Sprintf("Account %d", i), alias, "USD", nil)
	}

	filter := http.QueryHeader{
		Limit:     3,
		Page:      1,
		SortOrder: "asc",
		StartDate: time.Now().Add(-24 * time.Hour),
		EndDate:   time.Now().Add(24 * time.Hour),
	}

	// First read
	accounts1, err := uc.GetAllAccount(ctx, orgID, ledgerID, nil, filter)
	require.NoError(t, err, "first GetAllAccount should succeed")

	// Second read
	accounts2, err := uc.GetAllAccount(ctx, orgID, ledgerID, nil, filter)
	require.NoError(t, err, "second GetAllAccount should succeed")

	// Assert: same length
	require.Equal(t, len(accounts1), len(accounts2), "both reads should return same number of items")

	// Assert: same order
	for i := range accounts1 {
		assert.Equal(t, accounts1[i].ID, accounts2[i].ID,
			"item %d should have same ID in both reads: %s vs %s",
			i, accounts1[i].ID, accounts2[i].ID)
	}
}
