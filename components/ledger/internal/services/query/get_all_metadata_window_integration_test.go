// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package query

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

const windowMetadataTag = "window-tag"

// windowMetadataSeedTime is the created_at/updated_at of every seeded metadata document.
var windowMetadataSeedTime = time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)

// windowMetadataEnv holds the use case wired to real PostgreSQL and MongoDB stores and the
// identifiers of the seeded data set. No document is rewritten after seeding.
type windowMetadataEnv struct {
	uc *UseCase

	orgA     uuid.UUID
	ledgerA1 uuid.UUID
	ledgerA2 uuid.UUID
	orgB     uuid.UUID
	ledgerB1 uuid.UUID

	// a1Deleted is the soft-deleted A1 portfolio whose metadata document is still present; its id
	// sorts before every other A1 portfolio.
	a1Deleted uuid.UUID
	// a1Inactive is the A1 portfolio whose status is INACTIVE.
	a1Inactive uuid.UUID
	// a1Live is every live A1 portfolio, in id order, a1Inactive among them.
	a1Live []uuid.UUID
	// a2Portfolio is the A2 portfolio carrying the same tag.
	a2Portfolio uuid.UUID
	// b1Portfolio is the org B portfolio carrying the same tag.
	b1Portfolio uuid.UUID
}

// setupWindowMetadataEnv seeds, in id order, one tagged metadata document per portfolio:
//   - org A / ledger A1: a soft-deleted portfolio and three live portfolios (the second INACTIVE);
//   - org A / ledger A2: one portfolio;
//   - org B / ledger B1: one portfolio.
//
// The metadata store holds no organization or ledger, so every listing walks all six documents and
// PostgreSQL alone decides which rows belong to the route.
func setupWindowMetadataEnv(t *testing.T) *windowMetadataEnv {
	t.Helper()

	var (
		pgContainer    *pgtestutil.ContainerResult
		mongoContainer *mongotestutil.ContainerResult
		wg             sync.WaitGroup
	)

	wg.Add(2)

	go func() {
		defer wg.Done()

		pgContainer = pgtestutil.SetupMigratedContainer(t, "onboarding")
	}()

	go func() {
		defer wg.Done()

		mongoContainer = mongotestutil.SetupReusableContainer(t)
	}()

	wg.Wait()

	require.NotNil(t, pgContainer, "postgres container must start")
	require.NotNil(t, mongoContainer, "mongo container must start")

	connStr := pgtestutil.BuildConnectionString(pgContainer.Host, pgContainer.Port, pgContainer.Config)
	pgConn := pgtestutil.ConnectPostgresClient(t.Context(), t, connStr, connStr)
	mongoConn := mongotestutil.CreateConnection(t, mongoContainer.URI, mongoContainer.DBName)

	env := &windowMetadataEnv{
		uc: &UseCase{
			PortfolioRepo:          portfolio.NewPortfolioPostgreSQLRepository(pgConn),
			OnboardingMetadataRepo: mongodb.NewMetadataMongoDBRepository(mongoConn),
		},
	}

	db := pgContainer.DB

	env.orgA = pgtestutil.CreateTestOrganization(t, db)
	env.ledgerA1 = pgtestutil.CreateTestLedger(t, db, env.orgA)
	env.ledgerA2 = pgtestutil.CreateTestLedger(t, db, env.orgA)
	env.orgB = pgtestutil.CreateTestOrganization(t, db)
	env.ledgerB1 = pgtestutil.CreateTestLedger(t, db, env.orgB)

	deletedAt := windowMetadataSeedTime.Add(time.Hour)
	deletedParams := pgtestutil.DefaultPortfolioParams()
	deletedParams.DeletedAt = &deletedAt
	env.a1Deleted = pgtestutil.CreateTestPortfolioWithParams(t, db, env.orgA, env.ledgerA1, deletedParams)

	for i := range 3 {
		params := pgtestutil.DefaultPortfolioParams()
		if i == 1 {
			params.Status = "INACTIVE"
		}

		id := pgtestutil.CreateTestPortfolioWithParams(t, db, env.orgA, env.ledgerA1, params)
		if i == 1 {
			env.a1Inactive = id
		}

		env.a1Live = append(env.a1Live, id)
	}

	env.a2Portfolio = pgtestutil.CreateTestPortfolio(t, db, env.orgA, env.ledgerA2)
	env.b1Portfolio = pgtestutil.CreateTestPortfolio(t, db, env.orgB, env.ledgerB1)

	seeded := slices.Concat([]uuid.UUID{env.a1Deleted}, env.a1Live, []uuid.UUID{env.a2Portfolio, env.b1Portfolio})
	require.True(t, slices.IsSortedFunc(seeded, compareUUIDs),
		"portfolio ids are time-ordered, so the deleted portfolio sorts first")

	fixtures := make([]mongotestutil.MetadataFixture, len(seeded))
	for i, id := range seeded {
		fixtures[i] = windowPortfolioFixture(id)
	}

	mongotestutil.InsertManyMetadata(t, mongoContainer.Database, strings.ToLower(constant.EntityPortfolio), fixtures)

	return env
}

