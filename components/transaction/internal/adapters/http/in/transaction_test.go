package in

import (
	"errors"
	"net/http/httptest"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"go.uber.org/mock/gomock"
)

func newTestTransactionData(orgID, ledgerID, tranID uuid.UUID) *transaction.Transaction {
	return &transaction.Transaction{
		ID:             tranID.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		AssetCode:      "BRL",
		Status:         transaction.Status{Code: "PENDING"},
	}
}


// TestGetTransaction_WriteBehindHit verifies that GetTransaction returns 200 from write-behind cache,
// skipping both Postgres lookup and operations query.
func TestGetTransaction_WriteBehindHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	tranID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	queryUC := &query.UseCase{RedisRepo: mockRedisRepo}
	handler := &TransactionHandler{
		Command: &command.UseCase{},
		Query:   queryUC,
	}

	// Write-behind hit
	tran := newTestTransactionData(orgID, ledgerID, tranID)
	wbData, err := msgpack.Marshal(tran)
	require.NoError(t, err)

	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), gomock.Any()).
		Return(wbData, nil).
		Times(1)

	// No TransactionRepo mock → proves Postgres is never called

	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		c.Locals("organization_id", orgID)
		c.Locals("ledger_id", ledgerID)
		c.Locals("transaction_id", tranID)
		return handler.GetTransaction(c)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "true", resp.Header.Get("X-Cache-Hit"))
}

// TestCancelTransaction_WriteBehindMiss_PostgresMiss verifies that CancelTransaction returns error
// when both write-behind and Postgres fail.
func TestCancelTransaction_WriteBehindMiss_PostgresMiss(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	tranID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	queryUC := &query.UseCase{
		RedisRepo:       mockRedisRepo,
		TransactionRepo: mockTransactionRepo,
	}
	handler := &TransactionHandler{
		Command: &command.UseCase{},
		Query:   queryUC,
	}

	// Write-behind miss
	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("redis: nil")).
		Times(1)

	// Postgres miss
	mockTransactionRepo.EXPECT().
		Find(gomock.Any(), orgID, ledgerID, tranID).
		Return(nil, errors.New("record not found")).
		Times(1)

	app := fiber.New()
	app.Post("/test", func(c *fiber.Ctx) error {
		c.Locals("organization_id", orgID)
		c.Locals("ledger_id", ledgerID)
		c.Locals("transaction_id", tranID)
		return handler.CancelTransaction(c)
	})

	req := httptest.NewRequest("POST", "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.True(t, resp.StatusCode >= 400, "Expected error status code, got %d", resp.StatusCode)
}

// TestCancelTransaction_WriteBehindMiss_PostgresHit verifies fallback to Postgres when write-behind misses.
func TestCancelTransaction_WriteBehindMiss_PostgresHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	tranID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	queryUC := &query.UseCase{
		RedisRepo:       mockRedisRepo,
		TransactionRepo: mockTransactionRepo,
		MetadataRepo:    mockMetadataRepo,
	}
	handler := &TransactionHandler{
		Command: &command.UseCase{RedisRepo: mockRedisRepo},
		Query:   queryUC,
	}

	tran := newTestTransactionData(orgID, ledgerID, tranID)

	// Write-behind miss
	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("redis: nil")).
		Times(1)

	// Postgres hit
	mockTransactionRepo.EXPECT().
		Find(gomock.Any(), orgID, ledgerID, tranID).
		Return(tran, nil).
		Times(1)

	// Metadata lookup (returns nil = no metadata)
	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).
		Times(1)

	// commitOrCancelTransaction: SetNX short-circuits (we're only testing the lookup path)
	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, errors.New("lock error")).
		Times(1)

	app := fiber.New()
	app.Post("/test", func(c *fiber.Ctx) error {
		c.Locals("organization_id", orgID)
		c.Locals("ledger_id", ledgerID)
		c.Locals("transaction_id", tranID)
		return handler.CancelTransaction(c)
	})

	req := httptest.NewRequest("POST", "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	// Response is an error (from SetNX), but the important thing is Find WAS called (fallback worked)
	assert.True(t, resp.StatusCode >= 400)
}

// TestCancelTransaction_WriteBehindHit_PostgresNotCalled verifies that when write-behind hits,
// Postgres is not queried.
func TestCancelTransaction_WriteBehindHit_PostgresNotCalled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	tranID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	queryUC := &query.UseCase{RedisRepo: mockRedisRepo}
	handler := &TransactionHandler{
		Command: &command.UseCase{RedisRepo: mockRedisRepo},
		Query:   queryUC,
	}

	// Write-behind hit
	tran := newTestTransactionData(orgID, ledgerID, tranID)
	wbData, err := msgpack.Marshal(tran)
	require.NoError(t, err)

	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), gomock.Any()).
		Return(wbData, nil).
		Times(1)

	// No TransactionRepo mock → proves Postgres is never called

	// commitOrCancelTransaction: SetNX short-circuits
	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, errors.New("lock error")).
		Times(1)

	app := fiber.New()
	app.Post("/test", func(c *fiber.Ctx) error {
		c.Locals("organization_id", orgID)
		c.Locals("ledger_id", ledgerID)
		c.Locals("transaction_id", tranID)
		return handler.CancelTransaction(c)
	})

	req := httptest.NewRequest("POST", "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	// Error from SetNX short-circuit, but write-behind was used and Postgres was NOT called
	assert.True(t, resp.StatusCode >= 400)
}

