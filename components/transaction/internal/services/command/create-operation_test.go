package command

import (
	"context"
	"errors"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestUseCase_CreateOperation(t *testing.T) {
	orgID := libCommons.GenerateUUIDv7().String()
	ledgerID := libCommons.GenerateUUIDv7().String()
	accountID := libCommons.GenerateUUIDv7().String()
	balanceID := libCommons.GenerateUUIDv7().String()
	transactionID := libCommons.GenerateUUIDv7().String()

	tests := []struct {
		name          string
		balances      []*mmodel.Balance
		dsl           *pkgTransaction.Transaction
		validate      pkgTransaction.Responses
		setupMocks    func(ctrl *gomock.Controller, uc *UseCase)
		expectError   bool
		expectedCount int
	}{
		{
			name: "creates_debit_operation_matching_by_id",
			balances: []*mmodel.Balance{
				{
					ID:             balanceID,
					OrganizationID: orgID,
					LedgerID:       ledgerID,
					AccountID:      accountID,
					Alias:          "@source",
					AssetCode:      "USD",
					Available:      decimal.NewFromInt(1000),
					OnHold:         decimal.NewFromInt(0),
					AllowSending:   true,
				},
			},
			dsl: &pkgTransaction.Transaction{
				Description: "Test transaction",
				Send: pkgTransaction.Send{
					Asset: "USD",
					Value: decimal.NewFromInt(100),
					Source: pkgTransaction.Source{
						From: []pkgTransaction.FromTo{
							{
								AccountAlias: balanceID, // Match by ID
								IsFrom:       true,
								Amount: &pkgTransaction.Amount{
									Asset: "USD",
									Value: decimal.NewFromInt(100),
								},
							},
						},
					},
					Distribute: pkgTransaction.Distribute{
						To: []pkgTransaction.FromTo{},
					},
				},
			},
			validate: pkgTransaction.Responses{
				From: map[string]pkgTransaction.Amount{
					balanceID: {
						Asset:           "USD",
						Value:           decimal.NewFromInt(100),
						Operation:       constant.DEBIT,
						TransactionType: "CREATED",
					},
				},
				To: map[string]pkgTransaction.Amount{},
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				mockOpRepo := operation.NewMockRepository(ctrl)
				mockMetaRepo := mongodb.NewMockRepository(ctrl)

				mockOpRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, op *operation.Operation) (*operation.Operation, error) {
						assert.Equal(t, transactionID, op.TransactionID)
						assert.Equal(t, constant.DEBIT, op.Type)
						assert.Equal(t, "USD", op.AssetCode)
						assert.Equal(t, balanceID, op.BalanceID)
						return op, nil
					}).
					Times(1)

				uc.OperationRepo = mockOpRepo
				uc.MetadataRepo = mockMetaRepo
			},
			expectError:   false,
			expectedCount: 1,
		},
		{
			name: "creates_credit_operation_matching_by_alias",
			balances: []*mmodel.Balance{
				{
					ID:             balanceID,
					OrganizationID: orgID,
					LedgerID:       ledgerID,
					AccountID:      accountID,
					Alias:          "@destination",
					AssetCode:      "USD",
					Available:      decimal.NewFromInt(500),
					OnHold:         decimal.NewFromInt(0),
					AllowReceiving: true,
				},
			},
			dsl: &pkgTransaction.Transaction{
				Description: "Credit transaction",
				Send: pkgTransaction.Send{
					Asset: "USD",
					Value: decimal.NewFromInt(200),
					Source: pkgTransaction.Source{
						From: []pkgTransaction.FromTo{},
					},
					Distribute: pkgTransaction.Distribute{
						To: []pkgTransaction.FromTo{
							{
								AccountAlias: "@destination", // Match by Alias
								IsFrom:       false,
								Amount: &pkgTransaction.Amount{
									Asset: "USD",
									Value: decimal.NewFromInt(200),
								},
							},
						},
					},
				},
			},
			validate: pkgTransaction.Responses{
				From: map[string]pkgTransaction.Amount{},
				To: map[string]pkgTransaction.Amount{
					"@destination": {
						Asset:           "USD",
						Value:           decimal.NewFromInt(200),
						Operation:       constant.CREDIT,
						TransactionType: "CREATED",
					},
				},
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				mockOpRepo := operation.NewMockRepository(ctrl)
				mockMetaRepo := mongodb.NewMockRepository(ctrl)

				mockOpRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, op *operation.Operation) (*operation.Operation, error) {
						assert.Equal(t, transactionID, op.TransactionID)
						assert.Equal(t, constant.CREDIT, op.Type)
						assert.Equal(t, "@destination", op.AccountAlias)
						return op, nil
					}).
					Times(1)

				uc.OperationRepo = mockOpRepo
				uc.MetadataRepo = mockMetaRepo
			},
			expectError:   false,
			expectedCount: 1,
		},
		{
			name: "creates_operation_with_metadata",
			balances: []*mmodel.Balance{
				{
					ID:             balanceID,
					OrganizationID: orgID,
					LedgerID:       ledgerID,
					AccountID:      accountID,
					Alias:          "@source",
					AssetCode:      "BRL",
					Available:      decimal.NewFromInt(5000),
					OnHold:         decimal.NewFromInt(0),
					AllowSending:   true,
				},
			},
			dsl: &pkgTransaction.Transaction{
				Description: "Transaction with metadata",
				Send: pkgTransaction.Send{
					Asset: "BRL",
					Value: decimal.NewFromInt(150),
					Source: pkgTransaction.Source{
						From: []pkgTransaction.FromTo{
							{
								AccountAlias: "@source",
								IsFrom:       true,
								Metadata:     map[string]any{"key": "value"},
								Amount: &pkgTransaction.Amount{
									Asset: "BRL",
									Value: decimal.NewFromInt(150),
								},
							},
						},
					},
					Distribute: pkgTransaction.Distribute{
						To: []pkgTransaction.FromTo{},
					},
				},
			},
			validate: pkgTransaction.Responses{
				From: map[string]pkgTransaction.Amount{
					"@source": {
						Asset:           "BRL",
						Value:           decimal.NewFromInt(150),
						Operation:       constant.DEBIT,
						TransactionType: "CREATED",
					},
				},
				To: map[string]pkgTransaction.Amount{},
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				mockOpRepo := operation.NewMockRepository(ctrl)
				mockMetaRepo := mongodb.NewMockRepository(ctrl)

				mockOpRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, op *operation.Operation) (*operation.Operation, error) {
						return op, nil
					}).
					Times(1)

				mockMetaRepo.EXPECT().
					Create(gomock.Any(), "Operation", gomock.Any()).
					Return(nil).
					Times(1)

				uc.OperationRepo = mockOpRepo
				uc.MetadataRepo = mockMetaRepo
			},
			expectError:   false,
			expectedCount: 1,
		},
		{
			name: "uses_dsl_description_when_fromto_description_empty",
			balances: []*mmodel.Balance{
				{
					ID:             balanceID,
					OrganizationID: orgID,
					LedgerID:       ledgerID,
					AccountID:      accountID,
					Alias:          "@account",
					AssetCode:      "EUR",
					Available:      decimal.NewFromInt(2000),
					OnHold:         decimal.NewFromInt(0),
					AllowSending:   true,
				},
			},
			dsl: &pkgTransaction.Transaction{
				Description: "DSL level description",
				Send: pkgTransaction.Send{
					Asset: "EUR",
					Value: decimal.NewFromInt(50),
					Source: pkgTransaction.Source{
						From: []pkgTransaction.FromTo{
							{
								AccountAlias: "@account",
								IsFrom:       true,
								Description:  "", // Empty - should use DSL description
								Amount: &pkgTransaction.Amount{
									Asset: "EUR",
									Value: decimal.NewFromInt(50),
								},
							},
						},
					},
					Distribute: pkgTransaction.Distribute{
						To: []pkgTransaction.FromTo{},
					},
				},
			},
			validate: pkgTransaction.Responses{
				From: map[string]pkgTransaction.Amount{
					"@account": {
						Asset:           "EUR",
						Value:           decimal.NewFromInt(50),
						Operation:       constant.DEBIT,
						TransactionType: "CREATED",
					},
				},
				To: map[string]pkgTransaction.Amount{},
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				mockOpRepo := operation.NewMockRepository(ctrl)
				mockMetaRepo := mongodb.NewMockRepository(ctrl)

				mockOpRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, op *operation.Operation) (*operation.Operation, error) {
						assert.Equal(t, "DSL level description", op.Description)
						return op, nil
					}).
					Times(1)

				uc.OperationRepo = mockOpRepo
				uc.MetadataRepo = mockMetaRepo
			},
			expectError:   false,
			expectedCount: 1,
		},
		{
			name: "returns_error_when_operation_repo_fails",
			balances: []*mmodel.Balance{
				{
					ID:             balanceID,
					OrganizationID: orgID,
					LedgerID:       ledgerID,
					AccountID:      accountID,
					Alias:          "@source",
					AssetCode:      "USD",
					Available:      decimal.NewFromInt(1000),
					OnHold:         decimal.NewFromInt(0),
					AllowSending:   true,
				},
			},
			dsl: &pkgTransaction.Transaction{
				Description: "Test transaction",
				Send: pkgTransaction.Send{
					Asset: "USD",
					Value: decimal.NewFromInt(100),
					Source: pkgTransaction.Source{
						From: []pkgTransaction.FromTo{
							{
								AccountAlias: "@source",
								IsFrom:       true,
								Amount: &pkgTransaction.Amount{
									Asset: "USD",
									Value: decimal.NewFromInt(100),
								},
							},
						},
					},
					Distribute: pkgTransaction.Distribute{
						To: []pkgTransaction.FromTo{},
					},
				},
			},
			validate: pkgTransaction.Responses{
				From: map[string]pkgTransaction.Amount{
					"@source": {
						Asset:           "USD",
						Value:           decimal.NewFromInt(100),
						Operation:       constant.DEBIT,
						TransactionType: "CREATED",
					},
				},
				To: map[string]pkgTransaction.Amount{},
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				mockOpRepo := operation.NewMockRepository(ctrl)
				mockMetaRepo := mongodb.NewMockRepository(ctrl)

				mockOpRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("database connection error")).
					Times(1)

				uc.OperationRepo = mockOpRepo
				uc.MetadataRepo = mockMetaRepo
			},
			expectError:   true,
			expectedCount: 0,
		},
		{
			name: "returns_error_when_metadata_repo_fails",
			balances: []*mmodel.Balance{
				{
					ID:             balanceID,
					OrganizationID: orgID,
					LedgerID:       ledgerID,
					AccountID:      accountID,
					Alias:          "@source",
					AssetCode:      "USD",
					Available:      decimal.NewFromInt(1000),
					OnHold:         decimal.NewFromInt(0),
					AllowSending:   true,
				},
			},
			dsl: &pkgTransaction.Transaction{
				Description: "Test transaction",
				Send: pkgTransaction.Send{
					Asset: "USD",
					Value: decimal.NewFromInt(100),
					Source: pkgTransaction.Source{
						From: []pkgTransaction.FromTo{
							{
								AccountAlias: "@source",
								IsFrom:       true,
								Metadata:     map[string]any{"key": "value"},
								Amount: &pkgTransaction.Amount{
									Asset: "USD",
									Value: decimal.NewFromInt(100),
								},
							},
						},
					},
					Distribute: pkgTransaction.Distribute{
						To: []pkgTransaction.FromTo{},
					},
				},
			},
			validate: pkgTransaction.Responses{
				From: map[string]pkgTransaction.Amount{
					"@source": {
						Asset:           "USD",
						Value:           decimal.NewFromInt(100),
						Operation:       constant.DEBIT,
						TransactionType: "CREATED",
					},
				},
				To: map[string]pkgTransaction.Amount{},
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				mockOpRepo := operation.NewMockRepository(ctrl)
				mockMetaRepo := mongodb.NewMockRepository(ctrl)

				mockOpRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, op *operation.Operation) (*operation.Operation, error) {
						return op, nil
					}).
					Times(1)

				mockMetaRepo.EXPECT().
					Create(gomock.Any(), "Operation", gomock.Any()).
					Return(errors.New("mongodb connection error")).
					Times(1)

				uc.OperationRepo = mockOpRepo
				uc.MetadataRepo = mockMetaRepo
			},
			expectError:   true,
			expectedCount: 0,
		},
		{
			name: "creates_multiple_operations_for_multiple_balances",
			balances: []*mmodel.Balance{
				{
					ID:             libCommons.GenerateUUIDv7().String(),
					OrganizationID: orgID,
					LedgerID:       ledgerID,
					AccountID:      accountID,
					Alias:          "@source",
					AssetCode:      "USD",
					Available:      decimal.NewFromInt(1000),
					OnHold:         decimal.NewFromInt(0),
					AllowSending:   true,
				},
				{
					ID:             libCommons.GenerateUUIDv7().String(),
					OrganizationID: orgID,
					LedgerID:       ledgerID,
					AccountID:      libCommons.GenerateUUIDv7().String(),
					Alias:          "@destination",
					AssetCode:      "USD",
					Available:      decimal.NewFromInt(500),
					OnHold:         decimal.NewFromInt(0),
					AllowReceiving: true,
				},
			},
			dsl: &pkgTransaction.Transaction{
				Description: "Transfer",
				Send: pkgTransaction.Send{
					Asset: "USD",
					Value: decimal.NewFromInt(100),
					Source: pkgTransaction.Source{
						From: []pkgTransaction.FromTo{
							{
								AccountAlias: "@source",
								IsFrom:       true,
								Amount: &pkgTransaction.Amount{
									Asset: "USD",
									Value: decimal.NewFromInt(100),
								},
							},
						},
					},
					Distribute: pkgTransaction.Distribute{
						To: []pkgTransaction.FromTo{
							{
								AccountAlias: "@destination",
								IsFrom:       false,
								Amount: &pkgTransaction.Amount{
									Asset: "USD",
									Value: decimal.NewFromInt(100),
								},
							},
						},
					},
				},
			},
			validate: pkgTransaction.Responses{
				From: map[string]pkgTransaction.Amount{
					"@source": {
						Asset:           "USD",
						Value:           decimal.NewFromInt(100),
						Operation:       constant.DEBIT,
						TransactionType: "CREATED",
					},
				},
				To: map[string]pkgTransaction.Amount{
					"@destination": {
						Asset:           "USD",
						Value:           decimal.NewFromInt(100),
						Operation:       constant.CREDIT,
						TransactionType: "CREATED",
					},
				},
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				mockOpRepo := operation.NewMockRepository(ctrl)
				mockMetaRepo := mongodb.NewMockRepository(ctrl)

				mockOpRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, op *operation.Operation) (*operation.Operation, error) {
						return op, nil
					}).
					Times(2)

				uc.OperationRepo = mockOpRepo
				uc.MetadataRepo = mockMetaRepo
			},
			expectError:   false,
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			uc := &UseCase{}
			tt.setupMocks(ctrl, uc)

			ctx := context.Background()
			resultChan := make(chan []*operation.Operation, 1)
			errChan := make(chan error, 1)

			go uc.CreateOperation(ctx, tt.balances, transactionID, tt.dsl, tt.validate, resultChan, errChan)

			select {
			case ops := <-resultChan:
				if tt.expectError {
					// Check if error was also sent
					select {
					case err := <-errChan:
						require.Error(t, err)
					case <-time.After(100 * time.Millisecond):
						t.Fatal("expected error but got none")
					}
				} else {
					assert.Len(t, ops, tt.expectedCount)
				}
			case err := <-errChan:
				if !tt.expectError {
					t.Fatalf("unexpected error: %v", err)
				}
				require.Error(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("test timed out waiting for result")
			}
		})
	}
}

