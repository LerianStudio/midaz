//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transactionroute

import (
	"context"
	"database/sql"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

var optionalLinksCreatedAt = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

// optionalLinksFixture is a transaction route linking a required source, a
// required destination and an optional fee destination.
type optionalLinksFixture struct {
	repo                     *TransactionRoutePostgreSQLRepository
	db                       *sql.DB
	orgID, ledgerID          uuid.UUID
	route                    *mmodel.TransactionRoute
	source, destination, fee uuid.UUID
}

func newOptionalLinksFixture(t *testing.T) optionalLinksFixture {
	t.Helper()

	container := pgtestutil.SetupMigratedContainer(t, "transaction")
	f := optionalLinksFixture{
		repo:     createRepository(t, container),
		db:       container.DB,
		orgID:    uuid.Must(libCommons.GenerateUUIDv7()),
		ledgerID: uuid.Must(libCommons.GenerateUUIDv7()),
	}

	f.source = pgtestutil.CreateTestOperationRouteSimple(t, f.db, f.orgID, f.ledgerID, "Source", "source")
	f.destination = pgtestutil.CreateTestOperationRouteSimple(t, f.db, f.orgID, f.ledgerID, "Destination", "destination")
	f.fee = pgtestutil.CreateTestOperationRouteSimple(t, f.db, f.orgID, f.ledgerID, "Fee", "destination")

	route := &mmodel.TransactionRoute{
		ID:                        uuid.Must(libCommons.GenerateUUIDv7()),
		OrganizationID:            f.orgID,
		LedgerID:                  &f.ledgerID,
		Title:                     "Transfer with optional fee",
		OperationRoutes:           []mmodel.OperationRoute{{ID: f.source}, {ID: f.destination}, {ID: f.fee}},
		OptionalOperationRouteIDs: []uuid.UUID{f.fee},
		CreatedAt:                 optionalLinksCreatedAt,
		UpdatedAt:                 optionalLinksCreatedAt,
	}

	created, err := f.repo.Create(context.Background(), f.orgID, &f.ledgerID, route)
	require.NoError(t, err)

	f.route = created

	return f
}

// activeLinks reads the active link rows straight from the table: operation
// route ID to (link ID, optional).
func (f optionalLinksFixture) activeLinks(t *testing.T) map[uuid.UUID]struct {
	id       uuid.UUID
	optional bool
} {
	t.Helper()

	rows, err := f.db.Query(`SELECT id, operation_route_id, optional FROM operation_transaction_route WHERE transaction_route_id = $1 AND deleted_at IS NULL`, f.route.ID)
	require.NoError(t, err)

	defer rows.Close()

	links := map[uuid.UUID]struct {
		id       uuid.UUID
		optional bool
	}{}

	for rows.Next() {
		var linkID, operationRouteID uuid.UUID

		var optional bool

		require.NoError(t, rows.Scan(&linkID, &operationRouteID, &optional))

		links[operationRouteID] = struct {
			id       uuid.UUID
			optional bool
		}{linkID, optional}
	}

	require.NoError(t, rows.Err())

	return links
}

func TestIntegration_TransactionRouteRepository_OptionalLinks_CreateAndFindByID(t *testing.T) {
	f := newOptionalLinksFixture(t)

	found, err := f.repo.FindByID(context.Background(), f.orgID, f.route.ID)
	require.NoError(t, err)

	assert.Len(t, found.OperationRoutes, 3, "OperationRoutes keeps every link, optional ones included")
	assert.ElementsMatch(t, []uuid.UUID{f.fee}, found.OptionalOperationRouteIDs)

	links := f.activeLinks(t)
	assert.True(t, links[f.fee].optional)
	assert.False(t, links[f.source].optional)
	assert.False(t, links[f.destination].optional)
}

func TestIntegration_TransactionRouteRepository_OptionalLinks_UpdateRetagsKeptLinksInPlace(t *testing.T) {
	f := newOptionalLinksFixture(t)
	before := f.activeLinks(t)

	_, err := f.repo.Update(context.Background(), f.orgID, f.route.ID, &mmodel.TransactionRoute{}, LinkChanges{
		Retag: []OperationRouteLink{
			{OperationRouteID: f.fee, Optional: false},
			{OperationRouteID: f.destination, Optional: true},
		},
	})
	require.NoError(t, err)

	after := f.activeLinks(t)
	require.Len(t, after, 3)
	assert.False(t, after[f.fee].optional)
	assert.True(t, after[f.destination].optional)
	assert.False(t, after[f.source].optional)

	for operationRouteID, link := range before {
		assert.Equalf(t, link.id, after[operationRouteID].id, "the link to %s keeps its identity", operationRouteID)
	}
}

func TestIntegration_TransactionRouteRepository_OptionalLinks_UpdateAddsAndRemovesWithoutTouchingKeptFlags(t *testing.T) {
	f := newOptionalLinksFixture(t)
	added := pgtestutil.CreateTestOperationRouteSimple(t, f.db, f.orgID, f.ledgerID, "Second destination", "destination")

	_, err := f.repo.Update(context.Background(), f.orgID, f.route.ID, &mmodel.TransactionRoute{}, LinkChanges{
		Add:    []OperationRouteLink{{OperationRouteID: added}},
		Remove: []uuid.UUID{f.destination},
	})
	require.NoError(t, err)

	after := f.activeLinks(t)
	require.Len(t, after, 3)
	assert.True(t, after[f.fee].optional, "a kept optional link stays optional")
	assert.False(t, after[added].optional, "a link added without a flag is required")
	assert.NotContains(t, after, f.destination)
}

func TestIntegration_TransactionRouteRepository_OptionalLinks_DeleteRemovesOptionalLinksToo(t *testing.T) {
	f := newOptionalLinksFixture(t)

	require.NoError(t, f.repo.Delete(context.Background(), f.orgID, f.route.ID, []uuid.UUID{f.source, f.destination, f.fee}))

	assert.Empty(t, f.activeLinks(t))
}

func TestIntegration_TransactionRouteRepository_OptionalLinks_FindLinksByTransactionRouteIDs(t *testing.T) {
	f := newOptionalLinksFixture(t)

	required := &mmodel.TransactionRoute{
		ID: uuid.Must(libCommons.GenerateUUIDv7()), OrganizationID: f.orgID, LedgerID: &f.ledgerID, Title: "Required only",
		OperationRoutes: []mmodel.OperationRoute{{ID: f.source}, {ID: f.fee}},
		CreatedAt:       optionalLinksCreatedAt, UpdatedAt: optionalLinksCreatedAt,
	}
	_, err := f.repo.Create(context.Background(), f.orgID, &f.ledgerID, required)
	require.NoError(t, err)

	links, err := f.repo.FindOperationRouteLinksByTransactionRouteIDs(context.Background(), []uuid.UUID{f.route.ID, required.ID})
	require.NoError(t, err)

	assert.ElementsMatch(t, []OperationRouteLink{
		{OperationRouteID: f.source}, {OperationRouteID: f.destination}, {OperationRouteID: f.fee, Optional: true},
	}, links[f.route.ID])
	assert.ElementsMatch(t, []OperationRouteLink{
		{OperationRouteID: f.source}, {OperationRouteID: f.fee},
	}, links[required.ID], "the same operation route can be optional in one transaction route and required in another")
}

// A pod that predates the flag inserts links without naming the column.
func TestIntegration_TransactionRouteRepository_OptionalLinks_LinkWrittenWithoutTheColumnIsRequired(t *testing.T) {
	f := newOptionalLinksFixture(t)
	legacy := pgtestutil.CreateTestOperationRouteSimple(t, f.db, f.orgID, f.ledgerID, "Legacy destination", "destination")

	_, err := f.db.Exec(`INSERT INTO operation_transaction_route (id, operation_route_id, transaction_route_id, created_at) VALUES ($1, $2, $3, $4)`,
		uuid.Must(libCommons.GenerateUUIDv7()), legacy, f.route.ID, optionalLinksCreatedAt)
	require.NoError(t, err)

	found, err := f.repo.FindByID(context.Background(), f.orgID, f.route.ID)
	require.NoError(t, err)

	assert.Len(t, found.OperationRoutes, 4)
	assert.False(t, found.IsOptional(legacy))
}

// The flag is a column of the single active link row, so a route cannot be
// required and optional in the same transaction route at once.
func TestIntegration_TransactionRouteRepository_OptionalLinks_OneActiveLinkPerPair(t *testing.T) {
	f := newOptionalLinksFixture(t)

	_, err := f.db.Exec(`INSERT INTO operation_transaction_route (id, operation_route_id, transaction_route_id, created_at, optional) VALUES ($1, $2, $3, $4, false)`,
		uuid.Must(libCommons.GenerateUUIDv7()), f.fee, f.route.ID, optionalLinksCreatedAt)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "idx_operation_transaction_route_unique")

	links := f.activeLinks(t)
	assert.Len(t, links, 3)
	assert.True(t, links[f.fee].optional)
}
