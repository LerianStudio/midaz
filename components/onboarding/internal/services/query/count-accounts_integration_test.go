//go:build integration

package query

import (
	"context"
	"fmt"
	"testing"

	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/account"
	pgtestutil "github.com/LerianStudio/midaz/v3/pkg/testutils/postgres"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestIntegration_CountAccounts_Monotonic verifies that account count never decreases
// as new accounts are created within a ledger.
func TestIntegration_CountAccounts_Monotonic(t *testing.T) {
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

	// Setup: org + ledger + asset
	orgID := pgtestutil.CreateTestOrganization(t, container.DB)
	ledgerID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	pgtestutil.CreateTestAsset(t, container.DB, orgID, ledgerID, "USD")

	// Get initial count
	lastCount, err := uc.CountAccounts(ctx, orgID, ledgerID)
	require.NoError(t, err, "initial account count should succeed")

	// Create accounts and verify count never decreases
	for i := 0; i < 5; i++ {
		alias := fmt.Sprintf("account-%d", i)
		pgtestutil.CreateTestAccount(t, container.DB, orgID, ledgerID, nil,
			fmt.Sprintf("Account %d", i), alias, "USD", nil)

		newCount, err := uc.CountAccounts(ctx, orgID, ledgerID)
		require.NoError(t, err, "count after insert %d should succeed", i)

		assert.GreaterOrEqual(t, newCount, lastCount,
			"account count should never decrease: was %d, now %d after insert %d",
			lastCount, newCount, i)

		assert.Equal(t, lastCount+1, newCount,
			"account count should increase by 1: was %d, expected %d, got %d",
			lastCount, lastCount+1, newCount)

		lastCount = newCount
	}
}

// TestIntegration_CountAccounts_IsolatedByLedger verifies that account counts are properly
// isolated by ledger - accounts from one ledger should not affect counts in another.
func TestIntegration_CountAccounts_IsolatedByLedger(t *testing.T) {
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

	orgID := pgtestutil.CreateTestOrganization(t, container.DB)

	// Create 2 ledgers
	ledger1ID := pgtestutil.CreateTestLedger(t, container.DB, orgID)
	ledger2ID := pgtestutil.CreateTestLedger(t, container.DB, orgID)

	// Create assets in both ledgers
	pgtestutil.CreateTestAsset(t, container.DB, orgID, ledger1ID, "USD")
	pgtestutil.CreateTestAsset(t, container.DB, orgID, ledger2ID, "USD")

	// Create 4 accounts in ledger1
	for i := 0; i < 4; i++ {
		pgtestutil.CreateTestAccount(t, container.DB, orgID, ledger1ID, nil,
			fmt.Sprintf("L1-Account-%d", i), fmt.Sprintf("l1-acc-%d", i), "USD", nil)
	}

	// Create 2 accounts in ledger2
	for i := 0; i < 2; i++ {
		pgtestutil.CreateTestAccount(t, container.DB, orgID, ledger2ID, nil,
			fmt.Sprintf("L2-Account-%d", i), fmt.Sprintf("l2-acc-%d", i), "USD", nil)
	}

	// Count for ledger1 should be 4
	count1, err := uc.CountAccounts(ctx, orgID, ledger1ID)
	require.NoError(t, err)
	assert.Equal(t, int64(4), count1, "ledger1 should have exactly 4 accounts")

	// Count for ledger2 should be 2
	count2, err := uc.CountAccounts(ctx, orgID, ledger2ID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count2, "ledger2 should have exactly 2 accounts")

	// Count for non-existent ledger should be 0
	fakeLedgerID := uuid.New()
	count3, err := uc.CountAccounts(ctx, orgID, fakeLedgerID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count3, "non-existent ledger should have 0 accounts")
}