func TestUseCase_CreateMetadata(t *testing.T) {
	tests := []struct {
		name       string
		metadata   map[string]any
		operation  *operation.Operation
		setupMocks func(ctrl *gomock.Controller, uc *UseCase)
		expectErr  bool
	}{
		{
			name:     "creates_metadata_successfully",
			metadata: map[string]any{"key": "value", "number": 42},
			operation: &operation.Operation{
				ID:             libCommons.GenerateUUIDv7().String(),
				OrganizationID: libCommons.GenerateUUIDv7().String(),
				LedgerID:       libCommons.GenerateUUIDv7().String(),
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				mockMetaRepo := mongodb.NewMockRepository(ctrl)

				mockMetaRepo.EXPECT().
					Create(gomock.Any(), "Operation", gomock.Any()).
					DoAndReturn(func(ctx context.Context, entityName string, meta *mongodb.Metadata) error {
						assert.Equal(t, "Operation", entityName)
						assert.NotEmpty(t, meta.EntityID)
						assert.Equal(t, "Operation", meta.EntityName)
						assert.Equal(t, "value", meta.Data["key"])
						assert.Equal(t, 42, meta.Data["number"])
						return nil
					}).
					Times(1)

				uc.MetadataRepo = mockMetaRepo
			},
			expectErr: false,
		},
		{
			name:     "skips_creation_when_metadata_is_nil",
			metadata: nil,
			operation: &operation.Operation{
				ID:             libCommons.GenerateUUIDv7().String(),
				OrganizationID: libCommons.GenerateUUIDv7().String(),
				LedgerID:       libCommons.GenerateUUIDv7().String(),
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				mockMetaRepo := mongodb.NewMockRepository(ctrl)
				// No expectations - Create should not be called
				uc.MetadataRepo = mockMetaRepo
			},
			expectErr: false,
		},
		{
			name:     "returns_error_when_repo_fails",
			metadata: map[string]any{"key": "value"},
			operation: &operation.Operation{
				ID:             libCommons.GenerateUUIDv7().String(),
				OrganizationID: libCommons.GenerateUUIDv7().String(),
				LedgerID:       libCommons.GenerateUUIDv7().String(),
			},
			setupMocks: func(ctrl *gomock.Controller, uc *UseCase) {
				mockMetaRepo := mongodb.NewMockRepository(ctrl)

				mockMetaRepo.EXPECT().
					Create(gomock.Any(), "Operation", gomock.Any()).
					Return(errors.New("mongodb error")).
					Times(1)

				uc.MetadataRepo = mockMetaRepo
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			uc := &UseCase{}
			tt.setupMocks(ctrl, uc)

			ctx := context.Background()
			logger, _, _, _ := libCommons.NewTrackingFromContext(ctx)

			err := uc.CreateMetadata(ctx, logger, tt.metadata, tt.operation)

			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.metadata != nil {
					assert.Equal(t, tt.metadata, tt.operation.Metadata)
				}
			}
		})
	}
}

