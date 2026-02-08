package query

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestGetTransactionByID(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	// Helper to create a fresh transaction instance for each test case
	newBaseTran := func() *transaction.Transaction {
		return &transaction.Transaction{
			ID:             transactionID.String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
		}
	}

	testMetadata := map[string]any{"key": "value", "env": "test"}

	tests := []struct {
		name            string
		setupMocks      func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository)
		expectedErr     error
		expectNilResult bool
		expectedMeta    map[string]any
	}{
		{
			name: "without metadata",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(newBaseTran(), nil)
				mockMetaRepo.EXPECT().
					FindByEntity(gomock.Any(), reflect.TypeFor[transaction.Transaction]().Name(), transactionID.String()).
					Return(nil, nil)
			},
			expectedErr:     nil,
			expectNilResult: false,
			expectedMeta:    nil,
		},
		{
			name: "with metadata",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(newBaseTran(), nil)
				mockMetaRepo.EXPECT().
					FindByEntity(gomock.Any(), reflect.TypeFor[transaction.Transaction]().Name(), transactionID.String()).
					Return(&mongodb.Metadata{EntityID: transactionID.String(), Data: testMetadata}, nil)
			},
			expectedErr:     nil,
			expectNilResult: false,
			expectedMeta:    testMetadata,
		},
		{
			name: "transaction not found",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(nil, nil)
			},
			expectedErr:     nil,
			expectNilResult: true,
			expectedMeta:    nil,
		},
		{
			name: "transaction repo error",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(nil, errors.New("database connection error"))
			},
			expectedErr:     errors.New("database connection error"),
			expectNilResult: true,
			expectedMeta:    nil,
		},
		{
			name: "metadata repo error",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(newBaseTran(), nil)
				mockMetaRepo.EXPECT().
					FindByEntity(gomock.Any(), reflect.TypeFor[transaction.Transaction]().Name(), transactionID.String()).
					Return(nil, errors.New("mongodb connection error"))
			},
			expectedErr:     errors.New("mongodb connection error"),
			expectNilResult: true,
			expectedMeta:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockTxRepo := transaction.NewMockRepository(ctrl)
			mockMetaRepo := mongodb.NewMockRepository(ctrl)

			tt.setupMocks(mockTxRepo, mockMetaRepo)

			uc := &UseCase{
				TransactionRepo: mockTxRepo,
				MetadataRepo:    mockMetaRepo,
			}

			result, err := uc.GetTransactionByID(context.Background(), organizationID, ledgerID, transactionID)

			if tt.expectedErr != nil {
				require.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
				assert.Nil(t, result)
				return
			}

			require.NoError(t, err)

			if tt.expectNilResult {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			assert.Equal(t, transactionID.String(), result.ID)
			assert.Equal(t, organizationID.String(), result.OrganizationID)
			assert.Equal(t, ledgerID.String(), result.LedgerID)

			if tt.expectedMeta != nil {
				assert.Equal(t, tt.expectedMeta, result.Metadata)
			} else {
				assert.Nil(t, result.Metadata)
			}
		})
	}
}