// TestCommitTransaction_WriteBehindMiss_PostgresMiss verifies that CommitTransaction returns error
// when both write-behind and Postgres fail.
func TestCommitTransaction_WriteBehindMiss_PostgresMiss(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	tranID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	queryUC := &query.UseCase{
		RedisRepo:       mockRedisRepo,
		TransactionRepo: mockTransactionRepo,
	}
	handler := &TransactionHandler{
		Command: &command.UseCase{},
		Query:   queryUC,
	}

	// Write-behind miss
	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("redis: nil")).
		Times(1)

	// Postgres miss
	mockTransactionRepo.EXPECT().
		Find(gomock.Any(), orgID, ledgerID, tranID).
		Return(nil, errors.New("record not found")).
		Times(1)

	app := fiber.New()
	app.Post("/test", func(c *fiber.Ctx) error {
		c.Locals("organization_id", orgID)
		c.Locals("ledger_id", ledgerID)
		c.Locals("transaction_id", tranID)
		return handler.CommitTransaction(c)
	})

	req := httptest.NewRequest("POST", "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.True(t, resp.StatusCode >= 400, "Expected error status code, got %d", resp.StatusCode)
}

// TestCommitTransaction_WriteBehindMiss_PostgresHit verifies fallback to Postgres when write-behind misses.
func TestCommitTransaction_WriteBehindMiss_PostgresHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	tranID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	queryUC := &query.UseCase{
		RedisRepo:       mockRedisRepo,
		TransactionRepo: mockTransactionRepo,
		MetadataRepo:    mockMetadataRepo,
	}
	handler := &TransactionHandler{
		Command: &command.UseCase{RedisRepo: mockRedisRepo},
		Query:   queryUC,
	}

	tran := newTestTransactionData(orgID, ledgerID, tranID)

	// Write-behind miss
	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("redis: nil")).
		Times(1)

	// Postgres hit
	mockTransactionRepo.EXPECT().
		Find(gomock.Any(), orgID, ledgerID, tranID).
		Return(tran, nil).
		Times(1)

	// Metadata lookup
	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).
		Times(1)

	// commitOrCancelTransaction: SetNX short-circuits
	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, errors.New("lock error")).
		Times(1)

	app := fiber.New()
	app.Post("/test", func(c *fiber.Ctx) error {
		c.Locals("organization_id", orgID)
		c.Locals("ledger_id", ledgerID)
		c.Locals("transaction_id", tranID)
		return handler.CommitTransaction(c)
	})

	req := httptest.NewRequest("POST", "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	// Error from SetNX short-circuit, but Find WAS called (fallback worked)
	assert.True(t, resp.StatusCode >= 400)
}

// TestCommitTransaction_WriteBehindHit_PostgresNotCalled verifies that when write-behind hits,
// Postgres is not queried.
func TestCommitTransaction_WriteBehindHit_PostgresNotCalled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	tranID := libCommons.GenerateUUIDv7()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	queryUC := &query.UseCase{RedisRepo: mockRedisRepo}
	handler := &TransactionHandler{
		Command: &command.UseCase{RedisRepo: mockRedisRepo},
		Query:   queryUC,
	}

	// Write-behind hit
	tran := newTestTransactionData(orgID, ledgerID, tranID)
	wbData, err := msgpack.Marshal(tran)
	require.NoError(t, err)

	mockRedisRepo.EXPECT().
		GetBytes(gomock.Any(), gomock.Any()).
		Return(wbData, nil).
		Times(1)

	// No TransactionRepo mock → proves Postgres is never called

	// commitOrCancelTransaction: SetNX short-circuits
	mockRedisRepo.EXPECT().
		SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, errors.New("lock error")).
		Times(1)

	app := fiber.New()
	app.Post("/test", func(c *fiber.Ctx) error {
		c.Locals("organization_id", orgID)
		c.Locals("ledger_id", ledgerID)
		c.Locals("transaction_id", tranID)
		return handler.CommitTransaction(c)
	})

	req := httptest.NewRequest("POST", "/test", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	// Error from SetNX short-circuit, but write-behind was used and Postgres was NOT called
	assert.True(t, resp.StatusCode >= 400)
}