func windowPortfolioFixture(entityID uuid.UUID) mongotestutil.MetadataFixture {
	return mongotestutil.MetadataFixture{
		EntityID:   entityID.String(),
		EntityName: constant.EntityPortfolio,
		Data:       map[string]any{"tag": windowMetadataTag},
		CreatedAt:  windowMetadataSeedTime,
		UpdatedAt:  windowMetadataSeedTime,
	}
}

func windowMetadataFilter(tag string, limit, page int) http.QueryHeader {
	return http.QueryHeader{
		Metadata:    &bson.M{"metadata.tag": tag},
		UseMetadata: true,
		Limit:       limit,
		Page:        page,
		SortOrder:   "asc",
	}
}

func compareUUIDs(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) }

func portfolioIDs(portfolios []*mmodel.Portfolio) []string {
	out := make([]string, len(portfolios))
	for i, p := range portfolios {
		out[i] = p.ID
	}

	return out
}

func uuidStrings(ids []uuid.UUID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}

	return out
}

// listPages reads pages 1..n of the A1 listing.
func (env *windowMetadataEnv) listPages(t *testing.T, filter http.QueryHeader, n int) [][]string {
	t.Helper()

	pages := make([][]string, 0, n)

	for page := 1; page <= n; page++ {
		filter.Page = page

		portfolios, err := env.uc.GetAllMetadataPortfolios(context.Background(), env.orgA, env.ledgerA1, filter)
		require.NoError(t, err)
		require.NotNil(t, portfolios, "every page is a non-nil slice")

		for _, p := range portfolios {
			assert.Equal(t, env.ledgerA1.String(), p.LedgerID)
			assert.Equal(t, windowMetadataTag, p.Metadata["tag"], "metadata of portfolio %s must be attached", p.ID)
		}

		pages = append(pages, portfolioIDs(portfolios))
	}

	return pages
}

func TestIntegration_GetAllMetadataPortfolios_ListsLiveMatchesOfTheLedger(t *testing.T) {
	env := setupWindowMetadataEnv(t)

	pages := env.listPages(t, windowMetadataFilter(windowMetadataTag, 10, 1), 1)

	assert.Equal(t, uuidStrings(env.a1Live), pages[0], "exactly the live A1 portfolios, in id order")
	assert.NotContains(t, pages[0], env.a1Deleted.String(), "a soft-deleted portfolio is never listed")
	assert.NotContains(t, pages[0], env.a2Portfolio.String(), "another ledger's portfolio is excluded")
	assert.NotContains(t, pages[0], env.b1Portfolio.String(), "another organization's portfolio is excluded")
}