func TestGetTransactionByIDWithFallback(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	// Helper to create a fresh transaction instance for each test case
	newBaseTran := func() *transaction.Transaction {
		return &transaction.Transaction{
			ID:             transactionID.String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
		}
	}

	testMetadata := map[string]any{"key": "value", "env": "test"}

	tests := []struct {
		name            string
		setupMocks      func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository)
		expectedErr     error
		expectNilResult bool
		expectedMeta    map[string]any
	}{
		{
			name: "transaction found on replica without metadata",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					FindWithFallback(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(newBaseTran(), nil)
				mockMetaRepo.EXPECT().
					FindByEntity(gomock.Any(), reflect.TypeFor[transaction.Transaction]().Name(), transactionID.String()).
					Return(nil, nil)
			},
			expectedErr:     nil,
			expectNilResult: false,
			expectedMeta:    nil,
		},
		{
			name: "transaction found on replica with metadata",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					FindWithFallback(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(newBaseTran(), nil)
				mockMetaRepo.EXPECT().
					FindByEntity(gomock.Any(), reflect.TypeFor[transaction.Transaction]().Name(), transactionID.String()).
					Return(&mongodb.Metadata{EntityID: transactionID.String(), Data: testMetadata}, nil)
			},
			expectedErr:     nil,
			expectNilResult: false,
			expectedMeta:    testMetadata,
		},
		{
			name: "transaction not found on replica or primary",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					FindWithFallback(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(nil, nil)
			},
			expectedErr:     nil,
			expectNilResult: true,
			expectedMeta:    nil,
		},
		{
			name: "transaction repo returns error",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					FindWithFallback(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(nil, errors.New("database connection error"))
			},
			expectedErr:     errors.New("database connection error"),
			expectNilResult: true,
			expectedMeta:    nil,
		},
		{
			name: "metadata repo returns error",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					FindWithFallback(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(newBaseTran(), nil)
				mockMetaRepo.EXPECT().
					FindByEntity(gomock.Any(), reflect.TypeFor[transaction.Transaction]().Name(), transactionID.String()).
					Return(nil, errors.New("mongodb connection error"))
			},
			expectedErr:     errors.New("mongodb connection error"),
			expectNilResult: true,
			expectedMeta:    nil,
		},
		{
			name: "fallback repo returns entity not found error",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					FindWithFallback(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(nil, errors.New("entity not found"))
			},
			expectedErr:     errors.New("entity not found"),
			expectNilResult: true,
			expectedMeta:    nil,
		},
		{
			name: "transaction found with empty metadata data map",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					FindWithFallback(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(newBaseTran(), nil)
				mockMetaRepo.EXPECT().
					FindByEntity(gomock.Any(), reflect.TypeFor[transaction.Transaction]().Name(), transactionID.String()).
					Return(&mongodb.Metadata{EntityID: transactionID.String(), Data: map[string]any{}}, nil)
			},
			expectedErr:     nil,
			expectNilResult: false,
			expectedMeta:    map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockTxRepo := transaction.NewMockRepository(ctrl)
			mockMetaRepo := mongodb.NewMockRepository(ctrl)

			tt.setupMocks(mockTxRepo, mockMetaRepo)

			uc := &UseCase{
				TransactionRepo: mockTxRepo,
				MetadataRepo:    mockMetaRepo,
			}

			result, err := uc.GetTransactionByIDWithFallback(context.Background(), organizationID, ledgerID, transactionID)

			if tt.expectedErr != nil {
				require.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
				assert.Nil(t, result)
				return
			}

			require.NoError(t, err)

			if tt.expectNilResult {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			assert.Equal(t, transactionID.String(), result.ID)
			assert.Equal(t, organizationID.String(), result.OrganizationID)
			assert.Equal(t, ledgerID.String(), result.LedgerID)

			if tt.expectedMeta != nil {
				assert.Equal(t, tt.expectedMeta, result.Metadata)
			} else {
				assert.Nil(t, result.Metadata)
			}
		})
	}
}

