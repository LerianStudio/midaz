//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package backfill_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/holder"
	crmservices "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services/encryption"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/backfill"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	testutils "github.com/LerianStudio/midaz/v4/tests/utils"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// fixedTime is a deterministic timestamp for fixture inserts (no time.Now() in tests).
var fixedTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// tenantEnv holds one tenant's stores plus the context that injects them, so the
// runner resolves PG and Mongo exactly as the request path would.
type tenantEnv struct {
	pg      *pgtestutil.ContainerResult
	mongo   *mongotestutil.ContainerResult
	mongoDB *mongo.Database
	ctx     context.Context
	runner  *backfill.HolderBackfiller
}

func newTenantEnv(t *testing.T) *tenantEnv {
	t.Helper()

	pgContainer := pgtestutil.SetupContainer(t)
	mongoContainer := mongotestutil.SetupContainer(t)

	migrationsPath := pgtestutil.FindMigrationsPath(t, "onboarding")
	connStr := pgtestutil.BuildConnectionString(pgContainer.Host, pgContainer.Port, pgContainer.Config)
	pgClient := pgtestutil.CreatePostgresClient(t, connStr, connStr, pgContainer.Config.DBName, migrationsPath)

	orgRepo := organization.NewOrganizationPostgreSQLRepository(pgClient)

	mongoConn := mongotestutil.CreateConnection(t, mongoContainer.URI, mongoContainer.DBName)
	crypto := testutils.SetupCrypto(t)
	encResolver := encryption.NewProtectionStateResolver(nil, encryption.NewProtectionMetrics(nil))
	svc := encryption.NewEncryptionService(encResolver, nil, nil, crypto, encryption.NewProtectionMetrics(nil))
	fe := encryption.NewFieldEncryptorAdapter(svc)
	holderRepo, err := holder.NewMongoDBRepository(mongoConn, fe)
	require.NoError(t, err)

	crmService := &crmservices.UseCase{HolderRepo: holderRepo}

	runner := backfill.NewHolderBackfiller(orgRepo, crmService)

	resolver, err := pgClient.Resolver(context.Background())
	require.NoError(t, err)

	ctx := tmcore.ContextWithPG(context.Background(), resolver, constant.ModuleOnboarding)
	ctx = tmcore.ContextWithMB(ctx, mongoContainer.Database)

	return &tenantEnv{
		pg:      pgContainer,
		mongo:   mongoContainer,
		mongoDB: mongoContainer.Database,
		ctx:     ctx,
		runner:  runner,
	}
}

// seedOrg inserts an organization and returns its ID.
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

// seedLedger inserts a ledger under an org and returns its ID.
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

// seedAccount inserts an account with holder_id NULL and returns its ID. The type
// drives the @external exemption.
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

// TestIntegration_HolderBackfill_IsIdempotent proves gate 4: a double run yields
// an identical end state — self-holder count == org count, external accounts stay
// NULL, and the second pass materialises zero additional rows.
func TestIntegration_HolderBackfill_IsIdempotent(t *testing.T) {
	env := newTenantEnv(t)

	orgA := seedOrg(t, env.pg.DB, "Org A", "doc-a")
	orgB := seedOrg(t, env.pg.DB, "Org B", "doc-b")
	ledgerA := seedLedger(t, env.pg.DB, orgA)
	ledgerB := seedLedger(t, env.pg.DB, orgB)

	normalA := seedAccount(t, env.pg.DB, orgA, ledgerA, "@normal-a", "deposit")
	externalA := seedAccount(t, env.pg.DB, orgA, ledgerA, "@external/USD", "external")
	normalB := seedAccount(t, env.pg.DB, orgB, ledgerB, "@normal-b", "deposit")

	// First run.
	first, err := env.runner.RunTenant(env.ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, first.OrgsProcessed, "both orgs processed")
	assert.Equal(t, 2, first.HoldersProvisioned, "one self-holder per org")
	assert.Equal(t, int64(2), first.AccountsMaterialised, "two non-external accounts materialised")

	// Self-holder count == org count (one deterministic holder per org collection).
	assert.Equal(t, int64(1), countSelfHolder(t, env.mongoDB, orgA))
	assert.Equal(t, int64(1), countSelfHolder(t, env.mongoDB, orgB))

	// Non-external accounts now point at their org's deterministic self-holder.
	assert.Equal(t, command.DeriveSelfHolderID(orgA).String(), *holderID(t, env.pg.DB, normalA))
	assert.Equal(t, command.DeriveSelfHolderID(orgB).String(), *holderID(t, env.pg.DB, normalB))

	// External account stays NULL (D-3 exempt).
	assert.Nil(t, holderID(t, env.pg.DB, externalA), "external account must keep holder_id NULL")

	// Second run: idempotent — no new holders, no new materialisations.
	second, err := env.runner.RunTenant(env.ctx)
	require.NoError(t, err)
	assert.Equal(t, first.OrgsProcessed, second.OrgsProcessed)
	assert.Equal(t, first.HoldersProvisioned, second.HoldersProvisioned)
	assert.Equal(t, int64(0), second.AccountsMaterialised, "second pass materialises zero rows")

	// Diff: end state identical after the second run.
	assert.Equal(t, int64(1), countSelfHolder(t, env.mongoDB, orgA))
	assert.Equal(t, int64(1), countSelfHolder(t, env.mongoDB, orgB))
	assert.Equal(t, command.DeriveSelfHolderID(orgA).String(), *holderID(t, env.pg.DB, normalA))
	assert.Equal(t, command.DeriveSelfHolderID(orgB).String(), *holderID(t, env.pg.DB, normalB))
	assert.Nil(t, holderID(t, env.pg.DB, externalA))
}

