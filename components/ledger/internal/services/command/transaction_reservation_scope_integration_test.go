//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"database/sql"
	"testing"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// reserveGroupingIDs are the fixed ids every row of the divergent primary and
// replica share, so the only difference between the two databases is which
// grouping the account points at.
type reserveGroupingIDs struct {
	organization, ledger, account uuid.UUID
	oldPortfolio, newPortfolio    uuid.UUID
	oldSegment, newSegment        uuid.UUID
}

// seedReserveGrouping writes the same organization, ledger, portfolios and
// segments into db, and the account pointing at the given grouping.
func seedReserveGrouping(t *testing.T, db *sql.DB, ids reserveGroupingIDs, portfolioID, segmentID uuid.UUID) {
	t.Helper()

	pgtestutil.CreateTestOrganizationWithID(t, db, ids.organization)
	pgtestutil.CreateTestLedgerWithID(t, db, ids.ledger, ids.organization)

	now := time.Date(2026, time.October, 16, 12, 0, 0, 0, time.UTC)

	for _, id := range []uuid.UUID{ids.oldPortfolio, ids.newPortfolio} {
		_, err := db.Exec(`INSERT INTO portfolio (id, name, entity_id, ledger_id, organization_id, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, 'ACTIVE', $6, $6)`, id, "portfolio "+id.String(), "holder-"+id.String(), ids.ledger, ids.organization, now)
		require.NoError(t, err)
	}

	for _, id := range []uuid.UUID{ids.oldSegment, ids.newSegment} {
		_, err := db.Exec(`INSERT INTO segment (id, name, ledger_id, organization_id, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, 'ACTIVE', $5, $5)`, id, "segment "+id.String(), ids.ledger, ids.organization, now)
		require.NoError(t, err)
	}

	_, err := db.Exec(`INSERT INTO account (id, name, asset_code, organization_id, ledger_id, portfolio_id, segment_id, status, alias, type, blocked, created_at, updated_at)
		VALUES ($1, 'Grouped source', 'BRL', $2, $3, $4, $5, 'ACTIVE', '@grouped-source', 'deposit', false, $6, $6)`,
		ids.account, ids.organization, ids.ledger, portfolioID, segmentID, now)
	require.NoError(t, err)
}

// The reserve must carry the grouping the account has NOW. Two independent
// databases stand in for primary and replica: the replica still holds the
// account in its old portfolio and segment, the primary already moved it.
func TestIntegration_ReserveSourceAccount_ReadsGroupingFromThePrimary(t *testing.T) {
	primary := pgtestutil.SetupMigratedContainer(t, "onboarding")
	replica := pgtestutil.SetupMigratedContainer(t, "onboarding")

	conn, err := libPostgres.New(libPostgres.Config{
		PrimaryDSN: pgtestutil.BuildConnectionString(primary.Host, primary.Port, primary.Config),
		ReplicaDSN: pgtestutil.BuildConnectionString(replica.Host, replica.Port, replica.Config),
	})
	require.NoError(t, err)
	require.NoError(t, conn.Connect(context.Background()))

	t.Cleanup(func() {
		if closeErr := conn.Close(); closeErr != nil {
			t.Logf("failed to close postgres client: %v", closeErr)
		}
	})

	ids := reserveGroupingIDs{
		organization: uuid.MustParse("01994f13-29b7-7000-8000-00000000a001"),
		ledger:       uuid.MustParse("01994f13-29b7-7000-8000-00000000a002"),
		account:      uuid.MustParse("01994f13-29b7-7000-8000-00000000a003"),
		oldPortfolio: uuid.MustParse("01994f13-29b7-7000-8000-00000000a004"),
		newPortfolio: uuid.MustParse("01994f13-29b7-7000-8000-00000000a005"),
		oldSegment:   uuid.MustParse("01994f13-29b7-7000-8000-00000000a006"),
		newSegment:   uuid.MustParse("01994f13-29b7-7000-8000-00000000a007"),
	}
	seedReserveGrouping(t, replica.DB, ids, ids.oldPortfolio, ids.oldSegment)
	seedReserveGrouping(t, primary.DB, ids, ids.newPortfolio, ids.newSegment)

	repo := account.NewAccountPostgreSQLRepository(conn)
	ctx := context.Background()

	stale, err := repo.Find(ctx, ids.organization, ids.ledger, nil, ids.account, mmodel.HolderOffV1)
	require.NoError(t, err)
	require.NotNil(t, stale.PortfolioID)
	require.Equal(t, ids.oldPortfolio.String(), *stale.PortfolioID,
		"precondition: an ordinary read of this client goes to the lagging replica")

	uc := &UseCase{TracerReserver: &stubReserver{}, AccountRepo: repo}
	balances := []*mmodel.Balance{{
		OrganizationID: ids.organization.String(), LedgerID: ids.ledger.String(),
		AccountID: ids.account.String(), Alias: "@grouped-source", Key: "default", AccountType: "deposit",
	}}
	_, span := noop.NewTracerProvider().Tracer("test").Start(ctx, "test")

	for _, settings := range []mmodel.TracerSettings{
		{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureClosed},
		{Mode: mmodel.TracerModeAdvisory, FailPosture: mmodel.TracerFailPostureOpen},
	} {
		t.Run(string(settings.Mode), func(t *testing.T) {
			got, err := uc.reserveSourceAccount(ctx, span, &libLog.NopLogger{}, settings, false,
				[]string{"@grouped-source#default"}, balances)
			require.NoError(t, err)
			assert.Equal(t, tracer.ReserveAccount{
				AccountID:   ids.account.String(),
				Type:        "deposit",
				SegmentID:   ids.newSegment.String(),
				PortfolioID: ids.newPortfolio.String(),
			}, got, "the reserve carries the primary's grouping, never the replica's")
		})
	}
}