func TestGetTransactionWithOperationsByID(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	// Helper to create a fresh transaction instance for each test case
	newBaseTran := func(ops []*operation.Operation) *transaction.Transaction {
		return &transaction.Transaction{
			ID:             transactionID.String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Operations:     ops,
		}
	}

	testMetadata := map[string]any{"key": "value", "env": "test"}

	tests := []struct {
		name            string
		setupMocks      func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository)
		expectedErr     error
		expectNilResult bool
		expectedMeta    map[string]any
		expectedOpCount int
	}{
		{
			name: "without metadata",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					FindWithOperations(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(newBaseTran([]*operation.Operation{{ID: "op1"}, {ID: "op2"}}), nil)
				mockMetaRepo.EXPECT().
					FindByEntity(gomock.Any(), reflect.TypeFor[transaction.Transaction]().Name(), transactionID.String()).
					Return(nil, nil)
			},
			expectedErr:     nil,
			expectNilResult: false,
			expectedMeta:    nil,
			expectedOpCount: 2,
		},
		{
			name: "with metadata",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					FindWithOperations(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(newBaseTran([]*operation.Operation{{ID: "op1"}}), nil)
				mockMetaRepo.EXPECT().
					FindByEntity(gomock.Any(), reflect.TypeFor[transaction.Transaction]().Name(), transactionID.String()).
					Return(&mongodb.Metadata{EntityID: transactionID.String(), Data: testMetadata}, nil)
			},
			expectedErr:     nil,
			expectNilResult: false,
			expectedMeta:    testMetadata,
			expectedOpCount: 1,
		},
		{
			name: "transaction not found",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					FindWithOperations(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(nil, nil)
			},
			expectedErr:     nil,
			expectNilResult: true,
			expectedMeta:    nil,
			expectedOpCount: 0,
		},
		{
			name: "transaction repo error",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					FindWithOperations(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(nil, errors.New("database connection error"))
			},
			expectedErr:     errors.New("database connection error"),
			expectNilResult: true,
			expectedMeta:    nil,
			expectedOpCount: 0,
		},
		{
			name: "metadata repo error",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					FindWithOperations(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(newBaseTran([]*operation.Operation{{ID: "op1"}, {ID: "op2"}}), nil)
				mockMetaRepo.EXPECT().
					FindByEntity(gomock.Any(), reflect.TypeFor[transaction.Transaction]().Name(), transactionID.String()).
					Return(nil, errors.New("mongodb connection error"))
			},
			expectedErr:     errors.New("mongodb connection error"),
			expectNilResult: true,
			expectedMeta:    nil,
			expectedOpCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockTxRepo := transaction.NewMockRepository(ctrl)
			mockMetaRepo := mongodb.NewMockRepository(ctrl)

			tt.setupMocks(mockTxRepo, mockMetaRepo)

			uc := &UseCase{
				TransactionRepo: mockTxRepo,
				MetadataRepo:    mockMetaRepo,
			}

			result, err := uc.GetTransactionWithOperationsByID(context.Background(), organizationID, ledgerID, transactionID)

			if tt.expectedErr != nil {
				require.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
				assert.Nil(t, result)
				return
			}

			require.NoError(t, err)

			if tt.expectNilResult {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			assert.Equal(t, transactionID.String(), result.ID)
			assert.Equal(t, organizationID.String(), result.OrganizationID)
			assert.Equal(t, ledgerID.String(), result.LedgerID)
			assert.Len(t, result.Operations, tt.expectedOpCount)

			if tt.expectedMeta != nil {
				assert.Equal(t, tt.expectedMeta, result.Metadata)
			} else {
				assert.Nil(t, result.Metadata)
			}
		})
	}
}