// TestIntegration_HolderBackfill_TenantIsolation proves gate 5: running the
// backfill against tenant A's stores leaves tenant B's stores completely
// untouched, even when both tenants host an organization with the SAME UUID.
func TestIntegration_HolderBackfill_TenantIsolation(t *testing.T) {
	tenantA := newTenantEnv(t)
	tenantB := newTenantEnv(t)

	// Same org UUID in both tenants — the worst case for cross-tenant bleed.
	sharedOrgID := uuid.New()

	insertOrgWithID(t, tenantA.pg.DB, sharedOrgID, "Shared Org A", "doc-shared")
	insertOrgWithID(t, tenantB.pg.DB, sharedOrgID, "Shared Org B", "doc-shared")
	ledgerA := seedLedger(t, tenantA.pg.DB, sharedOrgID)
	ledgerB := seedLedger(t, tenantB.pg.DB, sharedOrgID)
	accountA := seedAccount(t, tenantA.pg.DB, sharedOrgID, ledgerA, "@normal", "deposit")
	accountB := seedAccount(t, tenantB.pg.DB, sharedOrgID, ledgerB, "@normal", "deposit")

	// Run ONLY tenant A's backfill.
	_, err := tenantA.runner.RunTenant(tenantA.ctx)
	require.NoError(t, err)

	// Tenant A is materialised.
	assert.Equal(t, command.DeriveSelfHolderID(sharedOrgID).String(), *holderID(t, tenantA.pg.DB, accountA))
	assert.Equal(t, int64(1), countSelfHolder(t, tenantA.mongoDB, sharedOrgID))

	// Tenant B is completely untouched — account NULL, no self-holder.
	assert.Nil(t, holderID(t, tenantB.pg.DB, accountB), "tenant B account must remain NULL")
	assert.Equal(t, int64(0), countSelfHolder(t, tenantB.mongoDB, sharedOrgID), "tenant B must have no self-holder")
}

func insertOrgWithID(t *testing.T, db *sql.DB, id uuid.UUID, legalName, legalDoc string) {
	t.Helper()

	_, err := db.Exec(`
		INSERT INTO organization (id, legal_name, legal_document, doing_business_as, address, status, created_at, updated_at, deleted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, id, legalName, legalDoc, nil, `{"city":"Test"}`, "ACTIVE", fixedTime, fixedTime, nil)
	require.NoError(t, err, "failed to seed organization with fixed id")
}

// countSelfHolder counts the deterministic self-holder document in an org's
// per-org collection.
func countSelfHolder(t *testing.T, db *mongo.Database, orgID uuid.UUID) int64 {
	t.Helper()

	collName := strings.ToLower("holders_" + orgID.String())

	return mongotestutil.CountDocuments(t, db, collName, bson.M{"_id": command.DeriveSelfHolderID(orgID)})
}
