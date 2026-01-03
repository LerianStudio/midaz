//go:build integration

package query

import (
	"context"
	"fmt"
	"testing"

	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/ledger"
	pgtestutil "github.com/LerianStudio/midaz/v3/pkg/testutils/postgres"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestIntegration_CountLedgers_Monotonic verifies that ledger count never decreases
// as new ledgers are created. This is a property-based test that validates
// the monotonicity invariant of the count metric.
func TestIntegration_CountLedgers_Monotonic(t *testing.T) {
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

	ledgerRepo := ledger.NewLedgerPostgreSQLRepository(conn)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockMetadata := mongodb.NewMockRepository(ctrl)
	mockMetadata.EXPECT().FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	uc := &UseCase{
		LedgerRepo:   ledgerRepo,
		MetadataRepo: mockMetadata,
	}

	ctx := context.Background()
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)

	// Get initial count
	lastCount, err := uc.CountLedgers(ctx, orgID)
	require.NoError(t, err, "initial count should succeed")

	// Create ledgers and verify count never decreases
	for i := 0; i < 5; i++ {
		params := pgtestutil.DefaultLedgerParams()
		params.Name = fmt.Sprintf("Ledger-%d", i)
		pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, params)

		newCount, err := uc.CountLedgers(ctx, orgID)
		require.NoError(t, err, "count after insert %d should succeed", i)

		assert.GreaterOrEqual(t, newCount, lastCount,
			"ledger count should never decrease: was %d, now %d after insert %d",
			lastCount, newCount, i)

		assert.Equal(t, lastCount+1, newCount,
			"ledger count should increase by 1: was %d, expected %d, got %d",
			lastCount, lastCount+1, newCount)

		lastCount = newCount
	}
}

// TestIntegration_CountLedgers_IsolatedByOrganization verifies that counts are properly
// isolated by organization - ledgers from one org should not affect counts in another.
func TestIntegration_CountLedgers_IsolatedByOrganization(t *testing.T) {
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

	ledgerRepo := ledger.NewLedgerPostgreSQLRepository(conn)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockMetadata := mongodb.NewMockRepository(ctrl)
	mockMetadata.EXPECT().FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	uc := &UseCase{
		LedgerRepo:   ledgerRepo,
		MetadataRepo: mockMetadata,
	}

	ctx := context.Background()

	// Create 2 organizations
	org1ID := pgtestutil.CreateTestOrganization(t, container.DB)
	org2ID := pgtestutil.CreateTestOrganization(t, container.DB)

	// Create 3 ledgers in org1
	for i := 0; i < 3; i++ {
		params := pgtestutil.DefaultLedgerParams()
		params.Name = fmt.Sprintf("Org1-Ledger-%d", i)
		pgtestutil.CreateTestLedgerWithParams(t, container.DB, org1ID, params)
	}

	// Create 2 ledgers in org2
	for i := 0; i < 2; i++ {
		params := pgtestutil.DefaultLedgerParams()
		params.Name = fmt.Sprintf("Org2-Ledger-%d", i)
		pgtestutil.CreateTestLedgerWithParams(t, container.DB, org2ID, params)
	}

	// Count for org1 should be 3
	count1, err := uc.CountLedgers(ctx, org1ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), count1, "org1 should have exactly 3 ledgers")

	// Count for org2 should be 2
	count2, err := uc.CountLedgers(ctx, org2ID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count2, "org2 should have exactly 2 ledgers")

	// Count for non-existent org should be 0
	fakeOrgID := uuid.New()
	count3, err := uc.CountLedgers(ctx, fakeOrgID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count3, "non-existent org should have 0 ledgers")
}
