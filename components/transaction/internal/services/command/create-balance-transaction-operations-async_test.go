package command

import (
	"context"
	"encoding/json"
	"errors"
	libLog "github.com/LerianStudio/lib-commons/commons/log"
	libTransaction "github.com/LerianStudio/lib-commons/commons/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"testing"
)

// MockLogger is a mock implementation of logger for testing
type MockLogger struct{}

func (m *MockLogger) Debug(args ...any)                 {}
func (m *MockLogger) Debugf(format string, args ...any) {}
func (m *MockLogger) Debugln(args ...any)               {}
func (m *MockLogger) Info(args ...any)                  {}
func (m *MockLogger) Infof(format string, args ...any)  {}
func (m *MockLogger) Infoln(args ...any)                {}
func (m *MockLogger) Warn(args ...any)                  {}
func (m *MockLogger) Warnf(format string, args ...any)  {}
func (m *MockLogger) Warnln(args ...any)                {}
func (m *MockLogger) Error(args ...any)                 {}
func (m *MockLogger) Errorf(format string, args ...any) {}
func (m *MockLogger) Errorln(args ...any)               {}
func (m *MockLogger) Fatal(args ...any)                 {}
func (m *MockLogger) Fatalf(format string, args ...any) {}
func (m *MockLogger) Fatalln(args ...any)               {}
func (m *MockLogger) Sync() error                       { return nil }
func (m *MockLogger) WithDefaultMessageTemplate(template string) libLog.Logger { return m }
func (m *MockLogger) WithFields(args ...any) libLog.Logger                    { return m }

func TestCreateBalanceTransactionOperationsAsync(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockOperationRepo := operation.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)

		// Create a UseCase with all required dependencies
		uc := &UseCase{
			TransactionRepo: mockTransactionRepo,
			OperationRepo:   mockOperationRepo,
			MetadataRepo:    mockMetadataRepo,
			BalanceRepo:     mockBalanceRepo,
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New().String()

		// Mock transaction data with correct types
		validate := &libTransaction.Responses{
			Aliases: []string{"alias1", "alias2"},
			From: map[string]libTransaction.Amount{
				"alias1": {
					Asset: "USD",
					Value: int64(50),
					Scale: int64(2),
				},
			},
			To: map[string]libTransaction.Amount{
				"alias2": {
					Asset: "EUR",
					Value: int64(40),
					Scale: int64(2),
				},
			},
		}

		balances := []*mmodel.Balance{
			{
				ID:             uuid.New().String(),
				AccountID:      uuid.New().String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Alias:          "alias1",
				Available:      100,
				OnHold:         0,
				Scale:          2,
				Version:        1,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
				AssetCode:      "USD",
			},
			{
				ID:             uuid.New().String(),
				AccountID:      uuid.New().String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Alias:          "alias2",
				Available:      200,
				OnHold:         0,
				Scale:          2,
				Version:        1,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
				AssetCode:      "EUR",
			},
		}

		tran := &transaction.Transaction{
			ID:             transactionID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Operations:     []*operation.Operation{},
			Metadata:       map[string]interface{}{},
		}

		parseDSL := &libTransaction.Transaction{}

		// Create a transaction queue with the necessary fields
		transactionQueue := transaction.TransactionQueue{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			ParseDSL:    parseDSL,
		}

		transactionBytes, _ := json.Marshal(transactionQueue)
		queueData := []mmodel.QueueData{
			{
				ID:    uuid.New(),
				Value: transactionBytes,
			},
		}

		queue := mmodel.Queue{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			QueueData:      queueData,
		}

		// Mock BalanceRepo.BalancesUpdate
		mockBalanceRepo.EXPECT().
			BalancesUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		// Mock TransactionRepo.Create
		mockTransactionRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(tran, nil).
			Times(1)

		// Mock MetadataRepo.Create for transaction metadata
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		// Call the method
		err := uc.CreateBalanceTransactionOperationsAsync(ctx, queue)

		assert.NoError(t, err)
	})

	t.Run("error_update_balances", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockOperationRepo := operation.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)

		// Create a UseCase with mock repositories
		uc := &UseCase{
			TransactionRepo: mockTransactionRepo,
			OperationRepo:   mockOperationRepo,
			MetadataRepo:    mockMetadataRepo,
			BalanceRepo:     mockBalanceRepo,
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New().String()

		// Mock transaction data with correct types
		validate := &libTransaction.Responses{
			Aliases: []string{"alias1", "alias2"},
			From: map[string]libTransaction.Amount{
				"alias1": {
					Asset: "USD",
					Value: int64(50),
					Scale: int64(2),
				},
			},
			To: map[string]libTransaction.Amount{
				"alias2": {
					Asset: "EUR",
					Value: int64(40),
					Scale: int64(2),
				},
			},
		}

		balances := []*mmodel.Balance{
			{
				ID:             uuid.New().String(),
				AccountID:      uuid.New().String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Alias:          "alias1",
				Available:      100,
				OnHold:         0,
				Scale:          2,
				Version:        1,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
				AssetCode:      "USD",
			},
		}

		tran := &transaction.Transaction{
			ID:             transactionID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Operations:     []*operation.Operation{},
			Metadata:       map[string]interface{}{},
		}

		parseDSL := &libTransaction.Transaction{}

		transactionQueue := transaction.TransactionQueue{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			ParseDSL:    parseDSL,
		}

		transactionBytes, _ := json.Marshal(transactionQueue)
		queueData := []mmodel.QueueData{
			{
				ID:    uuid.New(),
				Value: transactionBytes,
			},
		}

		queue := mmodel.Queue{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			QueueData:      queueData,
		}

		// Mock BalanceRepo.BalancesUpdate to return an error
		mockBalanceRepo.EXPECT().
			BalancesUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(errors.New("failed to update balances")).
			Times(1)

		// Call the method
		err := uc.CreateBalanceTransactionOperationsAsync(ctx, queue)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update balances")
	})

	t.Run("error_duplicate_transaction", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockOperationRepo := operation.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)

		// Create a UseCase with all required dependencies
		uc := &UseCase{
			TransactionRepo: mockTransactionRepo,
			OperationRepo:   mockOperationRepo,
			MetadataRepo:    mockMetadataRepo,
			BalanceRepo:     mockBalanceRepo,
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New().String()

		// Mock transaction data with correct types
		validate := &libTransaction.Responses{
			Aliases: []string{"alias1"},
			From: map[string]libTransaction.Amount{
				"alias1": {
					Asset: "USD",
					Value: int64(50),
					Scale: int64(2),
				},
			},
		}

		balances := []*mmodel.Balance{
			{
				ID:             uuid.New().String(),
				AccountID:      uuid.New().String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Alias:          "alias1",
				Available:      100,
				OnHold:         0,
				Scale:          2,
				Version:        1,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
				AssetCode:      "USD",
			},
		}

		tran := &transaction.Transaction{
			ID:             transactionID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Operations:     []*operation.Operation{},
			Metadata:       map[string]interface{}{},
		}

		parseDSL := &libTransaction.Transaction{}

		transactionQueue := transaction.TransactionQueue{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			ParseDSL:    parseDSL,
		}

		transactionBytes, _ := json.Marshal(transactionQueue)
		queueData := []mmodel.QueueData{
			{
				ID:    uuid.New(),
				Value: transactionBytes,
			},
		}

		queue := mmodel.Queue{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			QueueData:      queueData,
		}

		// Mock BalanceRepo.BalancesUpdate
		mockBalanceRepo.EXPECT().
			BalancesUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		// Mock TransactionRepo.Create with duplicate key error
		pgErr := &pgconn.PgError{Code: "23505"}
		mockTransactionRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(nil, pgErr).
			Times(1)

		// Mock MetadataRepo.Create for transaction metadata (should not be called due to duplicate error)
		// We don't need to mock this since the method returns early after handling the duplicate error

		err := uc.CreateBalanceTransactionOperationsAsync(ctx, queue)

		assert.NoError(t, err) // Duplicate key errors are handled gracefully
	})
}