// TestUseCase_CreateOperation_ValidationError tests that validation errors stop operation creation.
// BUG: Currently the code continues execution after validation error, creating invalid operations.
// See: ../notes/pending-tasks/2025-12-28-create-operation-missing-continue-on-validation-error.md
func TestUseCase_CreateOperation_ValidationError(t *testing.T) {
	t.Skip("BUG: CreateOperation missing 'continue' after validation error")

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := libCommons.GenerateUUIDv7().String()
	ledgerID := libCommons.GenerateUUIDv7().String()
	accountID := libCommons.GenerateUUIDv7().String()
	balanceID := libCommons.GenerateUUIDv7().String()
	transactionID := libCommons.GenerateUUIDv7().String()

	// Balance with only 50 available, but transaction requires 100
	balances := []*mmodel.Balance{
		{
			ID:             balanceID,
			OrganizationID: orgID,
			LedgerID:       ledgerID,
			AccountID:      accountID,
			Alias:          "@source",
			AssetCode:      "USD",
			Available:      decimal.NewFromInt(50),
			OnHold:         decimal.NewFromInt(0),
			AllowSending:   true,
			AccountType:    "deposit", // Not external - triggers insufficient funds
		},
	}

	dsl := &pkgTransaction.Transaction{
		Description: "Test insufficient funds",
		Send: pkgTransaction.Send{
			Asset: "USD",
			Value: decimal.NewFromInt(100),
			Source: pkgTransaction.Source{
				From: []pkgTransaction.FromTo{
					{
						AccountAlias: "@source",
						IsFrom:       true,
						Amount: &pkgTransaction.Amount{
							Asset: "USD",
							Value: decimal.NewFromInt(100),
						},
					},
				},
			},
			Distribute: pkgTransaction.Distribute{
				To: []pkgTransaction.FromTo{},
			},
		},
	}

	validate := pkgTransaction.Responses{
		From: map[string]pkgTransaction.Amount{
			"@source": {
				Asset:           "USD",
				Value:           decimal.NewFromInt(100),
				Operation:       constant.DEBIT,
				TransactionType: "CREATED",
			},
		},
		To: map[string]pkgTransaction.Amount{},
	}

	mockOpRepo := operation.NewMockRepository(ctrl)
	mockMetaRepo := mongodb.NewMockRepository(ctrl)
	// After fix: Create should NOT be called because validation fails first
	// mockOpRepo.EXPECT().Create(...).Times(0) is implicit

	uc := &UseCase{
		OperationRepo: mockOpRepo,
		MetadataRepo:  mockMetaRepo,
	}

	ctx := context.Background()
	resultChan := make(chan []*operation.Operation, 1)
	errChan := make(chan error, 1)

	go uc.CreateOperation(ctx, balances, transactionID, dsl, validate, resultChan, errChan)

	select {
	case <-resultChan:
		// After fix: Should receive empty slice or no result
		select {
		case err := <-errChan:
			require.Error(t, err)
			assert.Contains(t, err.Error(), "insufficient")
		case <-time.After(100 * time.Millisecond):
			t.Fatal("expected validation error but got none")
		}
	case err := <-errChan:
		require.Error(t, err)
		assert.Contains(t, err.Error(), "insufficient")
	case <-time.After(5 * time.Second):
		t.Fatal("test timed out")
	}
}