func TestGetTransactionWithOperationsByIDWithFallback(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	// Helper to create a fresh transaction instance for each test case
	newBaseTran := func(ops []*operation.Operation) *transaction.Transaction {
		return &transaction.Transaction{
			ID:             transactionID.String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Operations:     ops,
		}
	}

	testMetadata := map[string]any{"key": "value", "env": "test"}

	tests := []struct {
		name            string
		setupMocks      func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository)
		expectedErr     error
		expectNilResult bool
		expectedMeta    map[string]any
		expectedOpCount int
	}{
		{
			name: "transaction with operations found on replica without metadata",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					FindWithOperationsWithFallback(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(newBaseTran([]*operation.Operation{{ID: "op1"}, {ID: "op2"}}), nil)
				mockMetaRepo.EXPECT().
					FindByEntity(gomock.Any(), reflect.TypeFor[transaction.Transaction]().Name(), transactionID.String()).
					Return(nil, nil)
			},
			expectedErr:     nil,
			expectNilResult: false,
			expectedMeta:    nil,
			expectedOpCount: 2,
		},
		{
			name: "transaction with operations found on replica with metadata",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					FindWithOperationsWithFallback(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(newBaseTran([]*operation.Operation{{ID: "op1"}}), nil)
				mockMetaRepo.EXPECT().
					FindByEntity(gomock.Any(), reflect.TypeFor[transaction.Transaction]().Name(), transactionID.String()).
					Return(&mongodb.Metadata{EntityID: transactionID.String(), Data: testMetadata}, nil)
			},
			expectedErr:     nil,
			expectNilResult: false,
			expectedMeta:    testMetadata,
			expectedOpCount: 1,
		},
		{
			name: "transaction not found on replica or primary",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					FindWithOperationsWithFallback(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(nil, nil)
			},
			expectedErr:     nil,
			expectNilResult: true,
			expectedMeta:    nil,
			expectedOpCount: 0,
		},
		{
			name: "transaction repo returns error",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					FindWithOperationsWithFallback(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(nil, errors.New("database connection error"))
			},
			expectedErr:     errors.New("database connection error"),
			expectNilResult: true,
			expectedMeta:    nil,
			expectedOpCount: 0,
		},
		{
			name: "metadata repo returns error",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					FindWithOperationsWithFallback(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(newBaseTran([]*operation.Operation{{ID: "op1"}, {ID: "op2"}}), nil)
				mockMetaRepo.EXPECT().
					FindByEntity(gomock.Any(), reflect.TypeFor[transaction.Transaction]().Name(), transactionID.String()).
					Return(nil, errors.New("mongodb connection error"))
			},
			expectedErr:     errors.New("mongodb connection error"),
			expectNilResult: true,
			expectedMeta:    nil,
			expectedOpCount: 0,
		},
		{
			name: "fallback repo returns entity not found error",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					FindWithOperationsWithFallback(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(nil, errors.New("entity not found"))
			},
			expectedErr:     errors.New("entity not found"),
			expectNilResult: true,
			expectedMeta:    nil,
			expectedOpCount: 0,
		},
		{
			name: "transaction found with zero operations",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					FindWithOperationsWithFallback(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(newBaseTran([]*operation.Operation{}), nil)
				mockMetaRepo.EXPECT().
					FindByEntity(gomock.Any(), reflect.TypeFor[transaction.Transaction]().Name(), transactionID.String()).
					Return(nil, nil)
			},
			expectedErr:     nil,
			expectNilResult: false,
			expectedMeta:    nil,
			expectedOpCount: 0,
		},
		{
			name: "transaction found with empty metadata data map",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					FindWithOperationsWithFallback(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(newBaseTran([]*operation.Operation{{ID: "op1"}}), nil)
				mockMetaRepo.EXPECT().
					FindByEntity(gomock.Any(), reflect.TypeFor[transaction.Transaction]().Name(), transactionID.String()).
					Return(&mongodb.Metadata{EntityID: transactionID.String(), Data: map[string]any{}}, nil)
			},
			expectedErr:     nil,
			expectNilResult: false,
			expectedMeta:    map[string]any{},
			expectedOpCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockTxRepo := transaction.NewMockRepository(ctrl)
			mockMetaRepo := mongodb.NewMockRepository(ctrl)

			tt.setupMocks(mockTxRepo, mockMetaRepo)

			uc := &UseCase{
				TransactionRepo: mockTxRepo,
				MetadataRepo:    mockMetaRepo,
			}

			result, err := uc.GetTransactionWithOperationsByIDWithFallback(context.Background(), organizationID, ledgerID, transactionID)

			if tt.expectedErr != nil {
				require.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
				assert.Nil(t, result)
				return
			}

			require.NoError(t, err)

			if tt.expectNilResult {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			assert.Equal(t, transactionID.String(), result.ID)
			assert.Equal(t, organizationID.String(), result.OrganizationID)
			assert.Equal(t, ledgerID.String(), result.LedgerID)
			assert.Len(t, result.Operations, tt.expectedOpCount)

			if tt.expectedMeta != nil {
				assert.Equal(t, tt.expectedMeta, result.Metadata)
			} else {
				assert.Nil(t, result.Metadata)
			}
		})
	}
}
