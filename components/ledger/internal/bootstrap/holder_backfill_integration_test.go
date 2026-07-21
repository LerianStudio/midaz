//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"database/sql"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/backfill"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixedTime is a deterministic timestamp for fixture inserts (no time.Now() in tests).
var fixedTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// fakeProvisioner satisfies command.HolderProvisioner without touching Mongo. The
// self-holder step is not under test here; the PG materialisation (and the
// connection resolution that precedes it) is, so a no-op success keeps the test
// scoped to the single-tenant context-injection fix.
type fakeProvisioner struct{}

func (fakeProvisioner) CreateHolderWithID(_ context.Context, _ string, id uuid.UUID, _ *mmodel.CreateHolderInput) (*mmodel.Holder, error) {
	return &mmodel.Holder{ID: &id}, nil
}

// TestIntegration_HolderBackfillRunner_SingleTenant_Run drives the REAL Run
// entrypoint with a bare context.Background() — no manual ContextWithPG injection.
// Before the fix, Run's single-tenant branch handed a bare ctx to RunTenant and
// materialiseAccounts -> resolveOnboardingDB aborted on the first org with
// "onboarding postgres connection missing from context". This test seeds one org
// with a non-external account, runs the real entrypoint, and asserts the run
// succeeds and the account's holder_id was materialised — proving Run now injects
// the ambient onboarding PG connection into the context.
func TestIntegration_HolderBackfillRunner_SingleTenant_Run(t *testing.T) {
	pgContainer := pgtestutil.SetupContainer(t)

	migrationsPath := pgtestutil.FindMigrationsPath(t, "onboarding")
	connStr := pgtestutil.BuildConnectionString(pgContainer.Host, pgContainer.Port, pgContainer.Config)
	pgClient := pgtestutil.CreatePostgresClient(t, connStr, connStr, pgContainer.Config.DBName, migrationsPath)

	orgRepo := organization.NewOrganizationPostgreSQLRepository(pgClient)

	orgID := seedOrg(t, pgContainer.DB, "Org A", "doc-a")
	ledgerID := seedLedger(t, pgContainer.DB, orgID)
	accountID := seedAccount(t, pgContainer.DB, orgID, ledgerID, "@normal-a", "deposit")

	r := &HolderBackfillRunner{
		logger:             libLog.NewNop(),
		multiTenantEnabled: false,
		runner:             backfill.NewHolderBackfiller(orgRepo, fakeProvisioner{}),
		onbPG:              &onboardingPostgresComponents{connection: pgClient},
	}

	// Real entrypoint, bare context: this is the path the binary takes.
	err := r.Run(context.Background())
	require.NoError(t, err, "single-tenant Run must not fail on missing onboarding PG connection")

	// Proof the PG step ran against the injected connection: holder_id is set.
	assert.Equal(t, command.DeriveSelfHolderID(orgID).String(), *holderID(t, pgContainer.DB, accountID),
		"account holder_id must be materialised by the single-tenant backfill")
}

func seedOrg(t *testing.T, db *sql.DB, legalName, legalDoc string) uuid.UUID {
	t.Helper()

	id := uuid.New()
	_, err := db.Exec(`
		INSERT INTO organization (id, legal_name, legal_document, doing_business_as, address, status, created_at, updated_at, deleted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, id, legalName, legalDoc, nil, `{"city":"Test"}`, "ACTIVE", fixedTime, fixedTime, nil)
	require.NoError(t, err, "failed to seed organization")

	return id
}

func seedLedger(t *testing.T, db *sql.DB, orgID uuid.UUID) uuid.UUID {
	t.Helper()

	id := uuid.New()
	_, err := db.Exec(`
		INSERT INTO ledger (id, name, organization_id, status, created_at, updated_at, deleted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, id, "Test Ledger", orgID, "ACTIVE", fixedTime, fixedTime, nil)
	require.NoError(t, err, "failed to seed ledger")

	return id
}

func seedAccount(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID, alias, accountType string) uuid.UUID {
	t.Helper()

	id := uuid.New()
	_, err := db.Exec(`
		INSERT INTO account (id, name, asset_code, organization_id, ledger_id, status, alias, type, blocked, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, id, "Test Account", "USD", orgID, ledgerID, "ACTIVE", alias, accountType, false, fixedTime, fixedTime)
	require.NoError(t, err, "failed to seed account")

	return id
}

func holderID(t *testing.T, db *sql.DB, accountID uuid.UUID) *string {
	t.Helper()

	var hid *string
	err := db.QueryRow(`SELECT holder_id FROM account WHERE id = $1`, accountID).Scan(&hid)
	require.NoError(t, err, "failed to read holder_id")

	return hid
}