func TestIntegration_GetAllMetadataPortfolios_PagesOverLiveRowsOnly(t *testing.T) {
	env := setupWindowMetadataEnv(t)

	pages := env.listPages(t, windowMetadataFilter(windowMetadataTag, 2, 1), 3)

	assert.Equal(t, uuidStrings(env.a1Live[:2]), pages[0], "page 1 is full although the deleted portfolio sorts first")
	assert.Equal(t, uuidStrings(env.a1Live[2:]), pages[1], "page 2 holds the one remaining live portfolio")
	assert.Empty(t, pages[2], "a page past the live matches is empty, other ledgers' documents notwithstanding")
}

func TestIntegration_GetAllMetadataPortfolios_DescendingOrder(t *testing.T) {
	env := setupWindowMetadataEnv(t)

	filter := windowMetadataFilter(windowMetadataTag, 2, 1)
	filter.SortOrder = "desc"

	desc := slices.Clone(env.a1Live)
	slices.Reverse(desc)

	pages := env.listPages(t, filter, 3)

	assert.Equal(t, uuidStrings(desc[:2]), pages[0], "the other ledgers' portfolios, first in desc order, take no slot")
	assert.Equal(t, uuidStrings(desc[2:]), pages[1])
	assert.Empty(t, pages[2], "the deleted portfolio, last in desc order, is never listed")
}

func TestIntegration_GetAllMetadataPortfolios_CombinedFilterPagesOverItsMatches(t *testing.T) {
	env := setupWindowMetadataEnv(t)

	active := slices.DeleteFunc(slices.Clone(env.a1Live), func(id uuid.UUID) bool { return id == env.a1Inactive })

	t.Run("status", func(t *testing.T) {
		status := "ACTIVE"
		filter := windowMetadataFilter(windowMetadataTag, 1, 1)
		filter.Status = &status

		pages := env.listPages(t, filter, 3)

		assert.Equal(t, uuidStrings(active[:1]), pages[0], "the INACTIVE portfolio takes no slot")
		assert.Equal(t, uuidStrings(active[1:]), pages[1])
		assert.Empty(t, pages[2])
	})

	t.Run("created_at range holding every row", func(t *testing.T) {
		filter := windowMetadataFilter(windowMetadataTag, 2, 1)
		filter.StartDate = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
		filter.EndDate = time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)

		pages := env.listPages(t, filter, 2)

		assert.Equal(t, uuidStrings(env.a1Live[:2]), pages[0])
		assert.Equal(t, uuidStrings(env.a1Live[2:]), pages[1])
	})

	t.Run("created_at range holding no row", func(t *testing.T) {
		filter := windowMetadataFilter(windowMetadataTag, 2, 1)
		filter.StartDate = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
		filter.EndDate = time.Date(1970, 1, 2, 0, 0, 0, 0, time.UTC)

		pages := env.listPages(t, filter, 1)

		assert.Empty(t, pages[0])
	})
}

func TestIntegration_GetAllMetadataPortfolios_OtherLedgerListsOnlyItsOwn(t *testing.T) {
	env := setupWindowMetadataEnv(t)

	portfolios, err := env.uc.GetAllMetadataPortfolios(context.Background(), env.orgA, env.ledgerA2,
		windowMetadataFilter(windowMetadataTag, 10, 1))
	require.NoError(t, err)

	assert.Equal(t, []string{env.a2Portfolio.String()}, portfolioIDs(portfolios), "A2 lists its own match and none of A1's or B1's")
}

func TestIntegration_GetAllMetadataPortfolios_UnmatchedTagReturnsEmptyPage(t *testing.T) {
	env := setupWindowMetadataEnv(t)

	portfolios, err := env.uc.GetAllMetadataPortfolios(context.Background(), env.orgA, env.ledgerA1,
		windowMetadataFilter("tag-nobody-has", 10, 1))
	require.NoError(t, err)
	require.NotNil(t, portfolios, "an empty page must be a non-nil slice")
	assert.Empty(t, portfolios)
}
