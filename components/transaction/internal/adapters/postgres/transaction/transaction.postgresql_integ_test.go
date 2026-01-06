//go:build integration

package transaction

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/pkg"
	midazConstant "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const integrationPostgresImage = "postgres:14"

func setupPostgresDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        integrationPostgresImage,
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "test",
			"POSTGRES_PASSWORD": "test",
			"POSTGRES_DB":       "test",
		},
		WaitingFor: wait.ForAll(
			wait.ForListeningPort("5432/tcp"),
			wait.ForLog("database system is ready to accept connections"),
		).WithStartupTimeout(90 * time.Second),
	}

	pgContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start Postgres container")

	host, err := pgContainer.Host(ctx)
	require.NoError(t, err, "failed to get Postgres container host")

	port, err := pgContainer.MappedPort(ctx, "5432")
	require.NoError(t, err, "failed to get Postgres container port")

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable", host, "test", "test", "test", port.Port())
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err, "failed to open postgres connection")

	// Verify connection
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	require.NoError(t, db.PingContext(pingCtx), "failed to ping postgres")

	cleanup := func() {
		_ = db.Close()
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Logf("failed to terminate Postgres container: %v", err)
		}
	}

	return db, cleanup
}

func setupTransactionSchema(t *testing.T, db *sql.DB) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Minimal schema required for TransactionPostgreSQLRepository.Create + UpdateBalanceStatus
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS transaction (
			id UUID PRIMARY KEY,
			parent_transaction_id UUID NULL,
			description TEXT NOT NULL,
			status TEXT NOT NULL,
			status_description TEXT NULL,
			amount NUMERIC NULL,
			asset_code TEXT NOT NULL,
			chart_of_accounts_group_name TEXT NULL,
			ledger_id UUID NOT NULL,
			organization_id UUID NOT NULL,
			body JSONB NULL,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL,
			deleted_at TIMESTAMPTZ NULL,
			route UUID NULL,
			balance_status TEXT NULL CHECK (balance_status IS NULL OR balance_status IN ('PENDING', 'CONFIRMED', 'FAILED')),
			balance_persisted_at TIMESTAMPTZ NULL
		);
	`)
	require.NoError(t, err, "failed to create transaction table")
}

func newIntegrationRepo(t *testing.T, db *sql.DB) *TransactionPostgreSQLRepository {
	t.Helper()

	resolver := dbresolver.New(
		dbresolver.WithPrimaryDBs(db),
		dbresolver.WithReplicaDBs(db),
		dbresolver.WithLoadBalancer(dbresolver.RoundRobinLB),
	)

	logger := libZap.InitializeLogger()

	conn := &libPostgres.PostgresConnection{ConnectionDB: &resolver, Logger: logger}
	return &TransactionPostgreSQLRepository{connection: conn, tableName: "transaction"}
}

func fetchBalanceStatusRow(t *testing.T, db *sql.DB, orgID, ledgerID, txID uuid.UUID) (balanceStatus *midazConstant.BalanceStatus, persistedAt *time.Time, updatedAt time.Time) {
	t.Helper()

	row := db.QueryRowContext(context.Background(),
		`SELECT balance_status, balance_persisted_at, updated_at
		 FROM transaction
		 WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL`,
		orgID,
		ledgerID,
		txID,
	)

	var statusPtr *string
	var persistedPtr *time.Time
	var updated time.Time

	err := row.Scan(&statusPtr, &persistedPtr, &updated)
	require.NoError(t, err)

	// Convert *string to *constant.BalanceStatus
	var status *midazConstant.BalanceStatus
	if statusPtr != nil {
		bs := midazConstant.BalanceStatus(*statusPtr)
		status = &bs
	}

	return status, persistedPtr, updated
}

func createAsyncTransaction(t *testing.T, repo *TransactionPostgreSQLRepository, orgID, ledgerID, txID uuid.UUID, balanceStatus midazConstant.BalanceStatus) {
	t.Helper()

	createdAt := time.Now().UTC().Add(-2 * time.Hour)
	updatedAt := createdAt

	statusCopy := balanceStatus

	tran := &Transaction{
		ID:             txID.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		Description:    "integration test",
		Status: Status{
			Code: midazConstant.CREATED,
		},
		AssetCode:     "USD",
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
		BalanceStatus: &statusCopy,
		Operations:    []*operation.Operation{},
		Metadata:      map[string]any{},
	}

	_, err := repo.Create(context.Background(), tran)
	require.NoError(t, err)
}

func TestUpdateBalanceStatus_Success(t *testing.T) {
	db, cleanup := setupPostgresDB(t)
	defer cleanup()

	setupTransactionSchema(t, db)
	repo := newIntegrationRepo(t, db)

	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()

	createAsyncTransaction(t, repo, orgID, ledgerID, txID, midazConstant.BalanceStatusPending)

	beforeStatus, beforePersistedAt, beforeUpdatedAt := fetchBalanceStatusRow(t, db, orgID, ledgerID, txID)
	require.NotNil(t, beforeStatus)
	assert.Equal(t, midazConstant.BalanceStatusPending, *beforeStatus)
	assert.Nil(t, beforePersistedAt)

	// Act
	err := repo.UpdateBalanceStatus(context.Background(), orgID, ledgerID, txID, midazConstant.BalanceStatusConfirmed)
	require.NoError(t, err)

	// Assert
	afterStatus, afterPersistedAt, afterUpdatedAt := fetchBalanceStatusRow(t, db, orgID, ledgerID, txID)
	require.NotNil(t, afterStatus)
	assert.Equal(t, midazConstant.BalanceStatusConfirmed, *afterStatus)
	require.NotNil(t, afterPersistedAt)
	assert.True(t, afterUpdatedAt.After(beforeUpdatedAt), "updated_at must be refreshed")
}

func TestUpdateBalanceStatus_NotFound(t *testing.T) {
	db, cleanup := setupPostgresDB(t)
	defer cleanup()

	setupTransactionSchema(t, db)
	repo := newIntegrationRepo(t, db)

	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()

	err := repo.UpdateBalanceStatus(context.Background(), orgID, ledgerID, txID, midazConstant.BalanceStatusConfirmed)
	require.Error(t, err)

	var notFound pkg.EntityNotFoundError
	assert.ErrorAs(t, err, &notFound)
}

func TestUpdateBalanceStatus_InvalidTransition_IsIdempotent(t *testing.T) {
	db, cleanup := setupPostgresDB(t)
	defer cleanup()

	setupTransactionSchema(t, db)
	repo := newIntegrationRepo(t, db)

	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()

	createAsyncTransaction(t, repo, orgID, ledgerID, txID, midazConstant.BalanceStatusPending)

	// First move to CONFIRMED
	err := repo.UpdateBalanceStatus(context.Background(), orgID, ledgerID, txID, midazConstant.BalanceStatusConfirmed)
	require.NoError(t, err)

	confirmedStatus, confirmedPersistedAt, confirmedUpdatedAt := fetchBalanceStatusRow(t, db, orgID, ledgerID, txID)
	require.NotNil(t, confirmedStatus)
	assert.Equal(t, midazConstant.BalanceStatusConfirmed, *confirmedStatus)
	require.NotNil(t, confirmedPersistedAt)

	// Attempt invalid transition CONFIRMED -> FAILED (must be a no-op, no error)
	err = repo.UpdateBalanceStatus(context.Background(), orgID, ledgerID, txID, midazConstant.BalanceStatusFailed)
	require.NoError(t, err)

	afterStatus, afterPersistedAt, afterUpdatedAt := fetchBalanceStatusRow(t, db, orgID, ledgerID, txID)
	require.NotNil(t, afterStatus)
	assert.Equal(t, midazConstant.BalanceStatusConfirmed, *afterStatus)
	assert.Equal(t, confirmedPersistedAt, afterPersistedAt, "balance_persisted_at must not change on invalid transition")
	assert.Equal(t, confirmedUpdatedAt, afterUpdatedAt, "updated_at must not change on invalid transition")
}
