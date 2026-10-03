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

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// TestIntegration_GetAllMetadataAccounts_PagesOverTheConfinedSet seeds more matching
// accounts than a page holds, confines a partner to some of them, and requires the
// metadata listing to page over what the partner may see: a full first page while
// enough allowed accounts match, and every allowed account exactly once.
func TestIntegration_GetAllMetadataAccounts_PagesOverTheConfinedSet(t *testing.T) {
	pg := pgtestutil.SetupMigratedContainer(t, "onboarding")
	connStr := pgtestutil.BuildConnectionString(pg.Host, pg.Port, pg.Config)
	conn := pgtestutil.ConnectPostgresClient(t.Context(), t, connStr, connStr)

	mongo := mongotestutil.SetupReusableContainer(t)
	metadataRepo := mongodb.NewMetadataMongoDBRepository(mongotestutil.CreateConnection(t, mongo.URI, mongo.DBName))

	uc := &UseCase{AccountRepo: account.NewAccountPostgreSQLRepository(conn), OnboardingMetadataRepo: metadataRepo}
	ctx := context.Background()

	orgID := pgtestutil.CreateTestOrganization(t, pg.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, pg.DB, orgID)
	tag := uuid.NewString()

	var all, allowed []uuid.UUID

	for i := range 25 {
		id := pgtestutil.CreateTestAccount(t, pg.DB, orgID, ledgerID, nil, fmt.Sprintf("Meta %d", i), fmt.Sprintf("@meta-%02d", i), "USD", nil)
		require.NoError(t, metadataRepo.Create(ctx, constant.EntityAccount, &mongodb.Metadata{
			EntityID: id.String(), EntityName: constant.EntityAccount,
			Data: mongodb.JSON{"tag": tag}, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}))

		all = append(all, id)
		if i%2 == 0 {
			allowed = append(allowed, id)
		}
	}

	list := func(page int, scope http.ScopeConfinement) []uuid.UUID {
		t.Helper()

		accounts, err := uc.GetAllMetadataAccounts(ctx, orgID, ledgerID, nil, nil, http.QueryHeader{
			Metadata: &bson.M{"metadata.tag": tag}, UseMetadata: true,
			Limit: 10, Page: page, SortOrder: "asc",
			StartDate: time.Now().Add(-24 * time.Hour), EndDate: time.Now().Add(24 * time.Hour),
			Scope: scope,
		}, mmodel.HolderOnV2)
		require.NoError(t, err)

		ids := make([]uuid.UUID, 0, len(accounts))
		for _, acc := range accounts {
			ids = append(ids, uuid.MustParse(acc.ID))
		}

		return ids
	}

	t.Run("a confined listing fills its first page and lists every allowed account once", func(t *testing.T) {
		scope := http.ScopeConfinement{"accountId": allowed}

		page1, page2 := list(1, scope), list(2, scope)
		assert.Len(t, page1, 10, "13 allowed accounts match, so the first page is full")
		assert.Len(t, page2, 3)
		assert.ElementsMatch(t, allowed, append(page1, page2...))
	})

	t.Run("an unconfined listing pages over every match", func(t *testing.T) {
		page1, page2, page3 := list(1, nil), list(2, nil), list(3, nil)
		assert.Len(t, page1, 10)
		assert.Len(t, page2, 10)
		assert.Len(t, page3, 5)
		assert.ElementsMatch(t, all, append(append(page1, page2...), page3...))
	})
}