func TestCreateMetadataAsync(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		MetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	logger := &MockLogger{}
	metadata := map[string]any{"key": "value"}
	ID := uuid.New().String()
	collection := "Transaction"

	t.Run("success", func(t *testing.T) {
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), collection, gomock.Any()).
			Return(nil).
			Times(1)

		err := uc.CreateMetadataAsync(ctx, logger, metadata, ID, collection)
		assert.NoError(t, err)
	})

	t.Run("error", func(t *testing.T) {
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), collection, gomock.Any()).
			Return(errors.New("failed to create metadata")).
			Times(1)

		err := uc.CreateMetadataAsync(ctx, logger, metadata, ID, collection)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create metadata")
	})
}

func TestCreateBTOAsync(t *testing.T) {
	// This test simply verifies that CreateBTOAsync doesn't panic
	// Since it's just a wrapper around CreateBalanceTransactionOperationsAsync
	// which is tested separately, we don't need to test it extensively

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks for the repositories
	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)

	// Create a real UseCase with mock repositories
	uc := &UseCase{
		OperationRepo:   mockOperationRepo,
		TransactionRepo: mockTransactionRepo,
		MetadataRepo:    mockMetadataRepo,
		BalanceRepo:     mockBalanceRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	// Create a transaction queue with valid data
	validate := &libTransaction.Responses{
		Aliases: []string{"alias1"},
		From: map[string]libTransaction.Amount{
			"alias1": {
				Asset: "USD",
				Value: int64(50),
				Scale: int64(2),
			},
		},
	}

	balances := []*mmodel.Balance{
		{
			ID:             uuid.New().String(),
			AccountID:      uuid.New().String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Alias:          "alias1",
			Available:      100,
			OnHold:         0,
			Scale:          2,
			Version:        1,
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
			AssetCode:      "USD",
		},
	}

	tran := &transaction.Transaction{
		ID:             uuid.New().String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Operations:     []*operation.Operation{},
		Metadata:       map[string]interface{}{},
	}

	parseDSL := &libTransaction.Transaction{}

	transactionQueue := transaction.TransactionQueue{
		Transaction: tran,
		Validate:    validate,
		Balances:    balances,
		ParseDSL:    parseDSL,
	}

	transactionBytes, _ := json.Marshal(transactionQueue)
	queueData := []mmodel.QueueData{
		{
			ID:    uuid.New(),
			Value: transactionBytes,
		},
	}

	queue := mmodel.Queue{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		QueueData:      queueData,
	}

	// Mock all the necessary calls to avoid nil pointer dereference
	mockBalanceRepo.EXPECT().
		BalancesUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	mockTransactionRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(tran, nil).
		AnyTimes()

	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Call the method - this should not panic
	uc.CreateBTOAsync(ctx, queue)
}
